package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRankingCacheReusesFreshValue(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local)
	cache := newRankingCache(5 * time.Minute)
	cache.now = func() time.Time { return now }
	var calls atomic.Int32
	loader := func(context.Context) (DashboardMetrics, error) {
		calls.Add(1)
		return DashboardMetrics{TodayRows: 42}, nil
	}

	first, firstState, err := cache.Get(context.Background(), "30d|all", loader)
	if err != nil || first.TodayRows != 42 || firstState.Stale {
		t.Fatalf("first result = %#v, state = %#v, err = %v", first, firstState, err)
	}
	second, secondState, err := cache.Get(context.Background(), "30d|all", loader)
	if err != nil || second.TodayRows != 42 || secondState.Stale {
		t.Fatalf("second result = %#v, state = %#v, err = %v", second, secondState, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
}

func TestRankingCacheMergesConcurrentLoads(t *testing.T) {
	cache := newRankingCache(5 * time.Minute)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	loader := func(context.Context) (DashboardMetrics, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return DashboardMetrics{YesterdayRows: 9}, nil
	}

	var wait sync.WaitGroup
	wait.Add(2)
	errs := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			defer wait.Done()
			value, _, err := cache.Get(context.Background(), "30d|fw-a", loader)
			if err == nil && value.YesterdayRows != 9 {
				err = errors.New("unexpected cached value")
			}
			errs <- err
		}()
	}
	<-started
	close(release)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
}

func TestRankingCacheReturnsStaleValueWhenRefreshFails(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local)
	cache := newRankingCache(5 * time.Minute)
	cache.now = func() time.Time { return now }
	_, _, err := cache.Get(context.Background(), "30d|all", func(context.Context) (DashboardMetrics, error) {
		return DashboardMetrics{TodayRows: 17}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	value, state, err := cache.Get(context.Background(), "30d|all", func(context.Context) (DashboardMetrics, error) {
		return DashboardMetrics{}, errors.New("clickhouse unavailable")
	})
	if err != nil {
		t.Fatalf("stale fallback returned error: %v", err)
	}
	if value.TodayRows != 17 || !state.Stale || state.LoadedAt.IsZero() {
		t.Fatalf("value = %#v, state = %#v", value, state)
	}
}

func TestRankingCacheReturnsInitialLoadError(t *testing.T) {
	cache := newRankingCache(5 * time.Minute)
	want := errors.New("clickhouse unavailable")
	_, _, err := cache.Get(context.Background(), "30d|all", func(context.Context) (DashboardMetrics, error) {
		return DashboardMetrics{}, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
