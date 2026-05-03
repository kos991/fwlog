package main

import (
	"database/sql"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildIndexCreatesEmptyTableWhenNoLogFiles(t *testing.T) {
	tempDir := t.TempDir()
	previousConfig := currentConfig()
	previousDB := db
	defer func() {
		config = previousConfig
		db = previousDB
	}()

	config = Config{
		LogDir:              filepath.Join(tempDir, "logs"),
		DBFile:              filepath.Join(tempDir, "index", "nat_logs.duckdb"),
		Port:                defaultPort,
		Workers:             1,
		AutoScanEnabled:     false,
		AutoScanIntervalSec: defaultAutoScanSec,
	}
	if err := os.MkdirAll(filepath.Dir(currentConfig().DBFile), 0755); err != nil {
		t.Fatalf("create db directory: %v", err)
	}

	var err error
	db, err = sql.Open("duckdb", currentConfig().DBFile)
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	if err := ensureRuntimeTables(); err != nil {
		t.Fatalf("ensure runtime tables: %v", err)
	}

	if err := buildIndex(); err != nil {
		t.Fatalf("build index with no logs: %v", err)
	}
	if !tableExists() {
		t.Fatal("expected nat_logs table to exist")
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM nat_logs").Scan(&count); err != nil {
		t.Fatalf("count nat_logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected empty nat_logs table, got %d rows", count)
	}
}

func TestServeIndexServesRefactoredPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", serveIndex)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "FWLOG PRO") {
		t.Fatalf("expected refactored page content, got body prefix %q", body[:min(len(body), 80)])
	}
	if !strings.Contains(body, "/api/dashboard") {
		t.Fatal("expected dashboard API integration in refactored page")
	}
	for _, endpoint := range []string{"/api/settings", "/api/rebuild", "/api/sync", "/api/export"} {
		if !strings.Contains(body, endpoint) {
			t.Fatalf("expected complete operations UI to integrate %s", endpoint)
		}
	}
	for _, externalRef := range []string{"https://", "http://", "cdn."} {
		if strings.Contains(body, externalRef) {
			t.Fatalf("expected offline vendor assets, found external reference %s", externalRef)
		}
	}
	if !strings.Contains(body, "/assets/vendor/vue/vue.global.prod.js") {
		t.Fatal("expected local Vue vendor asset")
	}
	if !strings.Contains(body, "搜索设置") || !strings.Contains(body, "增量同步") {
		t.Fatal("expected settings and incremental sync controls in refactored page")
	}
}

func TestEmbeddedVendorAssetsAvailable(t *testing.T) {
	staticAssets, err := fs.Sub(assets, "assets")
	if err != nil {
		t.Fatalf("create static asset fs: %v", err)
	}

	for _, path := range []string{
		"vendor/vue/vue.global.prod.js",
		"vendor/echarts/echarts.min.js",
		"vendor/element-plus/index.css",
		"vendor/element-plus/index.full.js",
		"vendor/GeoLite2-City.mmdb",
	} {
		file, err := staticAssets.Open(path)
		if err != nil {
			t.Fatalf("open embedded vendor asset %s: %v", path, err)
		}
		_ = file.Close()
	}
}
