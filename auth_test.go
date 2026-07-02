package main

import (
	"strings"
	"testing"
)

func TestHashPasswordVerifiesOnlyOriginalPassword(t *testing.T) {
	encoded, err := HashPassword("admin-secret")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if encoded == "" || encoded == "admin-secret" {
		t.Fatalf("password hash should not be empty or plaintext: %q", encoded)
	}
	if !strings.HasPrefix(encoded, "pbkdf2-sha256$") {
		t.Fatalf("password hash should use PBKDF2-SHA256 format: %q", encoded)
	}
	if !VerifyPassword(encoded, "admin-secret") {
		t.Fatal("VerifyPassword should accept the original password")
	}
	if VerifyPassword(encoded, "wrong-password") {
		t.Fatal("VerifyPassword should reject a different password")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if VerifyPassword("not-a-valid-hash", "admin-secret") {
		t.Fatal("malformed hash should not verify")
	}
}
