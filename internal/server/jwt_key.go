package server

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fwlog/internal/security"
)

func loadJWTManager(path string) (*security.JWTManager, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return security.NewEphemeralJWTManager(15 * time.Minute)
	}
	key, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("读取 JWT 密钥失败: %w", err)
		}
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("创建 JWT 密钥目录失败: %w", err)
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, fmt.Errorf("写入 JWT 密钥失败: %w", err)
		}
	} else if len(key) < 32 {
		return nil, errors.New("JWT 密钥文件长度不足 32 字节")
	}
	if runtime.GOOS != "windows" {
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("JWT 密钥文件权限过宽，需要 0600")
		}
	}
	return security.NewJWTManager(key, 15*time.Minute)
}
