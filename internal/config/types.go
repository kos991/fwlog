package config

type Config struct {
	LogDir                    string
	LogTag                    string
	Port                      int
	TLSEnabled                bool
	TLSCertPath               string
	TLSKeyPath                string
	Workers                   int
	ClickHouseAddr            string
	ClickHouseDatabase        string
	ClickHouseUser            string
	ClickHousePassword        string
	CustomIPMapPath           string
	GeoIPDBPath               string
	CIDRAliases               []CIDRAliasSetting
	IPMapEnabled              bool
	GeoIPEnabled              bool
	AutoScanEnabled           bool
	AutoScanMode              string
	AutoScanTimes             string
	AutoScanIntervalSec       int
	AutoScanTimezone          string
	AutoScanJitterSec         int
	ExportRoot                string
	ThreatIntelligenceKeyFile string
}

type CIDRAliasSetting struct {
	CIDR    string `json:"cidr"`
	Alias   string `json:"alias"`
	Enabled bool   `json:"enabled"`
}
