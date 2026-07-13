package server

import "time"

const (
	defaultConcurrentWrites = 1
)

func formatDateTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02 15:04:05")
}

func formatDate(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02")
}

func startOfDay(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}
	return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())
}

func dateKey(ts time.Time) string {
	return startOfDay(ts).Format("2006-01-02")
}

func dateOnly(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}
