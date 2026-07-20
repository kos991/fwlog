package importer

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func DueAutoScanTime(settings map[string]string, now time.Time, dueWindow time.Duration) (time.Time, bool) {
	if !settingBoolOrFallback(settings, "auto_scan_enabled", false) {
		return time.Time{}, false
	}

	loc := AutoScanLocation(settings)
	localNow := now.In(loc)
	mode := strings.TrimSpace(settings["auto_scan_mode"])
	if mode == "interval" {
		return dueIntervalAutoScanTime(settings, localNow, loc, dueWindow)
	}
	return dueDailyAutoScanTime(settings, localNow, loc, dueWindow)
}

func dueDailyAutoScanTime(settings map[string]string, localNow time.Time, loc *time.Location, _ time.Duration) (time.Time, bool) {
	times := parseAutoScanTimes(settings["auto_scan_times"])
	if len(times) == 0 {
		return time.Time{}, false
	}
	sort.Strings(times)

	last := ParseAutoScanDateTime(settings["last_auto_scan_at"], loc)
	for index := len(times) - 1; index >= 0; index-- {
		item := times[index]
		scheduledAt, ok := dailyScanTime(localNow, item, loc)
		if !ok || scheduledAt.After(localNow) {
			continue
		}
		if !last.IsZero() && !last.Before(scheduledAt) {
			continue
		}
		return scheduledAt, true
	}
	return time.Time{}, false
}

func dueIntervalAutoScanTime(settings map[string]string, localNow time.Time, loc *time.Location, _ time.Duration) (time.Time, bool) {
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
	last := ParseAutoScanDateTime(settings["last_auto_scan_at"], loc)
	if !last.IsZero() && !last.Before(scheduledAt) {
		return time.Time{}, false
	}
	return scheduledAt, true
}

func AutoScanLocation(settings map[string]string) *time.Location {
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

func ParseAutoScanDateTime(value string, loc *time.Location) time.Time {
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
