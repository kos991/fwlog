package handlers

import (
	"net/http"

	"fwlog/internal/model"
)

type AccessTrendServicer interface {
	AccessTrend(*http.Request) (model.AccessTrendResponse, error)
}

type AccessTrendHandler struct{ service AccessTrendServicer }

func NewAccessTrendHandler(service AccessTrendServicer) http.Handler {
	return AccessTrendHandler{service: service}
}

func (h AccessTrendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.AccessTrend(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "access_trend_error", "message": err.Error()})
		return
	}
	writeJSON(w, response)
}
