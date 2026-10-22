package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fwlog/internal/config"
	"fwlog/internal/security"
)

func bearerTokenForInterfaceTest(t *testing.T, app *App) string {
	t.Helper()
	manager, err := security.NewJWTManager([]byte(strings.Repeat("i", 32)), time.Hour)
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	token, err := manager.Issue("admin", []string{"pwd"}, time.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	app.mu.Lock()
	app.jwtManager = manager
	app.mu.Unlock()
	return token
}

func postMCPForInterfaceTest(t *testing.T, router http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestDefaultInterfaceSettings(t *testing.T) {
	settings := defaultSettings(config.LoadConfig())
	if settings[externalAPIEnabledSettingKey] != "true" {
		t.Fatalf("external API default = %q, want true", settings[externalAPIEnabledSettingKey])
	}
	if settings[mcpEnabledSettingKey] != "false" {
		t.Fatalf("MCP default = %q, want false", settings[mcpEnabledSettingKey])
	}
}

func TestExternalAPISwitchOnlyBlocksBearerQuery(t *testing.T) {
	app := NewApp(config.LoadConfig())
	app.updateSettings(map[string]any{externalAPIEnabledSettingKey: false})
	router := app.Router()
	token := bearerTokenForInterfaceTest(t, app)

	bearerReq := httptest.NewRequest(http.MethodGet, "/api/query", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+token)
	bearerRes := httptest.NewRecorder()
	router.ServeHTTP(bearerRes, bearerReq)
	if bearerRes.Code != http.StatusServiceUnavailable || !strings.Contains(bearerRes.Body.String(), "external_api_disabled") {
		t.Fatalf("disabled bearer query = %d %s", bearerRes.Code, bearerRes.Body.String())
	}

	cookie := loginForTest(t, router)
	consoleReq := httptest.NewRequest(http.MethodGet, "/api/query", nil)
	consoleReq.AddCookie(cookie)
	consoleRes := httptest.NewRecorder()
	router.ServeHTTP(consoleRes, consoleReq)
	if consoleRes.Code != http.StatusOK {
		t.Fatalf("console query should remain available: %d %s", consoleRes.Code, consoleRes.Body.String())
	}
}

func TestMCPSwitchAndReadOnlyQueryTool(t *testing.T) {
	app := NewApp(config.LoadConfig())
	router := app.Router()
	token := bearerTokenForInterfaceTest(t, app)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

	disabled := postMCPForInterfaceTest(t, router, token, initialize)
	if disabled.Code != http.StatusServiceUnavailable || !strings.Contains(disabled.Body.String(), "mcp_disabled") {
		t.Fatalf("disabled MCP = %d %s", disabled.Code, disabled.Body.String())
	}

	app.updateSettings(map[string]any{mcpEnabledSettingKey: true})
	initialized := postMCPForInterfaceTest(t, router, token, initialize)
	if initialized.Code != http.StatusOK || !strings.Contains(initialized.Body.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize MCP = %d %s", initialized.Code, initialized.Body.String())
	}

	listed := postMCPForInterfaceTest(t, router, token, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"query_logs"`) {
		t.Fatalf("list MCP tools = %d %s", listed.Code, listed.Body.String())
	}

	called := postMCPForInterfaceTest(t, router, token, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"query_logs","arguments":{"ip":"192.0.2.1","page_size":10}}}`)
	if called.Code != http.StatusOK {
		t.Fatalf("call MCP tool = %d %s", called.Code, called.Body.String())
	}
	var response struct {
		Result struct {
			StructuredContent QueryResponse `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.NewDecoder(called.Body).Decode(&response); err != nil {
		t.Fatalf("decode MCP tool response: %v", err)
	}
	if response.Result.StructuredContent.Total != 0 || response.Result.StructuredContent.PageSize != 50 {
		t.Fatalf("unexpected MCP query response: %#v", response.Result.StructuredContent)
	}
}

func TestMCPRequiresBearerAuthentication(t *testing.T) {
	app := NewApp(config.LoadConfig())
	app.updateSettings(map[string]any{mcpEnabledSettingKey: true})
	res := postMCPForInterfaceTest(t, app.Router(), "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("MCP without bearer status = %d, want 401; body = %s", res.Code, res.Body.String())
	}
}

func TestMCPClientLoginReturnsBearerToken(t *testing.T) {
	app := NewApp(config.LoadConfig())
	hash, err := HashPassword("admin")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	app.mu.Lock()
	app.passwordHash = hash
	app.mu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"mode":"password","password":"admin","client":"mcp"}`))
	res := httptest.NewRecorder()
	app.loginHandler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("MCP client login = %d %s", res.Code, res.Body.String())
	}
	var response SessionResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatalf("decode MCP login response: %v", err)
	}
	if response.AccessToken == "" || response.TokenType != "Bearer" {
		t.Fatalf("MCP login should expose bearer token: %#v", response)
	}
}

func TestMCPTokenDownloadCreatesMCPOnlyCredential(t *testing.T) {
	app := NewApp(config.LoadConfig())
	app.updateSettings(map[string]any{mcpEnabledSettingKey: true})
	router := app.Router()
	adminToken := bearerTokenForInterfaceTest(t, app)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp/token", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("download MCP token = %d %s", res.Code, res.Body.String())
	}
	var response struct {
		AccessToken string    `json:"access_token"`
		TokenType   string    `json:"token_type"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.AccessToken == "" || response.TokenType != "Bearer" || time.Until(response.ExpiresAt) < 29*24*time.Hour {
		t.Fatalf("unexpected MCP token response: %#v", response)
	}

	listed := postMCPForInterfaceTest(t, router, response.AccessToken, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"query_logs"`) {
		t.Fatalf("MCP-only token cannot list tools: %d %s", listed.Code, listed.Body.String())
	}

	settingsReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	settingsReq.Header.Set("Authorization", "Bearer "+response.AccessToken)
	settingsRes := httptest.NewRecorder()
	router.ServeHTTP(settingsRes, settingsReq)
	if settingsRes.Code != http.StatusUnauthorized {
		t.Fatalf("MCP-only token accessed settings: %d %s", settingsRes.Code, settingsRes.Body.String())
	}
}
