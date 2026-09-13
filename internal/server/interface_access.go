package server

import "net/http"

const (
	externalAPIEnabledSettingKey = "external_api_enabled"
	mcpEnabledSettingKey         = "mcp_enabled"
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
		if bearerToken(r) == "" || !a.isAuthenticated(r) {
			writeUnauthenticated(w)
			return
		}
		next.ServeHTTP(w, r)
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
