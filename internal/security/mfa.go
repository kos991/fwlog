package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	recoveryCodeLen  = 10
)

func GenerateTOTPSecret() (string, error) {
	secret, err := totp.Generate(totp.GenerateOpts{Issuer: "FWLOG", AccountName: "admin"})
	if err != nil {
		return "", err
	}
	return secret.Secret(), nil
}

func GenerateTOTPCode(secret string, now time.Time) (string, error) {
	return totp.GenerateCode(secret, now)
}

func ValidateTOTP(secret, code string, now time.Time) bool {
	if strings.TrimSpace(secret) == "" || len(strings.TrimSpace(code)) != 6 {
		return false
	}
	valid, err := totp.ValidateCustom(strings.TrimSpace(code), strings.TrimSpace(secret), now, totp.ValidateOpts{Period: 30, Skew: 1, Digits: 6, Algorithm: otp.AlgorithmSHA1})
	return err == nil && valid
}

func GenerateRecoveryCodes(count int) ([]string, []string, error) {
	if count < 1 || count > 32 {
		return nil, nil, errors.New("恢复码数量必须在 1 到 32 之间")
	}
	codes := make([]string, count)
	hashes := make([]string, count)
	for i := range codes {
		buf := make([]byte, recoveryCodeLen)
		for j := range buf {
			var b [1]byte
			if _, err := rand.Read(b[:]); err != nil {
				return nil, nil, err
			}
			buf[j] = recoveryAlphabet[int(b[0])%len(recoveryAlphabet)]
		}
		codes[i] = string(buf)
		hash, err := hashRecoveryCode(codes[i])
		if err != nil {
			return nil, nil, err
		}
		hashes[i] = hash
	}
	return codes, hashes, nil
}

func hashRecoveryCode(code string) (string, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", fmt.Errorf("生成恢复码盐值失败: %w", err)
	}
	h := sha256.New()
	h.Write(salt[:])
	h.Write([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return "v1:" + base64.RawURLEncoding.EncodeToString(salt[:]) + ":" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}

func VerifyRecoveryCode(hashes []string, code string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if normalized == "" {
		return false
	}
	for _, encoded := range hashes {
		parts := strings.Split(encoded, ":")
		if len(parts) != 3 || parts[0] != "v1" {
			continue
		}
		salt, err1 := base64.RawURLEncoding.DecodeString(parts[1])
		want, err2 := base64.RawURLEncoding.DecodeString(parts[2])
		if err1 != nil || err2 != nil {
			continue
		}
		h := sha256.New()
		h.Write(salt)
		h.Write([]byte(normalized))
		got := h.Sum(nil)
		if len(got) == len(want) && subtle.ConstantTimeCompare(got, want) == 1 {
			return true
		}
	}
	return false
}

func ConsumeRecoveryCode(hashes []string, code string) ([]string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if normalized == "" {
		return hashes, false
	}
	for i, encoded := range hashes {
		if VerifyRecoveryCode([]string{encoded}, normalized) {
			remaining := make([]string, 0, len(hashes)-1)
			remaining = append(remaining, hashes[:i]...)
			remaining = append(remaining, hashes[i+1:]...)
			return remaining, true
		}
	}
	return hashes, false
}

type JWTClaims struct {
	AMR     []string `json:"amr"`
	Scope   string   `json:"scope,omitempty"`
	Version uint64   `json:"ver"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	key     []byte
	ttl     time.Duration
	mu      sync.RWMutex
	version uint64
}

func NewEphemeralJWTManager(ttl time.Duration) (*JWTManager, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return NewJWTManager(key, ttl)
}

func NewJWTManager(key []byte, ttl time.Duration) (*JWTManager, error) {
	if len(key) < 32 {
		return nil, errors.New("JWT 签名密钥至少需要 32 字节")
	}
	if ttl <= 0 {
		return nil, errors.New("JWT 有效期必须大于 0")
	}
	copyKey := append([]byte(nil), key...)
	return &JWTManager{key: copyKey, ttl: ttl}, nil
}

func (m *JWTManager) Issue(subject string, amr []string, now time.Time) (string, error) {
	return m.IssueScoped(subject, amr, "", now, 0)
}

func (m *JWTManager) IssueScoped(subject string, amr []string, scope string, now time.Time, ttl time.Duration) (string, error) {
	if strings.TrimSpace(subject) == "" {
		return "", errors.New("JWT subject 不能为空")
	}
	var jtiRaw [16]byte
	if _, err := rand.Read(jtiRaw[:]); err != nil {
		return "", err
	}
	m.mu.RLock()
	version := m.version
	defaultTTL := m.ttl
	key := append([]byte(nil), m.key...)
	m.mu.RUnlock()
	if ttl <= 0 {
		ttl = defaultTTL
	}
	claims := JWTClaims{AMR: append([]string(nil), amr...), Scope: strings.TrimSpace(scope), Version: version, RegisteredClaims: jwt.RegisteredClaims{
		Subject:   subject,
		ID:        base64.RawURLEncoding.EncodeToString(jtiRaw[:]),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

func (m *JWTManager) Parse(token string, now time.Time) (*JWTClaims, error) {
	m.mu.RLock()
	key := append([]byte(nil), m.key...)
	version := m.version
	m.mu.RUnlock()
	parsed, err := jwt.ParseWithClaims(token, &JWTClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("JWT 签名算法不受支持")
		}
		return key, nil
	}, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*JWTClaims)
	if !ok || !parsed.Valid || claims.Subject == "" || (claims.Scope == "" && claims.Version != version) {
		return nil, errors.New("JWT 无效")
	}
	return claims, nil
}

func (m *JWTManager) InvalidateAll() {
	m.mu.Lock()
	m.version++
	m.mu.Unlock()
}
