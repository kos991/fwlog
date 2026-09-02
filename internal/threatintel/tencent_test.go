package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTencentAdapterPostsIPAnalysis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if strings.Contains(r.URL.RawQuery, "test-key") {
			t.Fatalf("credential leaked in query: %s", r.URL.RawQuery)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"c_version": "3.0",
			"c_action":  "IPAnalysis",
			"c_appkey":  "test-key",
			"c_lang":    "zh",
			"type":      "ip",
			"key":       "8.8.8.8",
			"option":    float64(0),
		}
		if !reflect.DeepEqual(body, want) {
			t.Fatalf("body = %#v, want %#v", body, want)
		}

		io.WriteString(w, `{"return_code":0,"return_msg":"success","result":"black","threat_level":5,"confidence":96,"tags":["C2"],"first_seen":"2026-08-01T00:00:00Z","last_seen":"2026-08-30T00:00:00Z"}`)
	}))
	defer server.Close()

	result, err := NewTencentAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != ProviderTencent || result.IP != "8.8.8.8" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Verdict != "malicious" || result.RiskLevel != "critical" {
		t.Fatalf("result verdict/risk = %#v", result)
	}
	if result.ConfidenceScore == nil || *result.ConfidenceScore != 96 {
		t.Fatalf("confidence score = %#v", result.ConfidenceScore)
	}
	if !reflect.DeepEqual(result.Tags, []string{"C2"}) {
		t.Fatalf("tags = %#v", result.Tags)
	}
}

func TestTencentAdapterSupportsIPv6(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["key"] != "2001:4860:4860::8888" {
			t.Fatalf("key = %q, want IPv6 address only", body["key"])
		}
		io.WriteString(w, `{"return_code":0,"result":"info","threat_level":0}`)
	}))
	defer server.Close()

	result, err := NewTencentAdapter(server.Client(), server.URL).Analyze(context.Background(), "test-key", "2001:4860:4860::8888")
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "unknown" || result.RiskLevel != "unknown" {
		t.Fatalf("result = %#v", result)
	}
}

func TestTencentAdapterMapsVerdictsRiskConfidenceTagsAndTimes(t *testing.T) {
	tests := []struct {
		name           string
		result         string
		threatLevel    int
		confidenceJSON string
		wantVerdict    string
		wantRisk       string
		wantConfidence *float64
	}{
		{"black critical percentage", "black", 5, `96`, "malicious", "critical", floatPtr(96)},
		{"suspicious high fraction", "suspicious", 4, `0.96`, "suspicious", "high", floatPtr(96)},
		{"white medium clamps high", "white", 3, `120`, "benign", "medium", floatPtr(100)},
		{"info low clamps low", "info", 2, `-1`, "unknown", "low", floatPtr(0)},
		{"unexpected info no confidence", "clean", 1, `null`, "unknown", "info", nil},
		{"zero unknown", "info", 0, `null`, "unknown", "unknown", nil},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			raw := json.RawMessage(`{"return_code":0,"result":"` + tt.result + `","threat_level":` + strconv.Itoa(tt.threatLevel) + `,"confidence":` + tt.confidenceJSON + `,"tags":["C2"," botnet ","C2",""],"first_seen":"2026-08-01T00:00:00Z","last_seen":"2026-08-30T00:00:00Z"}`)

			result, err := mapTencentResponse("8.8.8.8", raw)
			if err != nil {
				t.Fatal(err)
			}
			if result.Verdict != tt.wantVerdict || result.RiskLevel != tt.wantRisk {
				t.Fatalf("verdict/risk = %q/%q, want %q/%q", result.Verdict, result.RiskLevel, tt.wantVerdict, tt.wantRisk)
			}
			if !equalFloatPtr(result.ConfidenceScore, tt.wantConfidence) {
				t.Fatalf("confidence = %#v, want %#v", result.ConfidenceScore, tt.wantConfidence)
			}
			if !reflect.DeepEqual(result.Tags, []string{"C2", "botnet"}) {
				t.Fatalf("tags = %#v", result.Tags)
			}
			if result.FirstSeen == nil || result.FirstSeen.Format(time.RFC3339) != "2026-08-01T00:00:00Z" {
				t.Fatalf("first seen = %#v", result.FirstSeen)
			}
			if result.LastSeen == nil || result.LastSeen.Format(time.RFC3339) != "2026-08-30T00:00:00Z" {
				t.Fatalf("last seen = %#v", result.LastSeen)
			}
			if tt.wantConfidence == nil && result.RiskLevel != "unknown" && result.ConfidenceScore != nil {
				t.Fatalf("confidence should not be inferred from risk: %#v", result)
			}
		})
	}
}

func TestTencentAdapterMapsBusinessErrorsWithoutLeakingReturnMessage(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		wantCode ErrorCode
	}{
		{"quota exhausted", 1004, ErrorQuotaExhausted},
		{"rate limited", 1005, ErrorRateLimited},
		{"other business error", 2001, ErrorInvalidResponse},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"return_code":`+strconv.Itoa(tt.code)+`,"return_msg":"credential-secret upstream detail"}`)
			}))
			defer server.Close()

			_, err := NewTencentAdapter(server.Client(), server.URL).Analyze(context.Background(), "credential-secret", "8.8.8.8")
			if err == nil {
				t.Fatal("business error should fail")
			}
			if got := ErrorCodeOf(err); got != tt.wantCode {
				t.Fatalf("ErrorCodeOf() = %q, want %q", got, tt.wantCode)
			}
			for current := err; current != nil; current = errors.Unwrap(current) {
				if strings.Contains(current.Error(), "credential-secret") || strings.Contains(current.Error(), "upstream detail") {
					t.Fatalf("error chain leaked credential or upstream message: %q", current.Error())
				}
			}
		})
	}
}

func TestTencentAdapterRedactsCredentialEchoInRawResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"return_code":0,"return_msg":"success","c_appkey":"credential-secret","result":"black","threat_level":5,"confidence":88,"tags":["botnet"],"details":{"api_key":"credential-secret","safe":"kept"}}`)
	}))
	defer server.Close()

	result, err := NewTencentAdapter(server.Client(), server.URL).Analyze(context.Background(), "credential-secret", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	body := string(result.RawResponse)
	if strings.Contains(body, "credential-secret") {
		t.Fatalf("raw response leaked credential: %s", body)
	}
	if !strings.Contains(body, `"c_appkey":"[redacted]"`) || !strings.Contains(body, `"safe":"kept"`) {
		t.Fatalf("raw response was not sanitized correctly: %s", body)
	}
}

func TestDefaultAdaptersRegistersFourFixedProductionEndpoints(t *testing.T) {
	seen := map[Provider]bool{}
	client := &http.Client{Transport: registryRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host + r.URL.Path {
		case "api.threatbook.cn/v3/scene/ip_reputation":
			seen[ProviderThreatBook] = true
			return registryJSON(r, `{"response_code":0,"data":{"8.8.8.8":{"is_malicious":false,"confidence_level":"low","severity":"info"}}}`), nil
		case "nti.nsfocus.com/api/v2/objects/ioc-ipv4/":
			seen[ProviderNSFocus] = true
			return registryJSON(r, `{"count":0,"objects":[]}`), nil
		case "webapi.ti.qianxin.com/ip/v3/reputation":
			seen[ProviderQianxin] = true
			return registryJSON(r, `{"status":10000,"data":{"summary_info":{"reputation":"benign"}}}`), nil
		case "xti.qq.com/api/v3/ti":
			seen[ProviderTencent] = true
			return registryJSON(r, `{"return_code":0,"result":"white","threat_level":0}`), nil
		default:
			t.Fatalf("unexpected production endpoint: %s", r.URL.String())
			return nil, errors.New("unexpected endpoint")
		}
	})}

	adapters := DefaultAdapters(client)
	if len(adapters) != 4 {
		t.Fatalf("adapter count = %d, want 4", len(adapters))
	}
	for _, provider := range []Provider{ProviderThreatBook, ProviderNSFocus, ProviderQianxin, ProviderTencent} {
		adapter := adapters[provider]
		if adapter == nil {
			t.Fatalf("missing adapter for %s", provider)
		}
		if _, err := adapter.Analyze(context.Background(), "test-key", "8.8.8.8"); err != nil {
			t.Fatalf("%s Analyze() error = %v", provider, err)
		}
	}
	if !reflect.DeepEqual(seen, map[Provider]bool{
		ProviderThreatBook: true,
		ProviderNSFocus:    true,
		ProviderQianxin:    true,
		ProviderTencent:    true,
	}) {
		t.Fatalf("seen endpoints = %#v", seen)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func equalFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type registryRoundTripFunc func(*http.Request) (*http.Response, error)

func (f registryRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func registryJSON(r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    r,
	}
}
