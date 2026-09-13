package clickhouse

import (
	"strings"
	"testing"
	"time"
)

func TestAccessTrendSQLReadsTargetSubnetPreaggregation(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	until := time.Date(2026, 9, 8, 0, 0, 0, 0, time.Local)
	sql, args := accessTrendSQL(from, until, "fw-a")
	for _, want := range []string{
		"FROM dashboard_daily_ip_counts",
		"dimension = 'dst_subnet'",
		"log_date >= ?",
		"log_date < ?",
		"source_id = ?",
		"toString(address) AS target_ip",
		"sum(rows) AS rows",
		"GROUP BY address",
		"ORDER BY rows DESC",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("access trend SQL missing %q: %s", want, sql)
		}
	}
	for _, forbidden := range []string{"FROM access_trend_daily_flows", "FROM nat_logs", "LIMIT"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("access trend SQL contains %q: %s", forbidden, sql)
		}
	}
	if len(args) != 3 || args[2] != "fw-a" {
		t.Fatalf("args = %#v", args)
	}
}

func TestAccessTrendSQLDefaultsToAllSources(t *testing.T) {
	sql, args := accessTrendSQL(time.Time{}, time.Time{}, "")
	if strings.Contains(sql, "source_id = ?") || len(args) != 2 {
		t.Fatalf("all-source SQL should not filter source_id: %s %#v", sql, args)
	}
}
