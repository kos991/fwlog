package clickhouse

import (
	"fmt"
	"strings"
	"time"
)

func startOfDay(ts time.Time) time.Time {
	year, month, day := ts.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, ts.Location())
}

func dateKey(ts time.Time) string {
	year, month, day := ts.Date()
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

func formatDate(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02")
}

func formatDateTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02 15:04:05")
}

func normalizeProtocol(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "6", "TCP":
		return "TCP"
	case "17", "UDP":
		return "UDP"
	case "1", "ICMP":
		return "ICMP"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}
