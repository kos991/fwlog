package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	importerpkg "fwlog/internal/importer"
)

func (a *App) importHandler(rebuild bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.currentStore() == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "clickhouse_not_connected", "message": "ClickHouse is not connected"})
			return
		}
		targetDate, err := parseImportTargetDate(r)
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_date", "message": "date/date_from/date_to must use YYYY-MM-DD format"})
			return
		}
		sources, found := selectImportSources(a.currentLogSources(), r.URL.Query().Get("source_id"))
		if !found {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "unknown_source", "message": "source_id is not enabled"})
			return
		}

		a.logger.Info("import request received",
			"rebuild", rebuild,
			"target_date_from", formatOptionalDate(targetDate.Start),
			"target_date_to", formatOptionalDate(targetDate.End),
			"sources_count", len(sources),
		)

		result := a.startBackgroundImportSources(rebuild, targetDate, sources)

		a.logger.Info("import request processed",
			"accepted", result.Accepted,
			"busy", result.Busy,
		)

		writeJSONStatus(w, http.StatusAccepted, map[string]any{
			"status": string(StatusImporting), "message": "import request accepted", "rebuild": rebuild,
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

func (a *App) currentLogSource() LogSource {
	sources := a.currentLogSources()
	if len(sources) > 0 {
		return sources[0]
	}
	return a.legacyLogSource()
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

func (a *App) legacyLogSource() LogSource {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return legacyLogSourceFromSettings(a.settings, a.cfg)
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
	SourceID       string `json:"source_id"`
	LogDir         string `json:"log_dir"`
	LogTag         string `json:"log_tag"`
	Enabled        *bool  `json:"enabled"`
	SourceType     string `json:"source_type"`
	ListenProtocol string `json:"listen_protocol"`
	ListenHost     string `json:"listen_host"`
	ListenPort     int    `json:"listen_port"`
	SpoolDir       string `json:"spool_dir"`
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
		logDir := strings.TrimSpace(item.LogDir)
		logTag := strings.TrimSpace(item.LogTag)
		sourceType := normalizeLogSourceType(item.SourceType)
		listenProtocol := normalizeListenProtocol(item.ListenProtocol)
		listenHost := strings.TrimSpace(item.ListenHost)
		listenPort := item.ListenPort
		spoolDir := strings.TrimSpace(item.SpoolDir)
		if sourceID == "" && logDir == "" && logTag == "" && spoolDir == "" {
			continue
		}
		if sourceID == "" {
			sourceID = "default"
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
			if logDir == "" {
				logDir = spoolDir
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
			SourceID:       sourceID,
			LogDir:         logDir,
			LogTag:         logTag,
			Enabled:        enabled,
			SourceType:     sourceType,
			ListenProtocol: listenProtocol,
			ListenHost:     listenHost,
			ListenPort:     listenPort,
			SpoolDir:       spoolDir,
			UpdatedAt:      now,
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
	if protocol == "udp" {
		return "udp"
	}
	return "udp"
}

const maxImportDuration = 2 * time.Hour

func (a *App) startBackgroundImport(rebuild bool, targetDate time.Time) bool {
	return len(a.startBackgroundImportSources(rebuild, singleImportTargetDate(targetDate), a.currentLogSources()).Accepted) > 0
}

func (a *App) startBackgroundImportSources(rebuild bool, targetDate importTargetDateRange, sources []LogSource) ImportStartResult {
	store := a.currentStore()
	if store == nil || len(sources) == 0 {
		return ImportStartResult{}
	}
	return a.imports.Start(context.Background(), sources, func(parent context.Context, source LogSource) error {
		start := time.Now()
		a.logger.Info("import source started",
			"source_id", source.SourceID,
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
				imported, skipped, err = a.importRunner(ctx, store, source, rebuild)
			} else {
				imported, skipped, err = importArchivedDatesWithGate(ctx, store, source, rebuild, a.imports)
			}
		} else {
			imported, skipped, err = importSourceDates(ctx, store, source, rebuild, targetDate, a.importRunner, a.imports)
		}

		duration := time.Since(start)

		if err != nil {
			a.logger.Error("import source failed",
				"source_id", source.SourceID,
				"error", err,
				"duration_sec", duration.Seconds(),
			)
			return err
		}

		a.logger.Info("import source completed",
			"source_id", source.SourceID,
			"imported_dates", len(imported),
			"skipped_dates", len(skipped),
			"duration_sec", duration.Seconds(),
		)

		return nil
	})
}

func importSourceDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, targetDate importTargetDateRange, runner importRunnerFunc, gate batchWriteGate) ([]string, []string, error) {
	if targetDate.IsZero() {
		if runner == nil {
			return importArchivedDatesWithGate(ctx, store, source, rebuild, gate)
		}
		return runner(ctx, store, source, rebuild)
	}

	imported := make([]string, 0)
	skipped := make([]string, 0)
	for _, date := range targetDate.Dates() {
		dateImported, dateSkipped, err := importSourceDate(ctx, store, source, rebuild, date, gate)
		if err != nil {
			return imported, skipped, err
		}
		imported = append(imported, dateImported...)
		skipped = append(skipped, dateSkipped...)
	}
	return imported, skipped, nil
}

func importSourceDate(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, targetDate time.Time, gate batchWriteGate) ([]string, []string, error) {
	date := startOfDay(targetDate)
	if !rebuild {
		state, found, err := store.LatestDateState(ctx, source.SourceID, date)
		if err != nil {
			return nil, nil, err
		}
		if found && state.Status == StatusReady {
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

func importArchivedDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool) ([]string, []string, error) {
	return importArchivedDatesWithGate(ctx, store, source, rebuild, nil)
}

func importArchivedDatesWithGate(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, gate batchWriteGate) ([]string, []string, error) {
	files, err := importerpkg.ScanArchivedLogFiles(source.LogDir, time.Now())
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

	imported := make([]string, 0, len(dates))
	skipped := make([]string, 0)
	for _, date := range dates {
		if !rebuild {
			state, found, err := store.LatestDateState(ctx, source.SourceID, date)
			if err != nil {
				return imported, skipped, err
			}
			if found && state.Status == StatusReady {
				skipped = append(skipped, formatDate(date))
				continue
			}
		}

		if err := imp.ImportDate(ctx, source, date); err != nil {
			return imported, skipped, err
		}
		imported = append(imported, formatDate(date))
	}

	return imported, skipped, nil
}
