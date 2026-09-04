package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureSelfSignedCertGeneratesUsableKeyPair(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	if err := ensureSelfSignedCert(certPath, keyPath); err != nil {
		t.Fatalf("ensureSelfSignedCert: %v", err)
	}

	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		t.Fatalf("generated key pair should be loadable: %v", err)
	}

	// Unix 权限位只在 Linux 部署环境有语义；Windows 上 os.WriteFile 的 mode 不生效。
	if runtime.GOOS != "windows" {
		keyInfo, err := os.Stat(keyPath)
		if err != nil {
			t.Fatalf("stat key: %v", err)
		}
		if keyInfo.Mode().Perm() != 0o600 {
			t.Fatalf("key perm = %o, want 0600", keyInfo.Mode().Perm())
		}
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("cert should decode as PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if len(cert.DNSNames) == 0 || cert.DNSNames[0] != "localhost" {
		t.Fatalf("cert SAN DNSNames = %v, want localhost first", cert.DNSNames)
	}
	if len(cert.IPAddresses) == 0 || cert.IPAddresses[0].String() != "127.0.0.1" {
		t.Fatalf("cert SAN IPAddresses = %v, want 127.0.0.1", cert.IPAddresses)
	}
}

func TestEnsureSelfSignedCertReusesExistingKeyPair(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	if err := ensureSelfSignedCert(certPath, keyPath); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	certBefore, _ := os.ReadFile(certPath)
	keyBefore, _ := os.ReadFile(keyPath)

	if err := ensureSelfSignedCert(certPath, keyPath); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	certAfter, _ := os.ReadFile(certPath)
	keyAfter, _ := os.ReadFile(keyPath)

	if string(certBefore) != string(certAfter) || string(keyBefore) != string(keyAfter) {
		t.Fatal("existing key pair should be reused, not regenerated")
	}
}

func TestGeneratedCertServesHTTPS(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	if err := ensureSelfSignedCert(certPath, keyPath); err != nil {
		t.Fatalf("ensureSelfSignedCert: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.ServeTLS(listener, certPath, keyPath) }()
	defer server.Close()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get("https://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("https request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestCookieSecureFlagReflectsTLS(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")

	app := &App{cfg: Config{TLSEnabled: true}}
	if !app.cookieSecureFlag() {
		t.Fatal("TLS enabled should force Secure cookie")
	}

	app.cfg.TLSEnabled = false
	if app.cookieSecureFlag() {
		t.Fatal("TLS disabled without COOKIE_SECURE should not force Secure cookie")
	}

	t.Setenv("COOKIE_SECURE", "true")
	if !app.cookieSecureFlag() {
		t.Fatal("COOKIE_SECURE=true should force Secure cookie")
	}
}
