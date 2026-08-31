package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderHTTPRedactsCredentialFieldsInRawResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"apikey":"echoed-secret","data":{"nested":{"API_KEY":"echoed-secret","key":"echoed-secret","token":"echoed-secret","secret":"echoed-secret","credential":"echoed-secret","Access_Token":"echoed-secret","REFRESH_TOKEN":"echoed-secret","Auth_Token":"echoed-secret","Authorization":"echoed-secret","PASSWORD":"echoed-secret","PassWd":"echoed-secret","Pwd":"echoed-secret","CLIENT_SECRET":"echoed-secret","safe":"kept"}}}`)
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := doProviderJSON(server.Client(), request)
	if err != nil {
		t.Fatal(err)
	}

	body := string(raw)
	if !json.Valid(raw) {
		t.Fatalf("raw response should remain valid JSON: %s", body)
	}
	if strings.Contains(body, "echoed-secret") {
		t.Fatalf("raw response leaked credential: %s", body)
	}
	if !strings.Contains(body, `"safe":"kept"`) {
		t.Fatalf("raw response removed non-credential field: %s", body)
	}
}

func TestProviderHTTPRejectsResponseJustOverSizeLimitWithoutLeakingBody(t *testing.T) {
	const maxResponseBytes = 4 << 20
	const prefix = `{"payload":"response-secret-`
	const suffix = `"}`
	body := prefix + strings.Repeat("x", maxResponseBytes+1-len(prefix)-len(suffix)) + suffix

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = doProviderJSON(server.Client(), request)
	if err == nil {
		t.Fatal("response just over size limit should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if strings.Contains(current.Error(), "response-secret") {
			t.Fatalf("size limit error chain leaked response body: %q", current.Error())
		}
	}
}

func TestProviderHTTPClassifiesStatusWithoutLeakingBody(t *testing.T) {
	tests := []struct {
		status   int
		wantCode ErrorCode
	}{
		{http.StatusUnauthorized, ErrorInvalidCredential},
		{http.StatusForbidden, ErrorInvalidCredential},
		{http.StatusTooManyRequests, ErrorRateLimited},
		{http.StatusInternalServerError, ErrorProviderUnavailable},
	}

	for _, tt := range tests {
		status := tt.status
		wantCode := tt.wantCode
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.WriteString(w, `upstream-secret-body`)
			}))
			defer server.Close()

			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = doProviderJSON(server.Client(), request)
			if err == nil {
				t.Fatal("HTTP non-success should fail")
			}
			if got := ErrorCodeOf(err); got != wantCode {
				t.Fatalf("ErrorCodeOf() = %q, want %q", got, wantCode)
			}
			if strings.Contains(err.Error(), "upstream-secret-body") {
				t.Fatalf("user message leaked response body: %q", err.Error())
			}
		})
	}
}

func TestProviderHTTPBoundsBodyBeforeCustomErrorClassification(t *testing.T) {
	body := `{"message":"` + strings.Repeat("x", maxProviderJSONResponseBytes) + `"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, body)
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	classifierCalled := false
	_, err = doProviderJSONWithErrorClassifier(server.Client(), request, func(status int, raw json.RawMessage) (ErrorCode, string, bool) {
		classifierCalled = true
		return ErrorInvalidCredential, "custom error", true
	})
	if err == nil {
		t.Fatal("oversized error response should fail")
	}
	if classifierCalled {
		t.Fatal("classifier should not receive an oversized response body")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
}

func TestProviderHTTPRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{not-json`)
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = doProviderJSON(server.Client(), request)
	if err == nil {
		t.Fatal("invalid JSON should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorInvalidResponse {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorInvalidResponse)
	}
}

func TestProviderHTTPWrapsTransportErrorsWithoutURL(t *testing.T) {
	client := &http.Client{Transport: failingRoundTripper{err: errors.New("network down")}}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid/path?apikey=secret", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = doProviderJSON(client, request)
	if err == nil {
		t.Fatal("transport error should fail")
	}
	if got := ErrorCodeOf(err); got != ErrorProviderUnavailable {
		t.Fatalf("ErrorCodeOf() = %q, want %q", got, ErrorProviderUnavailable)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "example.invalid") {
		t.Fatalf("transport error leaked URL or credential: %q", err.Error())
	}
}

type failingRoundTripper struct{ err error }

func (r failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, r.err
}
