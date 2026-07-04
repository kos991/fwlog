package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidUpgradeVersionRequiresReleaseTag(t *testing.T) {
	for _, version := range []string{"v1.1.0", "v2.0.0"} {
		if !validUpgradeVersion(version) {
			t.Fatalf("%s should be a valid upgrade version", version)
		}
	}
	for _, version := range []string{"", "1.1.0", "latest", "v1", "v1.1.0;reboot"} {
		if validUpgradeVersion(version) {
			t.Fatalf("%s should be rejected as an upgrade version", version)
		}
	}
}

func TestReleaseHasRequiredLinuxUpgradeAssets(t *testing.T) {
	release := githubRelease{
		TagName: "v1.1.0",
		HTMLURL: "https://github.com/kos991/fwlog/releases/tag/v1.1.0",
		Assets: []githubReleaseAsset{
			{Name: linuxUpgradeAssetName, BrowserDownloadURL: "https://example.test/linux"},
			{Name: "nat-query-service.service", BrowserDownloadURL: "https://example.test/service"},
			{Name: "deploy-142-from-release.sh", BrowserDownloadURL: "https://example.test/deploy"},
		},
	}

	assets, missing := releaseUpgradeAssets(release)

	if len(missing) != 0 {
		t.Fatalf("missing assets = %#v", missing)
	}
	if assets.BinaryURL != "https://example.test/linux" || assets.ServiceURL == "" || assets.DeployScriptURL == "" {
		t.Fatalf("assets = %#v", assets)
	}
}

func TestReleaseReportsMissingUpgradeAssets(t *testing.T) {
	_, missing := releaseUpgradeAssets(githubRelease{
		Assets: []githubReleaseAsset{{Name: linuxUpgradeAssetName, BrowserDownloadURL: "https://example.test/linux"}},
	})

	if len(missing) != 2 {
		t.Fatalf("missing assets = %#v, want service and deploy script", missing)
	}
}

func TestUpgradeAPIsRequireSession(t *testing.T) {
	app := NewApp(LoadConfig())
	router := app.Router()

	for _, tt := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/upgrade/check"},
		{method: http.MethodGet, path: "/api/upgrade/status"},
		{method: http.MethodPost, path: "/api/upgrade/run", body: `{"version":"v1.1.0"}`},
	} {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, body = %s", tt.method, tt.path, res.Code, res.Body.String())
		}
	}
}

func TestUpgradeRunRejectsConcurrentTask(t *testing.T) {
	app := NewApp(LoadConfig())
	started := make(chan struct{})
	release := make(chan struct{})
	app.upgradeRunner = func(ctx context.Context, target upgradeTarget) UpgradeStatus {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return UpgradeStatus{State: UpgradeStateSucceeded, TargetVersion: target.Version}
	}
	router := app.Router()
	cookie := loginForTest(t, router)

	firstReq := httptest.NewRequest(http.MethodPost, "/api/upgrade/run", bytes.NewBufferString(`{"version":"v1.1.0"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.AddCookie(cookie)
	firstRes := httptest.NewRecorder()
	router.ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusAccepted {
		t.Fatalf("first run status = %d, body = %s", firstRes.Code, firstRes.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upgrade runner did not start")
	}
	defer close(release)

	secondReq := httptest.NewRequest(http.MethodPost, "/api/upgrade/run", bytes.NewBufferString(`{"version":"v1.1.0"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.AddCookie(cookie)
	secondRes := httptest.NewRecorder()
	router.ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusConflict {
		t.Fatalf("second run status = %d, body = %s", secondRes.Code, secondRes.Body.String())
	}
}

func loginForTest(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", res.Code, res.Body.String())
	}
	var payload SessionResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if !payload.Authenticated {
		t.Fatalf("login payload = %#v", payload)
	}
	cookies := res.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a cookie")
	}
	return cookies[0]
}
