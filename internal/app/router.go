package app

import (
	"context"
	"fmt"
	"net/http"
)

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()

	queryHandler := NewQueryHandler(appQueryService{app: a})
	dashboardHandler := NewHealthDashboardHandler(appDashboardService{app: a})
	progressHandler := NewIngestProgressHandler(appDashboardService{app: a})
	passwordHandler := NewPasswordHandler(appSecurityService{app: a})
	ipReloadHandler := NewIPDataReloadHandler(appSecurityService{app: a})

	mux.Handle("/api/query", methodHandler(http.MethodGet, queryHandler))
	mux.Handle("/api/health-dashboard", methodHandler(http.MethodGet, dashboardHandler))
	mux.Handle("/api/ingest-progress", methodHandler(http.MethodGet, progressHandler))
	mux.Handle("/api/password", methodHandler(http.MethodPost, passwordHandler))
	mux.Handle("/api/ip-data/reload", methodHandler(http.MethodPost, ipReloadHandler))
	mux.Handle("/api/settings", settingsHandler(a))
	mux.Handle("/api/session", methodHandler(http.MethodGet, a.sessionHandler()))
	mux.Handle("/api/login", methodHandler(http.MethodPost, a.loginHandler()))
	mux.Handle("/api/logout", methodHandler(http.MethodPost, a.logoutHandler()))
	mux.Handle("/api/sync", methodHandler(http.MethodPost, a.importHandler(false)))
	mux.Handle("/api/rebuild", methodHandler(http.MethodPost, a.importHandler(true)))
	mux.Handle("/api/export", placeholderHandler(http.MethodPost, "export endpoint is not implemented yet"))
	mux.Handle("/api/upgrade/check", methodHandler(http.MethodGet, a.upgradeCheckHandler()))
	mux.Handle("/api/upgrade/status", methodHandler(http.MethodGet, a.upgradeStatusHandler()))
	mux.Handle("/api/upgrade/run", methodHandler(http.MethodPost, a.upgradeRunHandler()))

	mux.Handle("/", newStaticHandler())
	return mux
}

func (a *App) Run() error {
	ctx := context.Background()
	if err := a.Connect(ctx); err != nil {
		return err
	}

	a.startAutoScanScheduler(ctx)

	addr := fmt.Sprintf(":%d", a.cfg.Port)
	return http.ListenAndServe(addr, a.Router())
}

func methodHandler(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"error":   "method_not_allowed",
				"message": fmt.Sprintf("%s %s is not allowed", r.Method, r.URL.Path),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func placeholderHandler(method, message string) http.Handler {
	return methodHandler(method, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONStatus(w, http.StatusNotImplemented, map[string]any{
			"error":   "not_implemented",
			"message": message,
		})
	}))
}
