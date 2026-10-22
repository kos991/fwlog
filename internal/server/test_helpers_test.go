package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func dateOnly(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func TestMain(m *testing.M) {
	if err := os.Setenv("ADMIN_PASSWORD", "admin"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func repoPath(parts ...string) string {
	segments := append([]string{"..", ".."}, parts...)
	return filepath.Join(segments...)
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()

	content, err := os.ReadFile(repoPath(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(content)
}
