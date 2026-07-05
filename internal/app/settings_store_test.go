package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestSettingsQueriesUseFinalOnlyForSmallTables(t *testing.T) {
	if !strings.Contains(EnabledLogSourcesSQL(), "log_sources FINAL") {
		t.Fatalf("log_sources query should use FINAL: %s", EnabledLogSourcesSQL())
	}
	if !strings.Contains(AppSettingsSQL(), "app_settings FINAL") {
		t.Fatalf("app_settings query should use FINAL: %s", AppSettingsSQL())
	}
}

func TestEnabledLogSourcesSQLCoversLogSourceFields(t *testing.T) {
	sql := EnabledLogSourcesSQL()
	fields := []string{
		reflect.TypeOf(LogSource{}).Field(0).Tag.Get("json"),
		reflect.TypeOf(LogSource{}).Field(1).Tag.Get("json"),
		reflect.TypeOf(LogSource{}).Field(2).Tag.Get("json"),
		reflect.TypeOf(LogSource{}).Field(3).Tag.Get("json"),
		reflect.TypeOf(LogSource{}).Field(4).Tag.Get("json"),
	}
	for _, field := range fields {
		if !strings.Contains(sql, field) {
			t.Fatalf("enabled log sources query missing field %q: %s", field, sql)
		}
	}
}

func TestSettingsUpsertSQLIncludesAuditColumns(t *testing.T) {
	sql := SettingsUpsertSQL()
	for _, want := range []string{
		"INSERT INTO app_settings",
		"key, value, updated_at",
		"now()",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("settings upsert missing %q: %s", want, sql)
		}
	}
}
