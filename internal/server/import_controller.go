package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	importerpkg "fwlog/internal/importer"
)

func (a *App) importHandler(rebuild bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.currentStore() == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "clickhouse_not_connected", "message": "日志数据库尚未连接，请稍后重试"})
			return
		}
		targetDate, err := parseImportTargetDate(r)
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_date", "message": "日期格式无效，请使用 YYYY-MM-DD"})
			return
		}
		sources, found := selectImportSources(a.currentLogSources(), r.URL.Query().Get("source_id"))
		if !found {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "unknown_source", "message": "所选日志来源不存在或已停用"})
			return
		}

		a.logger.Info("import request received",
			"rebuild", rebuild,
			"target_date_from", formatOptionalDate(targetDate.Start),
			"target_date_to", formatOptionalDate(targetDate.End),
			"sources_count", len(sources),
		)

		result := a.startBackgroundImportSources(rebuild, targetDate, sources, time.Time{})

		a.logger.Info("import request processed",
			"accepted", result.Accepted,
			"busy", result.Busy,
		)
		if rebuild && len(result.Accepted) == 0 && len(result.Busy) > 0 {
			writeJSONStatus(w, http.StatusConflict, map[string]any{
				"error": "import_source_busy", "status": string(StatusImporting), "message": "所选日志来源正在执行入库任务，请稍后重试", "rebuild": rebuild,
				"accepted_sources": result.Accepted, "busy_sources": result.Busy,
			})
			return
		}

		writeJSONStatus(w, http.StatusAccepted, map[string]any{
			"status": string(StatusImporting), "message": "入库任务已开始", "rebuild": rebuild,
			"accepted_sources": result.Accepted, "busy_sources": result.Busy,
		})
	})
}

type importTargetDateRange struct {
	Start time.Time
	End   time.Time
}

func (r importTargetDateRange) IsZero() bool {
	return r.Start.IsZero() && r.End.IsZero()
}

func (r importTargetDateRange) Dates() []time.Time {
	if r.IsZero() {
		return nil
	}
	dates := make([]time.Time, 0)
	for date := startOfDay(r.Start); !date.After(startOfDay(r.End)); date = date.AddDate(0, 0, 1) {
		dates = append(dates, date)
	}
	return dates
}

func singleImportTargetDate(date time.Time) importTargetDateRange {
	if date.IsZero() {
		return importTargetDateRange{}
	}
	date = startOfDay(date)
	return importTargetDateRange{Start: date, End: date}
}

func parseImportTargetDate(r *http.Request) (importTargetDateRange, error) {
	values := r.URL.Query()
	dateValue := strings.TrimSpace(values.Get("date"))
	fromValue := strings.TrimSpace(values.Get("date_from"))
	toValue := strings.TrimSpace(values.Get("date_to"))

	if dateValue != "" && (fromValue != "" || toValue != "") {
		return importTargetDateRange{}, errors.New("date cannot be combined with date_from/date_to")
	}
	if dateValue != "" {
		date, err := time.ParseInLocation("2006-01-02", dateValue, time.Local)
		if err != nil {
			return importTargetDateRange{}, err
		}
		return singleImportTargetDate(date), nil
	}
	if fromValue == "" && toValue == "" {
		return importTargetDateRange{}, nil
	}
	if fromValue == "" || toValue == "" {
		return importTargetDateRange{}, errors.New("date_from and date_to must be provided together")
	}
	start, err := time.ParseInLocation("2006-01-02", fromValue, time.Local)
	if err != nil {
		return importTargetDateRange{}, err
	}
	end, err := time.ParseInLocation("2006-01-02", toValue, time.Local)
	if err != nil {
		return importTargetDateRange{}, err
	}
	start = startOfDay(start)
	end = startOfDay(end)
	if start.After(end) {
		return importTargetDateRange{}, errors.New("date_from cannot be after date_to")
	}
	return importTargetDateRange{Start: start, End: end}, nil
}

func selectImportSources(sources []LogSource, sourceID string) ([]LogSource, bool) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return sources, true
	}
	for _, source := range sources {
		if source.SourceID == sourceID {
			return []LogSource{source}, true
		}
	}
	return nil, false
}

func (a *App) currentStore() *ClickHouseStore {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store
}

func (a *App) currentLogSources() []LogSource {
	a.mu.RLock()
	settings := make(map[string]string, len(a.settings))
	for key, value := range a.settings {
		settings[key] = value
	}
	a.mu.RUnlock()

	if sources, configured := parseEnabledLogSources(settings["log_sources"]); configured {
		return sources
	}
	return []LogSource{legacyLogSourceFromSettings(settings, a.cfg)}
}

func effectiveStorageSourceID(source LogSource) string {
	if sourceID := strings.TrimSpace(source.StorageSourceID); sourceID != "" {
		return sourceID
	}
	return source.SourceID
}

func (a *App) storageSourceID(displaySourceID string) string {
	displaySourceID = strings.TrimSpace(displaySourceID)
	for _, source := range a.currentLogSources() {
		if source.SourceID == displaySourceID {
			return effectiveStorageSourceID(source)
		}
	}
	return displaySourceID
}

func (a *App) configuredDateStates(states []DateIngestState, displaySourceID string) []DateIngestState {
	sources := a.currentLogSources()
	byStorageID := make(map[string]LogSource, len(sources))
	for _, source := range sources {
		if displaySourceID != "" && source.SourceID != displaySourceID {
			continue
		}
		byStorageID[effectiveStorageSourceID(source)] = source
	}

	configured := make([]DateIngestState, 0, len(states))
	for _, state := range states {
		source, exists := byStorageID[state.SourceID]
		if !exists {
			continue
		}
		state.SourceID = source.SourceID
		state.LogTag = source.LogTag
		configured = append(configured, state)
	}
	return configured
}

func legacyLogSourceFromSettings(settings map[string]string, cfg Config) LogSource {
	return LogSource{
		SourceID:   settingOrFallback(settings, "source_id", "default"),
		LogDir:     settingOrFallback(settings, "log_dir", cfg.LogDir),
		LogTag:     settingOrFallback(settings, "log_tag", cfg.LogTag),
		Enabled:    true,
		SourceType: "file",
		UpdatedAt:  time.Now(),
	}
}

type logSourcePayload struct {
	SourceID             string `json:"source_id"`
	StorageSourceID      string `json:"storage_source_id"`
	SerialNumber         string `json:"serial_number"`
	LogDir               string `json:"log_dir"`
	LogTag               string `json:"log_tag"`
	Enabled              *bool  `json:"enabled"`
	SourceType           string `json:"source_type"`
	ListenProtocol       string `json:"listen_protocol"`
	ListenHost           string `json:"listen_host"`
	ListenPort           int    `json:"listen_port"`
	SpoolDir             string `json:"spool_dir"`
	ClientIP             string `json:"client_ip"`
	ArchiveDir           string `json:"archive_dir"`
	ArchiveRetentionDays int    `json:"archive_retention_days"`
}

func parseEnabledLogSources(raw string) ([]LogSource, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	var payload []logSourcePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}

	return normalizeLogSourcePayloads(payload, true), true
}

func normalizeLogSourcesSetting(value any) (string, bool) {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(strings.TrimSpace(typed))
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", false
		}
		raw = encoded
	}
	if len(raw) == 0 {
		return "[]", true
	}

	var payload []logSourcePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}

	normalized := normalizeLogSourcePayloads(payload, false)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func normalizeLogSourcePayloads(payload []logSourcePayload, enabledOnly bool) []LogSource {
	now := time.Now()
	sources := make([]LogSource, 0, len(payload))
	for _, item := range payload {
		sourceID := strings.TrimSpace(item.SourceID)
		storageSourceID := strings.TrimSpace(item.StorageSourceID)
		serialNumber := strings.TrimSpace(item.SerialNumber)
		logDir := strings.TrimSpace(item.LogDir)
		logTag := strings.TrimSpace(item.LogTag)
		sourceType := normalizeLogSourceType(item.SourceType)
		listenProtocol := normalizeListenProtocol(item.ListenProtocol)
		listenHost := strings.TrimSpace(item.ListenHost)
		listenPort := item.ListenPort
		spoolDir := strings.TrimSpace(item.SpoolDir)
		clientIP := strings.TrimSpace(item.ClientIP)
		archiveDir := strings.TrimSpace(item.ArchiveDir)
		if sourceID == "" && logDir == "" && logTag == "" && spoolDir == "" {
			continue
		}
		if sourceID == "" {
			sourceID = "default"
		}
		if storageSourceID == "" {
			storageSourceID = sourceID
		}
		if sourceType == "rsyslog" {
			if listenHost == "" {
				listenHost = "0.0.0.0"
			}
			if listenPort <= 0 {
				listenPort = 5514
			}
			if spoolDir == "" {
				spoolDir = filepath.ToSlash(filepath.Join("/data/fwlog/received", sourceID))
			}
			logDir = spoolDir
			if archiveDir != "" {
				logDir = archiveDir
			}
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		if enabledOnly && !enabled {
			continue
		}
		sources = append(sources, LogSource{
			SourceID:             sourceID,
			StorageSourceID:      storageSourceID,
			SerialNumber:         serialNumber,
			LogDir:               logDir,
			LogTag:               logTag,
			Enabled:              enabled,
			SourceType:           sourceType,
			ListenProtocol:       listenProtocol,
			ListenHost:           listenHost,
			ListenPort:           listenPort,
			SpoolDir:             spoolDir,
			ClientIP:             clientIP,
			ArchiveDir:           archiveDir,
			ArchiveRetentionDays: item.ArchiveRetentionDays,
			UpdatedAt:            now,
		})
	}
	return sources
}

func normalizeLogSourceType(sourceType string) string {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	if sourceType == "rsyslog" {
		return "rsyslog"
	}
	return "file"
}

func normalizeListenProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "tcp" {
		return "tcp"
	}
	return "udp"
}

const maxImportDuration = 2 * time.Hour

func (a *App) startBackgroundImportSources(rebuild bool, targetDate importTargetDateRange, sources []LogSource, archiveBefore time.Time) ImportStartResult {
	store := a.currentStore()
	if store == nil || len(sources) == 0 {
		return ImportStartResult{}
	}
	return a.imports.Start(context.Background(), sources, func(parent context.Context, source LogSource) error {
		displaySourceID := source.SourceID
		if storageSourceID := strings.TrimSpace(source.StorageSourceID); storageSourceID != "" {
			source.SourceID = storageSourceID
		}
		start := time.Now()
		a.logger.Info("import source started",
			"source_id", displaySourceID,
			"storage_source_id", source.SourceID,
			"log_dir", source.LogDir,
			"log_tag", source.LogTag,
			"rebuild", rebuild,
		)

		ctx, cancel := context.WithTimeout(parent, maxImportDuration)
		defer cancel()

		var err error
		var imported, skipped []string

		if targetDate.IsZero() {
			if a.importRunner != nil {
				imported, skipped, err = a.importRunner(ctx, store, source, rebuild, archiveBefore)
			} else {
				imported, skipped, err = importArchivedDatesBefore(ctx, store, source, rebuild, archiveBefore, a.imports, a.cfg.Workers)
			}
		} else {
			imported, skipped, err = importSourceDates(ctx, store, source, rebuild, targetDate, a.importRunner, a.imports, a.cfg.Workers)
		}

		duration := time.Since(start)

		if err != nil {
			a.logger.Error("import source failed",
				"source_id", displaySourceID,
				"storage_source_id", source.SourceID,
				"error", err,
				"duration_sec", duration.Seconds(),
			)
			return err
		}

		a.logger.Info("import source completed",
			"source_id", displaySourceID,
			"storage_source_id", source.SourceID,
			"imported_dates", len(imported),
			"skipped_dates", len(skipped),
			"duration_sec", duration.Seconds(),
		)

		return nil
	})
}

func importSourceDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, targetDate importTargetDateRange, runner importRunnerFunc, gate batchWriteGate, workers int) ([]string, []string, error) {
	if targetDate.IsZero() {
		if runner == nil {
			return importArchivedDatesBefore(ctx, store, source, rebuild, time.Time{}, gate, workers)
		}
		return runner(ctx, store, source, rebuild, time.Time{})
	}

	return runImportDates(targetDate.Dates(), workers, func(date time.Time) ([]string, []string, error) {
		return importSourceDate(ctx, store, source, rebuild, date, gate)
	})
}

func importSourceDate(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, targetDate time.Time, gate batchWriteGate) ([]string, []string, error) {
	date := startOfDay(targetDate)
	if !rebuild {
		state, found, err := store.LatestDateState(ctx, source.SourceID, date)
		if err != nil {
			return nil, nil, err
		}
		if found && isCompletedImportStatus(state.Status) {
			return nil, []string{formatDate(date)}, nil
		}
	}

	imp := importerpkg.NewImporter(store, gate)
	if err := imp.ImportDate(ctx, source, date); err != nil {
		return nil, nil, err
	}
	return []string{formatDate(date)}, nil, nil
}

func formatOptionalDate(date time.Time) string {
	if date.IsZero() {
		return ""
	}
	return date.Format("2006-01-02")
}

func importArchivedDatesBefore(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, beforeDate time.Time, gate batchWriteGate, workers int) ([]string, []string, error) {
	now := time.Now()
	files, err := importerpkg.ScanArchivedLogFilesBefore(source.LogDir, now, beforeDate)
	if err != nil {
		return nil, nil, err
	}

	dateSet := make(map[string]time.Time)
	for _, file := range files {
		dateSet[dateKey(file.LogDate)] = startOfDay(file.LogDate)
	}

	dates := make([]time.Time, 0, len(dateSet))
	for _, date := range dateSet {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})

	imp := importerpkg.NewImporter(store, gate)

	return runImportDates(dates, workers, func(date time.Time) ([]string, []string, error) {
		if !rebuild {
			state, found, err := store.LatestDateState(ctx, source.SourceID, date)
			if err != nil {
				return nil, nil, err
			}
			if found && isCompletedImportStatus(state.Status) {
				return nil, []string{formatDate(date)}, nil
			}
		}

		if err := imp.ImportDate(ctx, source, date); err != nil {
			return nil, nil, err
		}
		return []string{formatDate(date)}, nil, nil
	})
}

type importDateFunc func(time.Time) ([]string, []string, error)

// runImportDates imports up to workers dates at once. Each date owns its own
// (source_id, log_date) partitions and state rows, so dates never share writes.
// Results and errors keep the input date order regardless of completion order.
func runImportDates(dates []time.Time, workers int, importDate importDateFunc) ([]string, []string, error) {
	if workers > len(dates) {
		workers = len(dates)
	}
	if workers < 1 {
		workers = 1
	}
	type dateResult struct {
		imported []string
		skipped  []string
		err      error
	}
	results := make([]dateResult, len(dates))
	next := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range next {
				imported, skipped, err := importDate(dates[index])
				results[index] = dateResult{imported: imported, skipped: skipped, err: err}
			}
		}()
	}
	for index := range dates {
		next <- index
	}
	close(next)
	wg.Wait()

	imported := make([]string, 0, len(dates))
	skipped := make([]string, 0)
	dateErrors := make([]error, 0)
	for index, result := range results {
		imported = append(imported, result.imported...)
		skipped = append(skipped, result.skipped...)
		if result.err != nil {
			dateErrors = append(dateErrors, fmt.Errorf("%s: %w", formatDate(dates[index]), result.err))
		}
	}
	return imported, skipped, errors.Join(dateErrors...)
}

func isCompletedImportStatus(status IngestStatus) bool {
	return status == StatusReady || status == StatusNoData
}
