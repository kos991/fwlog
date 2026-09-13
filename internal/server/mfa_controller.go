package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fwlog/internal/security"
	"fwlog/internal/threatintel"
)

const (
	mfaEnabledSettingKey  = "mfa_enabled"
	mfaSecretSettingKey   = "mfa_totp_secret"
	mfaRecoverySettingKey = "mfa_recovery_codes"
)

type mfaStatus struct {
	Enabled       bool `json:"enabled"`
	Configured    bool `json:"configured"`
	RecoveryCodes int  `json:"recovery_codes"`
}

func (a *App) mfaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/mfa")
		switch {
		case path == "/status" && r.Method == http.MethodGet:
			a.mu.RLock()
			status := mfaStatus{Enabled: a.mfaEnabled, Configured: a.mfaSecret != "", RecoveryCodes: len(a.recoveryCodeHashes)}
			a.mu.RUnlock()
			writeJSON(w, status)
		case path == "/setup" && r.Method == http.MethodPost:
			a.mfaSetup(w, r)
		case path == "/enable" && r.Method == http.MethodPost:
			a.mfaEnable(w, r)
		case path == "/disable" && r.Method == http.MethodPost:
			a.mfaDisable(w, r)
		case path == "/recovery-codes" && r.Method == http.MethodPost:
			a.mfaRecoveryCodes(w, r)
		default:
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "not_found", "message": "MFA 接口不存在"})
		}
	})
}

func (a *App) loadMFASettingsLocked() {
	a.mfaEnabled = strings.EqualFold(strings.TrimSpace(a.settings[mfaEnabledSettingKey]), "true")
	a.mfaSecret = ""
	if encrypted := strings.TrimSpace(a.settings[mfaSecretSettingKey]); encrypted != "" {
		if secret, err := threatintel.NewCredentialCipher(a.cfg.ThreatIntelligenceKeyFile).Decrypt(encrypted); err == nil {
			a.mfaSecret = secret
		} else {
			a.mfaEnabled = false
		}
	}
	a.recoveryCodeHashes = nil
	if raw := strings.TrimSpace(a.settings[mfaRecoverySettingKey]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &a.recoveryCodeHashes)
	}
	if a.mfaSecret == "" {
		a.mfaEnabled = false
	}
}

type mfaCodeRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

func decodeMFACodeRequest(r *http.Request) (mfaCodeRequest, error) {
	var payload mfaCodeRequest
	if err := decodeJSONBodyLimit(r, &payload, 4*1024); err != nil {
		return payload, err
	}
	payload.Code = strings.TrimSpace(payload.Code)
	if payload.Code == "" || len(payload.Code) > 64 {
		return payload, errors.New("验证码不能为空")
	}
	return payload, nil
}

func (a *App) mfaSetup(w http.ResponseWriter, r *http.Request) {
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mfa_setup_failed", "message": err.Error()})
		return
	}
	a.mu.Lock()
	a.mfaPendingSecret = secret
	a.mu.Unlock()
	writeJSON(w, map[string]any{"secret": secret, "otpauth_uri": buildTOTPURI(secret)})
}

func (a *App) mfaEnable(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeMFACodeRequest(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "message": err.Error()})
		return
	}
	a.mu.RLock()
	secret := a.mfaPendingSecret
	a.mu.RUnlock()
	if secret == "" || !security.ValidateTOTP(secret, payload.Code, time.Now()) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_mfa_code", "message": "验证码错误或已过期"})
		return
	}
	codes, hashes, err := security.GenerateRecoveryCodes(8)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mfa_setup_failed", "message": err.Error()})
		return
	}
	if err := a.persistMFA(secret, true, hashes); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mfa_save_failed", "message": err.Error()})
		return
	}
	a.mu.Lock()
	a.mfaPendingSecret = ""
	a.mu.Unlock()
	writeJSON(w, map[string]any{"enabled": true, "recovery_codes": codes})
}

func (a *App) mfaDisable(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeMFACodeRequest(r)
	if err != nil || !a.verifyPassword(payload.Password) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials", "message": "密码或验证码错误"})
		return
	}
	a.mu.RLock()
	secret := a.mfaSecret
	a.mu.RUnlock()
	if !security.ValidateTOTP(secret, payload.Code, time.Now()) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials", "message": "密码或验证码错误"})
		return
	}
	if err := a.persistMFA("", false, nil); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mfa_save_failed", "message": err.Error()})
		return
	}
	a.invalidateSessions()
	writeJSON(w, mfaStatus{Enabled: false, Configured: false})
}

func (a *App) mfaRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeMFACodeRequest(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "message": err.Error()})
		return
	}
	a.mu.RLock()
	secret := a.mfaSecret
	a.mu.RUnlock()
	if !security.ValidateTOTP(secret, payload.Code, time.Now()) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_mfa_code", "message": "验证码错误或已过期"})
		return
	}
	codes, hashes, err := security.GenerateRecoveryCodes(8)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mfa_setup_failed", "message": err.Error()})
		return
	}
	if err := a.persistMFA(secret, true, hashes); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "mfa_save_failed", "message": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"recovery_codes": codes})
}

func (a *App) persistMFA(secret string, enabled bool, recovery []string) error {
	encodedRecovery, err := json.Marshal(recovery)
	if err != nil {
		return err
	}
	encrypted := ""
	if secret != "" {
		encrypted, err = threatintel.NewCredentialCipher(a.cfg.ThreatIntelligenceKeyFile).Encrypt(secret)
		if err != nil {
			return err
		}
	}
	updates := map[string]string{mfaEnabledSettingKey: fmt.Sprintf("%t", enabled), mfaSecretSettingKey: encrypted, mfaRecoverySettingKey: string(encodedRecovery)}
	if err := a.saveNormalizedSettings(context.Background(), updates); err != nil {
		return err
	}
	a.mu.Lock()
	a.mfaEnabled, a.mfaSecret, a.recoveryCodeHashes = enabled, secret, append([]string(nil), recovery...)
	a.mu.Unlock()
	return nil
}

func buildTOTPURI(secret string) string {
	return "otpauth://totp/FWLOG:admin?secret=" + secret + "&issuer=FWLOG&algorithm=SHA1&digits=6&period=30"
}

func randomChallenge() string {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
