package clickhouse

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestDateStatePointQueryOrdersByUpdatedAt(t *testing.T) {
	sql := DateStatePointQuery()
	if !strings.Contains(sql, "ORDER BY updated_at DESC") || !strings.Contains(sql, "LIMIT 1") {
		t.Fatalf("point query must read newest state: %s", sql)
	}
	if !strings.Contains(sql, "multiIf(status = 'failed', 4, status IN ('ready', 'succeeded'), 3") {
		t.Fatalf("point query must prefer terminal states when timestamps tie: %s", sql)
	}
	if strings.Contains(sql, "FINAL") {
		t.Fatalf("point query must not use FINAL: %s", sql)
	}
}

func TestDateStatePointQueryCoversDateIngestStateFields(t *testing.T) {
	sql := DateStatePointQuery()
	typ := reflect.TypeOf(DateIngestState{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i).Tag.Get("json")
		if field == "" {
			t.Fatalf("field %s missing json tag", typ.Field(i).Name)
		}
		if !strings.Contains(sql, field) {
			t.Fatalf("point query missing field %q: %s", field, sql)
		}
	}
}

func TestStateUpsertsWriteStatusTables(t *testing.T) {
	dateSQL := DateStateUpsertSQL()
	for _, want := range []string{
		"INSERT INTO ingest_dates",
		"source_id, log_tag, log_date, status",
		"retry_count, next_retry_at, error, updated_at",
	} {
		if !strings.Contains(dateSQL, want) {
			t.Fatalf("date upsert missing %q: %s", want, dateSQL)
		}
	}

	fileSQL := FileStateUpsertSQL()
	for _, want := range []string{
		"INSERT INTO ingest_files",
		"path, source_id, log_tag, log_date, status",
		"retry_count, next_retry_at, error, updated_at",
	} {
		if !strings.Contains(fileSQL, want) {
			t.Fatalf("file upsert missing %q: %s", want, fileSQL)
		}
	}
}

func TestFileStatePointQueryReadsNewestState(t *testing.T) {
	sql := FileStatePointQuery()
	for _, want := range []string{
		"FROM ingest_files",
		"WHERE path = ?",
		"ORDER BY updated_at DESC",
		"LIMIT 1",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("file point query missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "FINAL") {
		t.Fatalf("file point query must not use FINAL: %s", sql)
	}
	if !strings.Contains(sql, "multiIf(status = 'failed', 4, status IN ('ready', 'succeeded'), 3") {
		t.Fatalf("file point query must prefer terminal states when timestamps tie: %s", sql)
	}
}

func TestDateStateListQueryUsesArgMax(t *testing.T) {
	sql := DateStateListQuery()
	for _, want := range []string{
		"argMax(status, tuple(ingest_dates.updated_at, multiIf(ingest_dates.status = 'failed', 4",
		"argMax(retry_count, tuple(ingest_dates.updated_at, multiIf(ingest_dates.status = 'failed', 4",
		"argMax(next_retry_at, tuple(ingest_dates.updated_at, multiIf(ingest_dates.status = 'failed', 4",
		"WHERE log_date >= toDate(?)",
		"GROUP BY source_id, log_date",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("list query missing %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "FINAL") {
		t.Fatalf("list query must not use FINAL: %s", sql)
	}
}

func TestDateStateListQueryDoesNotShadowUpdatedAtColumn(t *testing.T) {
	sql := DateStateListQuery()
	if strings.Contains(sql, "AS updated_at") {
		t.Fatalf("list query must not alias an aggregate as updated_at because ClickHouse expands aliases inside other aggregate calls: %s", sql)
	}
	if !strings.Contains(sql, "max(updated_at) AS latest_updated_at") {
		t.Fatalf("list query should expose the latest timestamp with a non-conflicting alias: %s", sql)
	}
}

func TestDateStateListQueryQualifiesStatusVersionColumns(t *testing.T) {
	sql := DateStateListQuery()
	for _, want := range []string{"ingest_dates.updated_at", "ingest_dates.status = 'failed'", "ingest_dates.status IN ('ready', 'succeeded')"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("list query must qualify state version column %q to avoid alias expansion: %s", want, sql)
		}
	}
	if strings.Contains(sql, "tuple(updated_at, multiIf(status") {
		t.Fatalf("unqualified state version columns can expand the status alias into a nested aggregate: %s", sql)
	}
}

func TestIngestStateTimestampsUseMicroseconds(t *testing.T) {
	ddl := strings.Join(ClickHouseDDL(), "\n")
	for _, table := range []string{"ingest_dates", "ingest_files"} {
		start := strings.Index(ddl, "CREATE TABLE IF NOT EXISTS "+table)
		if start < 0 {
			t.Fatalf("missing %s DDL", table)
		}
		segment := ddl[start:]
		if next := strings.Index(segment[1:], "CREATE TABLE IF NOT EXISTS "); next >= 0 {
			segment = segment[:next+1]
		}
		if !strings.Contains(segment, "updated_at DateTime64(6) DEFAULT now64(6)") {
			t.Fatalf("%s must use a microsecond state version: %s", table, segment)
		}
	}
}

func TestIngestStateMigrationRepairsCompletedImportingStates(t *testing.T) {
	statements := strings.Join(IngestStateMigrationSQL(), "\n")
	for _, want := range []string{
		"INSERT INTO ingest_dates",
		"maxIf(updated_at, status = 'importing' AND progress_pct >= 100",
		"files_done >= files_total) = max(updated_at)",
		"maxIf(updated_at, status = 'failed')",
		"addSeconds(now(), 1)",
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("ingest state migration must repair completed importing states; missing %q: %s", want, statements)
		}
	}
	if strings.Contains(statements, "MODIFY COLUMN updated_at") {
		t.Fatalf("migration must not alter a ReplacingMergeTree version column in place: %s", statements)
	}
}

func TestIsNoRowsErrorRecognizesSQLNoRows(t *testing.T) {
	if !isNoRowsError(sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows should be treated as missing state")
	}
}

func TestIngestStatusScansClickHouseStringValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  IngestStatus
	}{
		{name: "string", value: "ready", want: StatusReady},
		{name: "bytes", value: []byte("failed"), want: StatusFailed},
		{name: "nil", value: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got IngestStatus
			if err := got.Scan(tt.value); err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Scan(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestIngestStatusRejectsUnsupportedScanValues(t *testing.T) {
	var got IngestStatus
	err := got.Scan(123)
	if err == nil {
		t.Fatal("Scan should reject unsupported values")
	}
	if want := "cannot scan int into IngestStatus"; err.Error() != want {
		t.Fatalf("Scan error = %q, want %q", err.Error(), want)
	}
	if got != "" {
		t.Fatalf("Scan should not set status on unsupported values, got %q", got)
	}
	if !strings.Contains(fmt.Sprint(err), "IngestStatus") {
		t.Fatalf("Scan error should name IngestStatus: %v", err)
	}
}
