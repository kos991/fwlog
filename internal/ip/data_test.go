package ip

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReloadIPEngineIgnoresMissingCustomMap(t *testing.T) {
	old := NewIPEngine()
	next, status := ReloadIPEngine(Config{
		IPMapEnabled:    true,
		CustomIPMapPath: "Z:/missing/custom.csv",
		GeoIPEnabled:    false,
	}, old)

	if next == old {
		t.Fatal("missing custom map should not block a usable engine")
	}
	if status.Error != "" {
		t.Fatalf("missing custom map should be non-fatal: %#v", status)
	}
	if !status.Loaded {
		t.Fatalf("reload should be marked loaded: %#v", status)
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

func TestReloadIPEngineLoadsCIDRAliasOverDefaultSegment(t *testing.T) {
	next, status := ReloadIPEngine(Config{
		IPMapEnabled: false,
		GeoIPEnabled: false,
		CIDRAliases: []CIDRAliasSetting{
			{CIDR: "2.55.80.0/24", Alias: "office", Enabled: true},
		},
	}, NewIPEngine())

	if !status.Loaded || status.Error != "" {
		t.Fatalf("reload status = %#v", status)
	}
	tag := next.GetTag("2.55.80.9")
	if tag.Label != "office" || tag.Location != "内网" {
		t.Fatalf("cidr alias should override default segment: %#v", tag)
	}
}
