package app

import "net/http"

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isAuthenticated(r) {
			writeUnauthenticated(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
