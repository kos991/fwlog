package main

import "testing"

func TestNormalizeLogTagDefaultsToSangforNat(t *testing.T) {
	if got := normalizeLogTag(""); got != defaultLogTag {
		t.Fatalf("normalizeLogTag empty = %q, want %q", got, defaultLogTag)
	}
}

func TestNormalizeLogTagTrimsCustomValue(t *testing.T) {
	if got := normalizeLogTag("  总部出口防火墙  "); got != "总部出口防火墙" {
		t.Fatalf("normalizeLogTag custom = %q", got)
	}
}
