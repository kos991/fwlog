package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetupStatusAndCreateAdmin(t *testing.T) {
	app := NewApp(Config{})
	app.mu.Lock()
	app.passwordHash = ""
	app.mu.Unlock()
	app.settingsSaver = func(context.Context, map[string]string) error { return nil }
	router := app.Router()
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	var payload map[string]any
	if err := json.NewDecoder(status.Body).Decode(&payload); err != nil || payload["required"] != true {
		t.Fatalf("initial setup status = %d %#v", status.Code, payload)
	}
	create := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewBufferString(`{"password":"first-admin","confirm_password":"first-admin"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, req)
	if create.Code != http.StatusCreated || !app.verifyPassword("first-admin") {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	second := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewBufferString(`{"password":"second-admin","confirm_password":"second-admin"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(second, req)
	if second.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d", second.Code)
	}
}

func TestSetupRejectsMismatchAndShortPassword(t *testing.T) {
	app := NewApp(Config{})
	app.mu.Lock()
	app.passwordHash = ""
	app.mu.Unlock()
	app.settingsSaver = func(context.Context, map[string]string) error { return nil }
	for _, body := range []string{`{"password":"short","confirm_password":"different"}`, `{"password":"short","confirm_password":"short"}`} {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		app.Router().ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("setup status = %d body=%s", res.Code, res.Body.String())
		}
	}
}
