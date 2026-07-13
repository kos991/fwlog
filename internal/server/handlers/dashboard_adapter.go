package handlers

import (
	"net/http"

	"fwlog/internal/dashboard"
)

type DashboardServicer interface {
	HealthDashboard(r *http.Request) (dashboard.HealthDashboardResponse, error)
	IngestProgress(r *http.Request) (dashboard.IngestProgressResponse, error)
}

type HealthDashboardHandler struct {
	service DashboardServicer
}

type IngestProgressHandler struct {
	service DashboardServicer
}

func NewHealthDashboardHandler(service DashboardServicer) http.Handler {
	return HealthDashboardHandler{service: service}
}

func NewIngestProgressHandler(service DashboardServicer) http.Handler {
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
