package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fwlog/internal/model"
	"fwlog/internal/receiver"
)

func TestArchiveReadyMapIncludesOnlyReadySourceDates(t *testing.T) {
	states := []DateIngestState{
		{SourceID: "a", LogDate: dateOnly(2026, 7, 1), Status: StatusReady},
		{SourceID: "a", LogDate: dateOnly(2026, 7, 2), Status: StatusFailed},
		{SourceID: "b", LogDate: dateOnly(2026, 7, 1), Status: StatusImporting},
	}

	ready := archiveReadyMap(states)
	key := receiver.ArchiveReadyKey{SourceID: "a", Date: "2026-07-01"}
	if !ready[key] || len(ready) != 1 {
		t.Fatalf("ready = %#v", ready)
	}
}

func TestArchiveStatusResultsIncludesSuccessfulIdleSources(t *testing.T) {
	now := time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local)
	results := archiveStatusResults(
		[]LogSource{{SourceID: "a"}, {SourceID: "b"}},
		[]receiver.ArchiveResult{{SourceID: "a", Error: "failed", CompletedAt: now}},
		now,
	)

	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].SourceID != "a" || results[0].Error != "failed" {
		t.Fatalf("error result changed: %#v", results[0])
	}
	if results[1].SourceID != "b" || results[1].Error != "" || !results[1].CompletedAt.Equal(now) {
		t.Fatalf("idle source result = %#v", results[1])
	}
}

func TestRunRSyslogArchiveLoadsRequiredRetentionWindow(t *testing.T) {
	app := NewApp(LoadConfig())
	configureArchiveSource(t, app, model.LogSource{
		SourceID:             "fw-a",
		SourceType:           "rsyslog",
		SpoolDir:             t.TempDir(),
		ArchiveRetentionDays: 7,
		Enabled:              false,
	})

	var gotSince time.Time
	app.dateStatesLoader = func(_ context.Context, since time.Time) ([]DateIngestState, error) {
		gotSince = since
		return []DateIngestState{{
			SourceID: "fw-a", LogDate: dateOnly(2026, 7, 1), Status: StatusReady,
		}}, nil
	}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.Local)
	app.runRSyslogArchive(context.Background(), now)

	wantSince := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	if !gotSince.Equal(wantSince) {
		t.Fatalf("ListDateStates since = %v; want %v", gotSince, wantSince)
	}
}

func TestRunRSyslogArchiveSkipsStateQueryForPermanentRetention(t *testing.T) {
	app := NewApp(LoadConfig())
	spoolDir := t.TempDir()
	configureArchiveSource(t, app, model.LogSource{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: spoolDir, ArchiveRetentionDays: 0, Enabled: true,
	})

	called := false
	app.dateStatesLoader = func(context.Context, time.Time) ([]DateIngestState, error) {
		called = true
		return nil, nil
	}
	app.runRSyslogArchive(context.Background(), time.Date(2026, 7, 14, 12, 0, 0, 0, time.Local))
	if called {
		t.Fatal("permanent retention should not query ingest date states")
	}
}

func TestRSyslogArchiveSchedulerRunsImmediately(t *testing.T) {
	app := NewApp(LoadConfig())
	spoolDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(spoolDir, "2026-07-13.log"), []byte("startup scan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configureArchiveSource(t, app, model.LogSource{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: spoolDir, ArchiveRetentionDays: 0, Enabled: false,
	})

	app.rsyslogArchiveNow = func() time.Time {
		return time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.startRSyslogArchiveScheduler(ctx)

	want := filepath.Join(spoolDir, "fw-a_2026-07-13.log-20260714.gz")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(want); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("startup archive was not created at %s", want)
}

func configureArchiveSource(t *testing.T, app *App, source model.LogSource) {
	t.Helper()
	encoded, err := json.Marshal([]model.LogSource{source})
	if err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.settings["log_sources"] = string(encoded)
	app.mu.Unlock()
}
