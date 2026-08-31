package threatintel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestThreatBookAdapterBuildsRequestAndMapsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		query := r.URL.Query()
		if query.Get("resource") != "8.8.8.8" || query.Get("lang") != "zh" {
			t.Fatalf("query resource/lang = %q/%q", query.Get("resource"), query.Get("lang"))
		}
		if query.Get("apikey") != "test-key" {
			t.Fatal("missing apikey")
		}
		io.WriteString(w, `{"response_code":0,"data":{"8.8.8.8":{"is_malicious":true,"confidence_level":"high","severity":"critical","judgments":["僵尸网络","恶意软件","僵尸网络"],"update_time":"2026-08-30 10:20:30"}}}`)
	}))
	defer server.Close()

	result, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}

	if result.Provider != ProviderThreatBook || result.IP != "8.8.8.8" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Verdict != "malicious" || result.RiskLevel != "critical" || result.ConfidenceLevel != "high" {
		t.Fatalf("result verdict/risk/confidence = %#v", result)
	}
	if !reflect.DeepEqual(result.Tags, []string{"僵尸网络", "恶意软件"}) {
		t.Fatalf("tags = %#v", result.Tags)
	}
	if result.SourceUpdatedAt == nil || result.SourceUpdatedAt.Format("2006-01-02 15:04:05") != "2026-08-30 10:20:30" {
		t.Fatalf("source updated at = %#v", result.SourceUpdatedAt)
	}
	if result.AnalyzedAt.IsZero() {
		t.Fatal("analyzed at should be set")
	}
	if result.Summary != "微步在线判定 8.8.8.8 为 malicious，风险 critical，置信度 high，标签：僵尸网络、恶意软件" {
		t.Fatalf("summary = %q", result.Summary)
	}
}

func TestThreatBookAdapterMapsBenignResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"response_code":0,"data":{"1.1.1.1":{"is_malicious":false,"confidence_level":"low","severity":"info","judgments":[]}}}`)
	}))
	defer server.Close()

	result, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}

	if result.Verdict != "benign" || result.RiskLevel != "info" {
		t.Fatalf("result = %#v", result)
	}
}

func TestThreatBookAdapterMissingTargetIsUnknownSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"response_code":0,"apikey":"echoed-secret","data":{}}`)
	}))
	defer server.Close()

	result, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}

	if result.Provider != ProviderThreatBook || result.IP != "9.9.9.9" || result.Verdict != "unknown" {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(string(result.RawResponse), "echoed-secret") {
		t.Fatalf("raw response leaked credential: %s", string(result.RawResponse))
	}
}

func TestThreatBookAdapterRejectsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"response_code":-1,"verbose_msg":"secret upstream text"}`)
	}))
	defer server.Close()

	_, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err == nil {
		t.Fatal("business error should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
	if strings.Contains(err.Error(), "secret upstream text") {
		t.Fatalf("error leaked upstream body: %q", err.Error())
	}
}

func TestThreatBookAdapterRejectsInvalidSuccessStructure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"response_code":0,"data":{"8.8.8.8":{"is_malicious":"yes"}}}`)
	}))
	defer server.Close()

	_, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err == nil {
		t.Fatal("invalid success structure should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
}

func TestThreatBookAdapterRejectsMissingMaliciousFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"response_code":0,"data":{"8.8.8.8":{"severity":"info"}}}`)
	}))
	defer server.Close()

	_, err := NewThreatBookAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err == nil {
		t.Fatal("missing is_malicious should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
}
