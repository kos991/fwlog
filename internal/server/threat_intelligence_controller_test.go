package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fwlog/internal/threatintel"
)

func TestThreatIntelligenceResultDoesNotAnalyze(t *testing.T) {
	service := &fakeThreatIntelligenceService{localResult: &threatintel.Result{
		Provider: threatintel.ProviderThreatBook, IP: "8.8.8.8", Verdict: "benign",
	}}
	app := NewApp(LoadConfig())
	app.threatIntelligenceService = service

	req := httptest.NewRequest(http.MethodGet, "/api/threat-intelligence/providers/threatbook/results?ip=8.8.8.8", nil)
	res := httptest.NewRecorder()
	threatIntelligenceHandler(app).ServeHTTP(res, req)

	if res.Code != http.StatusOK || service.analyzeCalls != 0 {
		t.Fatalf("status=%d analyze=%d", res.Code, service.analyzeCalls)
	}
	if !strings.Contains(res.Body.String(), `"verdict":"benign"`) {
		t.Fatalf("body = %s", res.Body.String())
	}
}

func TestThreatIntelligenceRoutesRequireAuthentication(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/threat-intelligence/providers"},
		{http.MethodPost, "/api/threat-intelligence/providers/threatbook"},
		{http.MethodPost, "/api/threat-intelligence/providers/threatbook/test"},
		{http.MethodGet, "/api/threat-intelligence/providers/threatbook/results?ip=8.8.8.8"},
		{http.MethodPost, "/api/threat-intelligence/providers/threatbook/analyze"},
	}
	for _, tt := range paths {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"ip":"8.8.8.8"}`))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status=%d, want 401", tt.method, tt.path, res.Code)
		}
	}
}

func TestThreatIntelligenceControllerMapsMethodsAndErrors(t *testing.T) {
	app := NewApp(LoadConfig())
	service := &fakeThreatIntelligenceService{analyzeErr: &threatintel.ServiceError{Code: threatintel.ErrorTimeout, Message: "分析超时"}}
	app.threatIntelligenceService = service
	app.sessionToken = "test-token"

	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test-token"})
		res := httptest.NewRecorder()
		app.Router().ServeHTTP(res, req)
		return res
	}

	if res := request(http.MethodGet, "/api/threat-intelligence/providers/threatbook/analyze", ""); res.Code != http.StatusMethodNotAllowed || res.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method response = %d allow=%q", res.Code, res.Header().Get("Allow"))
	}
	if res := request(http.MethodPost, "/api/threat-intelligence/providers/no-such/analyze", `{ "ip": "8.8.8.8" }`); res.Code != http.StatusNotFound {
		t.Fatalf("unknown provider status = %d", res.Code)
	}
	if res := request(http.MethodPost, "/api/threat-intelligence/providers/threatbook/analyze", `{ "ip": "10.0.0.1" }`); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid ip status = %d", res.Code)
	}
	if res := request(http.MethodPost, "/api/threat-intelligence/providers/threatbook/analyze", `{ "ip": "8.8.8.8" }`); res.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout status = %d", res.Code)
	}
	if service.analyzeCalls != 2 {
		t.Fatalf("analyze calls = %d, want 2", service.analyzeCalls)
	}
}

func TestThreatIntelligenceUpdateRejectsCredentialClearConflict(t *testing.T) {
	app := NewApp(LoadConfig())
	service := &fakeThreatIntelligenceService{}
	app.threatIntelligenceService = service
	app.sessionToken = "test-token"
	req := httptest.NewRequest(http.MethodPost, "/api/threat-intelligence/providers/threatbook", strings.NewReader(`{"enabled":true,"credential":"secret","clear_credential":true}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test-token"})
	res := httptest.NewRecorder()
	app.Router().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || service.updateCalls != 0 {
		t.Fatalf("status=%d update_calls=%d", res.Code, service.updateCalls)
	}
}

type fakeThreatIntelligenceService struct {
	localResult  *threatintel.Result
	analyzeErr   error
	analyzeCalls int
	updateCalls  int
}

func (s *fakeThreatIntelligenceService) Providers(context.Context) ([]threatintel.ProviderStatus, error) {
	return []threatintel.ProviderStatus{{Provider: threatintel.ProviderThreatBook, Name: "微步", Enabled: true, Configured: true}}, nil
}

func (s *fakeThreatIntelligenceService) UpdateProvider(context.Context, threatintel.Provider, threatintel.ProviderConfigUpdate) (threatintel.ProviderStatus, error) {
	s.updateCalls++
	return threatintel.ProviderStatus{}, nil
}

func (s *fakeThreatIntelligenceService) TestProvider(context.Context, threatintel.Provider) (threatintel.ProviderTestStatus, error) {
	return threatintel.ProviderTestStatus{Status: "success", Message: "连接测试成功"}, nil
}

func (s *fakeThreatIntelligenceService) Result(context.Context, threatintel.Provider, string) (*threatintel.Result, error) {
	return s.localResult, nil
}

func (s *fakeThreatIntelligenceService) Analyze(_ context.Context, _ threatintel.Provider, ip string) (threatintel.AnalyzeOutcome, error) {
	s.analyzeCalls++
	if _, err := threatintel.NormalizePublicIP(ip); err != nil {
		return threatintel.AnalyzeOutcome{}, &threatintel.ServiceError{Code: threatintel.ErrorInvalidIP, Message: err.Error()}
	}
	if s.analyzeErr != nil {
		return threatintel.AnalyzeOutcome{}, s.analyzeErr
	}
	return threatintel.AnalyzeOutcome{}, errors.New("unexpected analyze")
}
