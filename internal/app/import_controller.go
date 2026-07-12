package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (a *App) importHandler(rebuild bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.currentStore() == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "clickhouse_not_connected", "message": "ClickHouse is not connected"})
			return
		}
		targetDate, err := parseImportTargetDate(r)
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_date", "message": "date must use YYYY-MM-DD format"})
			return
		}
		sources, found := selectImportSources(a.currentLogSources(), r.URL.Query().Get("source_id"))
		if !found {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "unknown_source", "message": "source_id is not enabled"})
			return
		}
		result := a.startBackgroundImportSources(rebuild, targetDate, sources)
		writeJSONStatus(w, http.StatusAccepted, map[string]any{
			"status": string(StatusImporting), "message": "import request accepted", "rebuild": rebuild,
			"accepted_sources": result.Accepted, "busy_sources": result.Busy,
		})
	})
}

func parseImportTargetDate(r *http.Request) (time.Time, error) {
	value := strings.TrimSpace(r.URL.Query().Get("date"))
	if value == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", value, time.Local)
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
		SourceID:  settingOrFallback(settings, "source_id", "default"),
		LogDir:    settingOrFallback(settings, "log_dir", cfg.LogDir),
		LogTag:    settingOrFallback(settings, "log_tag", cfg.LogTag),
		Enabled:   true,
		UpdatedAt: time.Now(),
	}
}

func parseEnabledLogSources(raw string) ([]LogSource, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	type logSourcePayload struct {
		SourceID string `json:"source_id"`
		LogDir   string `json:"log_dir"`
		LogTag   string `json:"log_tag"`
		Enabled  *bool  `json:"enabled"`
	}

	var payload []logSourcePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}

	now := time.Now()
	sources := make([]LogSource, 0, len(payload))
	for _, item := range payload {
		sourceID := strings.TrimSpace(item.SourceID)
		logDir := strings.TrimSpace(item.LogDir)
		logTag := strings.TrimSpace(item.LogTag)
		if sourceID == "" && logDir == "" && logTag == "" {
			continue
		}
		if sourceID == "" {
			sourceID = "default"
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		if !enabled {
			continue
		}
		sources = append(sources, LogSource{
			SourceID:  sourceID,
			LogDir:    logDir,
			LogTag:    logTag,
			Enabled:   true,
			UpdatedAt: now,
		})
	}
	return sources, true
}

const maxImportDuration = 2 * time.Hour

func (a *App) startBackgroundImport(rebuild bool, targetDate time.Time) bool {
	return len(a.startBackgroundImportSources(rebuild, targetDate, a.currentLogSources()).Accepted) > 0
}

func (a *App) startBackgroundImportSources(rebuild bool, targetDate time.Time, sources []LogSource) ImportStartResult {
	store := a.currentStore()
	if store == nil || len(sources) == 0 {
		return ImportStartResult{}
	}
	return a.imports.Start(context.Background(), sources, func(parent context.Context, source LogSource) error {
		ctx, cancel := context.WithTimeout(parent, maxImportDuration)
		defer cancel()
		if targetDate.IsZero() {
			if a.importRunner != nil {
				_, _, err := a.importRunner(ctx, store, source, rebuild)
				return err
			}
			_, _, err := importArchivedDatesWithGate(ctx, store, source, rebuild, a.imports)
			return err
		}
		_, _, err := importSourceDates(ctx, store, source, rebuild, targetDate, a.importRunner, a.imports)
		return err
	})
}

func importSourceDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, targetDate time.Time, runner importRunnerFunc, gate batchWriteGate) ([]string, []string, error) {
	if targetDate.IsZero() {
		if runner == nil {
			return importArchivedDatesWithGate(ctx, store, source, rebuild, gate)
		}
		return runner(ctx, store, source, rebuild)
	}

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

	importer := &Importer{
		store:     store,
		writer:    store.conn,
		states:    store,
		writeGate: gate,
	}
	if err := importer.ImportDate(ctx, source, date); err != nil {
		return nil, nil, err
	}
	return []string{formatDate(date)}, nil, nil
}

func importArchivedDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool) ([]string, []string, error) {
	return importArchivedDatesWithGate(ctx, store, source, rebuild, nil)
}

func importArchivedDatesWithGate(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool, gate batchWriteGate) ([]string, []string, error) {
	files, err := ScanArchivedLogFiles(source.LogDir, time.Now())
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

	importer := &Importer{
		store:     store,
		writer:    store.conn,
		states:    store,
		writeGate: gate,
	}

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

		if err := importer.ImportDate(ctx, source, date); err != nil {
			return imported, skipped, err
		}
		imported = append(imported, formatDate(date))
	}

	return imported, skipped, nil
}
