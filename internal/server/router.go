package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()

	queryHandler := NewQueryHandler(appQueryService{app: a})
	dashboardHandler := NewHealthDashboardHandler(appDashboardService{app: a})
	dashboardSummaryHandler := NewDashboardSummaryHandler(appDashboardService{app: a})
	dashboardRankingsHandler := NewDashboardRankingsHandler(appDashboardService{app: a})
	progressHandler := NewIngestProgressHandler(appDashboardService{app: a})
	passwordHandler := NewPasswordHandler(appSecurityService{app: a})
	ipReloadHandler := NewIPDataReloadHandler(appSecurityService{app: a})
	threatIntelligenceHandler := threatIntelligenceHandler(a)

	mux.Handle("/api/query", methodHandler(http.MethodGet, a.requireAuth(queryHandler)))
	mux.Handle("/api/health-dashboard", methodHandler(http.MethodGet, a.requireAuth(dashboardHandler)))
	mux.Handle("/api/health-dashboard/summary", methodHandler(http.MethodGet, a.requireAuth(dashboardSummaryHandler)))
	mux.Handle("/api/health-dashboard/rankings", methodHandler(http.MethodGet, a.requireAuth(dashboardRankingsHandler)))
	mux.Handle("/api/ingest-progress", methodHandler(http.MethodGet, a.requireAuth(progressHandler)))
	mux.Handle("/api/password", methodHandler(http.MethodPost, a.requireAuth(passwordHandler)))
	mux.Handle("/api/ip-data/reload", methodHandler(http.MethodPost, a.requireAuth(ipReloadHandler)))
	mux.Handle("/api/settings", a.requireAuth(settingsHandler(a)))
	mux.Handle("/api/receiver/status", methodHandler(http.MethodGet, a.requireAuth(a.receiverStatusHandler())))
	mux.Handle("/api/session", methodHandler(http.MethodGet, a.sessionHandler()))
	mux.Handle("/api/login", methodHandler(http.MethodPost, a.loginHandler()))
	mux.Handle("/api/logout", methodHandler(http.MethodPost, a.requireAuth(a.logoutHandler())))
	mux.Handle("/api/sync", methodHandler(http.MethodPost, a.requireAuth(a.importHandler(false))))
	mux.Handle("/api/rebuild", methodHandler(http.MethodPost, a.requireAuth(a.importHandler(true))))
	mux.Handle("/api/export", placeholderHandler(http.MethodPost, "export endpoint is not implemented yet"))
	mux.Handle("/api/upgrade/check", methodHandler(http.MethodGet, a.requireAuth(a.upgradeCheckHandler())))
	mux.Handle("/api/upgrade/status", methodHandler(http.MethodGet, a.requireAuth(a.upgradeStatusHandler())))
	mux.Handle("/api/upgrade/run", methodHandler(http.MethodPost, a.requireAuth(a.upgradeRunHandler())))
	mux.Handle("/api/upgrade/upload", methodHandler(http.MethodPost, a.requireAuth(a.upgradeUploadHandler())))
	mux.Handle("/api/version", methodHandler(http.MethodGet, a.requireAuth(versionHandler())))
	mux.Handle(threatIntelligenceProvidersPath, a.requireAuth(threatIntelligenceHandler))
	mux.Handle(threatIntelligenceProvidersPath+"/", a.requireAuth(threatIntelligenceHandler))

	mux.Handle("/", newStaticHandler())

	return a.loggingMiddleware(mux)
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装 ResponseWriter 以捕获状态码
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		a.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (a *App) Run(ctx context.Context) error {
	if err := a.Connect(ctx); err != nil {
		return err
	}

	a.startAutoScanScheduler(ctx)
	a.startRSyslogArchiveScheduler(ctx)
	a.applyReceiverFromSettings()

	addr := fmt.Sprintf(":%d", a.cfg.Port)

	server := &http.Server{
		Addr:         addr,
		Handler:      a.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	if a.cfg.TLSEnabled {
		if err := ensureSelfSignedCert(a.cfg.TLSCertPath, a.cfg.TLSKeyPath); err != nil {
			return fmt.Errorf("prepare TLS certificate: %w", err)
		}
		a.logger.Info("starting https server", "addr", addr, "cert", a.cfg.TLSCertPath)
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		go func() {
			if err := server.ListenAndServeTLS(a.cfg.TLSCertPath, a.cfg.TLSKeyPath); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	} else {
		a.logger.Info("starting http server", "addr", addr)
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		a.logger.Info("shutting down http server")
		if a.receiver != nil {
			a.receiver.Close()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		a.logger.Error("http server error", "error", err)
		return err
	}
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
