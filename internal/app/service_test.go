package app

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestSystemdServiceAllowsRuntimeDataDirectory(t *testing.T) {
	content := []byte(readRepoFile(t, "nat-query-service.service"))

	match := regexp.MustCompile(`(?m)^ReadWritePaths=(.+)$`).FindStringSubmatch(string(content))
	if match == nil {
		t.Fatal("ReadWritePaths is required when ProtectSystem is enabled")
	}

	paths := strings.Fields(match[1])
	if !contains(paths, "/data") {
		t.Fatalf("ReadWritePaths must include /data so the service can create /data/index and /data/export; got %q", match[1])
	}
	if !contains(paths, "/opt/nat-query") {
		t.Fatalf("ReadWritePaths must include /opt/nat-query so the service can replace its binary during Release upgrades; got %q", match[1])
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCIWorkflowMatchesClickHouseBuildFlow(t *testing.T) {
	assertWorkflowMatchesBuildFlow(
		t,
		".github/workflows/ci.yml",
		[]string{
			"node-version: 20",
			"TZ: Asia/Shanghai",
			"working-directory: web",
			"npm ci",
			"npm run build",
			"go test ./...",
			"sudo apt-get install -y rpm",
			"go build -trimpath -ldflags \"-s -w\" -o dist/nat-query-service_linux_amd64 ./cmd/nat-query-service",
			"packaging/build-server-packages.sh --version 0.0.0 --binary dist/nat-query-service_linux_amd64 --output dist",
			"dist/nat-query-service_kylin-server_amd64.rpm",
			"dist/nat-query-service_debian-server_amd64.deb",
			"name: server-packages-amd64",
		},
	)
}

func TestReleaseWorkflowMatchesClickHouseBuildFlow(t *testing.T) {
	assertWorkflowMatchesBuildFlow(
		t,
		".github/workflows/release-build.yml",
		[]string{
			"node-version: 20",
			"TZ: Asia/Shanghai",
			"working-directory: web",
			"npm ci",
			"npm run build",
			"go build -trimpath -ldflags \"-s -w -X nat-query-service/internal/app.appVersion=$version\" -o \"release/$asset\" ./cmd/nat-query-service",
			"GOOS: linux",
			"GOARCH: amd64",
			"-X nat-query-service/internal/app.appVersion",
			"sudo apt-get install -y rpm",
			"packaging/build-server-packages.sh --version \"$version\" --binary \"release/$asset\" --output release",
			"nat-query-service_kylin-server_amd64.rpm",
			"nat-query-service_debian-server_amd64.deb",
			"gh release delete-asset",
		},
	)
}

func TestServerPackageAssetsAreRequiredForUpgradeChecks(t *testing.T) {
	release := githubRelease{
		Assets: []githubReleaseAsset{
			{Name: linuxUpgradeAssetName, BrowserDownloadURL: "https://example.test/binary"},
			{Name: kylinServerPackageAssetName, BrowserDownloadURL: "https://example.test/rpm"},
			{Name: debianServerPackageAssetName, BrowserDownloadURL: "https://example.test/deb"},
		},
	}

	assets, missing := releaseUpgradeAssets(release)

	if len(missing) != 0 {
		t.Fatalf("missing assets = %#v", missing)
	}
	if assets.BinaryURL == "" || assets.KylinServerPackageURL == "" || assets.DebianServerPackageURL == "" {
		t.Fatalf("assets = %#v", assets)
	}
}

func TestReleaseDeployScriptStopsServiceBeforeReplacingBinary(t *testing.T) {
	text := readRepoFile(t, "scripts", "deploy-142-from-release.sh")
	for _, want := range []string{
		"systemctl stop nat-query-service",
		"install -m 0755 \"${work_dir}/nat-query-service\" /opt/nat-query/nat-query-service",
		"systemctl restart nat-query-service",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("deploy script missing %q", want)
		}
	}
	assertComesBefore(t, "scripts/deploy-142-from-release.sh", text, "systemctl stop nat-query-service", "install -m 0755 \"${work_dir}/nat-query-service\" /opt/nat-query/nat-query-service")
	assertComesBefore(t, "scripts/deploy-142-from-release.sh", text, "install -m 0755 \"${work_dir}/nat-query-service\" /opt/nat-query/nat-query-service", "systemctl restart nat-query-service")
}

func assertWorkflowMatchesBuildFlow(t *testing.T, path string, required []string) {
	t.Helper()

	content, err := os.ReadFile(repoPath(path))
	if err != nil {
		t.Fatalf("read workflow %s: %v", path, err)
	}

	text := string(content)
	lowerText := strings.ToLower(text)
	for _, forbidden := range forbiddenWorkflowTerms() {
		if strings.Contains(lowerText, strings.ToLower(forbidden)) {
			t.Fatalf("%s must not contain %q after ClickHouse replacement", path, forbidden)
		}
	}

	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("%s missing %q", path, want)
		}
	}

	assertComesBefore(t, path, text, "npm run build", "go test ./...")
	assertComesBefore(t, path, text, "npm run build", "go build")

	explicitFileBuild := strings.Join([]string{"main.go", "ip_engine.go"}, " ")
	if strings.Contains(text, explicitFileBuild) {
		t.Fatalf("%s must use package-level go build instead of explicit files", path)
	}
}

func assertComesBefore(t *testing.T, path, text, first, second string) {
	t.Helper()

	firstIndex := strings.Index(text, first)
	if firstIndex == -1 {
		t.Fatalf("%s missing %q", path, first)
	}

	secondIndex := strings.Index(text, second)
	if secondIndex == -1 {
		t.Fatalf("%s missing %q", path, second)
	}

	if firstIndex > secondIndex {
		t.Fatalf("%s requires %q before %q", path, first, second)
	}
}

func forbiddenWorkflowTerms() []string {
	return []string{
		strings.Join([]string{"duck", "db"}, ""),
		strings.Join([]string{"lib", "duck", "db"}, ""),
		strings.Join([]string{"duck", "db_use_", "lib"}, ""),
		strings.Join([]string{"go-", "duck", "db"}, ""),
	}
}
