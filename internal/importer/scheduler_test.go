package importer

import (
	"testing"
	"time"
)

func TestNextRetryAtUsesExpectedBackoff(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.Local)

	tests := []struct {
		retry uint8
		want  time.Duration
	}{
		{retry: 0, want: time.Minute},
		{retry: 1, want: 5 * time.Minute},
		{retry: 2, want: 15 * time.Minute},
	}

	for _, tt := range tests {
		got := NextRetryAt(tt.retry, now)
		if got.Sub(now) != tt.want {
			t.Fatalf("retry %d backoff = %s, want %s", tt.retry, got.Sub(now), tt.want)
		}
	}
}
