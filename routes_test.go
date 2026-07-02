package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 这些路由测试依赖 `web/dist/index.html` 和打包后的 `/assets/*` 文件；
// 干净检出环境下请先执行 `cd web && npm.cmd run build`，再运行 `go test ./...`。
func TestRouterRegistersAPIRoutes(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/query"},
		{method: http.MethodGet, path: "/api/health-dashboard"},
		{method: http.MethodGet, path: "/api/ingest-progress"},
		{method: http.MethodGet, path: "/api/session"},
		{method: http.MethodPost, path: "/api/login"},
		{method: http.MethodPost, path: "/api/logout"},
		{method: http.MethodPost, path: "/api/password"},
		{method: http.MethodPost, path: "/api/ip-data/reload"},
		{method: http.MethodGet, path: "/api/settings"},
		{method: http.MethodPost, path: "/api/settings"},
		{method: http.MethodPost, path: "/api/sync"},
		{method: http.MethodPost, path: "/api/rebuild"},
		{method: http.MethodPost, path: "/api/export"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code == http.StatusNotFound {
			t.Fatalf("%s %s should be registered", tt.method, tt.path)
		}
	}
}

func TestRouterServesEmbeddedSPAForNonAPIRoutes(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	for _, path := range []string{"/", "/dashboard", "/workbench/overview"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, res.Code, res.Body.String())
		}

		body := res.Body.String()
		if !strings.Contains(body, "<title>FWLOG V3</title>") {
			t.Fatalf("%s should serve the embedded frontend index.html, body = %s", path, body)
		}
		if strings.Contains(body, "QJKJ Team") {
			t.Fatalf("%s should not serve legacy assets/index.html, body = %s", path, body)
		}
	}
}

func TestRouterServesEmbeddedFrontendAssets(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRes := httptest.NewRecorder()
	router.ServeHTTP(indexRes, indexReq)
	if indexRes.Code != http.StatusOK {
		t.Fatalf("index status = %d, body = %s", indexRes.Code, indexRes.Body.String())
	}

	assetPath := findEmbeddedAssetPath(t, indexRes.Body.String())
	req := httptest.NewRequest(http.MethodGet, assetPath, nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); !strings.Contains(contentType, "text/css") && !strings.Contains(contentType, "javascript") {
		t.Fatalf("unexpected content type %q", contentType)
	}
}

func TestRouterSessionLifecycle(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	assertSession := func(cookie *http.Cookie, want bool) {
		t.Helper()

		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("session status = %d, body = %s", res.Code, res.Body.String())
		}

		var payload SessionResponse
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode session response: %v", err)
		}
		if payload.Authenticated != want {
			t.Fatalf("session authenticated = %t, want %t", payload.Authenticated, want)
		}
	}

	assertSession(nil, false)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	router.ServeHTTP(loginRes, loginReq)

	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRes.Code, loginRes.Body.String())
	}

	var loginPayload SessionResponse
	if err := json.NewDecoder(loginRes.Body).Decode(&loginPayload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !loginPayload.Authenticated {
		t.Fatalf("login response = %#v, want authenticated", loginPayload)
	}

	loginCookie := loginRes.Result().Cookies()
	if len(loginCookie) == 0 {
		t.Fatal("login should set session cookie")
	}

	assertSession(loginCookie[0], true)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(loginCookie[0])
	logoutRes := httptest.NewRecorder()
	router.ServeHTTP(logoutRes, logoutReq)
	if logoutRes.Code != http.StatusOK {
		t.Fatalf("logout status = %d, body = %s", logoutRes.Code, logoutRes.Body.String())
	}

	assertSession(nil, false)
}

func TestRouterPasswordChangeRequiresCurrentPassword(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	router.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRes.Code, loginRes.Body.String())
	}
	cookies := loginRes.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login should set session cookie")
	}

	badChangeReq := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString(`{"current_password":"wrong","new_password":"next-admin"}`))
	badChangeReq.Header.Set("Content-Type", "application/json")
	badChangeReq.AddCookie(cookies[0])
	badChangeRes := httptest.NewRecorder()
	router.ServeHTTP(badChangeRes, badChangeReq)
	if badChangeRes.Code != http.StatusBadRequest {
		t.Fatalf("bad password change status = %d, body = %s", badChangeRes.Code, badChangeRes.Body.String())
	}

	changeReq := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString(`{"current_password":"admin","new_password":"next-admin"}`))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(cookies[0])
	changeRes := httptest.NewRecorder()
	router.ServeHTTP(changeRes, changeReq)
	if changeRes.Code != http.StatusOK {
		t.Fatalf("password change status = %d, body = %s", changeRes.Code, changeRes.Body.String())
	}

	oldLoginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	oldLoginReq.Header.Set("Content-Type", "application/json")
	oldLoginRes := httptest.NewRecorder()
	router.ServeHTTP(oldLoginRes, oldLoginReq)
	if oldLoginRes.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, body = %s", oldLoginRes.Code, oldLoginRes.Body.String())
	}

	newLoginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"next-admin"}`))
	newLoginReq.Header.Set("Content-Type", "application/json")
	newLoginRes := httptest.NewRecorder()
	router.ServeHTTP(newLoginRes, newLoginReq)
	if newLoginRes.Code != http.StatusOK {
		t.Fatalf("new password login status = %d, body = %s", newLoginRes.Code, newLoginRes.Body.String())
	}
}

func TestRouterPasswordChangeReturnsUnauthenticatedJSONWhenSessionMissing(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString(`{"current_password":"admin","new_password":"next-admin"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unauthenticated password change status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json", contentType)
	}

	var payload SessionResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode unauthenticated password response: %v", err)
	}
	if payload.Authenticated {
		t.Fatalf("unauthenticated password change response = %#v, want authenticated=false", payload)
	}
}

func TestRouterLegacyAPIsReturnNotFound(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	for _, path := range []string{"/api/stats", "/api/dashboard"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		if res.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404, body = %s", path, res.Code, res.Body.String())
		}
	}
}

func TestRouterAPIRootDoesNotFallBackToSPA(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code == http.StatusOK {
		t.Fatalf("/api should not return 200 index, body = %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "<title>FWLOG V3</title>") {
		t.Fatalf("/api should not fall back to SPA index, body = %s", res.Body.String())
	}
}

func TestRouterReloadIPDataReturnsFailureStatusWhenCustomMapLoadFails(t *testing.T) {
	app := NewApp(LoadConfig())
	app.updateSettings(map[string]any{
		"custom_ip_map_path": "Z:/missing/custom.csv",
		"ip_map_enabled":     true,
		"geoip_enabled":      false,
	})
	router := app.Router()
	originalEngine := app.ipEngine

	req := httptest.NewRequest(http.MethodPost, "/api/ip-data/reload", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("reload status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload IPDataStatus
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode reload response: %v", err)
	}
	if payload.Loaded {
		t.Fatalf("reload response should not claim success: %#v", payload)
	}
	if payload.Error == "" {
		t.Fatalf("reload response should include error: %#v", payload)
	}
	if app.ipEngine != originalEngine {
		t.Fatal("failed reload should keep the previous engine")
	}
}

func TestRouterReloadIPDataLoadsCustomMapFromCurrentSettings(t *testing.T) {
	app := NewApp(LoadConfig())
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "custom.csv")
	if err := os.WriteFile(mapPath, []byte("10.0.0.1,办公终端,内网\n"), 0o600); err != nil {
		t.Fatalf("write custom map: %v", err)
	}

	app.updateSettings(map[string]any{
		"custom_ip_map_path": mapPath,
		"ip_map_enabled":     true,
		"geoip_enabled":      false,
	})
	router := app.Router()

	req := httptest.NewRequest(http.MethodPost, "/api/ip-data/reload", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("reload status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload IPDataStatus
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode reload response: %v", err)
	}
	if !payload.Loaded || payload.Error != "" {
		t.Fatalf("reload response = %#v", payload)
	}

	tag := app.ipEngine.GetTag("10.0.0.1")
	if tag.Label != "办公终端" || tag.Location != "内网" || !tag.IsManual {
		t.Fatalf("custom tag not loaded into app engine: %#v", tag)
	}
}

func findEmbeddedAssetPath(t *testing.T, indexHTML string) string {
	t.Helper()

	re := regexp.MustCompile(`/(assets/[^"' ]+\.(?:css|js))`)
	matches := re.FindStringSubmatch(indexHTML)
	if len(matches) != 2 {
		t.Fatalf("no embedded asset path found in index.html: %s", indexHTML)
	}
	return "/" + matches[1]
}
