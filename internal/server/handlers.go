package server

import (
	"encoding/json"
	"net/http"

	serverhandlers "fwlog/internal/server/handlers"
)

func NewQueryHandler(service serverhandlers.QueryServicer) http.Handler {
	return serverhandlers.NewQueryHandler(service)
}

func NewHealthDashboardHandler(service serverhandlers.DashboardServicer) http.Handler {
	return serverhandlers.NewHealthDashboardHandler(service)
}

func NewIngestProgressHandler(service serverhandlers.DashboardServicer) http.Handler {
	return serverhandlers.NewIngestProgressHandler(service)
}

func NewPasswordHandler(service serverhandlers.SecurityServicer) http.Handler {
	return serverhandlers.NewPasswordHandler(service)
}

func NewIPDataReloadHandler(service serverhandlers.SecurityServicer) http.Handler {
	return serverhandlers.NewIPDataReloadHandler(service)
}

func writeJSON(w http.ResponseWriter, payload any) {
	writeJSONStatus(w, http.StatusOK, payload)
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
