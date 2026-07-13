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
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool) ([]string, []string, error) {
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

	if got := app.getSettings()["last_auto_scan_at"]; got != "2026-07-07 01:00:00" {
		t.Fatalf("last_auto_scan_at = %q, want scheduled time", got)
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
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool) ([]string, []string, error) {
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
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool) ([]string, []string, error) {
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
