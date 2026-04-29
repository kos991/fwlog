package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestProcessLogFileReadsOnlySnapshotSize(t *testing.T) {
	originalRegex := natRegex
	natRegex = regexp.MustCompile(`src=([0-9.]+)\s+sport=(\d+)\s+dst=([0-9.]+)\s+dport=(\d+)\s+proto=(\d+)\s+nat=([0-9.]+)\s+nport=(\d+)`)
	defer func() {
		natRegex = originalRegex
	}()

	dir := t.TempDir()
	path := dir + `\active.log`

	firstLine := "Apr 27 00:00:29 src=2.55.81.95 sport=44178 dst=17.253.114.43 dport=123 proto=17 nat=58.216.48.6 nport=44178\n"
	secondLine := "Apr 27 00:00:30 src=2.55.80.84 sport=36183 dst=175.31.209.15 dport=21882 proto=17 nat=58.216.48.6 nport=36183\n"

	if err := os.WriteFile(path, []byte(firstLine), 0644); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat initial log: %v", err)
	}
	snapshotSize := info.Size()

	if err := os.WriteFile(path, []byte(firstLine+secondLine), 0644); err != nil {
		t.Fatalf("append extra line: %v", err)
	}

	var out strings.Builder
	totalLines := 0
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	defer file.Close()

	if err := processLogReaderWithOffsets(path, file, 0, snapshotSize, &out, &totalLines); err != nil {
		t.Fatalf("process log file: %v", err)
	}

	if totalLines != 1 {
		t.Fatalf("expected 1 parsed line from snapshot, got %d", totalLines)
	}
	if int64(len(firstLine)) != snapshotSize {
		t.Fatalf("expected snapshot size %d to match first line size %d", snapshotSize, len(firstLine))
	}
	if strings.Contains(out.String(), "2.55.80.84") {
		t.Fatalf("unexpected appended line in output: %q", out.String())
	}
	if !strings.Contains(out.String(), "2.55.81.95") {
		t.Fatalf("expected first line in output: %q", out.String())
	}

	fields := strings.Split(strings.TrimSpace(out.String()), "|")
	if len(fields) != 11 {
		t.Fatalf("expected 11 output fields including source metadata, got %d: %q", len(fields), out.String())
	}
	if fields[9] != path {
		t.Fatalf("expected source file %q, got %q", path, fields[9])
	}
	if fields[10] != "0" {
		t.Fatalf("expected source offset 0, got %q", fields[10])
	}
}

func TestDiscoverCatchUpRangesIncludesAppendedAndNewFiles(t *testing.T) {
	initial := []LogFileSnapshot{
		{Path: "a.log", Size: 100},
		{Path: "b.log", Size: 200},
	}
	current := []LogFileSnapshot{
		{Path: "a.log", Size: 150},
		{Path: "b.log", Size: 200},
		{Path: "c.log", Size: 80},
	}

	ranges := discoverCatchUpRanges(initial, current)
	if len(ranges) != 2 {
		t.Fatalf("expected 2 catch-up ranges, got %d", len(ranges))
	}

	if ranges[0].Path != "a.log" || ranges[0].Start != 100 || ranges[0].End != 150 {
		t.Fatalf("unexpected appended file range: %+v", ranges[0])
	}
	if ranges[1].Path != "c.log" || ranges[1].Start != 0 || ranges[1].End != 80 {
		t.Fatalf("unexpected new file range: %+v", ranges[1])
	}
}

func TestRequiresFullRebuildDetectsShrinkAndRemoval(t *testing.T) {
	stored := []LogFileSnapshot{
		{Path: "a.log", Size: 100},
		{Path: "b.log", Size: 200},
	}

	if !requiresFullRebuild(stored, []LogFileSnapshot{{Path: "a.log", Size: 90}, {Path: "b.log", Size: 200}}) {
		t.Fatalf("expected shrink to require full rebuild")
	}

	if !requiresFullRebuild(stored, []LogFileSnapshot{{Path: "a.log", Size: 100}}) {
		t.Fatalf("expected missing file to require full rebuild")
	}

	if requiresFullRebuild(stored, []LogFileSnapshot{{Path: "a.log", Size: 120}, {Path: "b.log", Size: 200}}) {
		t.Fatalf("did not expect append-only changes to require full rebuild")
	}
}
