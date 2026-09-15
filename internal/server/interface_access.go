package server

import (
	"net/http"
	"time"
)

const (
	externalAPIEnabledSettingKey = "external_api_enabled"
	mcpEnabledSettingKey         = "mcp_enabled"
	mcpTokenScope                = "mcp"
	mcpTokenTTL                  = 30 * 24 * time.Hour
)

func (a *App) interfaceEnabled(key string, fallback bool) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return settingBoolOrFallback(a.settings, key, fallback)
}

func (a *App) requireExternalAPIEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearerToken(r) != "" && !a.interfaceEnabled(externalAPIEnabledSettingKey, true) {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "external_api_disabled",
				"message": "API 接口已停用",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) requireBearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isMCPAuthenticated(r) {
			writeUnauthenticated(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) isMCPAuthenticated(r *http.Request) bool {
	token := bearerToken(r)
	if token == "" {
		return false
	}
	a.mu.RLock()
	manager := a.jwtManager
	a.mu.RUnlock()
	if manager == nil {
		return false
	}
	claims, err := manager.Parse(token, time.Now())
	return err == nil && (claims.Scope == "" || claims.Scope == mcpTokenScope)
}

func (a *App) mcpTokenHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		a.mu.RLock()
		manager := a.jwtManager
		a.mu.RUnlock()
		if manager == nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "session_init_failed", "message": "JWT 管理器未初始化"})
			return
		}
		now := time.Now()
		token, err := manager.IssueScoped("mcp-client", []string{"admin-download"}, mcpTokenScope, now, mcpTokenTTL)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mcp_token_failed", "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_at":   now.Add(mcpTokenTTL).Format(time.RFC3339),
		})
	})
}

func (a *App) requireMCPEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.interfaceEnabled(mcpEnabledSettingKey, false) {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "mcp_disabled",
				"message": "MCP 接口已停用",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
