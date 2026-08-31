package threatintel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNSFocusAdapterUsesHeaderAndHighestActiveThreat(t *testing.T) {
	fixedNow := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.Header.Get("X-Ns-Nti-Key") != "test-key" {
			t.Fatal("missing X-Ns-Nti-Key")
		}
		if r.Header.Get("Accept") != "application/nsfocus.nti.spec+json; version=2.0" {
			t.Fatal("wrong Accept")
		}
		if r.URL.Query().Get("query") != "8.8.8.8" {
			t.Fatal("wrong query")
		}
		if strings.Contains(r.URL.RawQuery, "test-key") {
			t.Fatalf("credential leaked in query: %s", r.URL.RawQuery)
		}
		io.WriteString(w, `{"count":3,"key":"test-key","objects":[{"revoked":false,"valid_until":"2026-09-30T00:00:00Z","confidence":60,"threat_level":3,"categories":["spam"],"threat_types":["botnet"],"act_types":["scan"],"tags":["active"],"modified":"2026-08-29T00:00:00Z"},{"revoked":false,"valid_until":"2026-09-30T00:00:00Z","confidence":88,"threat_level":5,"categories":["botnet"],"threat_types":["c2"],"act_types":["scan"],"tags":["active"],"modified":"2026-08-30T00:00:00Z"},{"revoked":true,"threat_level":5,"confidence":100,"tags":["revoked"]}]}`)
	}))
	defer server.Close()

	adapter := NewNSFocusAdapter(server.Client(), server.URL, func() time.Time { return fixedNow })
	result, err := adapter.Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}

	if result.Provider != ProviderNSFocus || result.IP != "8.8.8.8" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Verdict != "malicious" || result.RiskLevel != "high" {
		t.Fatalf("result verdict/risk = %#v", result)
	}
	if result.ConfidenceScore == nil || *result.ConfidenceScore != 88 {
		t.Fatalf("confidence score = %#v", result.ConfidenceScore)
	}
	if !reflect.DeepEqual(result.Tags, []string{"active", "botnet", "c2", "scan", "spam"}) {
		t.Fatalf("tags = %#v", result.Tags)
	}
	if result.SourceUpdatedAt == nil || result.SourceUpdatedAt.Format(time.RFC3339) != "2026-08-30T00:00:00Z" {
		t.Fatalf("source updated at = %#v", result.SourceUpdatedAt)
	}
	if !result.AnalyzedAt.Equal(fixedNow) {
		t.Fatalf("analyzed at = %s, want %s", result.AnalyzedAt, fixedNow)
	}
	if strings.Contains(string(result.RawResponse), "test-key") {
		t.Fatalf("raw response leaked credential: %s", string(result.RawResponse))
	}
}

func TestNSFocusAdapterOnlyExpiredAndRevokedObjectsAreUnknown(t *testing.T) {
	fixedNow := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"count":2,"objects":[{"revoked":false,"valid_until":"2026-08-30T23:59:59Z","confidence":90,"threat_level":5,"tags":["expired"]},{"revoked":true,"valid_until":"2026-09-30T00:00:00Z","confidence":90,"threat_level":5,"tags":["revoked"]}]}`)
	}))
	defer server.Close()

	result, err := NewNSFocusAdapter(server.Client(), server.URL, func() time.Time { return fixedNow }).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}

	if result.Verdict != "unknown" || result.RiskLevel != "unknown" {
		t.Fatalf("result = %#v", result)
	}
	if result.ConfidenceScore != nil {
		t.Fatalf("confidence score = %#v, want nil", result.ConfidenceScore)
	}
	if len(result.Tags) != 0 {
		t.Fatalf("tags = %#v, want empty", result.Tags)
	}
}

func TestNSFocusAdapterTreatsValidUntilEqualNowAsActive(t *testing.T) {
	fixedNow := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"count":1,"objects":[{"revoked":false,"valid_until":"2026-08-31T00:00:00Z","confidence":40,"threat_level":1,"tags":["boundary"]}]}`)
	}))
	defer server.Close()

	result, err := NewNSFocusAdapter(server.Client(), server.URL, func() time.Time { return fixedNow }).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}

	if result.Verdict != "malicious" || result.RiskLevel != "low" {
		t.Fatalf("result = %#v", result)
	}
}

func TestNSFocusAdapterRejectsIPv6BeforeHTTP(t *testing.T) {
	client := &http.Client{Transport: failOnRoundTrip{t: t}}
	adapter := NewNSFocusAdapter(client, "https://nti.example.invalid/api", func() time.Time { return time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) })

	_, err := adapter.Analyze(context.Background(), "test-key", "2001:4860:4860::8888")
	if err == nil {
		t.Fatal("IPv6 should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorUnsupportedIP {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorUnsupportedIP)
	}
}

func TestNSFocusAdapterMapsBusinessErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantCode    ErrorCode
		wantMessage string
	}{
		{"invalid key", http.StatusUnauthorized, `{"message":"Invalid NTI key","detail":"response-secret"}`, ErrorInvalidCredential, "绿盟 NTI 凭据无效"},
		{"forbidden", http.StatusForbidden, `{"error":"Forbidden","detail":"response-secret"}`, ErrorInvalidCredential, "绿盟 NTI 访问被拒绝，请检查凭据和授权范围"},
		{"over limit", http.StatusTooManyRequests, `{"message":"Over limit","detail":"response-secret"}`, ErrorQuotaExhausted, "绿盟 NTI 查询额度已用尽"},
		{"authorization expired", http.StatusUnauthorized, `{"error":{"message":"Authorization expired","detail":"response-secret"}}`, ErrorInvalidCredential, "绿盟 NTI 授权已过期，请更新凭据"},
		{"ip mismatch", http.StatusForbidden, `{"message":"Authorization failed","detail":"Authorization IP is not allowed by whitelist","trace":"response-secret"}`, ErrorInvalidCredential, "绿盟 NTI 授权 IP 不匹配，请检查白名单配置"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("X-Ns-Nti-Key"); got != "credential-secret" {
					t.Fatalf("X-Ns-Nti-Key = %q, want credential", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()

			_, err := NewNSFocusAdapter(server.Client(), server.URL, func() time.Time { return time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) }).Analyze(context.Background(), "credential-secret", "8.8.8.8")
			if err == nil {
				t.Fatal("business error should fail")
			}
			if got := ErrorCodeOf(err); got != tt.wantCode {
				t.Fatalf("ErrorCodeOf() = %q, want %q", got, tt.wantCode)
			}
			if err.Error() != tt.wantMessage {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantMessage)
			}
			for current := err; current != nil; current = errors.Unwrap(current) {
				if strings.Contains(current.Error(), "credential-secret") || strings.Contains(current.Error(), "response-secret") || strings.Contains(current.Error(), tt.body) {
					t.Fatalf("error chain leaked credential or upstream body: %q", current.Error())
				}
			}
		})
	}
}

func TestNSFocusAdapterDoesNotClassifyObjectFieldsAsBusinessErrors(t *testing.T) {
	fixedNow := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"count":1,"objects":[{"revoked":false,"valid_until":"2026-09-30T00:00:00Z","confidence":75,"threat_level":5,"categories":["forbidden"],"threat_types":["over limit"],"act_types":["Invalid NTI key"],"tags":["Authorization expired","IP mismatch"]}]}`)
	}))
	defer server.Close()

	result, err := NewNSFocusAdapter(server.Client(), server.URL, func() time.Time { return fixedNow }).Analyze(context.Background(), "credential-secret", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "malicious" || result.RiskLevel != "high" {
		t.Fatalf("result = %#v", result)
	}
	wantTags := []string{"Authorization expired", "IP mismatch", "Invalid NTI key", "forbidden", "over limit"}
	if !reflect.DeepEqual(result.Tags, wantTags) {
		t.Fatalf("tags = %#v, want %#v", result.Tags, wantTags)
	}
}

type failOnRoundTrip struct {
	t *testing.T
}

func (f failOnRoundTrip) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatal("HTTP client should not be called for IPv6")
	return nil, errors.New("unexpected request")
}
