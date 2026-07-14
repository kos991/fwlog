package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"fwlog/internal/receiver"
)

const (
	rsyslogArchiveInterval = time.Minute
	rsyslogArchiveTimeout  = 30 * time.Second
)

func (a *App) startRSyslogArchiveScheduler(ctx context.Context) {
	go a.rsyslogArchiveScheduler(ctx)
}

func (a *App) rsyslogArchiveScheduler(ctx context.Context) {
	a.logger.Info("RSyslog 归档调度器已启动", "interval", rsyslogArchiveInterval)
	a.runRSyslogArchive(ctx, a.currentRSyslogArchiveTime())

	ticker := time.NewTicker(rsyslogArchiveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("RSyslog 归档调度器已停止")
			return
		case now := <-ticker.C:
			a.runRSyslogArchive(ctx, now)
		}
	}
}

func (a *App) runRSyslogArchive(ctx context.Context, now time.Time) {
	a.rsyslogArchiveMu.Lock()
	defer a.rsyslogArchiveMu.Unlock()

	sources := a.configuredRSyslogSources()
	if len(sources) == 0 {
		return
	}

	maxRetention := 0
	for _, source := range sources {
		if source.ArchiveRetentionDays > maxRetention {
			maxRetention = source.ArchiveRetentionDays
		}
	}

	var ready map[receiver.ArchiveReadyKey]bool
	if maxRetention > 0 {
		since := startOfDay(now).AddDate(0, 0, -maxRetention-1)
		states, err := a.loadArchiveDateStates(ctx, since)
		if err != nil {
			a.logger.Error("加载 RSyslog 归档入库状态失败", "since", formatDate(since), "error", err)
		} else {
			ready = archiveReadyMap(states)
		}
	}

	archiver := a.archiver
	if archiver == nil {
		archiver = receiver.NewArchiver()
	}
	results := archiver.Run(sources, ready, now)
	if a.receiver != nil {
		a.receiver.UpdateArchiveResults(archiveStatusResults(sources, results, now))
	}
	for _, result := range results {
		if result.Error == "" {
			continue
		}
		a.logger.Error("RSyslog 归档失败",
			"source_id", result.SourceID,
			"date", formatDate(result.Date),
			"path", result.Path,
			"error", result.Error,
		)
	}
}

func archiveStatusResults(sources []LogSource, results []receiver.ArchiveResult, completedAt time.Time) []receiver.ArchiveResult {
	statusResults := append([]receiver.ArchiveResult(nil), results...)
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		seen[result.SourceID] = struct{}{}
	}
	for _, source := range sources {
		if _, exists := seen[source.SourceID]; exists {
			continue
		}
		statusResults = append(statusResults, receiver.ArchiveResult{
			SourceID:    source.SourceID,
			CompletedAt: completedAt,
		})
	}
	return statusResults
}

func (a *App) triggerRSyslogArchive() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), rsyslogArchiveTimeout)
		defer cancel()
		a.runRSyslogArchive(ctx, a.currentRSyslogArchiveTime())
	}()
}

func (a *App) currentRSyslogArchiveTime() time.Time {
	if a.rsyslogArchiveNow != nil {
		return a.rsyslogArchiveNow()
	}
	return time.Now()
}

func (a *App) loadArchiveDateStates(ctx context.Context, since time.Time) ([]DateIngestState, error) {
	if a.dateStatesLoader != nil {
		return a.dateStatesLoader(ctx, since)
	}
	store := a.currentStore()
	if store == nil || !store.Ready() {
		return nil, errors.New("ClickHouse 尚未连接")
	}
	return store.ListDateStates(ctx, since)
}

func (a *App) configuredRSyslogSources() []LogSource {
	a.mu.RLock()
	raw := strings.TrimSpace(a.settings["log_sources"])
	a.mu.RUnlock()
	if raw == "" {
		return nil
	}

	var payload []logSourcePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	configured := normalizeLogSourcePayloads(payload, false)
	sources := make([]LogSource, 0, len(configured))
	for _, source := range configured {
		if strings.EqualFold(source.SourceType, "rsyslog") {
			sources = append(sources, source)
		}
	}
	return sources
}

func archiveReadyMap(states []DateIngestState) map[receiver.ArchiveReadyKey]bool {
	ready := make(map[receiver.ArchiveReadyKey]bool)
	for _, state := range states {
		if state.Status != StatusReady {
			continue
		}
		ready[receiver.ArchiveReadyKey{
			SourceID: state.SourceID,
			Date:     formatDate(state.LogDate),
		}] = true
	}
	return ready
}
