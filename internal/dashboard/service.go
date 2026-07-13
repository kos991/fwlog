package dashboard

import (
	"runtime"
	"sort"
	"strings"
	"time"
)

type HealthDashboardResponse struct {
	DataHealth      DataHealth      `json:"data_health"`
	IngestHealth    IngestHealth    `json:"ingest_health"`
	SystemHealth    SystemHealth    `json:"system_health"`
	LogTrend        []LogTrendPoint `json:"log_trend"`
	IPDistribution  IPDistribution  `json:"ip_distribution"`
	GeoDistribution GeoDistribution `json:"geo_distribution"`
}

type DataHealth struct {
	TotalLogs                uint64 `json:"total_logs"`
	ReadyDates               int    `json:"ready_dates"`
	PendingDates             int    `json:"pending_dates"`
	ImportingDates           int    `json:"importing_dates"`
	FailedDates              int    `json:"failed_dates"`
	QueryableStartDate       string `json:"queryable_start_date"`
	QueryableEndDate         string `json:"queryable_end_date"`
	LastSuccessfulIngestTime string `json:"last_successful_ingest_time"`
	ClickHouseDiskUsedBytes  uint64 `json:"clickhouse_disk_used_bytes"`
	TodayRows                uint64 `json:"today_rows"`
	YesterdayRows            uint64 `json:"yesterday_rows"`
}

type IngestHealth struct {
	Status               IngestStatus           `json:"status"`
	SourceID             string                 `json:"source_id"`
	LogTag               string                 `json:"log_tag"`
	CurrentDate          string                 `json:"current_date"`
	CurrentFile          string                 `json:"current_file"`
	FilesTotal           uint64                 `json:"files_total"`
	FilesDone            uint64                 `json:"files_done"`
	BytesTotal           uint64                 `json:"bytes_total"`
	BytesDone            uint64                 `json:"bytes_done"`
	RowsImported         uint64                 `json:"rows_imported"`
	ProgressPct          float64                `json:"progress_pct"`
	Error                string                 `json:"error"`
	LastAutoScanAt       string                 `json:"last_auto_scan_at"`
	NextAutoScanAt       string                 `json:"next_auto_scan_at"`
	AutoScanPolicy       string                 `json:"auto_scan_policy"`
	AutoScanEnabled      bool                   `json:"auto_scan_enabled"`
	AutoScanMode         string                 `json:"auto_scan_mode"`
	ElapsedSec           int64                  `json:"elapsed_sec"`
	ETASeconds           int64                  `json:"eta_sec"`
	LastUpdatedAt        time.Time              `json:"last_updated_at"`
	LastSuccessfulIngest time.Time              `json:"last_successful_ingest_at"`
	Sources              []SourceIngestProgress `json:"sources"`
}

type SourceIngestProgress struct {
	SourceID     string       `json:"source_id"`
	LogTag       string       `json:"log_tag"`
	Status       IngestStatus `json:"status"`
	CurrentDate  string       `json:"current_date"`
	CurrentFile  string       `json:"current_file"`
	RowsImported uint64       `json:"rows_imported"`
	ProgressPct  float64      `json:"progress_pct"`
	Error        string       `json:"error"`
}

type IPDistribution struct {
	TopSourceIPs       []DistributionItem `json:"top_source_ips"`
	TopDestinationIPs  []DistributionItem `json:"top_destination_ips"`
	TopNATIPs          []DistributionItem `json:"top_nat_ips"`
	AddressTypeShares  []DistributionItem `json:"address_type_shares"`
	LogTagDistribution []DistributionItem `json:"log_tag_distribution"`
}

type GeoDistribution struct {
	TopCountries       []DistributionItem `json:"top_countries"`
	TopRegions         []DistributionItem `json:"top_regions"`
	UnrecognizedIPRate float64            `json:"unrecognized_ip_rate"`
	GeoIPLoaded        bool               `json:"geoip_loaded"`
	GeoIPStatus        string             `json:"geoip_status"`
}

type IngestProgressResponse struct {
	Status          IngestStatus           `json:"status"`
	SourceID        string                 `json:"source_id"`
	LogTag          string                 `json:"log_tag"`
	CurrentDate     string                 `json:"current_date"`
	CurrentFile     string                 `json:"current_file"`
	FilesTotal      uint64                 `json:"files_total"`
	FilesDone       uint64                 `json:"files_done"`
	BytesTotal      uint64                 `json:"bytes_total"`
	BytesDone       uint64                 `json:"bytes_done"`
	RowsImported    uint64                 `json:"rows_imported"`
	ProgressPct     float64                `json:"progress_pct"`
	ElapsedSec      int64                  `json:"elapsed_sec"`
	ETASeconds      int64                  `json:"eta_sec"`
	LastAutoScanAt  string                 `json:"last_auto_scan_at"`
	NextAutoScanAt  string                 `json:"next_auto_scan_at"`
	AutoScanPolicy  string                 `json:"auto_scan_policy"`
	AutoScanEnabled bool                   `json:"auto_scan_enabled"`
	AutoScanMode    string                 `json:"auto_scan_mode"`
	Error           string                 `json:"error"`
	Dates           []DateIngestState      `json:"dates"`
	Sources         []SourceIngestProgress `json:"sources"`
}

func BuildHealthDashboard(states []DateIngestState, metrics DashboardMetrics) HealthDashboardResponse {
	return HealthDashboardResponse{
		DataHealth:      buildDataHealth(states, metrics),
		IngestHealth:    buildIngestHealth(states, metrics),
		SystemHealth:    buildSystemHealth(metrics),
		LogTrend:        metrics.LogTrend,
		IPDistribution:  buildIPDistribution(metrics),
		GeoDistribution: buildGeoDistribution(metrics),
	}
}

func BuildIngestProgress(states []DateIngestState, includeReady bool, metricArgs ...DashboardMetrics) IngestProgressResponse {
	var metrics DashboardMetrics
	if len(metricArgs) > 0 {
		metrics = metricArgs[0]
	}

	var current DateIngestState
	for _, state := range states {
		if state.Status == StatusImporting {
			current = state
			break
		}
		if current.Status == "" && state.Status == StatusFailed {
			current = state
		}
	}

	response := IngestProgressResponse{
		Status:          StatusIdle,
		LastAutoScanAt:  formatDateTime(metrics.LastAutoScanAt),
		NextAutoScanAt:  formatDateTime(metrics.NextAutoScanAt),
		AutoScanPolicy:  metrics.AutoScanPolicy,
		AutoScanEnabled: metrics.AutoScanEnabled,
		AutoScanMode:    metrics.AutoScanMode,
		Dates:           make([]DateIngestState, 0, len(states)),
		Sources:         buildSourceIngestProgress(states),
	}
	if current.Status != "" {
		response.Status = current.Status
		response.SourceID = current.SourceID
		response.LogTag = current.LogTag
		response.CurrentDate = formatDate(current.LogDate)
		response.CurrentFile = current.CurrentFile
		response.FilesTotal = current.FilesTotal
		response.FilesDone = current.FilesDone
		response.BytesTotal = current.BytesTotal
		response.BytesDone = current.BytesDone
		response.RowsImported = current.RowsImported
		response.ProgressPct = current.ProgressPct
		response.Error = current.Error
	}

	for _, state := range states {
		if !includeReady && state.Status == StatusReady {
			continue
		}
		response.Dates = append(response.Dates, state)
	}

	return response
}

func buildDataHealth(states []DateIngestState, metrics DashboardMetrics) DataHealth {
	var health DataHealth
	health.ClickHouseDiskUsedBytes = metrics.ClickHouseDiskUsedBytes
	health.TodayRows = metrics.TodayRows
	health.YesterdayRows = metrics.YesterdayRows

	var firstReady time.Time
	var lastReady time.Time
	var lastSuccessful time.Time
	for _, state := range states {
		switch state.Status {
		case StatusReady:
			health.ReadyDates++
			health.TotalLogs += state.RowsImported
			if firstReady.IsZero() || state.LogDate.Before(firstReady) {
				firstReady = state.LogDate
			}
			if lastReady.IsZero() || state.LogDate.After(lastReady) {
				lastReady = state.LogDate
			}
			if state.UpdatedAt.After(lastSuccessful) {
				lastSuccessful = state.UpdatedAt
			}
		case StatusPending:
			health.PendingDates++
		case StatusImporting:
			health.ImportingDates++
		case StatusFailed:
			health.FailedDates++
		}
	}

	health.QueryableStartDate = formatDate(firstReady)
	health.QueryableEndDate = formatDate(lastReady)
	health.LastSuccessfulIngestTime = formatDateTime(lastSuccessful)
	return health
}

func buildIngestHealth(states []DateIngestState, metrics DashboardMetrics) IngestHealth {
	progress := BuildIngestProgress(states, false)
	health := IngestHealth{
		Status:          progress.Status,
		SourceID:        progress.SourceID,
		LogTag:          progress.LogTag,
		CurrentDate:     progress.CurrentDate,
		CurrentFile:     progress.CurrentFile,
		FilesTotal:      progress.FilesTotal,
		FilesDone:       progress.FilesDone,
		BytesTotal:      progress.BytesTotal,
		BytesDone:       progress.BytesDone,
		RowsImported:    progress.RowsImported,
		ProgressPct:     progress.ProgressPct,
		Error:           progress.Error,
		LastAutoScanAt:  formatDateTime(metrics.LastAutoScanAt),
		NextAutoScanAt:  formatDateTime(metrics.NextAutoScanAt),
		AutoScanPolicy:  metrics.AutoScanPolicy,
		AutoScanEnabled: metrics.AutoScanEnabled,
		AutoScanMode:    metrics.AutoScanMode,
		Sources:         buildSourceIngestProgress(states),
	}

	for _, state := range states {
		if state.UpdatedAt.After(health.LastUpdatedAt) {
			health.LastUpdatedAt = state.UpdatedAt
		}
		if state.Status == StatusReady && state.UpdatedAt.After(health.LastSuccessfulIngest) {
			health.LastSuccessfulIngest = state.UpdatedAt
		}
	}
	return health
}

func buildSourceIngestProgress(states []DateIngestState) []SourceIngestProgress {
	selected := make(map[string]DateIngestState)
	for _, state := range states {
		if state.SourceID == "" {
			continue
		}
		current, exists := selected[state.SourceID]
		if !exists || state.UpdatedAt.After(current.UpdatedAt) ||
			(state.UpdatedAt.Equal(current.UpdatedAt) && state.LogDate.After(current.LogDate)) {
			selected[state.SourceID] = state
		}
	}
	sourceIDs := make([]string, 0, len(selected))
	for sourceID := range selected {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)
	progress := make([]SourceIngestProgress, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		state := selected[sourceID]
		progress = append(progress, SourceIngestProgress{
			SourceID: sourceID, LogTag: state.LogTag, Status: state.Status,
			CurrentDate: formatDate(state.LogDate), CurrentFile: state.CurrentFile,
			RowsImported: state.RowsImported, ProgressPct: state.ProgressPct, Error: state.Error,
		})
	}
	return progress
}

func buildIPDistribution(metrics DashboardMetrics) IPDistribution {
	return IPDistribution{
		TopSourceIPs:       metrics.TopSourceIPs,
		TopDestinationIPs:  metrics.TopDestinationIPs,
		AddressTypeShares:  metrics.AddressTypeShares,
		LogTagDistribution: metrics.LogTagDistribution,
	}
}

func buildGeoDistribution(metrics DashboardMetrics) GeoDistribution {
	return GeoDistribution{
		TopCountries:       metrics.TopCountries,
		TopRegions:         metrics.TopRegions,
		UnrecognizedIPRate: metrics.UnrecognizedIPRate,
		GeoIPLoaded:        metrics.GeoIPLoaded,
		GeoIPStatus:        metrics.GeoIPStatus,
	}
}

func buildSystemHealth(metrics DashboardMetrics) SystemHealth {
	return metrics.SystemHealth
}

func EnrichGeoDistributionMetrics(metrics DashboardMetrics, engine *IPEngine) DashboardMetrics {
	return enrichGeoDistributionMetrics(metrics, engine)
}

func enrichGeoDistributionMetrics(metrics DashboardMetrics, engine *IPEngine) DashboardMetrics {
	if engine == nil || len(metrics.TopCountries) > 0 {
		return metrics
	}

	destinations := metrics.DestinationSubnets
	if len(destinations) == 0 {
		destinations = metrics.TopDestinationIPs
	}
	countries, regions := aggregateGeoDestinations(destinations, engine)

	metrics.TopCountries = topDistributionItems(countries, 10)
	metrics.TopRegions = topDistributionItems(regions, 10)
	return metrics
}

type geoDistributionTotals struct {
	countries map[string]uint64
	regions   map[string]uint64
}

func aggregateGeoDestinations(destinations []DistributionItem, engine *IPEngine) (map[string]uint64, map[string]uint64) {
	countries := make(map[string]uint64)
	regions := make(map[string]uint64)
	if len(destinations) == 0 || engine == nil {
		return countries, regions
	}

	workers := min(runtime.GOMAXPROCS(0), 8, len(destinations))
	chunkSize := (len(destinations) + workers - 1) / workers
	partials := make(chan geoDistributionTotals, workers)
	for worker := 0; worker < workers; worker++ {
		start := worker * chunkSize
		end := min(start+chunkSize, len(destinations))
		if start >= end {
			partials <- geoDistributionTotals{countries: map[string]uint64{}, regions: map[string]uint64{}}
			continue
		}
		go func(items []DistributionItem) {
			partial := geoDistributionTotals{countries: make(map[string]uint64), regions: make(map[string]uint64)}
			aggregateGeoDestinationChunk(items, engine, partial.countries, partial.regions)
			partials <- partial
		}(destinations[start:end])
	}

	for worker := 0; worker < workers; worker++ {
		partial := <-partials
		for name, value := range partial.countries {
			countries[name] += value
		}
		for name, value := range partial.regions {
			regions[name] += value
		}
	}
	return countries, regions
}

func aggregateGeoDestinationChunk(destinations []DistributionItem, engine *IPEngine, countries map[string]uint64, regions map[string]uint64) {
	for _, item := range destinations {
		if item.Name == "" || item.Value == 0 {
			continue
		}
		tag := engine.GetTag(item.Name)
		country, region := splitGeoLocation(tag.Location)
		if country != "" {
			countries[country] += item.Value
		}
		if region != "" {
			regions[region] += item.Value
		}
	}
}

func splitGeoLocation(location string) (string, string) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", ""
	}
	if strings.Contains(location, "公网") {
		return "未知", "未知"
	}
	parts := strings.Split(location, "/")
	country := strings.TrimSpace(parts[0])
	region := country
	if len(parts) > 1 {
		if right := strings.TrimSpace(parts[1]); right != "" {
			region = right
		}
	}
	return country, region
}

func topDistributionItems(values map[string]uint64, limit int) []DistributionItem {
	if len(values) == 0 || limit <= 0 {
		return nil
	}

	items := make([]DistributionItem, 0, len(values))
	for name, value := range values {
		if name == "" || value == 0 {
			continue
		}
		items = append(items, DistributionItem{Name: name, Value: value})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Value == items[j].Value {
			return items[i].Name < items[j].Name
		}
		return items[i].Value > items[j].Value
	})
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func formatDate(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02")
}

func formatDateTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02 15:04:05")
}
