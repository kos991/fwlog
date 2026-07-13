package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigUsesClickHouseDefaults(t *testing.T) {
	t.Setenv("LOG_DIR", "")
	t.Setenv("CLICKHOUSE_ADDR", "")
	t.Setenv("CLICKHOUSE_DATABASE", "")

	cfg := LoadConfig()

	if cfg.LogDir != "/data/sangfor_fw_log" {
		t.Fatalf("LogDir = %q", cfg.LogDir)
	}
	if cfg.ClickHouseAddr != "127.0.0.1:9000" {
		t.Fatalf("ClickHouseAddr = %q", cfg.ClickHouseAddr)
	}
	if cfg.ClickHouseDatabase != "default" {
		t.Fatalf("ClickHouseDatabase = %q", cfg.ClickHouseDatabase)
	}
	if cfg.LogTag != defaultLogTag {
		t.Fatalf("LogTag = %q", cfg.LogTag)
	}
	if cfg.ExportDir() != "/data/export" {
		t.Fatalf("ExportDir = %q", cfg.ExportDir())
	}
}

func TestLoadConfigReadsRuntimeSettingsFromEnvironment(t *testing.T) {
	t.Setenv("LOG_DIR", "/logs/fw")
	t.Setenv("LOG_TAG", "edge-firewall")
	t.Setenv("CLICKHOUSE_ADDR", "10.0.0.8:9000")
	t.Setenv("CLICKHOUSE_DATABASE", "nat")
	t.Setenv("CUSTOM_IP_MAP", "/opt/nat/custom.csv")
	t.Setenv("GEOIP_DB", "/opt/nat/GeoLite2-City.mmdb")
	t.Setenv("PORT", "18080")

	cfg := LoadConfig()

	if cfg.LogDir != "/logs/fw" || cfg.LogTag != "edge-firewall" {
		t.Fatalf("unexpected log source config: %#v", cfg)
	}
	if cfg.ClickHouseAddr != "10.0.0.8:9000" || cfg.ClickHouseDatabase != "nat" {
		t.Fatalf("unexpected ClickHouse config: %#v", cfg)
	}
	if cfg.CustomIPMapPath != "/opt/nat/custom.csv" || cfg.GeoIPDBPath != "/opt/nat/GeoLite2-City.mmdb" {
		t.Fatalf("unexpected IP data config: %#v", cfg)
	}
	if cfg.Port != 18080 {
		t.Fatalf("Port = %d", cfg.Port)
	}
}

func TestGetEnvBoolRecognizesSupportedValues(t *testing.T) {
	trueCases := []string{"yes", "on", "1", "true"}
	falseCases := []string{"no", "off", "0", "false"}

	for _, value := range trueCases {
		t.Setenv("BOOL_VALUE", value)
		if got := getEnvBool("BOOL_VALUE", false); !got {
			t.Fatalf("getEnvBool(%q) = false, want true", value)
		}
	}

	for _, value := range falseCases {
		t.Setenv("BOOL_VALUE", value)
		if got := getEnvBool("BOOL_VALUE", true); got {
			t.Fatalf("getEnvBool(%q) = true, want false", value)
		}
	}
}

func TestGetEnvBoolFallsBackForInvalidValue(t *testing.T) {
	t.Setenv("BOOL_VALUE", "maybe")

	if got := getEnvBool("BOOL_VALUE", true); !got {
		t.Fatal("getEnvBool should fall back to true for invalid non-empty value")
	}
	if got := getEnvBool("BOOL_VALUE", false); got {
		t.Fatal("getEnvBool should fall back to false for invalid non-empty value")
	}
}

func TestTask1FilesNoLongerReferenceDuckDB(t *testing.T) {
	assertFileExcludes(t, "go.mod", "duckdb", "go-duckdb")
	assertFileExcludes(t, filepath.Join("cmd", "fwlog", "main.go"), "duckdb", "go-duckdb", "database/sql")
	assertFileExcludes(t, "fwlog.service", "DB_FILE=", "nat_logs.duckdb")
	assertFileIncludes(t, "fwlog.service",
		`Environment="LOG_TAG=`,
		`Environment="CLICKHOUSE_ADDR=127.0.0.1:9000"`,
		`Environment="CLICKHOUSE_DATABASE=default"`,
	)
}

func assertFileExcludes(t *testing.T, path string, disallowed ...string) {
	t.Helper()

	content := readFile(t, path)
	lowerContent := strings.ToLower(content)
	for _, token := range disallowed {
		if strings.Contains(lowerContent, strings.ToLower(token)) {
			t.Fatalf("%s unexpectedly contains %q", path, token)
		}
	}
}

func assertFileIncludes(t *testing.T, path string, required ...string) {
	t.Helper()

	content := readFile(t, path)
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("%s does not contain %q", path, token)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	return readRepoFile(t, path)
}
