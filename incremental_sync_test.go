package main

import "testing"

func TestIncrementalSyncDecisionSkipsEmptyIndexedLogDirectory(t *testing.T) {
	decision := decideIncrementalSyncAction(true, true, 0, 0)
	if decision != incrementalSyncNoop {
		t.Fatalf("empty indexed log directory should not trigger rebuild, got %q", decision)
	}
}

func TestIncrementalSyncDecisionRebuildsMissingMetadataWithFiles(t *testing.T) {
	decision := decideIncrementalSyncAction(true, false, 0, 2)
	if decision != incrementalSyncFullRebuild {
		t.Fatalf("missing metadata with log files should trigger rebuild, got %q", decision)
	}
}
