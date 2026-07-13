package importer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsArchivedLogFileSkipsOnlineLog(t *testing.T) {
	cases := map[string]bool{
		"sangfor.log":              false,
		"sangfor.log-20260701":     true,
		"sangfor.log-20260701.gz":  true,
		"sangfor.log-20260701.tmp": false,
		"random.txt":               false,
	}

	for name, want := range cases {
		if got := IsArchivedLogFile(name); got != want {
			t.Fatalf("IsArchivedLogFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestExtractLogDateFromArchiveName(t *testing.T) {
	got, ok := ExtractLogDate("sangfor.log-20260701.gz")
	if !ok {
		t.Fatal("ExtractLogDate should parse archive date")
	}

	want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ExtractLogDate = %v, want %v", got, want)
	}
}

func TestExtractLogDatePrefersEventDateBeforeArchiveSuffix(t *testing.T) {
	got, ok := ExtractLogDate("10.10.10.1_2026-06-13.log-20260614.gz")
	if !ok {
		t.Fatal("ExtractLogDate should parse event date from device archive name")
	}

	want := time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ExtractLogDate = %v, want %v", got, want)
	}
}

func TestExtractLogDateFallsBackToArchiveSuffix(t *testing.T) {
	got, ok := ExtractLogDate("sangfor.log-20260701.gz")
	if !ok {
		t.Fatal("ExtractLogDate should parse archive suffix")
	}

	want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ExtractLogDate = %v, want %v", got, want)
	}
}

func TestExtractLogDateRejectsInvalidNamesAndDates(t *testing.T) {
	cases := []string{
		"sangfor.log",
		"sangfor.log-2026070",
		"sangfor.log-20261301",
		"sangfor.log-20260230.gz",
		"sangfor.log-20260701.tmp",
	}

	for _, name := range cases {
		if _, ok := ExtractLogDate(name); ok {
			t.Fatalf("ExtractLogDate(%q) should fail", name)
		}
	}
}

func TestScanArchivedLogFilesReturnsOnlyStableArchives(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)
	old := now.Add(-10 * time.Minute)
	boundary := now.Add(-5 * time.Minute)
	recent := now.Add(-4 * time.Minute)

	writeFileWithMTime(t, dir, "sangfor.log", old)
	writeFileWithMTime(t, dir, "sangfor.log-20260701", old)
	writeFileWithMTime(t, dir, "sangfor.log-20260702.gz", boundary)
	writeFileWithMTime(t, dir, "sangfor.log-20260703.gz", recent)
	writeFileWithMTime(t, dir, "sangfor.log-20261301.gz", old)

	files, err := ScanArchivedLogFiles(dir, now)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 archive files, got %#v", files)
	}

	for _, file := range files {
		if file.Path == filepath.Join(dir, "sangfor.log") {
			t.Fatalf("online log should not be scanned: %#v", files)
		}
		if file.Name == "sangfor.log-20260703.gz" {
			t.Fatalf("recent archive should be skipped: %#v", files)
		}
		if file.Name == "sangfor.log-20261301.gz" {
			t.Fatalf("invalid date archive should be skipped: %#v", files)
		}
		if now.Sub(file.ModTime) < 5*time.Minute {
			t.Fatalf("unstable archive should be skipped: %#v", file)
		}
	}
}

func writeFileWithMTime(t *testing.T, dir, name string, modTime time.Time) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}
