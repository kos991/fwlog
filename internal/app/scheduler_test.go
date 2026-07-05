package app

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

func TestShouldRetryDateHonorsFailureWindowAndRetryLimit(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.Local)

	tests := []struct {
		name  string
		state DateIngestState
		want  bool
	}{
		{
			name: "failed with retry budget retries immediately when next retry unset",
			state: DateIngestState{
				Status:     StatusFailed,
				RetryCount: 0,
			},
			want: true,
		},
		{
			name: "non failed status does not retry",
			state: DateIngestState{
				Status:     StatusSucceeded,
				RetryCount: 0,
			},
			want: false,
		},
		{
			name: "three failures stop retrying",
			state: DateIngestState{
				Status:     StatusFailed,
				RetryCount: 3,
			},
			want: false,
		},
		{
			name: "waits until next retry time",
			state: DateIngestState{
				Status:      StatusFailed,
				RetryCount:  2,
				NextRetryAt: now.Add(30 * time.Second),
			},
			want: false,
		},
		{
			name: "retries after next retry time passes",
			state: DateIngestState{
				Status:      StatusFailed,
				RetryCount:  2,
				NextRetryAt: now.Add(-30 * time.Second),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		if got := ShouldRetryDate(tt.state, now); got != tt.want {
			t.Fatalf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
}
