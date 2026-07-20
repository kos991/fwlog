package handlers

import (
	"net/http"

	"fwlog/internal/dashboard"
)

type DashboardServicer interface {
	HealthDashboard(r *http.Request) (dashboard.HealthDashboardResponse, error)
	DashboardSummary(r *http.Request) (dashboard.HealthDashboardResponse, error)
	DashboardRankings(r *http.Request) (dashboard.HealthDashboardResponse, error)
	IngestProgress(r *http.Request) (dashboard.IngestProgressResponse, error)
}

type HealthDashboardServicer interface {
	HealthDashboard(r *http.Request) (dashboard.HealthDashboardResponse, error)
}

type DashboardSummaryServicer interface {
	DashboardSummary(r *http.Request) (dashboard.HealthDashboardResponse, error)
}

type DashboardRankingsServicer interface {
	DashboardRankings(r *http.Request) (dashboard.HealthDashboardResponse, error)
}

type IngestProgressServicer interface {
	IngestProgress(r *http.Request) (dashboard.IngestProgressResponse, error)
}

type HealthDashboardHandler struct {
	service HealthDashboardServicer
}

type DashboardSummaryHandler struct{ service DashboardSummaryServicer }

type DashboardRankingsHandler struct{ service DashboardRankingsServicer }

type IngestProgressHandler struct {
	service IngestProgressServicer
}

func NewHealthDashboardHandler(service HealthDashboardServicer) http.Handler {
	return HealthDashboardHandler{service: service}
}

func NewDashboardSummaryHandler(service DashboardSummaryServicer) http.Handler {
	return DashboardSummaryHandler{service: service}
}

func NewDashboardRankingsHandler(service DashboardRankingsServicer) http.Handler {
	return DashboardRankingsHandler{service: service}
}

func NewIngestProgressHandler(service IngestProgressServicer) http.Handler {
	return IngestProgressHandler{service: service}
}

func (h HealthDashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.HealthDashboard(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":   "dashboard_error",
			"message": "仪表盘数据加载失败",
		})
		return
	}
	writeJSON(w, response)
}

func (h DashboardSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.DashboardSummary(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "dashboard_summary_error", "message": "数据概览加载失败"})
		return
	}
	writeJSON(w, response)
}

func (h DashboardRankingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.DashboardRankings(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "dashboard_rankings_error", "message": "流量排行加载失败"})
		return
	}
	writeJSON(w, response)
}

func (h IngestProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.IngestProgress(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":   "ingest_progress_error",
			"message": "入库进度加载失败",
		})
		return
	}
	writeJSON(w, response)
}
