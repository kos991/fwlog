package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestSystemdServiceAllowsRuntimeDataDirectory(t *testing.T) {
	content, err := os.ReadFile("nat-query-service.service")
	if err != nil {
		t.Fatalf("read service file: %v", err)
	}

	match := regexp.MustCompile(`(?m)^ReadWritePaths=(.+)$`).FindStringSubmatch(string(content))
	if match == nil {
		t.Fatal("ReadWritePaths is required when ProtectSystem is enabled")
	}

	paths := strings.Fields(match[1])
	if !contains(paths, "/data") {
		t.Fatalf("ReadWritePaths must include /data so the service can create /data/index and /data/export; got %q", match[1])
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
