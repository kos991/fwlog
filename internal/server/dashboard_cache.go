package server

import (
	"context"
	"sync"
	"time"
)

const dashboardRankingCacheTTL = 5 * time.Minute

type rankingCacheState struct {
	Stale    bool
	LoadedAt time.Time
}

type rankingCacheEntry struct {
	value       DashboardMetrics
	loadedAt    time.Time
	loading     chan struct{}
	lastAttempt time.Time
	lastErr     error
}

type rankingCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]*rankingCacheEntry
}

func newRankingCache(ttl time.Duration) *rankingCache {
	if ttl <= 0 {
		ttl = dashboardRankingCacheTTL
	}
	return &rankingCache{ttl: ttl, now: time.Now, entries: make(map[string]*rankingCacheEntry)}
}

func (c *rankingCache) Get(ctx context.Context, key string, loader func(context.Context) (DashboardMetrics, error)) (DashboardMetrics, rankingCacheState, error) {
	if c == nil {
		value, err := loader(ctx)
		return value, rankingCacheState{}, err
	}
	for {
		now := c.nowOrDefault()()
		c.mu.Lock()
		entry := c.entries[key]
		if entry == nil {
			entry = &rankingCacheEntry{}
			c.entries[key] = entry
		}
		if entry.loading != nil {
			wait := entry.loading
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return DashboardMetrics{}, rankingCacheState{}, ctx.Err()
			}
		}
		if !entry.loadedAt.IsZero() && now.Sub(entry.loadedAt) < c.ttl {
			value, state := entry.value, rankingCacheState{LoadedAt: entry.loadedAt}
			c.mu.Unlock()
			return value, state, nil
		}
		if entry.lastErr != nil && !entry.loadedAt.IsZero() && now.Sub(entry.lastAttempt) < time.Second {
			value, state := entry.value, rankingCacheState{Stale: true, LoadedAt: entry.loadedAt}
			c.mu.Unlock()
			return value, state, nil
		}
		wait := make(chan struct{})
		entry.loading = wait
		c.mu.Unlock()

		value, err := loader(ctx)
		c.mu.Lock()
		entry.lastAttempt = c.nowOrDefault()()
		entry.lastErr = err
		if err == nil {
			entry.value = value
			entry.loadedAt = entry.lastAttempt
		}
		entry.loading = nil
		close(wait)
		hasValue := !entry.loadedAt.IsZero()
		loadedAt := entry.loadedAt
		staleValue := entry.value
		c.mu.Unlock()
		if err == nil {
			return value, rankingCacheState{LoadedAt: loadedAt}, nil
		}
		if hasValue {
			return staleValue, rankingCacheState{Stale: true, LoadedAt: loadedAt}, nil
		}
		return DashboardMetrics{}, rankingCacheState{}, err
	}
}

func (c *rankingCache) nowOrDefault() func() time.Time {
	if c != nil && c.now != nil {
		return c.now
	}
	return time.Now
}
