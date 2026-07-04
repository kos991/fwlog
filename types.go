package main

import "fmt"

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

func (s *IngestStatus) Scan(value any) error {
	switch typed := value.(type) {
	case string:
		*s = IngestStatus(typed)
		return nil
	case []byte:
		*s = IngestStatus(string(typed))
		return nil
	case nil:
		*s = ""
		return nil
	default:
		return fmt.Errorf("cannot scan %T into IngestStatus", value)
	}
}

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
	CIDRAliases         []CIDRAliasSetting
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

type CIDRAliasSetting struct {
	CIDR    string `json:"cidr"`
	Alias   string `json:"alias"`
	Enabled bool   `json:"enabled"`
}
