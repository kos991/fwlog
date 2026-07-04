package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AutoScanPlan struct {
	Enabled bool
	Mode    string
	Policy  string
	NextAt  time.Time
}

func BuildAutoScanPlan(settings map[string]string, now time.Time) AutoScanPlan {
	enabled := settingBoolOrFallback(settings, "auto_scan_enabled", false)
	if !enabled {
		return AutoScanPlan{Enabled: false, Policy: "未启用"}
	}

	loc := time.Local
	if name := strings.TrimSpace(settings["auto_scan_timezone"]); name != "" {
		if loaded, err := time.LoadLocation(name); err == nil {
			loc = loaded
		}
	}
	localNow := now.In(loc)
	return buildDailyAutoScanPlan(settings, localNow, loc)
}

func buildDailyAutoScanPlan(settings map[string]string, now time.Time, loc *time.Location) AutoScanPlan {
	times := parseAutoScanTimes(settings["auto_scan_times"])
	if len(times) == 0 {
		return AutoScanPlan{Enabled: true, Mode: "daily", Policy: "配置待完善"}
	}

	sort.Strings(times)
	for _, item := range times {
		next, ok := dailyScanTime(now, item, loc)
		if ok && next.After(now) {
			return AutoScanPlan{
				Enabled: true,
				Mode:    "daily",
				Policy:  strings.Join(times, "、") + " 自动扫描",
				NextAt:  next,
			}
		}
	}
	next, _ := dailyScanTime(now.AddDate(0, 0, 1), times[0], loc)
	return AutoScanPlan{
		Enabled: true,
		Mode:    "daily",
		Policy:  strings.Join(times, "、") + " 自动扫描",
		NextAt:  next,
	}
}

func buildIntervalAutoScanPlan(settings map[string]string, now time.Time, loc *time.Location) AutoScanPlan {
	seconds, err := strconv.Atoi(strings.TrimSpace(settings["auto_scan_interval_sec"]))
	if err != nil || seconds <= 0 {
		seconds = defaultAutoScanSec
	}
	interval := time.Duration(seconds) * time.Second
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if now.Before(anchor) {
		anchor = anchor.AddDate(0, 0, -1)
	}
	elapsed := now.Sub(anchor)
	steps := int64(elapsed / interval)
	next := anchor.Add(time.Duration(steps+1) * interval)
	return AutoScanPlan{
		Enabled: true,
		Mode:    "interval",
		Policy:  "每 " + formatIntervalPolicy(interval) + "自动扫描",
		NextAt:  next,
	}
}

func parseAutoScanTimes(raw string) []string {
	items := strings.Split(raw, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if _, err := time.Parse("15:04", item); err == nil {
			result = append(result, item)
		}
	}
	return result
}

func dailyScanTime(now time.Time, clock string, loc *time.Location) (time.Time, bool) {
	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), true
}

func formatIntervalPolicy(interval time.Duration) string {
	if interval%time.Hour == 0 {
		return fmt.Sprintf("%d 小时", int(interval/time.Hour))
	}
	if interval%time.Minute == 0 {
		return fmt.Sprintf("%d 分钟", int(interval/time.Minute))
	}
	return fmt.Sprintf("%d 秒", int(interval/time.Second))
}
