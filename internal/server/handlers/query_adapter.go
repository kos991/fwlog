package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"fwlog/internal/query"
)

type QueryServicer interface {
	Query(r *http.Request) (query.QueryResponse, error)
}

type QueryHandler struct {
	service QueryServicer
}

func NewQueryHandler(service QueryServicer) http.Handler {
	return QueryHandler{service: service}
}

func (h QueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.Query(r)
	if err != nil {
		var queryErr *query.QueryError
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
	_ = json.NewEncoder(w).Encode(response)
}
