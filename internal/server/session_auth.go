package server

import (
	"context"
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
	Mode      string `json:"mode"`
	Password  string `json:"password"`
	Code      string `json:"mfa_code"`
	Challenge string `json:"challenge"`
	Recovery  bool   `json:"use_recovery_code"`
	Client    string `json:"client"`
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

		if payload.Challenge != "" {
			if !a.consumeMFAChallenge(payload.Challenge) {
				writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_mfa_challenge", "message": "登录验证已过期，请重新输入密码"})
				return
			}
			if !a.verifyMFACode(payload.Code, payload.Recovery) {
				a.loginLimiter.recordFailure(source)
				writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_mfa_code", "message": "验证码错误或已过期"})
				return
			}
			a.loginLimiter.reset(source)
			a.issueJWT(w, []string{"pwd", "totp"}, shouldExposeAccessToken(payload.Client))
			return
		}

		if payload.Mode == "mfa" {
			a.mu.RLock()
			mfaEnabled := a.mfaEnabled && a.mfaSecret != ""
			a.mu.RUnlock()
			if !mfaEnabled || !a.verifyMFACode(payload.Code, payload.Recovery) {
				a.loginLimiter.recordFailure(source)
				writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_mfa_code", "message": "验证码错误或 MFA 未启用"})
				return
			}
			a.loginLimiter.reset(source)
			a.issueJWT(w, []string{"totp"}, shouldExposeAccessToken(payload.Client))
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

		a.mu.RLock()
		mfaEnabled := a.mfaEnabled && a.mfaSecret != ""
		a.mu.RUnlock()
		// 显式选择密码登录时只使用密码认证；未传 mode 的旧客户端仍沿用原有的二阶段 MFA 流程。
		if mfaEnabled && payload.Mode == "" {
			challenge := randomChallenge()
			if challenge == "" {
				writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "session_init_failed", "message": "生成 MFA 挑战失败"})
				return
			}
			a.mu.Lock()
			for key, expires := range a.mfaChallenges {
				if time.Now().After(expires) {
					delete(a.mfaChallenges, key)
				}
			}
			a.mfaChallenges[challenge] = time.Now().Add(5 * time.Minute)
			a.mu.Unlock()
			writeJSON(w, SessionResponse{MFARequired: true, Challenge: challenge})
			return
		}

		a.loginLimiter.reset(source)
		a.issueJWT(w, []string{"pwd"}, shouldExposeAccessToken(payload.Client))
	})
}

func shouldExposeAccessToken(client string) bool {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case "chrome_extension", "api", "mcp":
		return true
	default:
		return false
	}
}

func (a *App) issueJWT(w http.ResponseWriter, amr []string, exposeToken bool) {
	a.mu.RLock()
	manager := a.jwtManager
	a.mu.RUnlock()
	if manager == nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "session_init_failed", "message": "JWT 管理器未初始化"})
		return
	}
	token, err := manager.Issue("admin", amr, time.Now())
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "session_init_failed", "message": err.Error()})
		return
	}
	a.mu.Lock()
	a.sessionToken = token
	a.mu.Unlock()
	http.SetCookie(w, buildSessionCookie(token, a.cookieSecureFlag()))
	response := SessionResponse{Authenticated: true}
	if exposeToken {
		response.AccessToken = token
		response.TokenType = "Bearer"
	}
	writeJSON(w, response)
}

func (a *App) logoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.sessionToken = ""
		a.mu.Unlock()
		a.invalidateSessions()

		http.SetCookie(w, clearSessionCookie(a.cookieSecureFlag()))
		writeJSON(w, SessionResponse{Authenticated: false})
	})
}

func decodeLoginRequest(r *http.Request) (loginRequest, error) {
	var payload loginRequest
	if err := decodeJSONBodyLimit(r, &payload, maxLoginBodyBytes); err != nil {
		return loginRequest{}, err
	}
	if strings.TrimSpace(payload.Challenge) != "" {
		payload.Challenge = strings.TrimSpace(payload.Challenge)
		payload.Code = strings.TrimSpace(payload.Code)
		if payload.Code == "" || len(payload.Code) > 64 {
			return loginRequest{}, errors.New("MFA 验证码不能为空")
		}
		return payload, nil
	}
	payload.Mode = strings.ToLower(strings.TrimSpace(payload.Mode))
	if payload.Mode == "mfa" {
		payload.Code = strings.TrimSpace(payload.Code)
		if payload.Code == "" || len(payload.Code) > 64 {
			return loginRequest{}, errors.New("MFA 验证码不能为空")
		}
		return payload, nil
	}
	if payload.Mode != "" && payload.Mode != "password" {
		return loginRequest{}, errors.New("不支持的登录方式")
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

func (a *App) cookieSecureFlag() bool {
	if a != nil && a.cfg.TLSEnabled {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE")))
	return v == "true" || v == "1"
}

func buildSessionCookie(token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionMaxAge,
	}
}

func clearSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	}
}

func (a *App) isAuthenticated(r *http.Request) bool {
	a.mu.RLock()
	manager, sessionToken := a.jwtManager, a.sessionToken
	a.mu.RUnlock()
	if manager != nil {
		if token := bearerToken(r); token != "" {
			if claims, err := manager.Parse(token, time.Now()); err == nil {
				return claims.Scope == ""
			}
			return false
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			return false
		}
		if _, err := manager.Parse(cookie.Value, time.Now()); err == nil {
			return true
		}
		// JWT 格式令牌解析失败时不得回退到旧的内存令牌比较，否则过期令牌会继续有效。
		if strings.Count(cookie.Value, ".") == 2 {
			return false
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && cookie.Value != "" && sessionToken != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(sessionToken)) == 1
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < len("Bearer ")+1 || !strings.EqualFold(value[:len("Bearer")], "Bearer") {
		return ""
	}
	return strings.TrimSpace(value[len("Bearer"):])
}

func (a *App) invalidateSessions() {
	a.mu.Lock()
	manager := a.jwtManager
	a.sessionToken = ""
	a.mu.Unlock()
	if manager != nil {
		manager.InvalidateAll()
	}
}

func (a *App) consumeMFAChallenge(challenge string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	expires, ok := a.mfaChallenges[challenge]
	if ok {
		delete(a.mfaChallenges, challenge)
	}
	return ok && time.Now().Before(expires)
}

func (a *App) verifyMFACode(code string, recovery bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if recovery {
		remaining, ok := security.ConsumeRecoveryCode(a.recoveryCodeHashes, code)
		if ok {
			a.recoveryCodeHashes = remaining
			go a.persistRecoveryHashes(remaining)
		}
		return ok
	}
	return security.ValidateTOTP(a.mfaSecret, code, time.Now())
}

func (a *App) persistRecoveryHashes(hashes []string) {
	data, err := json.Marshal(hashes)
	if err != nil {
		return
	}
	_ = a.saveNormalizedSettings(context.Background(), map[string]string{mfaRecoverySettingKey: string(data)})
}

func (a *App) verifyPassword(password string) bool {
	a.mu.RLock()
	passwordHash := a.passwordHash
	a.mu.RUnlock()
	return VerifyPassword(passwordHash, password)
}
