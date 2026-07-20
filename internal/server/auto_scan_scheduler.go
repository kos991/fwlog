package server

import (
	"context"
	"time"

	"fwlog/internal/importer"
)

const (
	autoScanCheckInterval = 30 * time.Second
	autoScanDueWindow     = 90 * time.Second
)

func (a *App) startAutoScanScheduler(ctx context.Context) {
	go a.autoScanScheduler(ctx)
}

func (a *App) autoScanScheduler(ctx context.Context) {
	a.logger.Info("auto scan scheduler started", "check_interval", autoScanCheckInterval)

	ticker := time.NewTicker(autoScanCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("auto scan scheduler stopped")
			return
		case now := <-ticker.C:
			a.runDueAutoScan(ctx, now)
		}
	}
}

func (a *App) runDueAutoScan(ctx context.Context, now time.Time) bool {
	settings := a.settingsSnapshot()
	scheduledAt, ok := dueAutoScanTime(settings, now)
	if !ok {
		return false
	}

	a.logger.Info("auto scan triggered", "scheduled_at", scheduledAt.Format("2006-01-02 15:04:05"))

	archiveBefore := autoScanArchiveBefore(settings, now)
	sources := a.currentLogSources()
	result := a.startBackgroundImportSources(false, importTargetDateRange{}, sources, archiveBefore)
	if len(result.Accepted) == 0 {
		a.logger.Warn("auto scan skipped: import already running")
		return false
	}

	go a.recordCompletedAutoScan(ctx, scheduledAt, len(result.Accepted) == len(sources), result.Done)
	return true
}

func (a *App) recordCompletedAutoScan(ctx context.Context, scheduledAt time.Time, allSourcesAccepted bool, done <-chan struct{}) {
	if done != nil {
		select {
		case <-ctx.Done():
			return
		case <-done:
		}
	}
	if !allSourcesAccepted {
		a.logger.Warn("auto scan not recorded: some sources were already running")
		return
	}

	value := formatDateTime(scheduledAt)
	payload := map[string]any{"last_auto_scan_at": value}
	a.updateSettings(payload)
	if err := a.saveSettings(ctx, payload); err != nil {
		a.logger.Error("save completed auto scan time failed", "scheduled_at", value, "error", err)
	}
}

func (a *App) settingsSnapshot() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	settings := make(map[string]string, len(a.settings))
	for key, value := range a.settings {
		settings[key] = value
	}
	return settings
}

func dueAutoScanTime(settings map[string]string, now time.Time) (time.Time, bool) {
	return importer.DueAutoScanTime(settings, now, autoScanDueWindow)
}

func autoScanArchiveBefore(settings map[string]string, now time.Time) time.Time {
	localNow := now.In(importer.AutoScanLocation(settings))
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location())
}
