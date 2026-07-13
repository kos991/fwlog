package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQueryHandlerReturnsQueryResponseContract(t *testing.T) {
	visibility := QueryVisibility{
		Partial: true,
		Message: "所选时间包含未完成入库日期，已自动只查询已入库部分。",
		QueriedRanges: []VisibleRange{
			{
				LogDate:   dateOnly(2026, 6, 30),
				StartTime: time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local),
				EndTime:   time.Date(2026, 6, 30, 10, 0, 0, 0, time.Local),
				Status:    StatusImporting,
			},
		},
	}

	handler := NewQueryHandler(fakeQueryService{
		response: QueryResponse{
			Records: []map[string]any{
				{"id": float64(1), "src_ip": "10.0.0.1"},
			},
			Total:       1,
			Page:        3,
			PageSize:    25,
			NextCursor:  "next-token",
			HasMore:     true,
			QueryTimeMS: 18,
			Visibility:  visibility,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/query", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for key := range map[string]struct{}{
		"records":       {},
		"total":         {},
		"page":          {},
		"page_size":     {},
		"next_cursor":   {},
		"has_more":      {},
		"query_time_ms": {},
		"visibility":    {},
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("response missing key %q: %#v", key, payload)
		}
	}
	if payload["total"].(float64) != 1 || payload["page"].(float64) != 3 || payload["page_size"].(float64) != 25 || payload["query_time_ms"].(float64) != 18 {
		t.Fatalf("unexpected pagination payload: %#v", payload)
	}
	if payload["next_cursor"] != "next-token" || payload["has_more"] != true {
		t.Fatalf("unexpected cursor payload: %#v", payload)
	}
}

func TestQueryHandlerReturnsStructuredQueryError(t *testing.T) {
	handler := NewQueryHandler(fakeQueryService{
		err: &QueryError{
			Code:    "query_busy",
			Message: "查询并发过高，请稍后重试",
			Status:  http.StatusTooManyRequests,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/query", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != "query_busy" || payload["message"] == "" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestHealthDashboardHandlerReturnsDashboardContract(t *testing.T) {
	handler := NewHealthDashboardHandler(fakeDashboardService{
		dashboard: HealthDashboardResponse{
			DataHealth: DataHealth{TotalLogs: 12, ReadyDates: 2},
			IngestHealth: IngestHealth{
				Status:      StatusImporting,
				CurrentDate: "2026-07-01",
			},
			IPDistribution: IPDistribution{
				TopSourceIPs: []DistributionItem{{Name: "10.0.0.1", Value: 10}},
			},
			GeoDistribution: GeoDistribution{
				TopCountries: []DistributionItem{{Name: "中国", Value: 8}},
				GeoIPLoaded:  true,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health-dashboard", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for key := range map[string]struct{}{
		"data_health":      {},
		"ingest_health":    {},
		"ip_distribution":  {},
		"geo_distribution": {},
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("response missing key %q: %#v", key, payload)
		}
	}
}

func TestIngestProgressHandlerReturnsProgressContract(t *testing.T) {
	handler := NewIngestProgressHandler(fakeDashboardService{
		progress: IngestProgressResponse{
			Status:       StatusImporting,
			CurrentDate:  "2026-07-01",
			CurrentFile:  "fw.log-20260701.gz",
			FilesTotal:   12,
			FilesDone:    5,
			RowsImported: 100,
			Dates: []DateIngestState{
				{LogDate: dateOnly(2026, 7, 1), Status: StatusImporting},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ingest-progress", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for key := range map[string]struct{}{
		"status":        {},
		"current_date":  {},
		"current_file":  {},
		"files_total":   {},
		"files_done":    {},
		"rows_imported": {},
		"dates":         {},
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("response missing key %q: %#v", key, payload)
		}
	}
}

func TestPasswordHandlerReturnsUpdatedSessionContract(t *testing.T) {
	handler := NewPasswordHandler(fakeSecurityService{session: SessionResponse{Authenticated: true}})

	req := httptest.NewRequest(http.MethodPost, "/api/password", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["authenticated"] != true {
		t.Fatalf("password response should include session state: %#v", payload)
	}
}

func TestIPDataReloadHandlerReturnsStatusContract(t *testing.T) {
	handler := NewIPDataReloadHandler(fakeSecurityService{
		ipStatus: IPDataStatus{
			Loaded:        true,
			CustomMapPath: "/opt/nat/custom.csv",
			GeoIPDBPath:   "/data/GeoLite2-City.mmdb",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ip-data/reload", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for key := range map[string]struct{}{
		"loaded":             {},
		"custom_ip_map_path": {},
		"geoip_db_path":      {},
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("response missing key %q: %#v", key, payload)
		}
	}
}

type fakeQueryService struct {
	response QueryResponse
	err      error
}

func (f fakeQueryService) Query(_ *http.Request) (QueryResponse, error) {
	return f.response, f.err
}

type fakeDashboardService struct {
	dashboard HealthDashboardResponse
	progress  IngestProgressResponse
	err       error
}

func (f fakeDashboardService) HealthDashboard(_ *http.Request) (HealthDashboardResponse, error) {
	return f.dashboard, f.err
}

func (f fakeDashboardService) IngestProgress(_ *http.Request) (IngestProgressResponse, error) {
	return f.progress, f.err
}

type fakeSecurityService struct {
	session  SessionResponse
	ipStatus IPDataStatus
	err      error
}

func (f fakeSecurityService) ChangePassword(_ *http.Request) (SessionResponse, error) {
	return f.session, f.err
}

func (f fakeSecurityService) ReloadIPData(_ *http.Request) (IPDataStatus, error) {
	return f.ipStatus, f.err
}
