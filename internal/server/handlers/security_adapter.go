package handlers

import (
	"net/http"

	"fwlog/internal/ip"
	"fwlog/internal/security"
)

type SecurityServicer interface {
	ChangePassword(r *http.Request) (security.SessionResponse, error)
	ReloadIPData(r *http.Request) (ip.IPDataStatus, error)
}

type PasswordHandler struct {
	service SecurityServicer
}

type IPDataReloadHandler struct {
	service SecurityServicer
}

func NewPasswordHandler(service SecurityServicer) http.Handler {
	return PasswordHandler{service: service}
}

func NewIPDataReloadHandler(service SecurityServicer) http.Handler {
	return IPDataReloadHandler{service: service}
}

func (h PasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.ChangePassword(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":   "password_change_failed",
			"message": err.Error(),
		})
		return
	}
	writeJSON(w, response)
}

func (h IPDataReloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.ReloadIPData(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":   "ip_data_reload_failed",
			"message": "IP 数据重载失败",
		})
		return
	}
	writeJSON(w, response)
}
