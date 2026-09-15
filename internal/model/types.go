package model

import (
	"fmt"
	"time"
)

type IngestStatus string

const (
	StatusIdle      IngestStatus = "idle"
	StatusPending   IngestStatus = "pending"
	StatusScanning  IngestStatus = "scanning"
	StatusImporting IngestStatus = "importing"
	StatusReady     IngestStatus = "ready"
	StatusNoData    IngestStatus = "no_data"
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

type LogSource struct {
	SourceID             string    `json:"source_id"`
	StorageSourceID      string    `json:"storage_source_id,omitempty"`
	SerialNumber         string    `json:"serial_number,omitempty"`
	LogDir               string    `json:"log_dir"`
	LogTag               string    `json:"log_tag"`
	Enabled              bool      `json:"enabled"`
	SourceType           string    `json:"source_type,omitempty"`
	ListenProtocol       string    `json:"listen_protocol,omitempty"`
	ListenHost           string    `json:"listen_host,omitempty"`
	ListenPort           int       `json:"listen_port,omitempty"`
	SpoolDir             string    `json:"spool_dir,omitempty"`
	ClientIP             string    `json:"client_ip,omitempty"`
	ArchiveDir           string    `json:"archive_dir,omitempty"`
	ArchiveRetentionDays int       `json:"archive_retention_days,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type DateIngestState struct {
	SourceID            string       `json:"source_id"`
	LogTag              string       `json:"log_tag"`
	LogDate             time.Time    `json:"log_date"`
	Status              IngestStatus `json:"status"`
	FilesTotal          uint64       `json:"files_total"`
	FilesDone           uint64       `json:"files_done"`
	RowsImported        uint64       `json:"rows_imported"`
	BytesTotal          uint64       `json:"bytes_total"`
	BytesDone           uint64       `json:"bytes_done"`
	CurrentFile         string       `json:"current_file"`
	ProgressPct         float64      `json:"progress_pct"`
	MaxVisibleTimestamp time.Time    `json:"max_visible_timestamp"`
	RetryCount          uint8        `json:"retry_count"`
	NextRetryAt         time.Time    `json:"next_retry_at"`
	Error               string       `json:"error"`
	UpdatedAt           time.Time    `json:"updated_at"`
}

type FileIngestState struct {
	Path         string       `json:"path"`
	SourceID     string       `json:"source_id"`
	LogTag       string       `json:"log_tag"`
	LogDate      time.Time    `json:"log_date"`
	Status       IngestStatus `json:"status"`
	RowsImported uint64       `json:"rows_imported"`
	BytesTotal   uint64       `json:"bytes_total"`
	BytesDone    uint64       `json:"bytes_done"`
	ProgressPct  float64      `json:"progress_pct"`
	RetryCount   uint8        `json:"retry_count"`
	NextRetryAt  time.Time    `json:"next_retry_at"`
	Error        string       `json:"error"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type QueryPageOptions struct {
	Page     int
	PageSize int
	Cursor   any
}

type DistributionItem struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

type LogTrendPoint struct {
	Date     string `json:"date"`
	SourceID string `json:"source_id"`
	LogTag   string `json:"log_tag"`
	Value    uint64 `json:"value"`
}

type CPUHealth struct {
	Status      string  `json:"status"`
	LoadPercent float64 `json:"load_percent"`
	LoadAverage float64 `json:"load_average"`
	Cores       int     `json:"cores"`
	Description string  `json:"description"`
}

type MemoryHealth struct {
	Status         string  `json:"status"`
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	Description    string  `json:"description"`
}

type DatabaseHealth struct {
	Status               string  `json:"status"`
	Version              string  `json:"version"`
	ActiveQueries        uint64  `json:"active_queries"`
	ActiveMerges         uint64  `json:"active_merges"`
	ActiveParts          uint64  `json:"active_parts"`
	TotalRows            uint64  `json:"total_rows"`
	DiskUsedBytes        uint64  `json:"disk_used_bytes"`
	DiskTotalBytes       uint64  `json:"disk_total_bytes"`
	DiskAvailableBytes   uint64  `json:"disk_available_bytes"`
	DiskAvailablePercent float64 `json:"disk_available_percent"`
	Description          string  `json:"description"`
}

type SystemHealth struct {
	CPU      CPUHealth      `json:"cpu"`
	Memory   MemoryHealth   `json:"memory"`
	Database DatabaseHealth `json:"database"`
	HostDisk DiskHealth     `json:"host_disk"`
}

type DiskHealth struct {
	Path             string  `json:"path"`
	TotalBytes       uint64  `json:"total_bytes"`
	AvailableBytes   uint64  `json:"available_bytes"`
	AvailablePercent float64 `json:"available_percent"`
}

type DashboardMetrics struct {
	ClickHouseDiskUsedBytes uint64
	TodayRows               uint64
	YesterdayRows           uint64
	TopSourceIPs            []DistributionItem
	TopDestinationIPs       []DistributionItem
	DestinationSubnets      []DistributionItem
	TopNATIPs               []DistributionItem
	AddressTypeShares       []DistributionItem
	LogTagDistribution      []DistributionItem
	TopCountries            []DistributionItem
	TopRegions              []DistributionItem
	UnrecognizedIPRate      float64
	GeoIPLoaded             bool
	GeoIPStatus             string
	SystemHealth            SystemHealth
	LogTrend                []LogTrendPoint
	LastAutoScanAt          time.Time
	NextAutoScanAt          time.Time
	AutoScanPolicy          string
	AutoScanEnabled         bool
	AutoScanMode            string
}

type AccessTrendPoint struct {
	ID           string    `json:"id"`
	TargetIP     string    `json:"target_ip"`
	TargetRegion string    `json:"target_region"`
	Count        uint64    `json:"count"`
	Mode         string    `json:"mode"`
	Coord        []float64 `json:"coord,omitempty"`
}

type AccessTrendSummary struct {
	Total    uint64 `json:"total"`
	Domestic uint64 `json:"domestic"`
	Overseas uint64 `json:"overseas"`
	Unknown  uint64 `json:"unknown"`
}

type AccessTrendResponse struct {
	Points  []AccessTrendPoint `json:"points"`
	Summary AccessTrendSummary `json:"summary"`
}

type IPDataStatus struct {
	Loaded        bool      `json:"loaded"`
	Error         string    `json:"error"`
	CustomMapPath string    `json:"custom_map_path"`
	GeoIPDBPath   string    `json:"geoip_db_path"`
	IPMapEnabled  bool      `json:"ip_map_enabled"`
	GeoIPEnabled  bool      `json:"geoip_enabled"`
	UpdatedAt     time.Time `json:"updated_at"`
}
