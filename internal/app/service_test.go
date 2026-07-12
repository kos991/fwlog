package app

import (
	"context"
	"errors"
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

func TestSystemdServiceDefaultsAutoScanOff(t *testing.T) {
	content := readRepoFile(t, "nat-query-service.service")

	if !strings.Contains(content, `Environment="AUTO_SCAN_ENABLED=false"`) {
		t.Fatal("nat-query-service.service must explicitly default AUTO_SCAN_ENABLED to false for new installs")
	}
}

func TestOpenClickHouseWithRetryRetriesStartupFailures(t *testing.T) {
	oldOpen := openClickHouse
	oldAttempts := connectRetryAttempts
	oldDelay := connectRetryDelay
	defer func() {
		openClickHouse = oldOpen
		connectRetryAttempts = oldAttempts
		connectRetryDelay = oldDelay
	}()

	attempts := 0
	openClickHouse = func(context.Context, Config) (*ClickHouseStore, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("connection refused")
		}
		return &ClickHouseStore{}, nil
	}
	connectRetryAttempts = 3
	connectRetryDelay = 0

	app := NewApp(Config{})
	store, err := app.openClickHouseWithRetry(context.Background())
	if err != nil {
		t.Fatalf("openClickHouseWithRetry returned error: %v", err)
	}
	if store == nil {
		t.Fatal("openClickHouseWithRetry returned nil store")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
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
			"pull-requests: read",
			"node-version: 22",
			"TZ: Asia/Shanghai",
			"working-directory: web",
			"npm ci",
			"npm run build",
			"npm test",
			"go test -race -count=1 ./...",
			"sudo apt-get install -y cpio rpm",
			"go build -trimpath -ldflags \"-s -w\" -o dist/nat-query-service_linux_amd64 ./cmd/nat-query-service",
			"PACKAGING_MODE=full bash packaging/build-server-packages.sh --version 0.0.0 --binary dist/nat-query-service_linux_amd64 --output dist",
			"PACKAGING_MODE=upgrade bash packaging/build-server-packages.sh --version 0.0.0 --binary dist/nat-query-service_linux_amd64 --output dist",
			"Smoke test offline bundle and upgrade package contents",
			"sha256sum -c ../checksums.txt",
			"dpkg-deb --contents dist/fwlog-upgrade_0.0.0_amd64.deb",
			"rpm -qpl dist/fwlog-upgrade-v0.0.0.x86_64.rpm",
			"Test DEB package upgrade transaction",
			"fwlog-full_0.0.0_amd64.deb",
			"fwlog-upgrade_0.0.1_amd64.deb",
			"Test RPM package upgrade transaction",
			"fwlog-full-v0.0.0.x86_64.rpm",
			"fwlog-upgrade-v0.0.1.x86_64.rpm",
			"dist/fwlog-full-v0.0.0.x86_64.rpm",
			"dist/fwlog-full_0.0.0_amd64.deb",
			"dist/fwlog-full-v0.0.0-amd64.tar.gz",
			"dist/fwlog-upgrade-v0.0.0.x86_64.rpm",
			"dist/fwlog-upgrade_0.0.0_amd64.deb",
			"name: server-packages-amd64",
		},
	)
}

func TestReleaseWorkflowMatchesClickHouseBuildFlow(t *testing.T) {
	assertWorkflowMatchesBuildFlow(
		t,
		".github/workflows/release-build.yml",
		[]string{
			"pull-requests: read",
			"node-version: 22",
			"TZ: Asia/Shanghai",
			"working-directory: web",
			"npm ci",
			"npm run build",
			"npm test",
			"go test -race -count=1 ./...",
			"go build -trimpath -ldflags \"-s -w -X nat-query-service/internal/app.appVersion=$version\" -o \"release/$asset\" ./cmd/nat-query-service",
			"GOOS: linux",
			"GOARCH: amd64",
			"-X nat-query-service/internal/app.appVersion",
			"sudo apt-get install -y cpio rpm",
			"PACKAGING_MODE=full bash packaging/build-server-packages.sh --version \"$version\" --binary \"release/$asset\" --output release",
			"PACKAGING_MODE=upgrade bash packaging/build-server-packages.sh --version \"$version\" --binary \"release/$asset\" --output release",
			"cp \"release/fwlog-upgrade-v${pkg_version}.x86_64.rpm\" \"release/nat-query-service_kylin-server_amd64.rpm\"",
			"cp \"release/fwlog-upgrade_${pkg_version}_amd64.deb\" \"release/nat-query-service_debian-server_amd64.deb\"",
			"nat-query-service_linux_amd64",
			"nat-query-service_kylin-server_amd64.rpm",
			"nat-query-service_debian-server_amd64.deb",
			"fwlog-full-v${pkg_version}.x86_64.rpm",
			"fwlog-full_${pkg_version}_amd64.deb",
			"fwlog-full-v${pkg_version}-amd64.tar.gz",
			"fwlog-upgrade-v${pkg_version}.x86_64.rpm",
			"fwlog-upgrade_${pkg_version}_amd64.deb",
			"checksums.txt",
			"latest.json",
			"gh release delete-asset",
		},
	)
}

func TestServerPackageBuildBundlesGeoIPDatabase(t *testing.T) {
	buildScript := readRepoFile(t, "packaging", "build-server-packages.sh")
	for _, want := range []string{
		"GEOIP_DB_URL",
		"https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-City.mmdb",
		"GeoLite2-City.mmdb",
		"$rootfs/data/index/GeoLite2-City.mmdb",
	} {
		if !strings.Contains(buildScript, want) {
			t.Fatalf("packaging/build-server-packages.sh missing %q", want)
		}
	}

	rpmSpec := readRepoFile(t, "packaging", "rpm", "nat-query-service.spec")
	if !strings.Contains(rpmSpec, "/data/index/GeoLite2-City.mmdb") {
		t.Fatal("RPM spec must include /data/index/GeoLite2-City.mmdb")
	}
}

func TestServerPackageBuildSupportsFullAndUpgradeModes(t *testing.T) {
	buildScript := readRepoFile(t, "packaging", "build-server-packages.sh")
	for _, want := range []string{
		"PACKAGING_MODE",
		"full|upgrade",
		"fwlog-full-v${pkg_version}.${rpm_arch}.rpm",
		"fwlog-full_${pkg_version}_${deb_arch}.deb",
		"fwlog-upgrade-v${pkg_version}.${rpm_arch}.rpm",
		"fwlog-upgrade_${pkg_version}_${deb_arch}.deb",
		"include_clickhouse=false",
	} {
		if !strings.Contains(buildScript, want) {
			t.Fatalf("packaging/build-server-packages.sh missing %q", want)
		}
	}
}

func TestUpgradePackageDoesNotIncludeClickHouseRuntime(t *testing.T) {
	buildScript := readRepoFile(t, "packaging", "build-server-packages.sh")
	rpmSpec := readRepoFile(t, "packaging", "rpm", "nat-query-service.spec")

	for path, text := range map[string]string{
		"packaging/build-server-packages.sh":   buildScript,
		"packaging/rpm/nat-query-service.spec": rpmSpec,
	} {
		for _, want := range []string{
			"fwlog_include_clickhouse",
			"/opt/nat-query/clickhouse",
			"fwlog-clickhouse.service",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
	}
	assertComesBefore(t, "packaging/build-server-packages.sh", buildScript, "include_clickhouse=false", "stage_rootfs")
}
func TestServerPackagesPreserveAppSettingsDuringUpgrade(t *testing.T) {
	buildScript := readRepoFile(t, "packaging", "build-server-packages.sh")
	rpmSpec := readRepoFile(t, "packaging", "rpm", "nat-query-service.spec")

	for path, text := range map[string]string{
		"packaging/build-server-packages.sh":   buildScript,
		"packaging/rpm/nat-query-service.spec": rpmSpec,
	} {
		for _, want := range []string{
			"app_settings-before-package.tsv",
			"SELECT key, value, now() FROM app_settings FINAL FORMAT TabSeparated",
			"INSERT INTO app_settings (key, value, updated_at) FORMAT TabSeparated",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
		assertComesBefore(t, path, text, "SELECT key, value, now() FROM app_settings FINAL FORMAT TabSeparated", "INSERT INTO app_settings (key, value, updated_at) FORMAT TabSeparated")
	}
}

func TestServerPackagesReplaceLegacyNatQueryPackage(t *testing.T) {
	buildScript := readRepoFile(t, "packaging", "build-server-packages.sh")
	rpmSpec := readRepoFile(t, "packaging", "rpm", "nat-query-service.spec")

	for _, want := range []string{
		"Provides: nat-query-service = %{version}-%{release}",
		"Obsoletes: nat-query-service < %{version}-%{release}",
	} {
		if !strings.Contains(rpmSpec, want) {
			t.Fatalf("RPM spec missing legacy package replacement rule %q", want)
		}
	}
	for _, want := range []string{
		"Provides: nat-query-service",
		"Replaces: nat-query-service",
		"Breaks: nat-query-service",
	} {
		if !strings.Contains(buildScript, want) {
			t.Fatalf("DEB control generation missing legacy package replacement rule %q", want)
		}
	}
}

func TestReleaseWorkflowRegeneratesChecksumsForUploadedAssets(t *testing.T) {
	text := readRepoFile(t, ".github", "workflows", "release-build.yml")
	for _, want := range []string{
		"(cd release && sha256sum",
		"nat-query-service_linux_amd64",
		"nat-query-service_kylin-server_amd64.rpm",
		"nat-query-service_debian-server_amd64.deb",
		"fwlog-full-v${pkg_version}.x86_64.rpm",
		"fwlog-full_${pkg_version}_amd64.deb",
		"fwlog-full-v${pkg_version}-amd64.tar.gz",
		"fwlog-upgrade-v${pkg_version}.x86_64.rpm",
		"fwlog-upgrade_${pkg_version}_amd64.deb",
		"> checksums.txt",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow checksum generation missing %q", want)
		}
	}
}

func TestServerPackageAssetsAreRequiredForUpgradeChecks(t *testing.T) {
	release := githubRelease{
		TagName: "v1.1.0",
		Assets: []githubReleaseAsset{
			{Name: linuxUpgradeAssetName, BrowserDownloadURL: "https://example.test/binary"},
			{Name: "fwlog-upgrade-v1.1.0.x86_64.rpm", BrowserDownloadURL: "https://example.test/rpm"},
			{Name: "fwlog-upgrade_1.1.0_amd64.deb", BrowserDownloadURL: "https://example.test/deb"},
		},
	}

	assets, missing := releaseUpgradeAssets(release)

	if len(missing) != 0 {
		t.Fatalf("missing assets = %#v", missing)
	}
	if assets.LegacyBinaryURL == "" || assets.UpgradeRPMURL == "" || assets.UpgradeDEBURL == "" {
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

	assertComesBefore(t, path, text, "npm run build", "go test -race")
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
