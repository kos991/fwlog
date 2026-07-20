package server

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunImportDatesContinuesAfterOneDateFails(t *testing.T) {
	dates := []time.Time{
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local),
		time.Date(2026, 7, 3, 0, 0, 0, 0, time.Local),
	}
	visited := make([]string, 0, len(dates))

	imported, skipped, err := runImportDates(dates, func(date time.Time) ([]string, []string, error) {
		key := formatDate(date)
		visited = append(visited, key)
		if key == "2026-07-02" {
			return nil, nil, errors.New("broken archive")
		}
		return []string{key}, nil, nil
	})

	if got := strings.Join(visited, ","); got != "2026-07-01,2026-07-02,2026-07-03" {
		t.Fatalf("visited dates = %s", got)
	}
	if got := strings.Join(imported, ","); got != "2026-07-01,2026-07-03" {
		t.Fatalf("imported dates = %s", got)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped dates = %#v", skipped)
	}
	if err == nil || !strings.Contains(err.Error(), "2026-07-02") || !strings.Contains(err.Error(), "broken archive") {
		t.Fatalf("combined error = %v", err)
	}
}
