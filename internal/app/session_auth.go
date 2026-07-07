package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const sessionCookieName = "fwlog_session"

type loginRequest struct {
	Password string `json:"password"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (a *App) sessionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, SessionResponse{Authenticated: a.isAuthenticated(r)})
	})
}

func (a *App) loginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := decodeLoginRequest(r)
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{
				"error":   "invalid_request",
				"message": err.Error(),
			})
			return
		}

		if !a.verifyPassword(payload.Password) {
			writeJSONStatus(w, http.StatusUnauthorized, map[string]any{
				"error":   "invalid_password",
				"message": "管理员密码错误",
			})
			return
		}

		token, err := newSessionToken()
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
				"error":   "session_init_failed",
				"message": err.Error(),
			})
			return
		}

		a.mu.Lock()
		a.sessionToken = token
		a.mu.Unlock()

		http.SetCookie(w, buildSessionCookie(token))
		writeJSON(w, SessionResponse{Authenticated: true})
	})
}

func (a *App) logoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.sessionToken = ""
		a.mu.Unlock()

		http.SetCookie(w, clearSessionCookie())
		writeJSON(w, SessionResponse{Authenticated: false})
	})
}

func decodeLoginRequest(r *http.Request) (loginRequest, error) {
	var payload loginRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		return loginRequest{}, err
	}
	if strings.TrimSpace(payload.Password) == "" {
		return loginRequest{}, errors.New("password 不能为空")
	}
	return payload, nil
}

func decodePasswordChangeRequest(r *http.Request) (passwordChangeRequest, error) {
	var payload passwordChangeRequest
	if err := decodeJSONBody(r, &payload); err != nil {
		return passwordChangeRequest{}, err
	}
	if strings.TrimSpace(payload.CurrentPassword) == "" {
		return passwordChangeRequest{}, errors.New("current_password 不能为空")
	}
	if strings.TrimSpace(payload.NewPassword) == "" {
		return passwordChangeRequest{}, errors.New("new_password 不能为空")
	}
	if err := validateNewAdminPassword(payload.NewPassword); err != nil {
		return passwordChangeRequest{}, err
	}
	return payload, nil
}

func validateNewAdminPassword(password string) error {
	if len(password) < 6 {
		return errors.New(`新密码至少需要 6 个字符`)
	}
	return nil
}

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func newSessionToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func buildSessionCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	}
}

func (a *App) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	return cookie.Value != "" && a.sessionToken != "" && cookie.Value == a.sessionToken
}

func (a *App) verifyPassword(password string) bool {
	a.mu.RLock()
	passwordHash := a.passwordHash
	a.mu.RUnlock()
	return VerifyPassword(passwordHash, password)
}
