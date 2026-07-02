package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestDateStatePointQueryOrdersByUpdatedAt(t *testing.T) {
	sql := DateStatePointQuery()
	if !strings.Contains(sql, "ORDER BY updated_at DESC") || !strings.Contains(sql, "LIMIT 1") {
		t.Fatalf("point query must read newest state: %s", sql)
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
}

func TestDateStateListQueryUsesArgMax(t *testing.T) {
	sql := DateStateListQuery()
	for _, want := range []string{
		"argMax(status, updated_at)",
		"argMax(retry_count, updated_at)",
		"argMax(next_retry_at, updated_at)",
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
