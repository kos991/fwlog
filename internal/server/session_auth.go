package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"fwlog/internal/security"
)

const sessionCookieName = security.SessionCookieName

const (
	maxLoginFailures    = 5
	loginCooldown       = 5 * time.Minute
	maxLoginBodyBytes   = 4 * 1024
	maxJSONBodyBytes    = 1024 * 1024
	maxPasswordBytes    = 256
	maxConcurrentLogins = 4
	sessionMaxAge       = security.SessionMaxAge
)

type loginRequest struct {
	Password string `json:"password"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginFailureBucket
}

type loginFailureBucket struct {
	failures    int
	lastFailure time.Time
}

func (l *loginLimiter) check(source string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[source]
	if bucket.failures >= maxLoginFailures && time.Since(bucket.lastFailure) < loginCooldown {
		remaining := loginCooldown - time.Since(bucket.lastFailure)
		return fmt.Errorf("登录尝试过于频繁，请 %d 分钟后重试", int(remaining.Minutes())+1)
	}
	return nil
}

func (l *loginLimiter) recordFailure(source string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]loginFailureBucket)
	}
	now := time.Now()
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastFailure) >= loginCooldown {
			delete(l.buckets, key)
		}
	}
	bucket := l.buckets[source]
	bucket.failures++
	bucket.lastFailure = now
	l.buckets[source] = bucket
}

func (l *loginLimiter) reset(source string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, source)
}

func (a *App) sessionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, SessionResponse{Authenticated: a.isAuthenticated(r)})
	})
}

func (a *App) loginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case a.loginSem <- struct{}{}:
			defer func() { <-a.loginSem }()
		default:
			writeJSONStatus(w, http.StatusTooManyRequests, map[string]any{
				"error":   "login_busy",
				"message": "登录请求较多，请稍后重试",
			})
			return
		}

		source := loginSource(r)
		if err := a.loginLimiter.check(source); err != nil {
			writeJSONStatus(w, http.StatusTooManyRequests, map[string]any{
				"error":   "rate_limited",
				"message": err.Error(),
			})
			return
		}

		payload, err := decodeLoginRequest(r)
		if err != nil {
			status := http.StatusBadRequest
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				status = http.StatusRequestEntityTooLarge
			}
			writeJSONStatus(w, status, map[string]any{
				"error":   "invalid_request",
				"message": err.Error(),
			})
			return
		}

		if !a.verifyPassword(payload.Password) {
			a.loginLimiter.recordFailure(source)
			writeJSONStatus(w, http.StatusUnauthorized, map[string]any{
				"error":   "invalid_password",
				"message": "管理员密码错误",
			})
			return
		}

		a.loginLimiter.reset(source)

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
	if err := decodeJSONBodyLimit(r, &payload, maxLoginBodyBytes); err != nil {
		return loginRequest{}, err
	}
	if strings.TrimSpace(payload.Password) == "" {
		return loginRequest{}, errors.New("password 不能为空")
	}
	if len(payload.Password) > maxPasswordBytes {
		return loginRequest{}, fmt.Errorf("password 不能超过 %d 字节", maxPasswordBytes)
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
	if len(payload.CurrentPassword) > maxPasswordBytes || len(payload.NewPassword) > maxPasswordBytes {
		return passwordChangeRequest{}, fmt.Errorf("密码不能超过 %d 字节", maxPasswordBytes)
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
	return decodeJSONBodyLimit(r, target, maxJSONBodyBytes)
}

func decodeJSONBodyLimit(r *http.Request, target any, maxBytes int64) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	defer r.Body.Close()

	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("请求体只能包含一个 JSON 对象")
		}
		return err
	}
	return nil
}

func loginSource(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr == "" {
		return "unknown"
	}
	return r.RemoteAddr
}

func newSessionToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func cookieSecureFlag() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE")))
	return v == "true" || v == "1"
}

func buildSessionCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecureFlag(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionMaxAge,
	}
}

func clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecureFlag(),
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

	if cookie.Value == "" || a.sessionToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(a.sessionToken)) == 1
}

func (a *App) verifyPassword(password string) bool {
	a.mu.RLock()
	passwordHash := a.passwordHash
	a.mu.RUnlock()
	return VerifyPassword(passwordHash, password)
}
