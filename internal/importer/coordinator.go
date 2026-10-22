package importer

import (
	"context"
	"fmt"
	"os"
	"sync"
)

const (
	defaultConcurrentSources = 1
	defaultConcurrentWrites  = 1
)

type ImportStartResult struct {
	Accepted  []string        `json:"accepted_sources"`
	Busy      []string        `json:"busy_sources"`
	Done      <-chan struct{} `json:"-"`
	Succeeded <-chan bool     `json:"-"`
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
	done := make(chan struct{})
	succeeded := make(chan bool, 1)
	result := ImportStartResult{Accepted: make([]string, 0, len(sources)), Busy: make([]string, 0), Done: done, Succeeded: succeeded}
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
	workerResults := make(chan bool, len(accepted))
	for _, source := range accepted {
		go func(source LogSource) {
			workerResults <- c.runSource(ctx, source, worker)
		}(source)
	}
	go func() {
		allSucceeded := true
		for range accepted {
			allSucceeded = <-workerResults && allSucceeded
		}
		succeeded <- allSucceeded
		close(succeeded)
		close(done)
	}()
	return result
}

func (c *ImportCoordinator) runSource(ctx context.Context, source LogSource, worker sourceWorkerFunc) (succeeded bool) {
	defer c.release(source.SourceID)
	defer func() {
		if recovered := recover(); recovered != nil {
			succeeded = false
			fmt.Fprintf(os.Stderr, "import source %s panic: %v\n", source.SourceID, recovered)
		}
	}()
	select {
	case c.sourceSem <- struct{}{}:
		defer func() { <-c.sourceSem }()
	case <-ctx.Done():
		return false
	}
	if err := worker(ctx, source); err != nil {
		fmt.Fprintf(os.Stderr, "import source %s failed: %v\n", source.SourceID, err)
		return false
	}
	return true
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

func (c *ImportCoordinator) release(sourceID string) {
	c.mu.Lock()
	delete(c.running, sourceID)
	c.mu.Unlock()
}
