package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// 这些路由测试依赖 `web/dist/index.html` 和打包后的 `/assets/*` 文件；
// 干净检出环境下请先执行 `cd web && npm run build`，再运行 `go test ./...`。
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
		{method: http.MethodGet, path: "/api/upgrade/check"},
		{method: http.MethodGet, path: "/api/upgrade/status"},
		{method: http.MethodPost, path: "/api/upgrade/run"},
		{method: http.MethodPost, path: "/api/upgrade/upload"},
		{method: http.MethodGet, path: "/api/version"},
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
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
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

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRes := httptest.NewRecorder()
	router.ServeHTTP(sessionRes, sessionReq)
	if sessionRes.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionRes.Code, sessionRes.Body.String())
	}
	var sessionPayload SessionResponse
	if err := json.NewDecoder(sessionRes.Body).Decode(&sessionPayload); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if sessionPayload.Authenticated {
		t.Fatalf("password change should clear current session: %#v", sessionPayload)
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

func TestRouterPasswordChangeRejectsInvalidNewPasswordPolicy(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	router.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRes.Code, loginRes.Body.String())
	}
	cookie := loginRes.Result().Cookies()[0]

	cases := []string{
		`{"current_password":"admin","new_password":"short"}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("password change status = %d, want 400, body = %s", res.Code, res.Body.String())
		}
	}
}

func TestRouterPasswordChangeAcceptsAnyCharactersAtLeastSixLong(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	router := app.Router()

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	router.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRes.Code, loginRes.Body.String())
	}
	cookie := loginRes.Result().Cookies()[0]

	newPassword := "复杂 密码@2026! this-password-is-long"
	body, err := json.Marshal(map[string]string{
		"current_password": "admin",
		"new_password":     newPassword,
	})
	if err != nil {
		t.Fatalf("marshal password payload: %v", err)
	}
	changeReq := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewReader(body))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(cookie)
	changeRes := httptest.NewRecorder()
	router.ServeHTTP(changeRes, changeReq)
	if changeRes.Code != http.StatusOK {
		t.Fatalf("password change status = %d, body = %s", changeRes.Code, changeRes.Body.String())
	}

	newLoginBody, err := json.Marshal(map[string]string{"password": newPassword})
	if err != nil {
		t.Fatalf("marshal login payload: %v", err)
	}
	newLoginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(newLoginBody))
	newLoginReq.Header.Set("Content-Type", "application/json")
	newLoginRes := httptest.NewRecorder()
	router.ServeHTTP(newLoginRes, newLoginReq)
	if newLoginRes.Code != http.StatusOK {
		t.Fatalf("new password login status = %d, body = %s", newLoginRes.Code, newLoginRes.Body.String())
	}
}
func TestAppUsesPersistedAdminPasswordHash(t *testing.T) {
	app := NewApp(LoadConfig())
	passwordHash, err := HashPassword("saved-admin-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	app.applySavedSettings(map[string]string{
		adminPasswordHashSettingKey: passwordHash,
	})

	if app.verifyPassword("admin") {
		t.Fatal("default admin password should not work after loading persisted password hash")
	}
	if !app.verifyPassword("saved-admin-password") {
		t.Fatal("persisted password hash should be used for login")
	}
	if _, ok := app.getSettings()[adminPasswordHashSettingKey]; ok {
		t.Fatal("admin password hash must not be returned by /api/settings")
	}
}

func TestAppIgnoresMalformedPersistedAdminPasswordHash(t *testing.T) {
	app := NewApp(LoadConfig())

	app.applySavedSettings(map[string]string{
		adminPasswordHashSettingKey: "not-a-password-hash",
	})

	if !app.verifyPassword("admin") {
		t.Fatal("malformed persisted password hash should not replace the default admin password")
	}
}

func TestRouterPasswordChangeReturnsUnauthenticatedJSONWhenSessionMissing(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString(`{"current_password":"admin","new_password":"next-admin"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated password change status = %d, want 401, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json", contentType)
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

func TestIngestProgressSinceSupportsAllRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/ingest-progress?range=all", nil)
	got := ingestProgressSince(req)
	want := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ingestProgressSince(range=all) = %v, want %v", got, want)
	}
}

func TestIngestProgressSinceDefaultsToAllProblemDates(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/ingest-progress", nil)
	got := ingestProgressSince(req)
	want := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ingestProgressSince(default) = %v, want %v", got, want)
	}
}

func TestDashboardMetricsSinceCanUseSeparateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health-dashboard?range=all&metrics_range=30d", nil)
	got := dashboardMetricsSince(req)
	all := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	if got.Equal(all) {
		t.Fatalf("dashboardMetricsSince should use metrics_range instead of range=all")
	}
	if time.Since(got) < 29*24*time.Hour || time.Since(got) > 31*24*time.Hour {
		t.Fatalf("dashboardMetricsSince = %v, want about 30 days ago", got)
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

func TestUpdateSettingsStoresStructuredValuesAsJSON(t *testing.T) {
	app := NewApp(LoadConfig())
	app.updateSettings(map[string]any{
		"log_sources": []any{
			map[string]any{
				"source_id": "fw-a",
				"log_tag":   "出口防火墙",
				"log_dir":   "/data/fw-a",
				"enabled":   true,
			},
		},
	})

	raw := app.getSettings()["log_sources"]
	if strings.Contains(raw, "map[") {
		t.Fatalf("structured settings must not be stored as Go fmt strings: %q", raw)
	}

	var sources []LogSource
	if err := json.Unmarshal([]byte(raw), &sources); err != nil {
		t.Fatalf("log_sources should be valid JSON, got %q: %v", raw, err)
	}
	if len(sources) != 1 || sources[0].SourceID != "fw-a" || sources[0].LogDir != "/data/fw-a" {
		t.Fatalf("decoded sources = %#v", sources)
	}
}

func TestUpdateSettingsNormalizesRSyslogLogSources(t *testing.T) {
	app := NewApp(LoadConfig())
	app.updateSettings(map[string]any{
		"log_sources": []any{
			map[string]any{
				"source_id":   "rsyslog-main",
				"log_tag":     "核心防火墙",
				"source_type": "rsyslog",
				"enabled":     true,
			},
		},
	})

	var sources []LogSource
	raw := app.getSettings()["log_sources"]
	if err := json.Unmarshal([]byte(raw), &sources); err != nil {
		t.Fatalf("log_sources should be valid JSON, got %q: %v", raw, err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources = %#v", sources)
	}
	source := sources[0]
	if source.SourceType != "rsyslog" || source.ListenProtocol != "udp" || source.ListenHost != "0.0.0.0" || source.ListenPort != 5514 {
		t.Fatalf("rsyslog defaults not applied: %#v", source)
	}
	if source.SpoolDir != "/data/fwlog/received/rsyslog-main" || source.LogDir != source.SpoolDir {
		t.Fatalf("rsyslog spool dir not applied: %#v", source)
	}
}

func TestRouterSettingsSaveDoesNotStartImportForEnabledLogSources(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()

	started := make(chan struct{}, 1)
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool) ([]string, []string, error) {
		started <- struct{}{}
		return []string{"2026-07-01"}, nil, nil
	}
	router := app.Router()
	cookie := loginForTest(t, router)

	body := `{"log_sources":[{"source_id":"fw-a","log_tag":"edge-a","log_dir":"/data/fw-a","enabled":true},{"source_id":"fw-b","log_tag":"edge-b","log_dir":"/data/fw-b","enabled":false}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body = %s", res.Code, res.Body.String())
	}

	select {
	case <-started:
		t.Fatal("saving settings must not start background import")
	case <-time.After(100 * time.Millisecond):
	}

	settings := app.getSettings()
	if !strings.Contains(settings["log_sources"], `"source_id":"fw-a"`) {
		t.Fatalf("settings should persist log_sources, got %q", settings["log_sources"])
	}
}

func TestRouterSettingsAppliesRSyslogReceiver(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	router := app.Router()
	cookie := loginForTest(t, router)

	port := freeServerUDPPort(t)
	spoolDir := filepath.ToSlash(filepath.Join(t.TempDir(), "rsyslog-main"))
	body := fmt.Sprintf(`{"log_sources":[{"source_id":"rsyslog-main","log_tag":"核心防火墙","source_type":"rsyslog","listen_protocol":"udp","listen_host":"127.0.0.1","listen_port":%d,"spool_dir":%q,"enabled":true}]}`, port, spoolDir)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body = %s", res.Code, res.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/receiver/status", nil)
	statusReq.AddCookie(cookie)
	statusRes := httptest.NewRecorder()
	router.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("receiver status = %d, body = %s", statusRes.Code, statusRes.Body.String())
	}
	var payload map[string]map[string]any
	if err := json.Unmarshal(statusRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode receiver status: %v", err)
	}
	status := payload["rsyslog-main"]
	if status["running"] != true || status["error"] != "" {
		t.Fatalf("receiver should be running after settings save: %#v", status)
	}
}

func TestNormalizeListenProtocolPreservesTCP(t *testing.T) {
	if got := normalizeListenProtocol(" TCP "); got != "tcp" {
		t.Fatalf("normalizeListenProtocol(TCP) = %q, want tcp", got)
	}
}

func TestRouterSyncUsesAllEnabledLogSources(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{
		"log_sources": []any{
			map[string]any{"source_id": "fw-a", "log_tag": "edge-a", "log_dir": "/data/fw-a", "enabled": true},
			map[string]any{"source_id": "fw-b", "log_tag": "edge-b", "log_dir": "/data/fw-b", "enabled": true},
			map[string]any{"source_id": "fw-c", "log_tag": "edge-c", "log_dir": "/data/fw-c", "enabled": false},
		},
	})

	importedSources := make(chan string, 2)
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, source LogSource, _ bool) ([]string, []string, error) {
		importedSources <- source.SourceID
		return []string{"2026-07-01"}, nil, nil
	}
	router := app.Router()
	cookie := loginForTest(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/sync", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("sync status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"message":"入库任务已开始"`) {
		t.Fatalf("sync response should include a clear user message: %s", res.Body.String())
	}
	got := make([]string, 0, 2)
	for len(got) < 2 {
		select {
		case sourceID := <-importedSources:
			got = append(got, sourceID)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for enabled sources, got %#v", got)
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "fw-a,fw-b" {
		t.Fatalf("imported sources = %#v", got)
	}
}

func freeServerUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate udp port: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func TestRouterSyncReturnsInProgressWhenBackgroundImportIsRunning(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()

	started := make(chan struct{})
	release := make(chan struct{})
	runs := 0
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, _ LogSource, _ bool) ([]string, []string, error) {
		runs++
		if runs == 1 {
			close(started)
			<-release
		}
		return nil, nil, nil
	}
	router := app.Router()
	cookie := loginForTest(t, router)

	app.updateSettings(map[string]any{
		"log_sources": []any{
			map[string]any{"source_id": "fw-a", "log_tag": "edge-a", "log_dir": "/data/fw-a", "enabled": true},
		},
	})

	firstSyncReq := httptest.NewRequest(http.MethodPost, "/api/sync", nil)
	firstSyncReq.AddCookie(cookie)
	firstSyncRes := httptest.NewRecorder()
	router.ServeHTTP(firstSyncRes, firstSyncReq)
	if firstSyncRes.Code != http.StatusAccepted {
		t.Fatalf("first sync status = %d, body = %s", firstSyncRes.Code, firstSyncRes.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background import did not start")
	}
	defer close(release)

	syncReq := httptest.NewRequest(http.MethodPost, "/api/sync", nil)
	syncReq.AddCookie(cookie)
	syncRes := httptest.NewRecorder()
	router.ServeHTTP(syncRes, syncReq)

	if syncRes.Code != http.StatusAccepted {
		t.Fatalf("sync status = %d, want 202, body = %s", syncRes.Code, syncRes.Body.String())
	}
	if runs != 1 {
		t.Fatalf("sync should not start another import while one is running, runs = %d", runs)
	}
}

func TestRouterSyncSelectsOneSourceAndReportsBusy(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{"log_sources": []any{
		map[string]any{"source_id": "fw-a", "log_dir": "/data/a", "enabled": true},
		map[string]any{"source_id": "fw-b", "log_dir": "/data/b", "enabled": true},
	}})
	started := make(chan string, 1)
	release := make(chan struct{})
	app.importRunner = func(_ context.Context, _ *ClickHouseStore, source LogSource, _ bool) ([]string, []string, error) {
		started <- source.SourceID
		<-release
		return nil, nil, nil
	}
	router := app.Router()
	cookie := loginForTest(t, router)

	first := httptest.NewRequest(http.MethodPost, "/api/sync?source_id=fw-a", nil)
	first.AddCookie(cookie)
	firstRes := httptest.NewRecorder()
	router.ServeHTTP(firstRes, first)
	if firstRes.Code != http.StatusAccepted || !strings.Contains(firstRes.Body.String(), `"accepted_sources":["fw-a"]`) {
		t.Fatalf("first response = %d %s", firstRes.Code, firstRes.Body.String())
	}
	if got := <-started; got != "fw-a" {
		t.Fatalf("started source = %q", got)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/sync?source_id=fw-a", nil)
	second.AddCookie(cookie)
	secondRes := httptest.NewRecorder()
	router.ServeHTTP(secondRes, second)
	if !strings.Contains(secondRes.Body.String(), `"busy_sources":["fw-a"]`) {
		t.Fatalf("busy response = %s", secondRes.Body.String())
	}
	close(release)
}

func TestRouterSyncRejectsUnknownSource(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	app.updateSettings(map[string]any{"log_sources": []any{map[string]any{"source_id": "fw-a", "enabled": true}}})
	router := app.Router()
	cookie := loginForTest(t, router)
	req := httptest.NewRequest(http.MethodPost, "/api/sync?source_id=missing", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest ||
		!strings.Contains(res.Body.String(), "unknown_source") ||
		!strings.Contains(res.Body.String(), "所选日志来源不存在或已停用") {
		t.Fatalf("response = %d %s", res.Code, res.Body.String())
	}
}

func TestRouterSyncExplainsUnavailableLogDatabase(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()
	cookie := loginForTest(t, router)
	req := httptest.NewRequest(http.MethodPost, "/api/sync", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable ||
		!strings.Contains(res.Body.String(), "clickhouse_not_connected") ||
		!strings.Contains(res.Body.String(), "日志数据库尚未连接，请稍后重试") {
		t.Fatalf("response = %d %s", res.Code, res.Body.String())
	}
}

func TestRouterSyncExplainsInvalidDateFormat(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	router := app.Router()
	cookie := loginForTest(t, router)
	req := httptest.NewRequest(http.MethodPost, "/api/sync?date=2026-7-14", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest ||
		!strings.Contains(res.Body.String(), "invalid_date") ||
		!strings.Contains(res.Body.String(), "日期格式无效，请使用 YYYY-MM-DD") {
		t.Fatalf("response = %d %s", res.Code, res.Body.String())
	}
}

func TestParseImportTargetDateAcceptsDateQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date=2026-06-13", nil)

	got, err := parseImportTargetDate(req)
	if err != nil {
		t.Fatalf("parseImportTargetDate returned error: %v", err)
	}

	want := time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local)
	if !got.Start.Equal(want) || !got.End.Equal(want) {
		t.Fatalf("target date = %#v, want %v", got, want)
	}
}

func TestParseImportTargetDateAcceptsDateRangeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date_from=2026-06-13&date_to=2026-06-15", nil)

	got, err := parseImportTargetDate(req)
	if err != nil {
		t.Fatalf("parseImportTargetDate returned error: %v", err)
	}

	wantStart := time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local)
	wantEnd := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	if !got.Start.Equal(wantStart) || !got.End.Equal(wantEnd) {
		t.Fatalf("target range = %#v, want %v - %v", got, wantStart, wantEnd)
	}
}

func TestParseImportTargetDateRejectsInvalidDateQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date=2026/06/13", nil)

	if _, err := parseImportTargetDate(req); err == nil {
		t.Fatal("invalid target date should return an error")
	}
}

func TestParseImportTargetDateRejectsInvertedDateRangeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date_from=2026-06-15&date_to=2026-06-13", nil)

	if _, err := parseImportTargetDate(req); err == nil {
		t.Fatal("inverted target date range should return an error")
	}
}

func TestParseImportTargetDateRejectsMixedDateAndRangeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/rebuild?date=2026-06-13&date_from=2026-06-13&date_to=2026-06-15", nil)

	if _, err := parseImportTargetDate(req); err == nil {
		t.Fatal("mixed date and date range should return an error")
	}
}

func TestTargetDateRangeDatesReturnsInclusiveDays(t *testing.T) {
	target := importTargetDateRange{
		Start: time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local),
	}

	var got []string
	for _, date := range target.Dates() {
		got = append(got, formatDate(date))
	}

	want := []string{"2026-06-13", "2026-06-14", "2026-06-15"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dates = %#v, want %#v", got, want)
	}
}

func TestCurrentLogSourcesDoesNotFallbackWhenConfiguredSourcesAreDisabled(t *testing.T) {
	app := NewApp(LoadConfig())
	app.updateSettings(map[string]any{
		"log_sources": []any{
			map[string]any{"source_id": "fw-a", "log_tag": "edge-a", "log_dir": "/data/fw-a", "enabled": false},
		},
	})

	if sources := app.currentLogSources(); len(sources) != 0 {
		t.Fatalf("disabled configured sources should not fall back to default source: %#v", sources)
	}
}

func TestRouterReloadIPDataIgnoresMissingCustomMap(t *testing.T) {
	app := NewApp(LoadConfig())
	app.updateSettings(map[string]any{
		"custom_ip_map_path": "Z:/missing/custom.csv",
		"ip_map_enabled":     true,
		"geoip_enabled":      false,
	})
	router := app.Router()
	cookie := loginForTest(t, router)
	originalEngine := app.ipEngine

	req := httptest.NewRequest(http.MethodPost, "/api/ip-data/reload", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("reload status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload IPDataStatus
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode reload response: %v", err)
	}
	if !payload.Loaded {
		t.Fatalf("reload response should claim usable engine: %#v", payload)
	}
	if payload.Error != "" {
		t.Fatalf("missing custom map should be non-fatal: %#v", payload)
	}
	if app.ipEngine == originalEngine {
		t.Fatal("reload should install a fresh usable engine")
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
	cookie := loginForTest(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/ip-data/reload", nil)
	req.AddCookie(cookie)
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

func TestProtectedAPIsRejectUnauthenticatedRequests(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	protected := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/query"},
		{method: http.MethodGet, path: "/api/health-dashboard"},
		{method: http.MethodGet, path: "/api/ingest-progress"},
		{method: http.MethodPost, path: "/api/password", body: `{"current_password":"a","new_password":"b"}`},
		{method: http.MethodPost, path: "/api/ip-data/reload"},
		{method: http.MethodGet, path: "/api/settings"},
		{method: http.MethodPost, path: "/api/settings", body: `{}`},
		{method: http.MethodPost, path: "/api/sync"},
		{method: http.MethodPost, path: "/api/rebuild"},
		{method: http.MethodPost, path: "/api/logout"},
		{method: http.MethodGet, path: "/api/upgrade/check"},
		{method: http.MethodGet, path: "/api/upgrade/status"},
		{method: http.MethodPost, path: "/api/upgrade/run", body: `{"version":"v2.0.15"}`},
	}

	for _, tt := range protected {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without session should return 401, got %d, body = %s", tt.method, tt.path, res.Code, res.Body.String())
		}
	}
}

func TestSettingsRejectsAdminPasswordHashKey(t *testing.T) {
	app := NewApp(LoadConfig())
	app.mu.Lock()
	app.store = &ClickHouseStore{}
	app.mu.Unlock()
	router := app.Router()
	cookie := loginForTest(t, router)

	hash, err := HashPassword("attacker-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	body := fmt.Sprintf(`{"admin_password_hash":%q}`, hash)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body = %s", res.Code, res.Body.String())
	}

	if app.verifyPassword("attacker-password") {
		t.Fatal("admin_password_hash must not be injectable via /api/settings")
	}
	if !app.verifyPassword("admin") {
		t.Fatal("default password should still work after rejected hash injection")
	}
}

func TestLoginRateLimitsAfterRepeatedFailures(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	for i := 0; i < maxLoginFailures; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, res.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited login status = %d, want 429, body = %s", res.Code, res.Body.String())
	}
}
