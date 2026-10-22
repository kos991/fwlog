package server

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunImportDatesContinuesAfterOneDateFails(t *testing.T) {
	dates := []time.Time{
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local),
		time.Date(2026, 7, 3, 0, 0, 0, 0, time.Local),
	}
	visited := make([]string, 0, len(dates))

	imported, skipped, err := runImportDates(dates, 1, func(date time.Time) ([]string, []string, error) {
		key := formatDate(date)
		visited = append(visited, key)
		if key == "2026-07-02" {
			return nil, nil, errors.New("broken archive")
		}
		return []string{key}, nil, nil
	})

	if got := strings.Join(visited, ","); got != "2026-07-01,2026-07-02,2026-07-03" {
		t.Fatalf("visited dates = %s", got)
	}
	if got := strings.Join(imported, ","); got != "2026-07-01,2026-07-03" {
		t.Fatalf("imported dates = %s", got)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped dates = %#v", skipped)
	}
	if err == nil || !strings.Contains(err.Error(), "2026-07-02") || !strings.Contains(err.Error(), "broken archive") {
		t.Fatalf("combined error = %v", err)
	}
}

func TestRunImportDatesBoundsParallelismAndKeepsDateOrder(t *testing.T) {
	dates := make([]time.Time, 8)
	for index := range dates {
		dates[index] = time.Date(2026, 7, index+1, 0, 0, 0, 0, time.Local)
	}
	const workers = 3
	var running, peak atomic.Int32
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(workers)

	type outcome struct {
		imported, skipped []string
		err               error
	}
	finished := make(chan outcome, 1)
	go func() {
		imported, skipped, err := runImportDates(dates, workers, func(date time.Time) ([]string, []string, error) {
			current := running.Add(1)
			for {
				previous := peak.Load()
				if current <= previous || peak.CompareAndSwap(previous, current) {
					break
				}
			}
			if date.Day() <= workers {
				started.Done()
				<-release
			}
			running.Add(-1)
			key := formatDate(date)
			switch date.Day() {
			case 2:
				return nil, []string{key}, nil
			case 5:
				return nil, nil, errors.New("broken archive")
			}
			return []string{key}, nil, nil
		})
		finished <- outcome{imported, skipped, err}
	}()

	started.Wait()
	close(release)
	result := <-finished

	if got := peak.Load(); got != workers {
		t.Fatalf("peak parallel dates = %d, want %d", got, workers)
	}
	if got := strings.Join(result.imported, ","); got != "2026-07-01,2026-07-03,2026-07-04,2026-07-06,2026-07-07,2026-07-08" {
		t.Fatalf("imported dates = %s", got)
	}
	if got := strings.Join(result.skipped, ","); got != "2026-07-02" {
		t.Fatalf("skipped dates = %s", got)
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "2026-07-05: broken archive") {
		t.Fatalf("combined error = %v", result.err)
	}
}
