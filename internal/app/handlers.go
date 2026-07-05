package app

import (
	"encoding/json"
	"errors"
	"net/http"
)

type queryServicer interface {
	Query(r *http.Request) (QueryResponse, error)
}

type dashboardServicer interface {
	HealthDashboard(r *http.Request) (HealthDashboardResponse, error)
	IngestProgress(r *http.Request) (IngestProgressResponse, error)
}

type securityServicer interface {
	ChangePassword(r *http.Request) (SessionResponse, error)
	ReloadIPData(r *http.Request) (IPDataStatus, error)
}

type QueryHandler struct {
	service queryServicer
}

type HealthDashboardHandler struct {
	service dashboardServicer
}

type IngestProgressHandler struct {
	service dashboardServicer
}

type PasswordHandler struct {
	service securityServicer
}

type IPDataReloadHandler struct {
	service securityServicer
}

func NewQueryHandler(service queryServicer) http.Handler {
	return QueryHandler{service: service}
}

func NewHealthDashboardHandler(service dashboardServicer) http.Handler {
	return HealthDashboardHandler{service: service}
}

func NewIngestProgressHandler(service dashboardServicer) http.Handler {
	return IngestProgressHandler{service: service}
}

func NewPasswordHandler(service securityServicer) http.Handler {
	return PasswordHandler{service: service}
}

func NewIPDataReloadHandler(service securityServicer) http.Handler {
	return IPDataReloadHandler{service: service}
}

func (h QueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.Query(r)
	if err != nil {
		var queryErr *QueryError
		if errors.As(err, &queryErr) {
			status := queryErr.Status
			if status == 0 {
				status = http.StatusBadRequest
			}
			writeJSONStatus(w, status, queryErr)
			return
		}
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h HealthDashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.HealthDashboard(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, response)
}

func (h IngestProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.IngestProgress(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, response)
}

func (h PasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.ChangePassword(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, response)
}

func (h IPDataReloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.ReloadIPData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, response)
}

func writeJSON(w http.ResponseWriter, payload any) {
	writeJSONStatus(w, http.StatusOK, payload)
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
