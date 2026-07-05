package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildVisibleRangesQueriesReadyAndPartialImportingDates(t *testing.T) {
	start := time.Date(2026, 6, 28, 1, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)
	states := []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusReady, MaxVisibleTimestamp: endOfDay(2026, 6, 28)},
		{LogDate: dateOnly(2026, 6, 30), Status: StatusImporting, MaxVisibleTimestamp: time.Date(2026, 6, 30, 10, 0, 0, 0, time.Local)},
		{LogDate: dateOnly(2026, 7, 1), Status: StatusPending},
	}

	visibility := BuildVisibleRanges(start, end, states)

	if !visibility.Partial {
		t.Fatal("visibility should be partial")
	}
	if visibility.Message != "所选时间包含未完成入库日期，已自动只查询已入库部分。" {
		t.Fatalf("visibility message = %q", visibility.Message)
	}
	if len(visibility.QueriedRanges) != 2 {
		t.Fatalf("queried ranges = %#v", visibility.QueriedRanges)
	}
	if got := visibility.QueriedRanges[0]; !got.StartTime.Equal(start) || !got.EndTime.Equal(endOfDay(2026, 6, 28)) || !got.LogDate.Equal(dateOnly(2026, 6, 28)) {
		t.Fatalf("first range = %#v", got)
	}
	if got := visibility.QueriedRanges[1]; !got.StartTime.Equal(dateOnly(2026, 6, 30)) || !got.EndTime.Equal(time.Date(2026, 6, 30, 10, 0, 0, 0, time.Local)) || got.Status != StatusImporting {
		t.Fatalf("second range = %#v", got)
	}
	if len(visibility.SkippedDates) != 2 {
		t.Fatalf("skipped dates = %#v", visibility.SkippedDates)
	}
	if got := visibility.SkippedDates[0]; !got.LogDate.Equal(dateOnly(2026, 6, 29)) || got.Reason != "当天没有入库状态，已跳过" {
		t.Fatalf("missing-state skipped date = %#v", got)
	}
	if got := visibility.SkippedDates[1]; !got.LogDate.Equal(dateOnly(2026, 7, 1)) || got.Status != StatusPending || got.Reason != "当天未完成入库，已跳过" {
		t.Fatalf("pending skipped date = %#v", got)
	}
}

func TestBuildVisibleRangesImportingFullyCoversRequestWithoutPartial(t *testing.T) {
	start := time.Date(2026, 6, 30, 8, 0, 0, 0, time.Local)
	end := time.Date(2026, 6, 30, 9, 0, 0, 0, time.Local)

	visibility := BuildVisibleRanges(start, end, []DateIngestState{
		{
			LogDate:             dateOnly(2026, 6, 30),
			Status:              StatusImporting,
			MaxVisibleTimestamp: time.Date(2026, 6, 30, 9, 0, 0, 0, time.Local),
		},
	})

	if visibility.Partial {
		t.Fatalf("visibility should not be partial: %#v", visibility)
	}
	if visibility.Message != "" {
		t.Fatalf("visibility message = %q", visibility.Message)
	}
	if len(visibility.QueriedRanges) != 1 {
		t.Fatalf("queried ranges = %#v", visibility.QueriedRanges)
	}
	if got := visibility.QueriedRanges[0]; !got.StartTime.Equal(start) || !got.EndTime.Equal(end) || got.Status != StatusImporting {
		t.Fatalf("queried range = %#v", got)
	}
	if len(visibility.SkippedDates) != 0 {
		t.Fatalf("skipped dates = %#v", visibility.SkippedDates)
	}
}

func TestBuildVisibleRangesImportingTruncatesRequest(t *testing.T) {
	start := time.Date(2026, 6, 30, 8, 0, 0, 0, time.Local)
	end := time.Date(2026, 6, 30, 12, 0, 0, 0, time.Local)

	visibility := BuildVisibleRanges(start, end, []DateIngestState{
		{
			LogDate:             dateOnly(2026, 6, 30),
			Status:              StatusImporting,
			MaxVisibleTimestamp: time.Date(2026, 6, 30, 9, 30, 0, 0, time.Local),
		},
	})

	if !visibility.Partial {
		t.Fatalf("visibility should be partial: %#v", visibility)
	}
	if len(visibility.QueriedRanges) != 1 {
		t.Fatalf("queried ranges = %#v", visibility.QueriedRanges)
	}
	if got := visibility.QueriedRanges[0]; !got.EndTime.Equal(time.Date(2026, 6, 30, 9, 30, 0, 0, time.Local)) {
		t.Fatalf("queried range = %#v", got)
	}
}

func TestBuildVisibleRangesMatchesLogDateByCalendarDateAcrossLocations(t *testing.T) {
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	utc := time.UTC
	start := time.Date(2026, 6, 30, 8, 0, 0, 0, shanghai)
	end := time.Date(2026, 6, 30, 9, 0, 0, 0, shanghai)

	visibility := BuildVisibleRanges(start, end, []DateIngestState{
		{LogDate: time.Date(2026, 6, 30, 0, 0, 0, 0, utc), Status: StatusReady},
	})

	if visibility.Partial {
		t.Fatalf("visibility should not be partial when the calendar date matches: %#v", visibility)
	}
	if len(visibility.QueriedRanges) != 1 {
		t.Fatalf("queried ranges = %#v", visibility.QueriedRanges)
	}
	if got := visibility.QueriedRanges[0]; !got.LogDate.Equal(time.Date(2026, 6, 30, 0, 0, 0, 0, shanghai)) {
		t.Fatalf("log date should use request location: %#v", got)
	}
}

func TestBuildVisibleRangesSkipsNonQueryableStatuses(t *testing.T) {
	start := dateOnly(2026, 6, 28)
	end := endOfDay(2026, 7, 2)
	states := []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusIdle},
		{LogDate: dateOnly(2026, 6, 29), Status: StatusScanning},
		{LogDate: dateOnly(2026, 6, 30), Status: StatusSucceeded},
		{LogDate: dateOnly(2026, 7, 1), Status: ""},
		{LogDate: dateOnly(2026, 7, 2), Status: IngestStatus("mystery")},
	}

	visibility := BuildVisibleRanges(start, end, states)

	if !visibility.Partial {
		t.Fatalf("visibility should be partial: %#v", visibility)
	}
	if len(visibility.QueriedRanges) != 0 {
		t.Fatalf("queried ranges = %#v", visibility.QueriedRanges)
	}
	if len(visibility.SkippedDates) != 5 {
		t.Fatalf("skipped dates = %#v", visibility.SkippedDates)
	}
	for i, want := range []struct {
		logDate time.Time
		status  IngestStatus
		reason  string
	}{
		{dateOnly(2026, 6, 28), StatusIdle, "当天状态不可查询，已跳过"},
		{dateOnly(2026, 6, 29), StatusScanning, "当天状态不可查询，已跳过"},
		{dateOnly(2026, 6, 30), StatusSucceeded, "当天状态不可查询，已跳过"},
		{dateOnly(2026, 7, 1), "", "当天状态不可查询，已跳过"},
		{dateOnly(2026, 7, 2), IngestStatus("mystery"), "当天状态不可查询，已跳过"},
	} {
		got := visibility.SkippedDates[i]
		if !got.LogDate.Equal(want.logDate) || got.Status != want.status || got.Reason != want.reason {
			t.Fatalf("skippedDates[%d] = %#v, want %#v", i, got, want)
		}
	}
}

func TestVisibilityTextIsUTF8Chinese(t *testing.T) {
	visibility := BuildVisibleRanges(
		dateOnly(2026, 7, 1),
		endOfDay(2026, 7, 1),
		[]DateIngestState{{LogDate: dateOnly(2026, 7, 1), Status: StatusPending}},
	)

	if visibility.Message != "所选时间包含未完成入库日期，已自动只查询已入库部分。" {
		t.Fatalf("message should be real UTF-8 Chinese: %q", visibility.Message)
	}
	if len(visibility.SkippedDates) != 1 || visibility.SkippedDates[0].Reason != "当天未完成入库，已跳过" {
		t.Fatalf("reason should be real UTF-8 Chinese: %#v", visibility.SkippedDates)
	}
}

func TestBuildVisibleRangesIsNotPartialWhenAllDatesReady(t *testing.T) {
	start := time.Date(2026, 6, 28, 1, 0, 0, 0, time.Local)
	end := time.Date(2026, 6, 28, 12, 0, 0, 0, time.Local)

	visibility := BuildVisibleRanges(start, end, []DateIngestState{
		{LogDate: dateOnly(2026, 6, 28), Status: StatusReady, MaxVisibleTimestamp: endOfDay(2026, 6, 28)},
	})

	if visibility.Partial {
		t.Fatalf("visibility should not be partial: %#v", visibility)
	}
	if visibility.Message != "" {
		t.Fatalf("visibility message = %q", visibility.Message)
	}
	if len(visibility.SkippedDates) != 0 {
		t.Fatalf("skipped dates = %#v", visibility.SkippedDates)
	}
}

func TestBuildQuerySQLErrorsWhenNothingVisible(t *testing.T) {
	_, _, err := BuildQuerySQL(QueryRequest{}, QueryVisibility{})
	if err == nil {
		t.Fatal("BuildQuerySQL should fail when no visible ranges exist")
	}
}

func TestBuildQuerySQLBuildsVisibleDatePredicatesAndParameterizedFilters(t *testing.T) {
	start := time.Date(2026, 6, 28, 1, 2, 3, 0, time.Local)
	end := time.Date(2026, 6, 28, 4, 5, 6, 0, time.Local)
	visibility := QueryVisibility{
		QueriedRanges: []VisibleRange{
			{
				LogDate:   dateOnly(2026, 6, 28),
				StartTime: start,
				EndTime:   end,
				Status:    StatusReady,
			},
		},
	}
	req := QueryRequest{
		IP:       "10.0.0.1",
		Port:     8443,
		Protocol: "TCP",
		Action:   "allow",
		LogTag:   "edge-a",
	}

	sql, args, err := BuildQuerySQL(req, visibility)
	if err != nil {
		t.Fatalf("BuildQuerySQL returned error: %v", err)
	}

	if !strings.Contains(sql, "(log_date = ? AND timestamp >= ? AND timestamp <= ?)") {
		t.Fatalf("sql missing visible range predicate: %s", sql)
	}
	for _, want := range []string{
		"(src_ip = ? OR dst_ip = ? OR nat_ip = ?)",
		"(src_port = ? OR dst_port = ? OR nat_port = ?)",
		"protocol IN (?, ?, ?)",
		"action = ?",
		"log_tag = ?",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("sql missing filter %q: %s", want, sql)
		}
	}
	if strings.Contains(sql, "10.0.0.1") || strings.Contains(sql, "allow") || strings.Contains(sql, "edge-a") {
		t.Fatalf("sql should be parameterized: %s", sql)
	}

	wantArgs := []any{
		dateOnly(2026, 6, 28),
		start,
		end,
		"10.0.0.1",
		"10.0.0.1",
		"10.0.0.1",
		uint16(8443),
		uint16(8443),
		uint16(8443),
		"TCP",
		"6",
		"6,",
		"allow",
		"edge-a",
	}
	if len(args) != len(wantArgs) {
		t.Fatalf("args len = %d, want %d (%#v)", len(args), len(wantArgs), args)
	}
	for i, want := range wantArgs {
		if args[i] != want {
			t.Fatalf("args[%d] = %#v, want %#v", i, args[i], want)
		}
	}
}

func TestQueryCursorRoundTripsAsBase64URLJSON(t *testing.T) {
	cursor := QueryCursor{
		Timestamp:    time.Date(2026, 7, 4, 12, 0, 0, 0, time.Local),
		SourceID:     "default",
		SourceFile:   "fw.log-20260704",
		SourceOffset: 123456,
	}

	encoded, err := encodeQueryCursor(cursor)
	if err != nil {
		t.Fatalf("encodeQueryCursor returned error: %v", err)
	}
	if strings.ContainsAny(encoded, "+/=") {
		t.Fatalf("cursor should use raw base64url encoding: %q", encoded)
	}

	decoded, err := decodeQueryCursor(encoded)
	if err != nil {
		t.Fatalf("decodeQueryCursor returned error: %v", err)
	}
	if !decoded.Timestamp.Equal(cursor.Timestamp) || decoded.SourceID != cursor.SourceID || decoded.SourceFile != cursor.SourceFile || decoded.SourceOffset != cursor.SourceOffset {
		t.Fatalf("decoded cursor = %#v, want %#v", decoded, cursor)
	}
}

func TestValidateQueryProtectionRejectsBroadUnfilteredQuery(t *testing.T) {
	req := QueryRequest{
		Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 7, 3, 0, 0, 0, 0, time.Local),
	}

	err := validateQueryProtection(req, QueryPageOptions{Page: 1, PageSize: 100})
	if err == nil {
		t.Fatal("validateQueryProtection should reject broad unfiltered query")
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %T %v, want QueryError", err, err)
	}
	if queryErr.Code != "query_too_broad" || queryErr.Status != 400 {
		t.Fatalf("query error = %#v", queryErr)
	}
	if !strings.Contains(queryErr.Message, "无筛选查询最多支持 1 天") || !strings.Contains(queryErr.Message, "带筛选查询最多支持 31 天") {
		t.Fatalf("message = %q, want unfiltered and filtered limits", queryErr.Message)
	}
}

func TestValidateQueryProtectionAllowsFilteredMonthQuery(t *testing.T) {
	req := QueryRequest{
		Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local),
		SrcIP: "2.55.80.66",
	}

	if err := validateQueryProtection(req, QueryPageOptions{Page: 1, PageSize: 100}); err != nil {
		t.Fatalf("filtered month query should be allowed: %v", err)
	}
}

func TestValidateQueryProtectionExplainsFilteredLimit(t *testing.T) {
	req := QueryRequest{
		Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 8, 2, 0, 0, 0, 0, time.Local),
		SrcIP: "2.55.80.66",
	}

	err := validateQueryProtection(req, QueryPageOptions{Page: 1, PageSize: 100})
	if err == nil {
		t.Fatal("filtered query beyond one month should be rejected")
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %T %v, want QueryError", err, err)
	}
	if !strings.Contains(queryErr.Message, "带筛选查询最多支持 31 天") || strings.Contains(queryErr.Message, "增加 IP/端口/协议") {
		t.Fatalf("message = %q, want filtered limit without suggesting filters", queryErr.Message)
	}
}

func TestValidateQueryProtectionRejectsDeepLegacyPage(t *testing.T) {
	req := QueryRequest{
		Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 7, 1, 1, 0, 0, 0, time.Local),
		SrcIP: "2.55.80.66",
	}

	err := validateQueryProtection(req, QueryPageOptions{Page: 21, PageSize: 100})
	if err == nil {
		t.Fatal("deep legacy pagination should be rejected")
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) || queryErr.Code != "query_too_broad" {
		t.Fatalf("error = %#v, want query_too_broad", err)
	}
}

func TestAcquireQuerySlotRejectsWhenBusy(t *testing.T) {
	app := &App{querySem: make(chan struct{}, 1)}
	app.querySem <- struct{}{}
	service := appQueryService{app: app}

	_, err := service.acquireQuerySlot(context.Background())
	if err == nil {
		t.Fatal("acquireQuerySlot should reject when the query semaphore is full")
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %T %v, want QueryError", err, err)
	}
	if queryErr.Code != "query_busy" || queryErr.Status != http.StatusTooManyRequests {
		t.Fatalf("query error = %#v", queryErr)
	}
}

func TestEnrichQueryRecordsAddsLabelsAndNormalizesProtocol(t *testing.T) {
	engine := NewIPEngine()
	if err := engine.AddSegment("2.55.80.0/24", "办公网络"); err != nil {
		t.Fatalf("add segment: %v", err)
	}
	records := []map[string]any{
		{
			"src_ip":   "2.55.80.9",
			"dst_ip":   "8.8.8.8",
			"protocol": "6,",
		},
	}

	enrichQueryRecords(records, engine)

	if records[0]["src_ip_label"] != "办公网络" {
		t.Fatalf("source label not enriched: %#v", records[0])
	}
	if records[0]["protocol"] != "TCP" {
		t.Fatalf("protocol not normalized: %#v", records[0])
	}
	if records[0]["dst_geo"] == "" {
		t.Fatalf("destination geo should be filled: %#v", records[0])
	}
}

func TestQueryResponseJSONFields(t *testing.T) {
	resp := QueryResponse{
		Records:     []map[string]any{{"id": 1, "src_ip": "10.0.0.1"}},
		Total:       12,
		Page:        2,
		PageSize:    50,
		NextCursor:  "next-token",
		HasMore:     true,
		QueryTimeMS: 37,
		Visibility: QueryVisibility{
			Partial: true,
			Message: "所选时间包含未完成入库日期，已自动只查询已入库部分。",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal QueryResponse: %v", err)
	}

	text := string(data)
	for _, want := range []string{
		"\"records\"",
		"\"total\"",
		"\"page\"",
		"\"page_size\"",
		"\"next_cursor\"",
		"\"has_more\"",
		"\"query_time_ms\"",
		"\"visibility\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("response json missing %s: %s", want, text)
		}
	}
}

func TestParseQueryRequestFallsBackWhenPageSizeAll(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/query?start=2026-06-10&end=2026-06-10&page_size=all", nil)

	_, _, pageSize, err := parseQueryRequest(r)
	if err != nil {
		t.Fatalf("parseQueryRequest returned error: %v", err)
	}
	if pageSize != 50 {
		t.Fatalf("pageSize = %d, want fallback 50", pageSize)
	}
}

func dateOnly(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func endOfDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 23, 59, 59, 0, time.Local)
}
