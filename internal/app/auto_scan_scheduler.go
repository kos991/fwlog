package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	autoScanCheckInterval = 30 * time.Second
	autoScanDueWindow     = 90 * time.Second
)

func (a *App) startAutoScanScheduler(ctx context.Context) {
	go a.autoScanScheduler(ctx)
}

func (a *App) autoScanScheduler(ctx context.Context) {
	ticker := time.NewTicker(autoScanCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
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
	if !a.startBackgroundImport(false, time.Time{}) {
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
	if !settingBoolOrFallback(settings, "auto_scan_enabled", false) {
		return time.Time{}, false
	}

	loc := autoScanLocation(settings)
	localNow := now.In(loc)
	mode := strings.TrimSpace(settings["auto_scan_mode"])
	if mode == "interval" {
		return dueIntervalAutoScanTime(settings, localNow, loc)
	}
	return dueDailyAutoScanTime(settings, localNow, loc)
}

func dueDailyAutoScanTime(settings map[string]string, localNow time.Time, loc *time.Location) (time.Time, bool) {
	times := parseAutoScanTimes(settings["auto_scan_times"])
	if len(times) == 0 {
		return time.Time{}, false
	}
	sort.Strings(times)

	last := parseAutoScanDateTime(settings["last_auto_scan_at"], loc)
	for _, item := range times {
		scheduledAt, ok := dailyScanTime(localNow, item, loc)
		if !ok || scheduledAt.After(localNow) {
			continue
		}
		if localNow.Sub(scheduledAt) > autoScanDueWindow {
			continue
		}
		if !last.IsZero() && last.Equal(scheduledAt) {
			continue
		}
		return scheduledAt, true
	}
	return time.Time{}, false
}

func dueIntervalAutoScanTime(settings map[string]string, localNow time.Time, loc *time.Location) (time.Time, bool) {
	seconds, err := parseAutoScanIntervalSeconds(settings["auto_scan_interval_sec"])
	if err != nil || seconds <= 0 {
		return time.Time{}, false
	}
	interval := time.Duration(seconds) * time.Second
	if interval < time.Minute {
		interval = time.Minute
	}
	anchor := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	if localNow.Before(anchor) {
		anchor = anchor.AddDate(0, 0, -1)
	}
	elapsed := localNow.Sub(anchor)
	steps := int64(elapsed / interval)
	scheduledAt := anchor.Add(time.Duration(steps) * interval)
	if localNow.Sub(scheduledAt) > autoScanDueWindow {
		return time.Time{}, false
	}
	last := parseAutoScanDateTime(settings["last_auto_scan_at"], loc)
	if !last.IsZero() && !last.Before(scheduledAt) {
		return time.Time{}, false
	}
	return scheduledAt, true
}

func autoScanLocation(settings map[string]string) *time.Location {
	name := strings.TrimSpace(settings["auto_scan_timezone"])
	if name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}

func parseAutoScanDateTime(value string, loc *time.Location) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, loc)
	if err == nil {
		return parsed
	}
	return time.Time{}
}

func parseAutoScanIntervalSeconds(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAutoScanSec, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return seconds, nil
}
