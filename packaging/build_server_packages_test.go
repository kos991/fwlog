package packaging_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltPackagesContainModeSpecificUnitsAndStrictRestartScripts(t *testing.T) {
	if testing.Short() {
		t.Skip("短测试模式不构建系统安装包")
	}
	for _, tool := range []string{"bash", "cpio", "dpkg-deb", "rpm", "rpm2cpio", "rpmbuild"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("缂哄皯 %s锛岀湡瀹炲寘妫€鏌ヤ粎鍦?Linux 鍙戝竷鐜杩愯", tool)
		}
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	assets := t.TempDir()
	binaryPath := filepath.Join(assets, "fwlog")
	clickHousePath := filepath.Join(assets, "clickhouse")
	geoIPPath := filepath.Join(assets, "GeoLite2-City.mmdb")
	for _, path := range []string{binaryPath, clickHousePath, geoIPPath} {
		if err := os.WriteFile(path, []byte("package-test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		mode       string
		unitNeedle string
		unitReject string
	}{
		{"full", "Requires=network-online.target fwlog-clickhouse.service", "clickhouse-server.service"},
		{"upgrade", "After=network-online.target fwlog-clickhouse.service clickhouse-server.service", "Requires=network-online.target fwlog-clickhouse.service"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			outputDir := filepath.Join(t.TempDir(), "dist")
			cmd := exec.Command("bash", filepath.Join(repoRoot, "packaging", "build-server-packages.sh"),
				"--version", "9.9.9", "--binary", binaryPath, "--output", outputDir, "--mode", tc.mode)
			cmd.Env = append(os.Environ(), "CLICKHOUSE_BINARY="+clickHousePath, "GEOIP_DB_PATH="+geoIPPath)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("鏋勫缓 %s 鍖呭け璐? %v\n%s", tc.mode, err, output)
			}

			deb := firstMatch(t, filepath.Join(outputDir, "*.deb"))
			debRoot := filepath.Join(t.TempDir(), "deb")
			run(t, "dpkg-deb", "-x", deb, debRoot)
			debControl := filepath.Join(t.TempDir(), "control")
			run(t, "dpkg-deb", "-e", deb, debControl)
			assertUnit(t, filepath.Join(debRoot, "etc", "systemd", "system", "fwlog.service"), tc.unitNeedle, tc.unitReject)
			assertStrictRestart(t, filepath.Join(debControl, "postinst"))
			assertDebMaintainerScriptsParse(t, debControl)

			rpmPackage := firstMatch(t, filepath.Join(outputDir, "*.rpm"))
			rpmScripts := run(t, "rpm", "-qp", "--scripts", rpmPackage)
			if strings.Contains(rpmScripts, "systemctl restart fwlog.service || true") {
				t.Fatal("RPM 瀹夎鍚庤剼鏈悶鎺変簡搴旂敤鏈嶅姟閲嶅惎澶辫触")
			}
			rpmRoot := filepath.Join(t.TempDir(), "rpm")
			if err := os.MkdirAll(rpmRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			runInDir(t, rpmRoot, "bash", "-c", fmt.Sprintf("rpm2cpio %s | cpio -idm --quiet", shellQuote(rpmPackage)))
			assertUnit(t, filepath.Join(rpmRoot, "etc", "systemd", "system", "fwlog.service"), tc.unitNeedle, tc.unitReject)
		})
	}
}

func firstMatch(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) != 1 {
		t.Fatalf("%s 搴斿尮閰嶅敮涓€浜х墿锛屽疄闄?%v锛岄敊璇?%v", pattern, matches, err)
	}
	return matches[0]
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	return runInDir(t, "", name, args...)
}

func runInDir(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s 鎵ц澶辫触: %v\n%s", name, err, output)
	}
	return string(output)
}

func assertUnit(t *testing.T, path, required, forbidden string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unit := string(data)
	if !strings.Contains(unit, required) || strings.Contains(unit, forbidden) {
		t.Fatalf("unit 渚濊禆璇箟涓嶆纭紝瑕佹眰 %q锛岀姝?%q\n%s", required, forbidden, unit)
	}
}

func assertStrictRestart(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "systemctl restart fwlog.service || true") {
		t.Fatalf("%s 吞掉了应用服务重启失败", path)
	}
}

func TestPackagedServiceUnitMatchesPackageMode(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	scriptData, err := os.ReadFile(filepath.Join(repoRoot, "packaging", "build-server-packages.sh"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(scriptData), "stage_rootfs() {")
	end := strings.Index(string(scriptData), "\nbuild_deb() {")
	if start < 0 || end <= start {
		t.Fatal("could not locate stage_rootfs in packaging script")
	}
	stageFunction := string(scriptData)[start:end]
	if strings.Contains(string(scriptData), "systemctl restart fwlog.service || true") {
		t.Fatal("package install must fail when the application service cannot restart")
	}
	if !strings.Contains(string(scriptData), `cd "\$bundle_dir/packages"`) || !strings.Contains(string(scriptData), `sha256sum -c ../checksums.txt`) {
		t.Fatal("offline installer must verify package checksums from the packages directory")
	}

	for _, tc := range []struct {
		name              string
		includeClickHouse string
		required          string
		forbidden         string
		hasRuntime        bool
	}{
		{
			name:              "full",
			includeClickHouse: "true",
			required:          "Requires=network-online.target fwlog-clickhouse.service",
			forbidden:         "clickhouse-server.service",
			hasRuntime:        true,
		},
		{
			name:              "upgrade",
			includeClickHouse: "false",
			required:          "After=network-online.target fwlog-clickhouse.service clickhouse-server.service",
			forbidden:         "Requires=network-online.target fwlog-clickhouse.service",
			hasRuntime:        false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			binaryPath := filepath.Join(tempDir, "fwlog")
			geoIPPath := filepath.Join(tempDir, "GeoLite2-City.mmdb")
			clickHousePath := filepath.Join(tempDir, "clickhouse")
			for _, path := range []string{binaryPath, geoIPPath, clickHousePath} {
				if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			rootfs := filepath.Join(tempDir, "rootfs")
			shellScript := "set -euo pipefail\n" +
				"repo_root=" + shellQuote(filepath.ToSlash(repoRoot)) + "\n" +
				"binary_path=" + shellQuote(filepath.ToSlash(binaryPath)) + "\n" +
				"pkg_version=9.9.9\n" +
				"CLICKHOUSE_VERSION=25.8.27.1\n" +
				"include_clickhouse=" + shellQuote(tc.includeClickHouse) + "\n" +
				stageFunction + "\n" +
				"stage_rootfs " + shellQuote(filepath.ToSlash(rootfs)) + " " + shellQuote(filepath.ToSlash(clickHousePath)) + " " + shellQuote(filepath.ToSlash(geoIPPath)) + "\n"
			cmd := exec.Command("bash", "-c", shellScript)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("stage_rootfs failed: %v\n%s", err, output)
			}
			unitPath := filepath.Join(rootfs, "etc", "systemd", "system", "fwlog.service")
			unitData, err := os.ReadFile(unitPath)
			if err != nil {
				t.Fatal(err)
			}
			unit := strings.ReplaceAll(string(unitData), "\r\n", "\n")
			if !strings.Contains(unit, tc.required) {
				t.Fatalf("service unit does not contain required dependency %q\n%s", tc.required, unit)
			}
			if strings.Contains(unit, tc.forbidden) {
				t.Fatalf("service unit contains forbidden dependency %q\n%s", tc.forbidden, unit)
			}
			versionData, err := os.ReadFile(filepath.Join(rootfs, "opt", "fwlog", "VERSION"))
			if err != nil || strings.TrimSpace(string(versionData)) != "VERSION=v9.9.9" {
				t.Fatalf("VERSION file is invalid: %q, error %v", versionData, err)
			}
			runtimePath := filepath.Join(rootfs, "opt", "fwlog", "RUNTIME_VERSION")
			runtimeData, err := os.ReadFile(runtimePath)
			if tc.hasRuntime {
				if err != nil || strings.TrimSpace(string(runtimeData)) != "RUNTIME_VERSION=clickhouse-25.8.27.1" {
					t.Fatalf("RUNTIME_VERSION file is invalid: %q, error %v", runtimeData, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("upgrade rootfs must not contain RUNTIME_VERSION: %q, error %v", runtimeData, err)
			}
		})
	}
}

func TestPackageInstallScriptsPropagateServiceRestartFailure(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		filepath.Join(repoRoot, "packaging", "build-server-packages.sh"),
		filepath.Join(repoRoot, "packaging", "rpm", "fwlog.spec"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "systemctl restart fwlog.service || true") {
			t.Fatalf("%s suppresses application restart failure", path)
		}
		if strings.Contains(string(data), "runtime_backup") {
			t.Fatalf("%s preserves runtime metadata with maintainer script backup instead of package ownership", path)
		}
	}

	spec, err := os.ReadFile(filepath.Join(repoRoot, "packaging", "rpm", "fwlog.spec"))
	if err != nil {
		t.Fatal(err)
	}
	specText := string(spec)
	if !strings.Contains(specText, "/usr/bin/clickhouse-client") || !strings.Contains(specText, "for candidate in") {
		t.Fatal("RPM upgrade must discover the ClickHouse client installed by the existing deployment")
	}
}

func TestPackageInstallScriptsStartEmbeddedDatabaseBeforeApplication(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	buildScript, err := os.ReadFile(filepath.Join(repoRoot, "packaging", "build-server-packages.sh"))
	if err != nil {
		t.Fatal(err)
	}
	debPostinst := between(t, string(buildScript), `cat > "$debroot/DEBIAN/postinst"`, "\nEOF")
	assertEmbeddedDatabaseStartupOrder(t, "DEB postinst", debPostinst)

	rpmSpec, err := os.ReadFile(filepath.Join(repoRoot, "packaging", "rpm", "fwlog.spec"))
	if err != nil {
		t.Fatal(err)
	}
	rpmPost := between(t, string(rpmSpec), "%post\n", "\n%preun")
	assertEmbeddedDatabaseStartupOrder(t, "RPM post", rpmPost)
}

func assertEmbeddedDatabaseStartupOrder(t *testing.T, name, script string) {
	t.Helper()
	stopApp := strings.Index(script, "systemctl stop fwlog.service")
	detectEmbedded := strings.Index(script, "systemctl cat fwlog-clickhouse.service")
	startDatabase := strings.Index(script, "systemctl start fwlog-clickhouse.service")
	waitDatabase := strings.Index(script, `client --query "SELECT 1"`)
	startApp := strings.Index(script, "systemctl start fwlog.service")

	for action, index := range map[string]int{
		"停止应用":     stopApp,
		"检测嵌入式数据库": detectEmbedded,
		"启动嵌入式数据库": startDatabase,
		"等待数据库就绪":  waitDatabase,
		"启动应用":     startApp,
	} {
		if index < 0 {
			t.Fatalf("%s 缺少%s步骤", name, action)
		}
	}
	if !(stopApp < startDatabase && detectEmbedded < startDatabase && startDatabase < waitDatabase && waitDatabase < startApp) {
		t.Fatalf("%s 启动顺序错误", name)
	}
}

func between(t *testing.T, text, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(text, startMarker)
	if start < 0 {
		t.Fatalf("找不到起始标记 %q", startMarker)
	}
	end := strings.Index(text[start+len(startMarker):], endMarker)
	if end < 0 {
		t.Fatalf("找不到结束标记 %q", endMarker)
	}
	return text[start : start+len(startMarker)+end]
}

func assertDebMaintainerScriptsParse(t *testing.T, controlDir string) {
	t.Helper()
	for _, name := range []string{"preinst", "postinst", "prerm", "postrm"} {
		run(t, "sh", "-n", filepath.Join(controlDir, name))
	}
}

func TestFullAndUpgradeArtifactsKeepRuntimeOwnerInstalled(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("build-server-packages.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `package_name="fwlog-full"`) || !strings.Contains(text, `package_name="fwlog-upgrade"`) {
		t.Fatal("full must keep owning runtime files while the thin upgrade package replaces only shared application files")
	}
	if strings.Contains(text, `deb_breaks`) || strings.Contains(text, `Breaks:`) {
		t.Fatal("full and upgrade DEBs must coexist so the full package keeps runtime ownership")
	}
	if !strings.Contains(text, `deb_replaces="fwlog, fwlog-full"`) || !strings.Contains(text, `deb_replaces="fwlog, fwlog-upgrade"`) {
		t.Fatal("full and upgrade DEBs must replace only their shared application files")
	}
	if strings.Contains(text, "runtime_backup") {
		t.Fatal("thin upgrade must preserve runtime metadata by package ownership, not by maintainer script backup")
	}
}

func TestCIDebStatusFormatIsNotExpandedByOuterShell(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `dpkg-query -W -f=`) {
		t.Fatal("DEB status check must not pass a format expression through two shell layers")
	}
	if !strings.Contains(text, `dpkg -s fwlog-upgrade | grep -Fx "Status: install ok installed"`) {
		t.Fatal("DEB status check must use the package status field without shell expansion")
	}
}

func TestCIRPMTransactionInstallsDeclaredSystemdDependency(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	install := "dnf install -y systemd"
	transaction := "rpm -Uvh /packages/fwlog-full-v0.0.0.x86_64.rpm"
	if !strings.Contains(text, install) {
		t.Fatal("RPM transaction container must install the package's systemd dependency")
	}
	if strings.Index(text, install) > strings.Index(text, transaction) {
		t.Fatal("RPM transaction container must install systemd before the full package")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
