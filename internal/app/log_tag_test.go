package app

import "testing"

func TestNormalizeLogTagDefaultsToSangforNat(t *testing.T) {
	if got := normalizeLogTag(""); got != defaultLogTag {
		t.Fatalf("normalizeLogTag empty = %q, want %q", got, defaultLogTag)
	}
}

func TestNormalizeLogTagTrimsCustomValue(t *testing.T) {
	if got := normalizeLogTag("  edge-firewall  "); got != "edge-firewall" {
		t.Fatalf("normalizeLogTag custom = %q", got)
	}
}
