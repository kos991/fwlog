package query

import (
	"fmt"
	"strings"
	"time"
)

func formatDateTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02 15:04:05")
}

func parseTimeQuery(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02",
	} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", value)
}

func normalizeProtocol(raw string) string {
	value := strings.ToUpper(strings.Trim(strings.TrimSpace(raw), ",;"))
	switch value {
	case "6", "TCP":
		return "TCP"
	case "17", "UDP":
		return "UDP"
	case "1", "ICMP":
		return "ICMP"
	default:
		return value
	}
}

func NormalizeProtocol(raw string) string {
	return normalizeProtocol(raw)
}
