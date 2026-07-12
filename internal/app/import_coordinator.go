package app

import (
	"context"
	"fmt"
	"os"
	"sync"
)

const (
	defaultConcurrentSources = 2
	defaultConcurrentWrites  = 1
)

type ImportStartResult struct {
	Accepted []string `json:"accepted_sources"`
	Busy     []string `json:"busy_sources"`
}

type sourceWorkerFunc func(context.Context, LogSource) error

type ImportCoordinator struct {
	mu        sync.Mutex
	running   map[string]struct{}
	sourceSem chan struct{}
	writeSem  chan struct{}
}

func NewImportCoordinator(maxSources, maxWrites int) *ImportCoordinator {
	if maxSources <= 0 {
		maxSources = defaultConcurrentSources
	}
	if maxWrites <= 0 {
		maxWrites = defaultConcurrentWrites
	}
	return &ImportCoordinator{
		running:   make(map[string]struct{}),
		sourceSem: make(chan struct{}, maxSources),
		writeSem:  make(chan struct{}, maxWrites),
	}
}

func (c *ImportCoordinator) Start(ctx context.Context, sources []LogSource, worker sourceWorkerFunc) ImportStartResult {
	result := ImportStartResult{Accepted: make([]string, 0, len(sources)), Busy: make([]string, 0)}
	c.mu.Lock()
	accepted := make([]LogSource, 0, len(sources))
	for _, source := range sources {
		if _, exists := c.running[source.SourceID]; exists {
			result.Busy = append(result.Busy, source.SourceID)
			continue
		}
		c.running[source.SourceID] = struct{}{}
		accepted = append(accepted, source)
		result.Accepted = append(result.Accepted, source.SourceID)
	}
	c.mu.Unlock()
	for _, source := range accepted {
		go c.runSource(ctx, source, worker)
	}
	return result
}

func (c *ImportCoordinator) runSource(ctx context.Context, source LogSource, worker sourceWorkerFunc) {
	defer c.release(source.SourceID)
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(os.Stderr, "import source %s panic: %v\n", source.SourceID, recovered)
		}
	}()
	select {
	case c.sourceSem <- struct{}{}:
		defer func() { <-c.sourceSem }()
	case <-ctx.Done():
		return
	}
	if err := worker(ctx, source); err != nil {
		fmt.Fprintf(os.Stderr, "import source %s failed: %v\n", source.SourceID, err)
	}
}

func (c *ImportCoordinator) WithWriteSlot(ctx context.Context, write func() error) error {
	select {
	case c.writeSem <- struct{}{}:
		defer func() { <-c.writeSem }()
		return write()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *ImportCoordinator) IsRunning(sourceID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exists := c.running[sourceID]
	return exists
}

func (c *ImportCoordinator) release(sourceID string) {
	c.mu.Lock()
	delete(c.running, sourceID)
	c.mu.Unlock()
}
