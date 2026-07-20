package server

import (
	"context"
	"testing"
	"time"
)

func TestRunDueAutoScanStartsImportAtConfiguredTime(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{
		"auto_scan_enabled":  "true",
		"auto_scan_mode":     "daily",
		"auto_scan_times":    "01:00",
		"auto_scan_timezone": "Asia/Shanghai",
		"log_sources": []any{
			map[string]any{"source_id": "fw-a", "log_tag": "edge-a", "log_dir": "/data/fw-a", "enabled": true},
		},
	})

	started := make(chan struct{}, 1)
	var archiveBefore time.Time
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool, before time.Time) ([]string, []string, error) {
		archiveBefore = before
		started <- struct{}{}
		return nil, nil, nil
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	if !app.runDueAutoScan(context.Background(), time.Date(2026, 7, 7, 1, 0, 30, 0, loc)) {
		t.Fatal("auto scan should start at configured scan time")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for auto scan import")
	}
	deadline := time.Now().Add(time.Second)
	for app.getSettings()["last_auto_scan_at"] != "2026-07-07 01:00:00" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := app.getSettings()["last_auto_scan_at"]; got != "2026-07-07 01:00:00" {
		t.Fatalf("last_auto_scan_at = %q, want scheduled time after completion", got)
	}
	if archiveBefore.Format("2006-01-02 15:04:05") != "2026-07-07 00:00:00" {
		t.Fatalf("archive cutoff = %v, want current day start", archiveBefore)
	}
}

func TestRunDueAutoScanDoesNotRecordBeforeAllSourcesComplete(t *testing.T) {
	cfg := LoadConfig()
	cfg.Workers = 2
	app := NewApp(cfg)
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{
		"auto_scan_enabled":  "true",
		"auto_scan_mode":     "daily",
		"auto_scan_times":    "01:00",
		"auto_scan_timezone": "Asia/Shanghai",
		"log_sources": []any{
			map[string]any{"source_id": "fw-a", "log_dir": "/data/fw-a", "enabled": true},
			map[string]any{"source_id": "fw-b", "log_dir": "/data/fw-b", "enabled": true},
		},
	})

	started := make(chan string, 2)
	release := make(chan struct{})
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, source LogSource, _ bool, _ time.Time) ([]string, []string, error) {
		started <- source.SourceID
		<-release
		return nil, nil, nil
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	if !app.runDueAutoScan(context.Background(), time.Date(2026, 7, 7, 1, 0, 30, 0, loc)) {
		t.Fatal("auto scan should start")
	}
	<-started
	<-started
	if got := app.getSettings()["last_auto_scan_at"]; got != "" {
		t.Fatalf("last_auto_scan_at = %q before workers complete", got)
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for app.getSettings()["last_auto_scan_at"] != "2026-07-07 01:00:00" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := app.getSettings()["last_auto_scan_at"]; got != "2026-07-07 01:00:00" {
		t.Fatalf("last_auto_scan_at = %q after workers complete", got)
	}
}

func TestRunDueAutoScanDoesNotStartTwiceForSameScheduledTime(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{
		"auto_scan_enabled":  "true",
		"auto_scan_mode":     "daily",
		"auto_scan_times":    "01:00",
		"auto_scan_timezone": "Asia/Shanghai",
		"last_auto_scan_at":  "2026-07-07 01:00:00",
	})

	called := false
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool, _ time.Time) ([]string, []string, error) {
		called = true
		return nil, nil, nil
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	if app.runDueAutoScan(context.Background(), time.Date(2026, 7, 7, 1, 0, 45, 0, loc)) {
		t.Fatal("auto scan should not start twice for the same scheduled time")
	}
	if called {
		t.Fatal("import runner should not be called for duplicate auto scan")
	}
}

func TestDueAutoScanTimeCatchesUpAfterDueWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	settings := map[string]string{
		"auto_scan_enabled":  "true",
		"auto_scan_mode":     "daily",
		"auto_scan_times":    "01:00",
		"auto_scan_timezone": "Asia/Shanghai",
	}

	got, ok := dueAutoScanTime(settings, time.Date(2026, 7, 7, 1, 5, 0, 0, loc))
	if !ok {
		t.Fatal("missed daily scan should be caught up after the old 90-second window")
	}
	if want := time.Date(2026, 7, 7, 1, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("scheduled time = %v, want %v", got, want)
	}
}

func TestDueIntervalAutoScanCatchesUpAfterDueWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	settings := map[string]string{
		"auto_scan_enabled":      "true",
		"auto_scan_mode":         "interval",
		"auto_scan_interval_sec": "3600",
		"auto_scan_timezone":     "Asia/Shanghai",
		"last_auto_scan_at":      "2026-07-07 00:00:00",
	}

	got, ok := dueAutoScanTime(settings, time.Date(2026, 7, 7, 2, 5, 0, 0, loc))
	if !ok {
		t.Fatal("missed interval scan should be caught up after the old 90-second window")
	}
	if want := time.Date(2026, 7, 7, 2, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("scheduled time = %v, want %v", got, want)
	}
}

func TestDueAutoScanTimeDoesNotGoBackToEarlierDailySlot(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	settings := map[string]string{
		"auto_scan_enabled":  "true",
		"auto_scan_mode":     "daily",
		"auto_scan_times":    "01:00,09:00",
		"auto_scan_timezone": "Asia/Shanghai",
		"last_auto_scan_at":  "2026-07-07 09:00:00",
	}

	if got, ok := dueAutoScanTime(settings, time.Date(2026, 7, 7, 9, 0, 30, 0, loc)); ok {
		t.Fatalf("completed 09:00 slot must not fall back and rerun 01:00, got %v", got)
	}
}

func TestRunDueAutoScanIgnoresDisabledPlan(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{
		"auto_scan_enabled":  "false",
		"auto_scan_times":    "01:00",
		"auto_scan_timezone": "Asia/Shanghai",
	})

	called := false
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool, _ time.Time) ([]string, []string, error) {
		called = true
		return nil, nil, nil
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	if app.runDueAutoScan(context.Background(), time.Date(2026, 7, 7, 1, 0, 30, 0, loc)) {
		t.Fatal("disabled auto scan should not start")
	}
	if called {
		t.Fatal("import runner should not be called when auto scan is disabled")
	}
}

func TestDashboardAutoScanPlanIncludesLastAutoScanAt(t *testing.T) {
	app := NewApp(LoadConfig())
	app.updateSettings(map[string]any{
		"auto_scan_enabled":  "true",
		"auto_scan_times":    "01:00",
		"auto_scan_timezone": "Asia/Shanghai",
		"last_auto_scan_at":  "2026-07-07 01:00:00",
	})

	metrics := appDashboardService{app: app}.withAutoScanPlan(DashboardMetrics{})
	if got := formatDateTime(metrics.LastAutoScanAt); got != "2026-07-07 01:00:00" {
		t.Fatalf("last auto scan time = %q, want saved value", got)
	}
}
