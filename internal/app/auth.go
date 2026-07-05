package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordHashVersion    = "pbkdf2-sha256"
	passwordHashIterations = 120000
	passwordHashKeyLen     = 32
)

type SessionResponse struct {
	Authenticated bool `json:"authenticated"`
}

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("管理员密码不能为空")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	sum := derivePasswordKey(salt, password, passwordHashIterations, passwordHashKeyLen)
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordHashVersion,
		passwordHashIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashVersion {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}

	got := derivePasswordKey(salt, password, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func derivePasswordKey(salt []byte, password string, iterations, keyLen int) []byte {
	if keyLen <= 0 {
		return nil
	}

	passwordBytes := []byte(password)
	hashLen := sha256.Size
	blockCount := (keyLen + hashLen - 1) / hashLen
	derived := make([]byte, 0, blockCount*hashLen)
	for block := 1; block <= blockCount; block++ {
		derived = append(derived, pbkdf2Block(passwordBytes, salt, iterations, uint32(block))...)
	}
	return derived[:keyLen]
}

func pbkdf2Block(password, salt []byte, iterations int, blockIndex uint32) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	var index [4]byte
	binary.BigEndian.PutUint32(index[:], blockIndex)
	mac.Write(index[:])
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)

	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
