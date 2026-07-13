package config

import (
	"os"
	"path/filepath"
	"testing"
)

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()

	segments := append([]string{"..", ".."}, parts...)
	path := filepath.Join(segments...)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(content)
}
