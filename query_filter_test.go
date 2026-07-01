package main

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildSearchQueriesUsesParsedTimestampForRelativeRange(t *testing.T) {
	filters := SearchFilters{
		Range:     "7d",
		Page:      1,
		PageSize:  25,
		PortScope: "any",
	}

	query, countQuery, _ := buildSearchQueries(filters, true)

	for _, sql := range []string{query, countQuery} {
		if strings.Contains(sql, "timestamp >=") {
			t.Fatalf("relative range must not compare syslog timestamps as strings: %s", sql)
		}
		if !strings.Contains(sql, "strptime") {
			t.Fatalf("relative range should parse syslog timestamps before comparing: %s", sql)
		}
	}
}

func TestIncrementalSyncDoesNotBlockReadOnlyRequests(t *testing.T) {
	state := RebuildState{Status: "running", Mode: "incremental_sync"}
	if rebuildBlocksReadOnlyRequests(state) {
		t.Fatal("incremental sync should keep existing query results available")
	}

	state = RebuildState{Status: "running", Mode: "full_rebuild"}
	if !rebuildBlocksReadOnlyRequests(state) {
		t.Fatal("full rebuild should block read-only requests while the index table is replaced")
	}
}

func TestParseFiltersFromQueryRejectsInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/api/query?page=abc", nil)

	if _, err := parseFiltersFromQuery(context); err == nil {
		t.Fatal("invalid page should return a clear validation error")
	}
}

func TestSearchIndexStatementsIncludeCatchUpDedupIndex(t *testing.T) {
	for _, statement := range searchIndexStatements {
		if statement.name == "idx_source_file_offset" &&
			strings.Contains(statement.sql, "source_file") &&
			strings.Contains(statement.sql, "source_offset") {
			return
		}
	}
	t.Fatal("incremental catch-up needs a source_file/source_offset index for overlap deletes")
}

func TestIsSupportedLogFileOnlyMatchesArchivedLogs(t *testing.T) {
	cases := []struct {
		name      string
		supported bool
	}{
		{"sangfor.log", false},
		{"sangfor.LOG", false},
		{"sangfor.log-20260701", true},
		{"sangfor.LOG-20260701", true},
		{"sangfor.log-20260701.gz", true},
		{"sangfor.log-20260701.tmp", false},
		{"sangfor.log-old.gz", false},
		{"other.gz", false},
	}

	for _, tc := range cases {
		if got := isSupportedLogFile(tc.name); got != tc.supported {
			t.Fatalf("isSupportedLogFile(%q) = %v, want %v", tc.name, got, tc.supported)
		}
	}
}

func TestSnapshotLogFilesSkipsOnlineLogFiles(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"sangfor.log",
		"sangfor.log-20260701",
		"sangfor.log-20260702.gz",
		"readme.txt",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	snapshots, err := snapshotLogFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshots) != 2 {
		t.Fatalf("snapshotLogFiles should only include archived logs, got %d snapshots: %#v", len(snapshots), snapshots)
	}
	for _, snapshot := range snapshots {
		if strings.HasSuffix(strings.ToLower(snapshot.Path), ".log") {
			t.Fatalf("online log file must not be indexed: %s", snapshot.Path)
		}
	}
}

func TestCatchUpWithBytesButNoParsedRowsIsNotSuccessful(t *testing.T) {
	err := validateCatchUpImportResult([]LogFileRange{{Path: "sangfor.log-20260701", Start: 0, End: 128}}, 0)
	if err == nil {
		t.Fatal("catch-up ranges with bytes but zero parsed rows should not be marked as synced")
	}

	if err := validateCatchUpImportResult(nil, 0); err != nil {
		t.Fatalf("empty catch-up ranges should stay a no-op: %v", err)
	}
}

func TestIncrementalImportDoesNotRebuildIndexes(t *testing.T) {
	executor := &recordingExecutor{}
	err := importCatchUpCSVWithExecutor(executor, "tmp_import_catchup.csv", []LogFileRange{
		{Path: "sangfor.log-20260701", Start: 0, End: 128},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, query := range executor.queries {
		upperQuery := strings.ToUpper(query)
		if strings.Contains(upperQuery, "DROP INDEX") || strings.Contains(upperQuery, "CREATE INDEX") {
			t.Fatalf("incremental imports should not rebuild indexes while read queries are allowed: %s", query)
		}
	}
}

func TestSettingsCanDisableAutoSyncWhileTaskIsRunning(t *testing.T) {
	if err := validateSettingsUpdateWhileRunning(false); err != nil {
		t.Fatalf("disabling auto sync should be allowed while a task is running: %v", err)
	}

	if err := validateSettingsUpdateWhileRunning(true); err == nil {
		t.Fatal("changing log directory should still be blocked while a task is running")
	}
}

type recordingExecutor struct {
	queries []string
}

func (executor *recordingExecutor) Exec(query string, args ...interface{}) (sql.Result, error) {
	executor.queries = append(executor.queries, query)
	return nil, nil
}
