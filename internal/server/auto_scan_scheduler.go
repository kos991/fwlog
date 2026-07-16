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
	result := a.startBackgroundImportSources(false, importTargetDateRange{}, a.currentLogSources(), archiveBefore)
	if len(result.Accepted) == 0 {
		a.logger.Warn("auto scan skipped: import already running")
		return false
	}

	value := formatDateTime(scheduledAt)
	payload := map[string]any{"last_auto_scan_at": value}
	a.updateSettings(payload)
	_ = a.saveSettings(ctx, payload)
	return true
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
