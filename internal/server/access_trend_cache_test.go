package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccessTrendCacheReusesPreprocessedSnapshot(t *testing.T) {
	cache := newAccessTrendCache(5 * time.Minute)
	var calls atomic.Int32
	loader := func(context.Context) (AccessTrendSnapshot, error) {
		calls.Add(1)
		return AccessTrendSnapshot{Summary: AccessTrendSummary{Total: 42}}, nil
	}

	first, _, err := cache.Get(context.Background(), "7d|all", loader)
	if err != nil || first.Summary.Total != 42 {
		t.Fatalf("first result = %#v, err = %v", first, err)
	}
	second, _, err := cache.Get(context.Background(), "7d|all", loader)
	if err != nil || second.Summary.Total != 42 || calls.Load() != 1 {
		t.Fatalf("second result = %#v, calls = %d, err = %v", second, calls.Load(), err)
	}
}

func TestAccessTrendCacheDoesNotHideRefreshFailure(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.Local)
	cache := newAccessTrendCache(time.Minute)
	cache.now = func() time.Time { return now }
	_, _, err := cache.Get(context.Background(), "7d|all", func(context.Context) (AccessTrendSnapshot, error) {
		return AccessTrendSnapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	want := errors.New("clickhouse unavailable")
	_, _, err = cache.Get(context.Background(), "7d|all", func(context.Context) (AccessTrendSnapshot, error) {
		return AccessTrendSnapshot{}, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestAccessTrendCacheClearForcesReload(t *testing.T) {
	cache := newAccessTrendCache(time.Hour)
	var calls atomic.Int32
	loader := func(context.Context) (AccessTrendSnapshot, error) {
		return AccessTrendSnapshot{Summary: AccessTrendSummary{Total: uint64(calls.Add(1))}}, nil
	}
	first, _, _ := cache.Get(context.Background(), "7d|all", loader)
	cache.Clear()
	second, _, _ := cache.Get(context.Background(), "7d|all", loader)
	if first.Summary.Total != 1 || second.Summary.Total != 2 {
		t.Fatalf("totals before/after clear = %d/%d", first.Summary.Total, second.Summary.Total)
	}
}

func TestAccessTrendCacheKeyUsesWindowAndSourceOnly(t *testing.T) {
	from := time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local)
	until := time.Date(2026, 9, 12, 0, 0, 0, 0, time.Local)
	if got := accessTrendCacheKey(from, until, "fw-a"); got != "2026-09-05|2026-09-12|fw-a" {
		t.Fatalf("cache key = %q", got)
	}
}
