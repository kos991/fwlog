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
)

func TestQianxinAdapterRequestsFullReputation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.Header.Get("Api-Key") != "test-key" {
			t.Fatal("missing Api-Key")
		}
		query := r.URL.Query()
		if query.Get("param") != "2001:4860:4860::8888" || query.Get("mode") != "0" {
			t.Fatalf("bad query = %q", r.URL.RawQuery)
		}
		if strings.Contains(r.URL.RawQuery, "test-key") {
			t.Fatalf("credential leaked in query: %s", r.URL.RawQuery)
		}
		io.WriteString(w, `{"status":10000,"message":"success","data":{"summary_info":{"reputation":"suspicious","latest_reputation_time":"2026-08-30 08:00:00","malicious_label":["扫描"]},"normal_info":{},"geo":{}}}`)
	}))
	defer server.Close()

	result, err := NewQianxinAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "2001:4860:4860::8888")
	if err != nil {
		t.Fatal(err)
	}

	if result.Provider != ProviderQianxin || result.IP != "2001:4860:4860::8888" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Verdict != "suspicious" || result.RiskLevel != "unknown" || result.ConfidenceLevel != "unknown" {
		t.Fatalf("result verdict/risk/confidence = %#v", result)
	}
	if result.ConfidenceScore != nil {
		t.Fatalf("confidence score = %#v, want nil", result.ConfidenceScore)
	}
	if !reflect.DeepEqual(result.Tags, []string{"扫描"}) {
		t.Fatalf("tags = %#v", result.Tags)
	}
	if result.SourceUpdatedAt == nil || result.SourceUpdatedAt.Format("2006-01-02 15:04:05") != "2026-08-30 08:00:00" {
		t.Fatalf("source updated at = %#v", result.SourceUpdatedAt)
	}
	if result.AnalyzedAt.IsZero() {
		t.Fatal("analyzed at should be set")
	}
}

func TestQianxinAdapterUsesDefaultEndpoint(t *testing.T) {
	client := &http.Client{Transport: qianxinRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "webapi.ti.qianxin.com" || r.URL.Path != "/ip/v3/reputation" {
			t.Fatalf("url = %s, want default Qianxin endpoint", r.URL.String())
		}
		if r.URL.Query().Get("param") != "8.8.8.8" || r.URL.Query().Get("mode") != "0" {
			t.Fatalf("bad query = %q", r.URL.RawQuery)
		}
		if r.Header.Get("Api-Key") != "test-key" {
			t.Fatal("missing Api-Key")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":10000,"data":{"summary_info":{"reputation":"benign"}}}`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})}

	result, err := NewQianxinAdapter(client, "").Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "benign" {
		t.Fatalf("verdict = %q, want benign", result.Verdict)
	}
}

func TestQianxinAdapterMapsReputationValues(t *testing.T) {
	tests := []struct {
		name       string
		reputation string
		want       string
	}{
		{"malicious", "malicious", "malicious"},
		{"suspicious", "suspicious", "suspicious"},
		{"benign", "benign", "benign"},
		{"unknown", "unknown", "unknown"},
		{"unexpected", "clean", "unknown"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"status":10000,"data":{"summary_info":{"reputation":"`+tt.reputation+`","malicious_label":["tag"]}}}`)
			}))
			defer server.Close()

			result, err := NewQianxinAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
			if err != nil {
				t.Fatal(err)
			}
			if result.Verdict != tt.want {
				t.Fatalf("verdict = %q, want %q", result.Verdict, tt.want)
			}
			if result.RiskLevel != "unknown" || result.ConfidenceScore != nil || result.ConfidenceLevel != "unknown" {
				t.Fatalf("risk/confidence should not be inferred: %#v", result)
			}
		})
	}
}

func TestQianxinAdapterMissingSummaryInfoIsUnknownSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":10000,"data":{"normal_info":{"owner":"kept"},"geo":{"country":"CN"}}}`)
	}))
	defer server.Close()

	result, err := NewQianxinAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}

	if result.Verdict != "unknown" || result.RiskLevel != "unknown" || result.ConfidenceLevel != "unknown" {
		t.Fatalf("result = %#v", result)
	}
	if result.ConfidenceScore != nil || len(result.Tags) != 0 || result.SourceUpdatedAt != nil {
		t.Fatalf("result should not infer missing fields: %#v", result)
	}
	if !strings.Contains(string(result.RawResponse), `"normal_info"`) || !strings.Contains(string(result.RawResponse), `"geo"`) {
		t.Fatalf("raw response should retain provider-only fields: %s", string(result.RawResponse))
	}
}

func TestQianxinAdapterRejectsBusinessStatusWithoutLeakingMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":20001,"message":"credential-secret upstream detail","data":{"summary_info":{"reputation":"unknown"}}}`)
	}))
	defer server.Close()

	_, err := NewQianxinAdapter(server.Client(), server.URL).Analyze(context.Background(), "credential-secret", "8.8.8.8")
	if err == nil {
		t.Fatal("business error should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if strings.Contains(current.Error(), "credential-secret") || strings.Contains(current.Error(), "upstream detail") {
			t.Fatalf("error chain leaked credential or upstream body: %q", current.Error())
		}
	}
}

func TestQianxinAdapterDoesNotLeakCredentialInRawResponseOrErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Api-Key") != "credential-secret" {
			t.Fatal("missing Api-Key")
		}
		io.WriteString(w, `{"status":10000,"Api-Key":"credential-secret","data":{"summary_info":{"reputation":"malicious","malicious_label":["botnet"]}}}`)
	}))
	defer server.Close()

	result, err := NewQianxinAdapter(server.Client(), server.URL).Analyze(context.Background(), "credential-secret", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result.RawResponse), "credential-secret") {
		t.Fatalf("raw response leaked credential: %s", string(result.RawResponse))
	}
	if !strings.Contains(string(result.RawResponse), `"Api-Key":"[redacted]"`) {
		t.Fatalf("raw response did not redact Api-Key: %s", string(result.RawResponse))
	}
	if !reflect.DeepEqual(result.Tags, []string{"botnet"}) {
		t.Fatalf("tags = %#v, want botnet", result.Tags)
	}
}

type qianxinRoundTripFunc func(*http.Request) (*http.Response, error)

func (f qianxinRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
