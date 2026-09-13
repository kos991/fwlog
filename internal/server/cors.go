package server

import (
	"net/http"
	"os"
	"strings"
)

// extensionCORSMiddleware permits the MV3 query extension to call the API.
// Chrome extension origins are explicit origins and do not carry user cookies.
func extensionCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && corsOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions && origin != "" && corsOriginAllowed(origin) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func corsOriginAllowed(origin string) bool {
	for _, configured := range strings.Split(os.Getenv("FWLOG_EXTENSION_ORIGINS"), ",") {
		if value := strings.TrimSpace(configured); value != "" && value == origin {
			return true
		}
	}
	return strings.HasPrefix(strings.ToLower(origin), "chrome-extension://")
}
