package ip

import "testing"

func TestIPEngineMatchesEquivalentIPv6OverrideText(t *testing.T) {
	engine := NewIPEngine()
	engine.AddOverride("2001:0db8:0000:0000:0000:0000:0000:0010", "IPv6 服务器", "机房")

	tag := engine.GetTag("2001:db8::10")
	if !tag.IsManual || tag.Label != "IPv6 服务器" || tag.Location != "机房" {
		t.Fatalf("equivalent IPv6 override did not match: %#v", tag)
	}
}

func TestIPEngineUsesLongestIPv6CIDRPrefix(t *testing.T) {
	engine := NewIPEngine()
	if err := engine.AddSegment("2001:db8::/32", "IPv6 总网"); err != nil {
		t.Fatalf("add /32 segment: %v", err)
	}
	if err := engine.AddSegment("2001:db8:1::/48", "IPv6 业务区"); err != nil {
		t.Fatalf("add /48 segment: %v", err)
	}

	tag := engine.GetTag("2001:db8:1::20")
	if tag.Label != "IPv6 业务区" {
		t.Fatalf("IPv6 longest-prefix label = %q", tag.Label)
	}
}

func TestIPEngineRecognizesIPv6PrivateAddress(t *testing.T) {
	tag := NewIPEngine().GetTag("fd00::10")
	if tag.Location != "内网" {
		t.Fatalf("IPv6 private location = %q", tag.Location)
	}
}
