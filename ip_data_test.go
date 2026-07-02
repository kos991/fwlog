package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReloadIPEngineKeepsOldEngineWhenPathFails(t *testing.T) {
	old := NewIPEngine()
	next, status := ReloadIPEngine(Config{
		IPMapEnabled:    true,
		CustomIPMapPath: "Z:/missing/custom.csv",
		GeoIPEnabled:    false,
	}, old)

	if next != old {
		t.Fatal("failed reload should keep old engine")
	}
	if status.Error == "" {
		t.Fatal("failed reload should return error status")
	}
	if status.Loaded {
		t.Fatalf("failed reload should not be marked loaded: %#v", status)
	}
}

func TestReloadIPEngineLoadsCustomMapWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "custom.csv")
	if err := os.WriteFile(mapPath, []byte("10.0.0.1,办公终端,内网\n"), 0o600); err != nil {
		t.Fatalf("write custom map: %v", err)
	}

	next, status := ReloadIPEngine(Config{
		IPMapEnabled:    true,
		CustomIPMapPath: mapPath,
		GeoIPEnabled:    false,
	}, NewIPEngine())

	if !status.Loaded || status.Error != "" {
		t.Fatalf("reload status = %#v", status)
	}
	tag := next.GetTag("10.0.0.1")
	if tag.Label != "办公终端" || tag.Location != "内网" || !tag.IsManual {
		t.Fatalf("custom tag not loaded: %#v", tag)
	}
}
