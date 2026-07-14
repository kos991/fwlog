package receiver

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fwlog/internal/importer"
	"fwlog/internal/model"
)

func TestArchiverCompressesClosedDayInPlaceWithImportableName(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "2026-07-13.log")
	if err := os.WriteFile(sourcePath, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local)
	results := NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: dir,
	}}, nil, now)

	want := filepath.Join(dir, "fw-a_2026-07-13.log-20260714.gz")
	assertGzipContent(t, want, "first\nsecond\n")
	if _, ok := importer.ExtractLogDate(filepath.Base(want)); !ok {
		t.Fatal("archive name is not importable")
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source file still exists: %v", err)
	}
	if len(results) != 1 || results[0].Error != "" || results[0].Path != want {
		t.Fatalf("results = %#v", results)
	}
}

func TestArchiverMovesClosedDayToConfiguredDirectory(t *testing.T) {
	spoolDir := t.TempDir()
	archiveDir := filepath.Join(t.TempDir(), "archives")
	sourcePath := filepath.Join(spoolDir, "2026-07-12.log")
	if err := os.WriteFile(sourcePath, []byte("moved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local)
	results := NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: spoolDir, ArchiveDir: archiveDir,
	}}, nil, now)

	want := filepath.Join(archiveDir, "fw-a_2026-07-12.log-20260714.gz")
	assertGzipContent(t, want, "moved\n")
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source file still exists: %v", err)
	}
	if len(results) != 1 || results[0].Path != want {
		t.Fatalf("results = %#v", results)
	}
}

func TestArchiverLeavesCurrentDayAndUnknownFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	for name := range map[string]bool{
		"2026-07-14.log": true,
		"current.log":    true,
		"2026-99-99.log": true,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	results := NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: dir,
	}}, nil, time.Date(2026, 7, 14, 23, 59, 0, 0, time.Local))
	if len(results) != 0 {
		t.Fatalf("results = %#v", results)
	}
	for _, name := range []string{"2026-07-14.log", "current.log", "2026-99-99.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was changed: %v", name, err)
		}
	}
}

func TestArchiverAcceptsExistingValidArchiveAndRemovesDuplicateSource(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "2026-07-13.log")
	targetPath := filepath.Join(dir, "fw-a_2026-07-13.log-20260714.gz")
	if err := os.WriteFile(sourcePath, []byte("duplicate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestGzip(t, targetPath, "already archived\n")

	results := NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: dir,
	}}, nil, time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local))
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("duplicate source still exists: %v", err)
	}
	assertGzipContent(t, targetPath, "already archived\n")
	if len(results) != 1 || results[0].Error != "" {
		t.Fatalf("results = %#v", results)
	}
}

func TestArchiverKeepsSourceWhenExistingArchiveIsCorrupt(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "2026-07-13.log")
	targetPath := filepath.Join(dir, "fw-a_2026-07-13.log-20260714.gz")
	if err := os.WriteFile(sourcePath, []byte("must survive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}

	results := NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: dir,
	}}, nil, time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local))
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source was removed: %v", err)
	}
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("results = %#v", results)
	}
}

func TestArchiverRetentionZeroNeverDeletes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fw-a_2026-01-01.log-20260102.gz")
	writeTestGzip(t, path, "old\n")

	NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: dir, ArchiveRetentionDays: 0,
	}}, nil, time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("permanent archive was removed: %v", err)
	}
}

func TestArchiverDeletesOnlyReadyExpiredArchive(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "fw-a_2026-06-01.log-20260602.gz")
	notReadyPath := filepath.Join(dir, "fw-a_2026-06-02.log-20260603.gz")
	recentPath := filepath.Join(dir, "fw-a_2026-07-10.log-20260711.gz")
	otherSourcePath := filepath.Join(dir, "fw-b_2026-06-01.log-20260602.gz")
	for path, content := range map[string]string{
		readyPath:       "ready\n",
		notReadyPath:    "not ready\n",
		recentPath:      "recent\n",
		otherSourcePath: "other\n",
	} {
		writeTestGzip(t, path, content)
	}
	ready := map[ArchiveReadyKey]bool{
		{SourceID: "fw-a", Date: "2026-06-01"}: true,
		{SourceID: "fw-a", Date: "2026-07-10"}: true,
	}

	results := NewArchiver().Run([]model.LogSource{{
		SourceID: "fw-a", SourceType: "rsyslog", SpoolDir: dir, ArchiveRetentionDays: 7,
	}}, ready, time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local))
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Fatalf("ready expired archive still exists: %v", err)
	}
	for _, path := range []string{notReadyPath, recentPath, otherSourcePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected archive %s was removed: %v", path, err)
		}
	}
	if len(results) != 1 || !results[0].Deleted || results[0].Path != readyPath {
		t.Fatalf("results = %#v", results)
	}
}

func TestManagerUpdateArchiveResultsPreservesReceiveStatus(t *testing.T) {
	manager := NewManager()
	manager.statuses["fw-a"] = Status{
		SourceID: "fw-a", Running: true, LastClientIP: "192.0.2.1", ReceivedMessages: 9,
	}
	completedAt := time.Date(2026, 7, 14, 1, 2, 3, 0, time.Local)
	manager.UpdateArchiveResults([]ArchiveResult{{
		SourceID: "fw-a", Error: "archive failed", CompletedAt: completedAt,
	}})

	status := manager.Status()["fw-a"]
	if status.ArchiveError != "archive failed" || !status.LastArchiveAt.Equal(completedAt) {
		t.Fatalf("archive status = %#v", status)
	}
	if !status.Running || status.LastClientIP != "192.0.2.1" || status.ReceivedMessages != 9 {
		t.Fatalf("receive status was reset: %#v", status)
	}
}

func assertGzipContent(t *testing.T, path, want string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("gzip content = %q; want %q", content, want)
	}
}

func writeTestGzip(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := io.Copy(writer, strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
