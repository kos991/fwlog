package threatintel

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCredentialCipherRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threat-intelligence.key")
	cipher := NewCredentialCipher(path)

	encrypted, err := cipher.Encrypt("test-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "test-secret-value") || !strings.HasPrefix(encrypted, "v1:") {
		t.Fatalf("unsafe ciphertext %q", encrypted)
	}

	plain, err := cipher.Decrypt(encrypted)
	if err != nil || plain != "test-secret-value" {
		t.Fatalf("Decrypt = %q, %v", plain, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("key mode = %o", info.Mode().Perm())
		}
	}
}

func TestCredentialCipherRejectsTampering(t *testing.T) {
	cipher := NewCredentialCipher(filepath.Join(t.TempDir(), "key"))
	encrypted, err := cipher.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cipher.Decrypt(tamperCiphertext(encrypted))
	if err == nil {
		t.Fatal("tampered ciphertext should fail")
	}
}

func TestCredentialCipherDecryptDoesNotCreateMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	first := NewCredentialCipher(path)
	encrypted, err := first.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	_, err = NewCredentialCipher(path).Decrypt(encrypted)
	if err == nil {
		t.Fatal("decrypting with missing key should fail")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("decrypt recreated key, stat error = %v", statErr)
	}
}

func TestCredentialCipherRejectsLooseKeyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(strings.Repeat("k", 32)), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewCredentialCipher(path).Encrypt("secret")
	if err == nil {
		t.Fatal("loose key permissions should fail")
	}
}

func tamperCiphertext(ciphertext string) string {
	index := len("v1:")
	if ciphertext[index] == 'A' {
		return ciphertext[:index] + "B" + ciphertext[index+1:]
	}
	return ciphertext[:index] + "A" + ciphertext[index+1:]
}
