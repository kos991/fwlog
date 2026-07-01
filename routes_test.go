package main

import (
	"os"
	"strings"
	"testing"
)

func TestSettingsPostRouteIsRegistered(t *testing.T) {
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	if !strings.Contains(string(content), `r.POST("/api/settings", handleSetLogDir)`) {
		t.Fatal("POST /api/settings must be registered for the settings form")
	}
}
