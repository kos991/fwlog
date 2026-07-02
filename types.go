package main

type IngestStatus string

const (
	StatusIdle      IngestStatus = "idle"
	StatusPending   IngestStatus = "pending"
	StatusScanning  IngestStatus = "scanning"
	StatusImporting IngestStatus = "importing"
	StatusReady     IngestStatus = "ready"
	StatusFailed    IngestStatus = "failed"
	StatusSucceeded IngestStatus = "succeeded"
)

type Config struct {
	LogDir              string
	LogTag              string
	Port                int
	Workers             int
	ClickHouseAddr      string
	ClickHouseDatabase  string
	ClickHouseUser      string
	ClickHousePassword  string
	CustomIPMapPath     string
	GeoIPDBPath         string
	IPMapEnabled        bool
	GeoIPEnabled        bool
	AutoScanEnabled     bool
	AutoScanMode        string
	AutoScanTimes       string
	AutoScanIntervalSec int
	AutoScanTimezone    string
	AutoScanJitterSec   int
	ExportRoot          string
}
