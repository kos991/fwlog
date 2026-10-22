package clickhouse

import (
	"strings"
	"testing"
)

func TestSettingsQueriesUseFinalOnlyForSmallTables(t *testing.T) {
	if !strings.Contains(AppSettingsSQL(), "app_settings FINAL") {
		t.Fatalf("app_settings query should use FINAL: %s", AppSettingsSQL())
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
