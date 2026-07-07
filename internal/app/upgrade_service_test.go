package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidUpgradeVersionRequiresReleaseTag(t *testing.T) {
	for _, version := range []string{"v1", "v1.1", "v1.1.0", "v2.0.0"} {
		if !validUpgradeVersion(version) {
			t.Fatalf("%s should be a valid upgrade version", version)
		}
	}
	for _, version := range []string{"", "1.1.0", "latest", "v1.1.1.1", "v1.1.0;reboot"} {
		if validUpgradeVersion(version) {
			t.Fatalf("%s should be rejected as an upgrade version", version)
		}
	}
}

func TestReleaseHasRequiredLinuxUpgradeAssets(t *testing.T) {
	release := githubRelease{
		TagName: "v1.1.0",
		HTMLURL: "https://github.com/kos991/fwlog/releases/tag/v1.1.0",
		Assets: []githubReleaseAsset{
			{Name: linuxUpgradeAssetName, BrowserDownloadURL: "https://example.test/legacy"},
			{Name: "fwlog-upgrade-v1.1.0.x86_64.rpm", BrowserDownloadURL: "https://example.test/upgrade.rpm"},
			{Name: "fwlog-upgrade_1.1.0_amd64.deb", BrowserDownloadURL: "https://example.test/upgrade.deb"},
		},
	}

	assets, missing := releaseUpgradeAssets(release)

	if len(missing) != 0 {
		t.Fatalf("missing assets = %#v", missing)
	}
	if assets.LegacyBinaryURL != "https://example.test/legacy" || assets.UpgradeRPMURL != "https://example.test/upgrade.rpm" || assets.UpgradeDEBURL != "https://example.test/upgrade.deb" {
		t.Fatalf("assets = %#v", assets)
	}
}

func TestReleaseReportsMissingUpgradeAssets(t *testing.T) {
	_, missing := releaseUpgradeAssets(githubRelease{
		TagName: "v1.1.0",
		Assets:  []githubReleaseAsset{{Name: linuxUpgradeAssetName, BrowserDownloadURL: "https://example.test/linux"}},
	})

	want := []string{"fwlog-upgrade-v1.1.0.x86_64.rpm", "fwlog-upgrade_1.1.0_amd64.deb"}
	if len(missing) != len(want) || missing[0] != want[0] || missing[1] != want[1] {
		t.Fatalf("missing assets = %#v, want %#v", missing, want)
	}
}

func TestSelectUpgradePackageUsesDEBOnDebianWhenBothManagersAvailable(t *testing.T) {
	restore := stubLookPath(t, map[string]bool{"rpm": true, "dpkg": true})
	defer restore()
	restoreOS := stubOSRelease(t, "ID=debian\nID_LIKE=debian\n")
	defer restoreOS()

	pkg, err := selectUpgradePackage(upgradeAssets{
		UpgradeRPMURL: "https://example.test/fwlog-upgrade.rpm",
		UpgradeDEBURL: "https://example.test/fwlog-upgrade.deb",
	})
	if err != nil {
		t.Fatalf("select package: %v", err)
	}
	if pkg.Format != upgradePackageDEB || pkg.URL != "https://example.test/fwlog-upgrade.deb" {
		t.Fatalf("package = %#v", pkg)
	}
}

func TestSelectUpgradePackagePrefersRPMOnRHELWhenBothManagersAvailable(t *testing.T) {
	restore := stubLookPath(t, map[string]bool{"rpm": true, "dpkg": true})
	defer restore()
	restoreOS := stubOSRelease(t, "ID=\"kylin\"\nID_LIKE=\"rhel fedora\"\n")
	defer restoreOS()

	pkg, err := selectUpgradePackage(upgradeAssets{
		UpgradeRPMURL: "https://example.test/fwlog-upgrade.rpm",
		UpgradeDEBURL: "https://example.test/fwlog-upgrade.deb",
	})
	if err != nil {
		t.Fatalf("select package: %v", err)
	}
	if pkg.Format != upgradePackageRPM || pkg.URL != "https://example.test/fwlog-upgrade.rpm" {
		t.Fatalf("package = %#v", pkg)
	}
}

func TestSelectUpgradePackageUsesDEBWhenRPMUnavailable(t *testing.T) {
	restore := stubLookPath(t, map[string]bool{"dpkg": true})
	defer restore()

	pkg, err := selectUpgradePackage(upgradeAssets{
		UpgradeRPMURL: "https://example.test/fwlog-upgrade.rpm",
		UpgradeDEBURL: "https://example.test/fwlog-upgrade.deb",
	})
	if err != nil {
		t.Fatalf("select package: %v", err)
	}
	if pkg.Format != upgradePackageDEB || pkg.URL != "https://example.test/fwlog-upgrade.deb" {
		t.Fatalf("package = %#v", pkg)
	}
}

func TestValidateUpgradePackageContentsRejectsClickHouseFiles(t *testing.T) {
	calls := stubCommandRunner(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "rpm" || len(args) != 2 || args[0] != "-qpl" || args[1] != "/tmp/fwlog-upgrade.rpm" {
			t.Fatalf("unexpected command: %s %#v", name, args)
		}
		return []byte("/opt/nat-query/nat-query-service\n/opt/nat-query/clickhouse/bin/clickhouse\n"), nil
	})
	defer calls.restore()

	err := validateUpgradePackageContents(context.Background(), upgradePackage{
		Format: upgradePackageRPM,
		Path:   "/tmp/fwlog-upgrade.rpm",
	})
	if err == nil {
		t.Fatal("expected ClickHouse file validation error")
	}
}

func TestInstallUpgradePackageUsesPackageManager(t *testing.T) {
	calls := stubCommandRunner(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "systemd-run" {
			t.Fatalf("unexpected command: %s %#v", name, args)
		}
		if len(args) != 11 {
			t.Fatalf("unexpected systemd-run args: %#v", args)
		}
		if args[0] != "--wait" || args[1] != "--collect" || args[2] != "--pipe" || args[3] != "--service-type=oneshot" || args[4] != "--property=ProtectSystem=off" || args[5] != "--property=ProtectHome=off" || args[6] != "--property=PrivateTmp=no" || args[7] != "--property=ReadWritePaths=/ /var/lib/dpkg /var/lib/rpm /opt/nat-query /data" || args[8] != "dpkg" || args[9] != "-i" || args[10] != "/tmp/fwlog-upgrade.deb" {
			t.Fatalf("unexpected systemd-run args: %#v", args)
		}
		return []byte("installed"), nil
	})
	defer calls.restore()
	restoreLookPath := stubLookPath(t, map[string]bool{"systemd-run": true})
	defer restoreLookPath()

	if err := installUpgradePackage(context.Background(), upgradePackage{
		Format: upgradePackageDEB,
		Path:   "/tmp/fwlog-upgrade.deb",
	}); err != nil {
		t.Fatalf("install package: %v", err)
	}
	if calls.count != 1 {
		t.Fatalf("command count = %d, want 1", calls.count)
	}
}

func TestExecuteSystemUpgradeInstallsUpgradePackage(t *testing.T) {
	restoreLookPath := stubLookPath(t, map[string]bool{"rpm": true, "dpkg": true, "systemd-run": true})
	defer restoreLookPath()
	restoreOS := stubOSRelease(t, "ID=debian\nID_LIKE=debian\n")
	defer restoreOS()
	restoreHTTP := stubHTTPClient(t, map[string]string{
		"https://api.github.com/repos/kos991/fwlog/releases/tags/v1.1.0": `{
			"tag_name":"v1.1.0",
			"html_url":"https://github.com/kos991/fwlog/releases/tag/v1.1.0",
			"assets":[
				{"name":"nat-query-service_linux_amd64","browser_download_url":"https://downloads.test/nat-query-service_linux_amd64"},
				{"name":"fwlog-upgrade-v1.1.0.x86_64.rpm","browser_download_url":"https://downloads.test/fwlog-upgrade.rpm"},
				{"name":"fwlog-upgrade_1.1.0_amd64.deb","browser_download_url":"https://downloads.test/fwlog-upgrade.deb"}
			]
		}`,
		"https://downloads.test/fwlog-upgrade.deb": "package-bytes",
	})
	defer restoreHTTP()
	originalUpgradeTempRoot := upgradeTempRoot
	upgradeTempRoot = t.TempDir()
	defer func() {
		upgradeTempRoot = originalUpgradeTempRoot
	}()

	var commands []string
	calls := stubCommandRunner(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		switch name {
		case "dpkg-deb":
			if len(args) != 2 || args[0] != "--contents" || !strings.HasSuffix(args[1], "fwlog-upgrade.deb") {
				t.Fatalf("unexpected contents command: %s %#v", name, args)
			}
			return []byte("/opt/nat-query/nat-query-service\n"), nil
		case "systemd-run":
			if len(args) != 11 || args[0] != "--wait" || args[1] != "--collect" || args[2] != "--pipe" || args[3] != "--service-type=oneshot" || args[4] != "--property=ProtectSystem=off" || args[5] != "--property=ProtectHome=off" || args[6] != "--property=PrivateTmp=no" || args[7] != "--property=ReadWritePaths=/ /var/lib/dpkg /var/lib/rpm /opt/nat-query /data" || args[8] != "dpkg" || args[9] != "-i" || !strings.HasSuffix(args[10], "fwlog-upgrade.deb") {
				t.Fatalf("unexpected install command: %s %#v", name, args)
			}
			return []byte("installed"), nil
		default:
			t.Fatalf("unexpected command: %s %#v", name, args)
			return nil, nil
		}
	})
	defer calls.restore()

	status := UpgradeStatus{}
	if err := executeSystemUpgrade(context.Background(), upgradeTarget{Version: "v1.1.0"}, &status); err != nil {
		t.Fatalf("execute upgrade: %v", err)
	}
	if status.BackupPath != "" {
		t.Fatalf("backup path = %q, want empty because package upgrade owns installation", status.BackupPath)
	}
	if calls.count != 2 {
		t.Fatalf("command count = %d, commands = %#v", calls.count, commands)
	}
	if !strings.HasPrefix(commands[0], "dpkg-deb --contents ") || !strings.Contains(commands[1], "systemd-run --wait --collect --pipe --service-type=oneshot --property=ProtectSystem=off --property=ProtectHome=off --property=PrivateTmp=no --property=ReadWritePaths=/ /var/lib/dpkg /var/lib/rpm /opt/nat-query /data dpkg -i ") {
		t.Fatalf("commands = %#v, want package validation then install", commands)
	}
}

func TestReplaceFileAtomicSwapsBinaryContent(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "downloaded")
	targetPath := filepath.Join(dir, "nat-query-service")
	if err := os.WriteFile(sourcePath, []byte("new-binary"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := replaceFileAtomic(sourcePath, targetPath, 0o755); err != nil {
		t.Fatalf("replace file: %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "new-binary" {
		t.Fatalf("target content = %q", content)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("target mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestUpgradeAPIsRequireSession(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	for _, tt := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/upgrade/check"},
		{method: http.MethodGet, path: "/api/upgrade/status"},
		{method: http.MethodPost, path: "/api/upgrade/run", body: `{"version":"v1.1.0"}`},
	} {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, body = %s", tt.method, tt.path, res.Code, res.Body.String())
		}
	}
}

func TestUpgradeRunRejectsConcurrentTask(t *testing.T) {
	app := NewApp(LoadConfig())
	started := make(chan struct{})
	release := make(chan struct{})
	app.upgradeRunner = func(ctx context.Context, target upgradeTarget) UpgradeStatus {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return UpgradeStatus{State: UpgradeStateSucceeded, TargetVersion: target.Version}
	}
	router := app.Router()
	cookie := loginForTest(t, router)

	firstReq := httptest.NewRequest(http.MethodPost, "/api/upgrade/run", bytes.NewBufferString(`{"version":"v1.1.0"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.AddCookie(cookie)
	firstRes := httptest.NewRecorder()
	router.ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusAccepted {
		t.Fatalf("first run status = %d, body = %s", firstRes.Code, firstRes.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upgrade runner did not start")
	}
	defer close(release)

	secondReq := httptest.NewRequest(http.MethodPost, "/api/upgrade/run", bytes.NewBufferString(`{"version":"v1.1.0"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.AddCookie(cookie)
	secondRes := httptest.NewRecorder()
	router.ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusConflict {
		t.Fatalf("second run status = %d, body = %s", secondRes.Code, secondRes.Body.String())
	}
}

func loginForTest(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", res.Code, res.Body.String())
	}
	var payload SessionResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if !payload.Authenticated {
		t.Fatalf("login payload = %#v", payload)
	}
	cookies := res.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a cookie")
	}
	return cookies[0]
}

func stubLookPath(t *testing.T, available map[string]bool) func() {
	t.Helper()
	original := lookPath
	lookPath = func(file string) (string, error) {
		if available[file] {
			return "/usr/bin/" + file, nil
		}
		return "", errors.New("not found")
	}
	return func() {
		lookPath = original
	}
}

func stubOSRelease(t *testing.T, content string) func() {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write os-release: %v", err)
	}
	original := osReleasePath
	osReleasePath = path
	return func() {
		osReleasePath = original
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stubHTTPClient(t *testing.T, responses map[string]string) func() {
	t.Helper()
	original := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, ok := responses[req.URL.String()]
			if !ok {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("not found")),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}
	return func() {
		http.DefaultClient = original
	}
}

type commandRunnerStub struct {
	count   int
	restore func()
}

func stubCommandRunner(t *testing.T, fn func(context.Context, string, ...string) ([]byte, error)) *commandRunnerStub {
	t.Helper()
	original := runCommand
	stub := &commandRunnerStub{}
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		stub.count++
		return fn(ctx, name, args...)
	}
	stub.restore = func() {
		runCommand = original
	}
	return stub
}
