package security

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTOTPSecretAndCode(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil || secret == "" {
		t.Fatalf("GenerateTOTPSecret() = %q, %v", secret, err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode() error = %v", err)
	}
	if !ValidateTOTP(secret, code, time.Now()) {
		t.Fatalf("ValidateTOTP() rejected generated code %q", code)
	}
	if ValidateTOTP(secret, "000000", time.Now()) {
		t.Fatal("ValidateTOTP() accepted an invalid code")
	}
}

func TestRecoveryCodesAreOneTimeVerifiable(t *testing.T) {
	codes, hashes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error = %v", err)
	}
	if len(codes) != 8 || len(hashes) != 8 {
		t.Fatalf("recovery code count = %d/%d, want 8/8", len(codes), len(hashes))
	}
	if strings.Contains(hashes[0], codes[0]) {
		t.Fatal("recovery hash contains plaintext code")
	}
	if !VerifyRecoveryCode(hashes, codes[0]) {
		t.Fatal("VerifyRecoveryCode() rejected generated code")
	}
	remaining, ok := ConsumeRecoveryCode(hashes, codes[0])
	if !ok || len(remaining) != 7 {
		t.Fatalf("ConsumeRecoveryCode() = %d, %v; want 7, true", len(remaining), ok)
	}
	if _, ok := ConsumeRecoveryCode(remaining, codes[0]); ok {
		t.Fatal("recovery code can be consumed twice")
	}
}

func TestJWTSessionExpiryAndInvalidation(t *testing.T) {
	manager, err := NewJWTManager([]byte("01234567890123456789012345678901"), time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}
	token, err := manager.Issue("admin", []string{"pwd", "totp"}, time.Now())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := manager.Parse(token, time.Now().Add(30*time.Second))
	if err != nil || claims.Subject != "admin" {
		t.Fatalf("Parse() = %#v, %v", claims, err)
	}
	if _, err := manager.Parse(token, time.Now().Add(2*time.Minute)); err == nil {
		t.Fatal("expired JWT was accepted")
	}
	manager.InvalidateAll()
	if _, err := manager.Parse(token, time.Now()); err == nil {
		t.Fatal("invalidated JWT was accepted")
	}
}

func TestJWTScopedTokenUsesIndependentTTL(t *testing.T) {
	manager, err := NewJWTManager([]byte("01234567890123456789012345678901"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token, err := manager.IssueScoped("mcp-client", []string{"admin-download"}, "mcp", now, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Parse(token, now.Add(29*24*time.Hour))
	if err != nil || claims.Scope != "mcp" {
		t.Fatalf("Parse() = %#v, %v", claims, err)
	}
	if _, err := manager.Parse(token, now.Add(31*24*time.Hour)); err == nil {
		t.Fatal("expired scoped JWT was accepted")
	}
	manager.InvalidateAll()
	if _, err := manager.Parse(token, now.Add(time.Hour)); err != nil {
		t.Fatalf("session invalidation revoked scoped JWT: %v", err)
	}
}
