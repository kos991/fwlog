package app

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

func TestBuildIngestProgressIncludesEachSource(t *testing.T) {
	states := []DateIngestState{
		{SourceID: "fw-b", LogDate: dateOnly(2026, 7, 1), Status: StatusReady},
		{SourceID: "fw-a", LogDate: dateOnly(2026, 7, 1), Status: StatusImporting, ProgressPct: 40},
		{SourceID: "fw-b", LogDate: dateOnly(2026, 7, 2), Status: StatusImporting, ProgressPct: 60},
	}
	progress := BuildIngestProgress(states, true)
	if len(progress.Sources) != 2 || progress.Sources[0].SourceID != "fw-a" || progress.Sources[1].SourceID != "fw-b" {
		t.Fatalf("sources = %#v", progress.Sources)
	}
	if progress.Sources[1].Status != StatusImporting || progress.Sources[1].ProgressPct != 60 {
		t.Fatalf("fw-b progress = %#v", progress.Sources[1])
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
		LogTrend:                []DistributionItem{{Name: "10:00", Value: 12}, {Name: "11:00", Value: 18}},
		SystemHealth: SystemHealth{
			CPU: CPUHealth{
				Status:      "ok",
				LoadPercent: 35.5,
				Cores:       8,
			},
			Memory: MemoryHealth{
				Status:      "warning",
				UsedPercent: 78.2,
			},
			Database: DatabaseHealth{
				Status:       "busy",
				Version:      "25.8.27.1",
				ActiveMerges: 1,
			},
		},
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
	if dashboard.SystemHealth.CPU.LoadPercent != 35.5 || dashboard.SystemHealth.Memory.Status != "warning" || dashboard.SystemHealth.Database.Version != "25.8.27.1" {
		t.Fatalf("system health not copied: %#v", dashboard.SystemHealth)
	}
	if len(dashboard.LogTrend) != 2 || dashboard.LogTrend[0].Name != "10:00" || dashboard.LogTrend[1].Value != 18 {
		t.Fatalf("log trend not copied: %#v", dashboard.LogTrend)
	}
}

func TestBuildHealthDashboardDoesNotExposeNATRanking(t *testing.T) {
	dashboard := BuildHealthDashboard(nil, DashboardMetrics{
		TopNATIPs: []DistributionItem{{Name: "58.216.48.6", Value: 100}},
	})

	if len(dashboard.IPDistribution.TopNATIPs) != 0 {
		t.Fatalf("NAT ranking should not be exposed on dashboard: %#v", dashboard.IPDistribution.TopNATIPs)
	}
}

func TestBuildSourceIngestProgressUsesLatestStateInsteadOfHistoricalFailure(t *testing.T) {
	oldFailure := DateIngestState{SourceID: "fw-a", LogTag: "A", LogDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Status: StatusFailed, UpdatedAt: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), Error: "old"}
	newReady := DateIngestState{SourceID: "fw-a", LogTag: "A", LogDate: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), Status: StatusReady, UpdatedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)}

	progress := buildSourceIngestProgress([]DateIngestState{oldFailure, newReady})
	if len(progress) != 1 || progress[0].Status != StatusReady || progress[0].CurrentDate != "2026-07-02" {
		t.Fatalf("latest source state should win: %#v", progress)
	}
}

func TestEnrichGeoDistributionMetricsAggregatesDestinationCountries(t *testing.T) {
	engine := NewIPEngine()
	engine.AddOverride("8.8.8.8", "Google DNS", "美国 / Mountain View")
	engine.AddOverride("1.1.1.1", "Cloudflare DNS", "美国 / Los Angeles")
	engine.AddOverride("114.114.114.114", "114 DNS", "中国 / 江苏")

	metrics := enrichGeoDistributionMetrics(DashboardMetrics{
		TopDestinationIPs: []DistributionItem{
			{Name: "8.8.8.8", Value: 10},
			{Name: "1.1.1.1", Value: 7},
			{Name: "114.114.114.114", Value: 5},
		},
	}, engine)

	if len(metrics.TopCountries) < 2 {
		t.Fatalf("countries should be populated from destination IPs: %#v", metrics.TopCountries)
	}
	if metrics.TopCountries[0].Name != "美国" || metrics.TopCountries[0].Value != 17 {
		t.Fatalf("top country should aggregate same country: %#v", metrics.TopCountries)
	}
	if len(metrics.TopRegions) == 0 || metrics.TopRegions[0].Name == "" {
		t.Fatalf("regions should be populated from destination locations: %#v", metrics.TopRegions)
	}
}

func TestBuildAutoScanPlanForDailyTime(t *testing.T) {
	now := time.Date(2026, 7, 4, 8, 30, 0, 0, time.Local)

	plan := BuildAutoScanPlan(map[string]string{
		"auto_scan_enabled":  "true",
		"auto_scan_mode":     "daily",
		"auto_scan_times":    "01:00,09:30",
		"auto_scan_timezone": "Asia/Shanghai",
	}, now)

	if !plan.Enabled || plan.NextAt.Format("2006-01-02 15:04:05") != "2026-07-04 09:30:00" {
		t.Fatalf("daily plan next time wrong: %#v", plan)
	}
	if plan.Policy != "01:00、09:30 自动扫描" {
		t.Fatalf("daily policy = %q", plan.Policy)
	}
}

func TestBuildAutoScanPlanUsesConfiguredScanTimeOnly(t *testing.T) {
	now := time.Date(2026, 7, 4, 8, 30, 0, 0, time.Local)

	plan := BuildAutoScanPlan(map[string]string{
		"auto_scan_enabled":      "true",
		"auto_scan_mode":         "interval",
		"auto_scan_times":        "01:00",
		"auto_scan_interval_sec": "21600",
		"auto_scan_timezone":     "Asia/Shanghai",
	}, now)

	if !plan.Enabled || plan.Mode != "interval" {
		t.Fatalf("interval plan should be enabled with interval mode: %#v", plan)
	}
	if plan.NextAt.Format("2006-01-02 15:04:05") != "2026-07-04 12:00:00" {
		t.Fatalf("interval plan next time wrong: %#v", plan)
	}
	if plan.Policy != "每 6 小时自动扫描" {
		t.Fatalf("interval plan policy = %q", plan.Policy)
	}
}

func TestEnrichGeoDistributionMetricsGroupsUnknownPublicIPsAsUnknownCountry(t *testing.T) {
	engine := NewIPEngine()

	metrics := enrichGeoDistributionMetrics(DashboardMetrics{
		TopDestinationIPs: []DistributionItem{
			{Name: "8.8.8.8", Value: 9},
			{Name: "10.0.0.8", Value: 3},
		},
	}, engine)

	if len(metrics.TopCountries) == 0 {
		t.Fatalf("countries should include unknown public IP bucket: %#v", metrics.TopCountries)
	}
	if metrics.TopCountries[0].Name != "未知" || metrics.TopCountries[0].Value != 9 {
		t.Fatalf("unknown public IPs should aggregate under unknown: %#v", metrics.TopCountries)
	}
	if len(metrics.TopRegions) == 0 || metrics.TopRegions[0].Name != "未知" {
		t.Fatalf("unknown public IPs should expose unknown region: %#v", metrics.TopRegions)
	}
}
