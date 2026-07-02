package main

import (
	"testing"
	"time"
)

func TestBuildIngestProgressShowsProblemDatesByDefault(t *testing.T) {
	states := []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusReady},
		{LogDate: dateOnly(2026, 6, 29), Status: StatusImporting},
		{LogDate: dateOnly(2026, 6, 30), Status: StatusPending},
		{LogDate: dateOnly(2026, 7, 1), Status: StatusFailed},
	}

	progress := BuildIngestProgress(states, false)

	if len(progress.Dates) != 3 {
		t.Fatalf("default list should hide ready dates, got %#v", progress.Dates)
	}
	for _, date := range progress.Dates {
		if date.Status == StatusReady {
			t.Fatalf("ready date should be hidden by default: %#v", progress.Dates)
		}
	}
}

func TestBuildIngestProgressCanIncludeReadyDates(t *testing.T) {
	states := []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusReady},
		{LogDate: dateOnly(2026, 6, 29), Status: StatusFailed},
	}

	progress := BuildIngestProgress(states, true)

	if len(progress.Dates) != 2 {
		t.Fatalf("includeReady should show all dates, got %#v", progress.Dates)
	}
}

func TestBuildIngestProgressPicksCurrentImportingDate(t *testing.T) {
	updatedAt := time.Date(2026, 7, 2, 9, 30, 0, 0, time.Local)
	states := []DateIngestState{
		{
			SourceID:     "src-a",
			LogTag:       "出口防火墙",
			LogDate:      dateOnly(2026, 7, 1),
			Status:       StatusImporting,
			FilesTotal:   12,
			FilesDone:    5,
			RowsImported: 5450000,
			BytesTotal:   2000,
			BytesDone:    1000,
			CurrentFile:  "fw.log-20260701.gz",
			ProgressPct:  50,
			UpdatedAt:    updatedAt,
		},
	}

	progress := BuildIngestProgress(states, false)

	if progress.Status != StatusImporting {
		t.Fatalf("status = %q", progress.Status)
	}
	if progress.SourceID != "src-a" || progress.LogTag != "出口防火墙" {
		t.Fatalf("source fields not copied: %#v", progress)
	}
	if progress.CurrentDate != "2026-07-01" || progress.CurrentFile != "fw.log-20260701.gz" {
		t.Fatalf("current fields not copied: %#v", progress)
	}
	if progress.FilesTotal != 12 || progress.FilesDone != 5 || progress.BytesTotal != 2000 || progress.BytesDone != 1000 || progress.RowsImported != 5450000 {
		t.Fatalf("progress counters not copied: %#v", progress)
	}
	if len(progress.Dates) != 1 || progress.Dates[0].UpdatedAt != updatedAt {
		t.Fatalf("date row not preserved: %#v", progress.Dates)
	}
}

func TestBuildIngestProgressIncludesAutoScanTimes(t *testing.T) {
	lastAutoScanAt := time.Date(2026, 7, 2, 1, 0, 0, 0, time.Local)
	nextAutoScanAt := time.Date(2026, 7, 3, 1, 0, 0, 0, time.Local)

	progress := BuildIngestProgress(nil, false, DashboardMetrics{
		LastAutoScanAt: lastAutoScanAt,
		NextAutoScanAt: nextAutoScanAt,
	})

	if progress.LastAutoScanAt != "2026-07-02 01:00:00" || progress.NextAutoScanAt != "2026-07-03 01:00:00" {
		t.Fatalf("auto scan times not copied: %#v", progress)
	}
}

func TestBuildHealthDashboardSummarizesDateStates(t *testing.T) {
	states := []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusReady, RowsImported: 100, UpdatedAt: time.Date(2026, 7, 2, 8, 0, 0, 0, time.Local)},
		{LogDate: dateOnly(2026, 6, 29), Status: StatusReady, RowsImported: 200, UpdatedAt: time.Date(2026, 7, 2, 9, 0, 0, 0, time.Local)},
		{LogDate: dateOnly(2026, 6, 30), Status: StatusPending},
		{LogDate: dateOnly(2026, 7, 1), Status: StatusImporting},
		{LogDate: dateOnly(2026, 7, 2), Status: StatusFailed},
	}

	dashboard := BuildHealthDashboard(states, DashboardMetrics{
		ClickHouseDiskUsedBytes: 4096,
		TopSourceIPs:            []DistributionItem{{Name: "10.0.0.1", Value: 10}},
		TopCountries:            []DistributionItem{{Name: "中国", Value: 8}},
		GeoIPLoaded:             true,
	})

	if dashboard.DataHealth.TotalLogs != 300 {
		t.Fatalf("total logs = %d", dashboard.DataHealth.TotalLogs)
	}
	if dashboard.DataHealth.ReadyDates != 2 || dashboard.DataHealth.PendingDates != 1 || dashboard.DataHealth.ImportingDates != 1 || dashboard.DataHealth.FailedDates != 1 {
		t.Fatalf("date counters wrong: %#v", dashboard.DataHealth)
	}
	if dashboard.DataHealth.QueryableStartDate != "2026-06-28" || dashboard.DataHealth.QueryableEndDate != "2026-06-29" {
		t.Fatalf("queryable range wrong: %#v", dashboard.DataHealth)
	}
	if dashboard.DataHealth.ClickHouseDiskUsedBytes != 4096 {
		t.Fatalf("disk used not copied: %#v", dashboard.DataHealth)
	}
	if dashboard.IngestHealth.Status != StatusImporting {
		t.Fatalf("ingest status = %q", dashboard.IngestHealth.Status)
	}
	if len(dashboard.IPDistribution.TopSourceIPs) != 1 || len(dashboard.GeoDistribution.TopCountries) != 1 || !dashboard.GeoDistribution.GeoIPLoaded {
		t.Fatalf("distribution metrics not copied: %#v", dashboard)
	}
}
