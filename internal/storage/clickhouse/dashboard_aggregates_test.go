package clickhouse

import (
	"strings"
	"testing"
	"time"
)

func TestDashboardAggregateDDLDefinesTablesAndViews(t *testing.T) {
	sql := strings.Join(DashboardAggregateDDL(), "\n")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS dashboard_daily_totals",
		"CREATE TABLE IF NOT EXISTS dashboard_daily_ip_counts",
		"ENGINE = SummingMergeTree",
		"dashboard_daily_totals_mv",
		"dashboard_daily_src_ip_mv",
		"dashboard_daily_dst_ip_mv",
		"dashboard_daily_dst_subnet_mv",
		"cutIPv6(dst_ip, 8, 1)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("聚合 DDL 缺少 %q:\n%s", want, sql)
		}
	}
	if strings.Index(sql, "CREATE TABLE IF NOT EXISTS dashboard_daily_totals") > strings.Index(sql, "dashboard_daily_totals_mv") {
		t.Fatalf("目标表必须先于物化视图创建:\n%s", sql)
	}
}

func TestDashboardSummaryQueriesOnlyUseAggregateTables(t *testing.T) {
	queries := []string{
		DashboardTotalRowsSQL(),
		DashboardRowsForDateSQL(),
		ClickHouseLogTrendSQL(),
	}
	for _, sql := range queries {
		if strings.Contains(sql, "FROM nat_logs") {
			t.Fatalf("概览查询不能扫描 nat_logs: %s", sql)
		}
		if !strings.Contains(sql, "dashboard_daily_totals") || !strings.Contains(sql, "sum(rows)") {
			t.Fatalf("概览查询必须汇总 dashboard_daily_totals: %s", sql)
		}
	}
}

func TestDashboardRankingSQLUsesAggregateTablesAndTwoThreads(t *testing.T) {
	since := time.Date(2026, 6, 4, 15, 30, 0, 0, time.Local)
	tests := []struct {
		dimension string
		wantTable string
		wantLimit bool
	}{
		{dimension: "src_ip", wantTable: "dashboard_daily_ip_counts", wantLimit: true},
		{dimension: "dst_ip", wantTable: "dashboard_daily_ip_counts", wantLimit: true},
		{dimension: "dst_subnet", wantTable: "dashboard_daily_ip_counts", wantLimit: false},
		{dimension: "log_tag", wantTable: "dashboard_daily_totals", wantLimit: true},
	}

	for _, tt := range tests {
		t.Run(tt.dimension, func(t *testing.T) {
			sql, args, err := dashboardRankingSQL(tt.dimension, since, "fw-a")
			if err != nil {
				t.Fatalf("dashboardRankingSQL 返回错误: %v", err)
			}
			for _, want := range []string{tt.wantTable, "sum(rows)", "log_date >= ?", "source_id = ?", "SETTINGS max_threads = 1"} {
				if !strings.Contains(sql, want) {
					t.Fatalf("排行 SQL 缺少 %q: %s", want, sql)
				}
			}
			if strings.Contains(sql, "FROM nat_logs") {
				t.Fatalf("排行查询不能扫描 nat_logs: %s", sql)
			}
			if got := strings.Contains(sql, "LIMIT 10"); got != tt.wantLimit {
				t.Fatalf("LIMIT 10 = %v, want %v: %s", got, tt.wantLimit, sql)
			}
			wantArgs := 2
			if tt.dimension != "log_tag" {
				wantArgs = 3
			}
			if len(args) != wantArgs || args[len(args)-1] != "fw-a" {
				t.Fatalf("args = %#v, want source as final argument", args)
			}
		})
	}
}

func TestDashboardRankingSQLRejectsUnsupportedDimension(t *testing.T) {
	if _, _, err := dashboardRankingSQL("source_file", time.Time{}, ""); err == nil {
		t.Fatal("不支持的排行维度必须返回错误")
	}
}

func TestClickHouseDiskUsageIncludesDashboardAggregateTables(t *testing.T) {
	sql := ClickHouseDiskUsageSQL()
	for _, want := range []string{"dashboard_daily_totals", "dashboard_daily_ip_counts"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("磁盘统计缺少 %q: %s", want, sql)
		}
	}
}
