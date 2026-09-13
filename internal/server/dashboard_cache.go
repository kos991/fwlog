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

type responseCacheEntry[T any] struct {
	value       T
	loadedAt    time.Time
	loading     chan struct{}
	lastAttempt time.Time
	lastErr     error
}

type responseCache[T any] struct {
	mu           sync.Mutex
	ttl          time.Duration
	now          func() time.Time
	staleOnError bool
	entries      map[string]*responseCacheEntry[T]
}

type rankingCache struct {
	*responseCache[DashboardMetrics]
}

type accessTrendCache struct {
	*responseCache[AccessTrendSnapshot]
}

func newRankingCache(ttl time.Duration) *rankingCache {
	return &rankingCache{responseCache: newResponseCache[DashboardMetrics](ttl, true)}
}

func newAccessTrendCache(ttl time.Duration) *accessTrendCache {
	return &accessTrendCache{responseCache: newResponseCache[AccessTrendSnapshot](ttl, false)}
}

func newResponseCache[T any](ttl time.Duration, staleOnError bool) *responseCache[T] {
	if ttl <= 0 {
		ttl = dashboardRankingCacheTTL
	}
	return &responseCache[T]{
		ttl:          ttl,
		now:          time.Now,
		staleOnError: staleOnError,
		entries:      make(map[string]*responseCacheEntry[T]),
	}
}

func (c *responseCache[T]) Get(ctx context.Context, key string, loader func(context.Context) (T, error)) (T, rankingCacheState, error) {
	var zero T
	if c == nil {
		value, err := loader(ctx)
		return value, rankingCacheState{}, err
	}
	for {
		now := c.nowOrDefault()()
		c.mu.Lock()
		entry := c.entries[key]
		if entry == nil {
			entry = &responseCacheEntry[T]{}
			c.entries[key] = entry
		}
		if entry.loading != nil {
			wait := entry.loading
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return zero, rankingCacheState{}, ctx.Err()
			}
		}
		if !entry.loadedAt.IsZero() && now.Sub(entry.loadedAt) < c.ttl {
			value, state := entry.value, rankingCacheState{LoadedAt: entry.loadedAt}
			c.mu.Unlock()
			return value, state, nil
		}
		if c.staleOnError && entry.lastErr != nil && !entry.loadedAt.IsZero() && now.Sub(entry.lastAttempt) < time.Second {
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
		if c.staleOnError && hasValue {
			return staleValue, rankingCacheState{Stale: true, LoadedAt: loadedAt}, nil
		}
		return zero, rankingCacheState{}, err
	}
}

func (c *responseCache[T]) nowOrDefault() func() time.Time {
	if c != nil && c.now != nil {
		return c.now
	}
	return time.Now
}

func (c *responseCache[T]) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]*responseCacheEntry[T])
	c.mu.Unlock()
}
