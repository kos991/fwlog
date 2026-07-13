package importer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestImportCoordinatorRejectsBusySource(t *testing.T) {
	c := NewImportCoordinator(2, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	first := c.Start(context.Background(), []LogSource{{SourceID: "fw-a"}}, func(context.Context, LogSource) error {
		close(started)
		<-release
		return nil
	})
	if len(first.Accepted) != 1 {
		t.Fatalf("first result = %#v", first)
	}
	<-started
	second := c.Start(context.Background(), []LogSource{{SourceID: "fw-a"}}, func(context.Context, LogSource) error { return nil })
	if len(second.Accepted) != 0 || len(second.Busy) != 1 || second.Busy[0] != "fw-a" {
		t.Fatalf("second result = %#v", second)
	}
	close(release)
	waitForSourceIdle(t, c, "fw-a")
}

func TestImportCoordinatorLimitsActiveSources(t *testing.T) {
	c := NewImportCoordinator(2, 1)
	release := make(chan struct{})
	started := make(chan struct{}, 3)
	var active atomic.Int32
	var maximum atomic.Int32
	c.Start(context.Background(), []LogSource{{SourceID: "a"}, {SourceID: "b"}, {SourceID: "c"}}, func(context.Context, LogSource) error {
		current := active.Add(1)
		for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return nil
	})
	<-started
	<-started
	select {
	case <-started:
		t.Fatal("third source started before a worker slot was released")
	default:
	}
	close(release)
	if maximum.Load() != 2 {
		t.Fatalf("maximum active sources = %d, want 2", maximum.Load())
	}
}

func TestImportCoordinatorLimitsWritesAndRecoversPanic(t *testing.T) {
	c := NewImportCoordinator(2, 1)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	write := func() error {
		current := active.Add(1)
		if current > maximum.Load() {
			maximum.Store(current)
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return nil
	}
	go func() { _ = c.WithWriteSlot(context.Background(), write) }()
	go func() { _ = c.WithWriteSlot(context.Background(), write) }()
	<-entered
	select {
	case <-entered:
		t.Fatal("second write entered concurrently")
	default:
	}
	close(release)
	if maximum.Load() != 1 {
		t.Fatalf("maximum writes = %d, want 1", maximum.Load())
	}

	c.Start(context.Background(), []LogSource{{SourceID: "panic-source"}}, func(context.Context, LogSource) error { panic("boom") })
	waitForSourceIdle(t, c, "panic-source")
	result := c.Start(context.Background(), []LogSource{{SourceID: "panic-source"}}, func(context.Context, LogSource) error { return nil })
	if len(result.Accepted) != 1 {
		t.Fatalf("source was not released after panic: %#v", result)
	}
}

func waitForSourceIdle(t *testing.T, c *ImportCoordinator, sourceID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for c.IsRunning(sourceID) {
		if time.Now().After(deadline) {
			t.Fatalf("source %s remained busy", sourceID)
		}
		time.Sleep(time.Millisecond)
	}
}
