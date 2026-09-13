package server

import (
	"net/http"
	"strings"
)

type setupRequest struct {
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

func (a *App) setupStatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		a.mu.RLock()
		required := !looksLikePasswordHash(a.passwordHash)
		a.mu.RUnlock()
		writeJSON(w, map[string]any{"required": required, "username": "admin"})
	})
}

func (a *App) setupHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload setupRequest
		if err := decodeJSONBodyLimit(r, &payload, 4*1024); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "message": err.Error()})
			return
		}
		if payload.Password != payload.ConfirmPassword {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "password_mismatch", "message": "两次输入的密码不一致"})
			return
		}
		if err := validateNewAdminPassword(payload.Password); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_password", "message": err.Error()})
			return
		}
		if strings.TrimSpace(payload.Password) == "" {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_password", "message": "密码不能为空"})
			return
		}

		a.setupMu.Lock()
		defer a.setupMu.Unlock()
		a.mu.RLock()
		alreadyInitialized := looksLikePasswordHash(a.passwordHash)
		a.mu.RUnlock()
		if alreadyInitialized {
			writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "already_initialized", "message": "管理员已初始化"})
			return
		}
		hash, err := HashPassword(payload.Password)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "password_hash_failed", "message": "管理员密码处理失败"})
			return
		}
		if err := a.saveAdminPasswordHashStrict(r.Context(), hash); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "password_persist_failed", "message": err.Error()})
			return
		}
		a.mu.Lock()
		a.passwordHash = hash
		a.mu.Unlock()
		writeJSONStatus(w, http.StatusCreated, map[string]any{"created": true, "username": "admin"})
	})
}
