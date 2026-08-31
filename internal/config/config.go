package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultLogDir                    = "/data/sangfor_fw_log"
	defaultLogTag                    = "\u6df1\u4fe1\u670d NAT"
	defaultExportDir                 = "/data/export"
	defaultThreatIntelligenceKeyFile = "/data/fwlog/threat-intelligence.key"
	defaultPort                      = 8080
	defaultWorkers                   = 1
	defaultAutoScanSec               = 3600
)

func LoadConfig() Config {
	return Config{
		LogDir:                    getEnv("LOG_DIR", defaultLogDir),
		LogTag:                    normalizeLogTag(getEnv("LOG_TAG", defaultLogTag)),
		Port:                      getEnvInt("PORT", defaultPort),
		Workers:                   getEnvInt("WORKERS", defaultWorkers),
		ClickHouseAddr:            getEnv("CLICKHOUSE_ADDR", "127.0.0.1:9000"),
		ClickHouseDatabase:        getEnv("CLICKHOUSE_DATABASE", "default"),
		ClickHouseUser:            getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword:        os.Getenv("CLICKHOUSE_PASSWORD"),
		CustomIPMapPath:           getEnv("CUSTOM_IP_MAP", "/opt/fwlog/custom_ip_map.csv"),
		GeoIPDBPath:               getEnv("GEOIP_DB", "/data/index/GeoLite2-City.mmdb"),
		IPMapEnabled:              getEnvBool("IP_MAP_ENABLED", true),
		GeoIPEnabled:              getEnvBool("GEOIP_ENABLED", true),
		AutoScanEnabled:           getEnvBool("AUTO_SCAN_ENABLED", false),
		AutoScanMode:              getEnv("AUTO_SCAN_MODE", "daily"),
		AutoScanTimes:             getEnv("AUTO_SCAN_TIMES", "01:00"),
		AutoScanIntervalSec:       getEnvInt("AUTO_SCAN_INTERVAL_SEC", defaultAutoScanSec),
		AutoScanTimezone:          getEnv("AUTO_SCAN_TIMEZONE", "Asia/Shanghai"),
		AutoScanJitterSec:         getEnvInt("AUTO_SCAN_JITTER_SEC", 60),
		ExportRoot:                getEnv("EXPORT_DIR", defaultExportDir),
		ThreatIntelligenceKeyFile: getEnv("THREAT_INTELLIGENCE_KEY_FILE", defaultThreatIntelligenceKeyFile),
	}
}

func (c Config) ExportDir() string {
	if strings.TrimSpace(c.ExportRoot) == "" {
		return defaultExportDir
	}
	return c.ExportRoot
}

func normalizeLogTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultLogTag
	}
	return value
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch {
	case value == "1", strings.EqualFold(value, "true"), strings.EqualFold(value, "yes"), strings.EqualFold(value, "on"):
		return true
	case value == "0", strings.EqualFold(value, "false"), strings.EqualFold(value, "no"), strings.EqualFold(value, "off"):
		return false
	default:
		return fallback
	}
}
