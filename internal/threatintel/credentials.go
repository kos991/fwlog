package threatintel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const (
	credentialPrefix = "v1:"
	credentialAAD    = "fwlog-threat-intelligence-v1"
	credentialKeyLen = 32
)

type CredentialCipher struct {
	path string
}

func NewCredentialCipher(path string) *CredentialCipher {
	return &CredentialCipher{path: path}
}

func (c *CredentialCipher) Encrypt(plaintext string) (string, error) {
	key, err := c.loadOrCreateKey()
	if err != nil {
		return "", err
	}
	gcm, err := newCredentialGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte(credentialAAD))
	return credentialPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (c *CredentialCipher) Decrypt(ciphertext string) (string, error) {
	if len(ciphertext) <= len(credentialPrefix) || ciphertext[:len(credentialPrefix)] != credentialPrefix {
		return "", errors.New("unsupported credential ciphertext")
	}
	key, err := c.readKey()
	if err != nil {
		return "", err
	}
	gcm, err := newCredentialGCM(key)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(ciphertext[len(credentialPrefix):])
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("credential ciphertext is too short")
	}
	nonce, payload := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, []byte(credentialAAD))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (c *CredentialCipher) loadOrCreateKey() ([]byte, error) {
	key, err := c.readKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return nil, err
	}
	key = make([]byte, credentialKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(c.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return c.readKey()
		}
		return nil, err
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		_ = os.Remove(c.path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(c.path)
		return nil, err
	}
	return key, nil
}

func (c *CredentialCipher) readKey() ([]byte, error) {
	info, err := os.Stat(c.path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&^0o600 != 0 {
		return nil, fmt.Errorf("credential key permissions are too broad: %o", info.Mode().Perm())
	}
	key, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}
	if len(key) != credentialKeyLen {
		return nil, fmt.Errorf("credential key length = %d, want %d", len(key), credentialKeyLen)
	}
	return key, nil
}

func newCredentialGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
