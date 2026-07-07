package app

import (
	"strings"
	"testing"
	"time"
)

func TestClickHouseDDLContainsCoreTables(t *testing.T) {
	sql := strings.Join(ClickHouseDDL(), "\n")

	for _, table := range []string{"app_settings", "log_sources", "ingest_dates", "ingest_files", "nat_logs"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("DDL 缺少表 %s:\n%s", table, sql)
		}
	}
}

func TestNatLogsDDLUsesExpectedPartitionAndOrderKey(t *testing.T) {
	sql := strings.Join(ClickHouseDDL(), "\n")

	if !strings.Contains(sql, "PARTITION BY log_date") {
		t.Fatalf("nat_logs 必须按 log_date 分区:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY (log_date, source_id, src_ip, timestamp)") {
		t.Fatalf("nat_logs 必须使用指定排序键:\n%s", sql)
	}
	if strings.Contains(sql, "Nullable(") {
		t.Fatalf("DDL 不能使用 Nullable:\n%s", sql)
	}
}

func TestStateTablesUseReplacingMergeTree(t *testing.T) {
	sql := strings.Join(ClickHouseDDL(), "\n")

	for _, snippet := range []string{
		"ENGINE = ReplacingMergeTree(updated_at)",
		"PRIMARY KEY (source_id, log_date)",
		"PRIMARY KEY path",
	} {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("DDL 缺少 %q:\n%s", snippet, sql)
		}
	}
}

func TestClickHouseDiskUsageSQLReadsActiveParts(t *testing.T) {
	sql := ClickHouseDiskUsageSQL()
	for _, want := range []string{
		"system.parts",
		"bytes_on_disk",
		"active",
		"currentDatabase()",
		"nat_logs",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("disk usage SQL missing %q: %s", want, sql)
		}
	}
}

func TestDistributionSQLFiltersByLogDate(t *testing.T) {
	since := time.Date(2026, 6, 4, 15, 30, 0, 0, time.Local)
	sql, args, err := distributionSQL("src_ip", since)
	if err != nil {
		t.Fatalf("distributionSQL returned error: %v", err)
	}

	if !strings.Contains(sql, "WHERE log_date >= ?") {
		t.Fatalf("distribution SQL should filter by log_date: %s", sql)
	}
	if !strings.Contains(sql, "GROUP BY src_ip") || !strings.Contains(sql, "LIMIT 10") {
		t.Fatalf("distribution SQL missing ranking clauses: %s", sql)
	}
	if len(args) != 1 {
		t.Fatalf("args = %#v, want one since arg", args)
	}
	got, ok := args[0].(time.Time)
	if !ok || !got.Equal(startOfDay(since)) {
		t.Fatalf("since arg = %#v, want start of day %v", args[0], startOfDay(since))
	}
}

func TestDistributionSQLRejectsUnsupportedColumn(t *testing.T) {
	if _, _, err := distributionSQL("source_file", time.Time{}); err == nil {
		t.Fatalf("unsupported distribution column should return error")
	}
}

func TestQueryNATLogsPageSQLUsesFastPathWithoutTimeSort(t *testing.T) {
	sql, args := queryNATLogsPageSQL("SELECT * FROM nat_logs WHERE log_date = ?", []any{"2026-06-10"}, QueryPageOptions{Page: 2, PageSize: 50}, QuerySortFast)

	if strings.Contains(sql, "ORDER BY timestamp DESC") {
		t.Fatalf("fast path should not force timestamp sort: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT ? OFFSET ?") {
		t.Fatalf("page SQL missing limit/offset: %s", sql)
	}
	if got := args; len(got) != 3 || got[1] != 50 || got[2] != 50 {
		t.Fatalf("args = %#v, want original args plus limit and offset", got)
	}
}

func TestQueryNATLogsPageSQLFallsBackWhenPageSizeIsZero(t *testing.T) {
	sql, args := queryNATLogsPageSQL("SELECT * FROM nat_logs WHERE log_date = ?", []any{"2026-06-10"}, QueryPageOptions{Page: 2, PageSize: 0}, QuerySortTimeAsc)

	if !strings.Contains(sql, "ORDER BY timestamp ASC, source_id ASC, source_file ASC, source_offset ASC") || !strings.Contains(sql, "LIMIT ? OFFSET ?") {
		t.Fatalf("zero page size should still be paged: %s", sql)
	}
	if len(args) != 3 || args[1] != 50 || args[2] != 50 {
		t.Fatalf("args = %#v, want original args plus fallback limit and offset", args)
	}
}

func TestQueryNATLogsPageSQLKeepsTimeSortForFilteredSearch(t *testing.T) {
	sql, _ := queryNATLogsPageSQL("SELECT * FROM nat_logs WHERE src_ip = ?", []any{"2.55.80.66"}, QueryPageOptions{Page: 2, PageSize: 50}, QuerySortTimeAsc)

	if !strings.Contains(sql, "ORDER BY timestamp ASC, source_id ASC, source_file ASC, source_offset ASC") {
		t.Fatalf("filtered search should keep timestamp sort: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT ? OFFSET ?") {
		t.Fatalf("page SQL missing limit/offset: %s", sql)
	}
}

func TestQueryNATLogsPageSQLUsesCursorWithoutOffset(t *testing.T) {
	cursor := QueryCursor{
		Timestamp:    time.Date(2026, 7, 4, 12, 0, 0, 0, time.Local),
		SourceID:     "default",
		SourceFile:   "fw.log-20260704",
		SourceOffset: 123456,
	}

	sql, args := queryNATLogsPageSQL(
		"SELECT * FROM nat_logs WHERE log_date = ?",
		[]any{"2026-07-04"},
		QueryPageOptions{Page: 1, PageSize: 50, Cursor: &cursor},
		QuerySortTimeAsc,
	)

	for _, want := range []string{
		"AND (timestamp > ? OR (timestamp = ? AND (source_id, source_file, source_offset) > (?, ?, ?)))",
		"ORDER BY timestamp ASC, source_id ASC, source_file ASC, source_offset ASC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("cursor SQL missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "OFFSET") {
		t.Fatalf("cursor SQL must not use OFFSET: %s", sql)
	}
	if len(args) != 7 {
		t.Fatalf("args = %#v, want original arg + cursor args + limit", args)
	}
	if args[1] != cursor.Timestamp || args[2] != cursor.Timestamp || args[3] != cursor.SourceID || args[4] != cursor.SourceFile || args[5] != cursor.SourceOffset || args[6] != 51 {
		t.Fatalf("unexpected cursor args: %#v", args)
	}
}

func TestQueryNATLogsCountSQLWrapsBaseQuery(t *testing.T) {
	sql := queryNATLogsCountSQL("SELECT * FROM nat_logs WHERE src_ip = ? AND action = ?")

	if sql != "SELECT count() FROM (SELECT * FROM nat_logs WHERE src_ip = ? AND action = ?) AS query_results" {
		t.Fatalf("unexpected count sql: %s", sql)
	}
}
