package app

import "time"

func NextRetryAt(retryCount uint8, now time.Time) time.Time {
	switch retryCount {
	case 0:
		return now.Add(time.Minute)
	case 1:
		return now.Add(5 * time.Minute)
	default:
		return now.Add(15 * time.Minute)
	}
}

func ShouldRetryDate(state DateIngestState, now time.Time) bool {
	if state.Status != StatusFailed {
		return false
	}
	if state.RetryCount >= 3 {
		return false
	}
	if state.NextRetryAt.IsZero() {
		return true
	}
	return !state.NextRetryAt.After(now)
}
