package threatintel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestCredentialCipherWaitsForConcurrentKeyCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := NewCredentialCipher(path).Encrypt("secret")
		result <- err
	}()

	time.Sleep(20 * time.Millisecond)
	key := strings.Repeat("k", 32)
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Encrypt should wait for complete concurrent key creation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Encrypt timed out waiting for concurrent key creation")
	}
}

func TestCredentialCipherConcurrentFirstEncryptUsesOneKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	const count = 32
	ciphertexts := make([]string, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ciphertexts[i], errs[i] = NewCredentialCipher(path).Encrypt(fmt.Sprintf("secret-%02d", i))
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Encrypt[%d] failed: %v", i, err)
		}
	}
	key, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}
	cipher := NewCredentialCipher(path)
	for i, ciphertext := range ciphertexts {
		plain, err := cipher.Decrypt(ciphertext)
		if err != nil || plain != fmt.Sprintf("secret-%02d", i) {
			t.Fatalf("Decrypt[%d] = %q, %v", i, plain, err)
		}
	}
}

func TestCredentialCipherTightensExistingParentDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix permission bits")
	}
	dir := filepath.Join(t.TempDir(), "keys")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := NewCredentialCipher(filepath.Join(dir, "key")).Encrypt("secret"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("parent dir mode = %o, want 700", info.Mode().Perm())
	}
}

func tamperCiphertext(ciphertext string) string {
	index := len("v1:")
	if ciphertext[index] == 'A' {
		return ciphertext[:index] + "B" + ciphertext[index+1:]
	}
	return ciphertext[:index] + "A" + ciphertext[index+1:]
}
