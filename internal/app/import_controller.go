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
		store := a.currentStore()
		if store == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "clickhouse_not_connected",
				"message": "ClickHouse 尚未连接，无法执行入库",
			})
			return
		}
		if !a.startBackgroundImport(rebuild) {
			writeJSONStatus(w, http.StatusAccepted, map[string]any{
				"status":  string(StatusImporting),
				"message": "已有入库任务正在执行",
				"rebuild": rebuild,
			})
			return
		}
		writeJSONStatus(w, http.StatusAccepted, map[string]any{
			"status":  string(StatusImporting),
			"message": "入库任务已开始",
			"rebuild": rebuild,
		})
	})
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

func (a *App) startBackgroundImport(rebuild bool) bool {
	store := a.currentStore()
	if store == nil {
		return false
	}
	if !a.tryBeginImport() {
		return false
	}
	go func() {
		defer a.endImport()
		ctx := context.Background()
		_, _, _, _, _ = a.importConfiguredSources(ctx, store, rebuild)
	}()
	return true
}

func (a *App) tryBeginImport() bool {
	a.importMu.Lock()
	defer a.importMu.Unlock()
	if a.importing {
		return false
	}
	a.importing = true
	return true
}

func (a *App) endImport() {
	a.importMu.Lock()
	defer a.importMu.Unlock()
	a.importing = false
}

func (a *App) importConfiguredSources(ctx context.Context, store *ClickHouseStore, rebuild bool) ([]string, []string, map[string][]string, map[string][]string, error) {
	sources := a.currentLogSources()
	importedAll := make([]string, 0)
	skippedAll := make([]string, 0)
	importedBySource := make(map[string][]string, len(sources))
	skippedBySource := make(map[string][]string, len(sources))

	runner := a.importRunner
	if runner == nil {
		runner = importArchivedDates
	}

	for _, source := range sources {
		imported, skipped, err := runner(ctx, store, source, rebuild)
		if len(imported) > 0 {
			importedBySource[source.SourceID] = imported
			importedAll = append(importedAll, imported...)
		}
		if len(skipped) > 0 {
			skippedBySource[source.SourceID] = skipped
			skippedAll = append(skippedAll, skipped...)
		}
		if err != nil {
			return importedAll, skippedAll, importedBySource, skippedBySource, err
		}
	}
	return importedAll, skippedAll, importedBySource, skippedBySource, nil
}

func importArchivedDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool) ([]string, []string, error) {
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
		store:  store,
		writer: store.conn,
		states: store,
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
