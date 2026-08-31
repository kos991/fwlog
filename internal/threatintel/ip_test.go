package threatintel

import (
	"errors"
	"testing"
)

func TestNormalizePublicIP(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"8.8.8.8", "8.8.8.8"},
		{"::ffff:8.8.8.8", "8.8.8.8"},
		{"2001:4860:4860::8888", "2001:4860:4860::8888"},
	}
	for _, tt := range tests {
		got, err := NormalizePublicIP(tt.raw)
		if err != nil || got != tt.want {
			t.Fatalf("NormalizePublicIP(%q) = %q, %v", tt.raw, got, err)
		}
	}
}

func TestNormalizePublicIPRejectsNonPublicAddresses(t *testing.T) {
	for _, raw := range []string{"", "not-an-ip", "10.0.0.1", "127.0.0.1", "169.254.1.1", "224.0.0.1", "255.255.255.255", "::"} {
		_, err := NormalizePublicIP(raw)
		if err == nil {
			t.Fatalf("NormalizePublicIP(%q) should fail", raw)
		}
	}
}

func TestParseProviderAcceptsSupportedProviders(t *testing.T) {
	tests := []struct {
		raw  string
		want Provider
	}{
		{"threatbook", ProviderThreatBook},
		{"nsfocus", ProviderNSFocus},
		{"qianxin", ProviderQianxin},
		{"tencent", ProviderTencent},
	}

	for _, tt := range tests {
		got, ok := ParseProvider(tt.raw)
		if !ok || got != tt.want {
			t.Fatalf("ParseProvider(%q) = %q, %v", tt.raw, got, ok)
		}
	}
}

func TestParseProviderRejectsUnsupportedProvider(t *testing.T) {
	got, ok := ParseProvider("unknown")
	if ok || got != "" {
		t.Fatalf("ParseProvider(%q) = %q, %v", "unknown", got, ok)
	}
}

func TestProviderNameReturnsDisplayName(t *testing.T) {
	tests := []struct {
		provider Provider
		want     string
	}{
		{ProviderThreatBook, "微步在线"},
		{ProviderNSFocus, "绿盟科技"},
		{ProviderQianxin, "奇安信"},
		{ProviderTencent, "腾讯安全"},
		{Provider("unknown"), "unknown"},
	}

	for _, tt := range tests {
		got := ProviderName(tt.provider)
		if got != tt.want {
			t.Fatalf("ProviderName(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestErrorCodeOfReturnsServiceErrorCode(t *testing.T) {
	err := newServiceError(ErrorRateLimited, "请求过于频繁", errors.New("upstream 429"))

	got := ErrorCodeOf(err)

	if got != ErrorRateLimited {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorRateLimited)
	}
}

func TestErrorCodeOfFallsBackToInternalError(t *testing.T) {
	got := ErrorCodeOf(errors.New("database unavailable"))

	if got != ErrorInternal {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInternal)
	}
}
