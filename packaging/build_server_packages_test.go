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
			t.Skipf("缺少 %s，真实包检查仅在 Linux 发布环境运行", tool)
		}
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	assets := t.TempDir()
	binaryPath := filepath.Join(assets, "nat-query-service")
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
				t.Fatalf("构建 %s 包失败: %v\n%s", tc.mode, err, output)
			}

			deb := firstMatch(t, filepath.Join(outputDir, "*.deb"))
			debRoot := filepath.Join(t.TempDir(), "deb")
			run(t, "dpkg-deb", "-x", deb, debRoot)
			debControl := filepath.Join(t.TempDir(), "control")
			run(t, "dpkg-deb", "-e", deb, debControl)
			assertUnit(t, filepath.Join(debRoot, "etc", "systemd", "system", "nat-query-service.service"), tc.unitNeedle, tc.unitReject)
			assertStrictRestart(t, filepath.Join(debControl, "postinst"))

			rpmPackage := firstMatch(t, filepath.Join(outputDir, "*.rpm"))
			rpmScripts := run(t, "rpm", "-qp", "--scripts", rpmPackage)
			if strings.Contains(rpmScripts, "systemctl restart nat-query-service.service || true") {
				t.Fatal("RPM 安装后脚本吞掉了应用服务重启失败")
			}
			rpmRoot := filepath.Join(t.TempDir(), "rpm")
			if err := os.MkdirAll(rpmRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			runInDir(t, rpmRoot, "bash", "-c", fmt.Sprintf("rpm2cpio %s | cpio -idm --quiet", shellQuote(rpmPackage)))
			assertUnit(t, filepath.Join(rpmRoot, "etc", "systemd", "system", "nat-query-service.service"), tc.unitNeedle, tc.unitReject)
		})
	}
}

func firstMatch(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) != 1 {
		t.Fatalf("%s 应匹配唯一产物，实际 %v，错误 %v", pattern, matches, err)
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
		t.Fatalf("%s 执行失败: %v\n%s", name, err, output)
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
		t.Fatalf("unit 依赖语义不正确，要求 %q，禁止 %q\n%s", required, forbidden, unit)
	}
}

func assertStrictRestart(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "systemctl restart nat-query-service.service || true") {
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
	if strings.Contains(string(scriptData), "systemctl restart nat-query-service.service || true") {
		t.Fatal("package install must fail when the application service cannot restart")
	}

	for _, tc := range []struct {
		name              string
		includeClickHouse string
		required          string
		forbidden         string
	}{
		{
			name:              "full",
			includeClickHouse: "true",
			required:          "Requires=network-online.target fwlog-clickhouse.service",
			forbidden:         "clickhouse-server.service",
		},
		{
			name:              "upgrade",
			includeClickHouse: "false",
			required:          "After=network-online.target fwlog-clickhouse.service clickhouse-server.service",
			forbidden:         "Requires=network-online.target fwlog-clickhouse.service",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			binaryPath := filepath.Join(tempDir, "nat-query-service")
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
			unitPath := filepath.Join(rootfs, "etc", "systemd", "system", "nat-query-service.service")
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
			versionData, err := os.ReadFile(filepath.Join(rootfs, "opt", "nat-query", "VERSION"))
			if err != nil || strings.TrimSpace(string(versionData)) != "VERSION=v9.9.9" {
				t.Fatalf("VERSION file is invalid: %q, error %v", versionData, err)
			}
			runtimePath := filepath.Join(rootfs, "opt", "nat-query", "RUNTIME_VERSION")
			if tc.includeClickHouse == "true" {
				runtimeData, err := os.ReadFile(runtimePath)
				if err != nil || strings.TrimSpace(string(runtimeData)) != "RUNTIME_VERSION=clickhouse-25.8.27.1" {
					t.Fatalf("RUNTIME_VERSION file is invalid: %q, error %v", runtimeData, err)
				}
			} else if _, err := os.Stat(runtimePath); !os.IsNotExist(err) {
				t.Fatal("upgrade rootfs must not contain RUNTIME_VERSION")
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
		filepath.Join(repoRoot, "packaging", "rpm", "nat-query-service.spec"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "systemctl restart nat-query-service.service || true") {
			t.Fatalf("%s suppresses application restart failure", path)
		}
	}
	spec, err := os.ReadFile(filepath.Join(repoRoot, "packaging", "rpm", "nat-query-service.spec"))
	if err != nil {
		t.Fatal(err)
	}
	specText := string(spec)
	if !strings.Contains(specText, "/usr/bin/clickhouse-client") || !strings.Contains(specText, "for candidate in") {
		t.Fatal("RPM upgrade must discover the ClickHouse client installed by the existing deployment")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
