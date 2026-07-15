package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestPackageFormatFromFilenameAcceptsOnlyUpgradePackages(t *testing.T) {
	tests := []struct {
		name   string
		format upgradePackageFormat
		ok     bool
	}{
		{"fwlog-upgrade-v2.0.0.x86_64.rpm", upgradePackageRPM, true},
		{"fwlog-upgrade_2.0.0_amd64.deb", upgradePackageDEB, true},
		{"fwlog-full-v2.0.0.x86_64.rpm", "", false},
		{"upgrade.zip", "", false},
		{"../fwlog-upgrade-v2.0.0.x86_64.rpm", upgradePackageRPM, true},
		{"fwlog-upgrade-v2.0.0.aarch64.rpm", "", false},
		{"fwlog-upgrade_2.0.0_arm64.deb", "", false},
	}
	for _, tt := range tests {
		format, ok := packageFormatFromFilename(tt.name)
		if ok != tt.ok || format != tt.format {
			t.Fatalf("packageFormatFromFilename(%q) = %q, %v", tt.name, format, ok)
		}
	}
}

func TestUpgradeUploadHandlerValidatesAndStartsDebPackage(t *testing.T) {
	t.Setenv("ALLOW_UNSIGNED_UPGRADE_UPLOAD", "true")
	oldRoot, oldRunCommand, oldLookPath := upgradeTempRoot, runCommand, lookPath
	upgradeTempRoot = t.TempDir()
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	called := make(chan struct{}, 1)
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "dpkg-deb" {
			return []byte("/opt/fwlog/fwlog\n/opt/fwlog/VERSION\n"), nil
		}
		if name == "dpkg" {
			called <- struct{}{}
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected command %s", name)
	}
	defer func() { upgradeTempRoot, runCommand, lookPath = oldRoot, oldRunCommand, oldLookPath }()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("package", "fwlog-upgrade_2.0.0_amd64.deb")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("fake-deb")
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/upgrade/upload", &body)
	request.Header.Set("Content-Type", mw.FormDataContentType())
	response := httptest.NewRecorder()
	app := NewApp(Config{})
	app.upgradeUploadHandler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	wantHash := sha256.Sum256(payload)
	if !bytes.Contains(response.Body.Bytes(), []byte(hex.EncodeToString(wantHash[:]))) {
		t.Fatalf("response does not contain upload sha256: %s", response.Body.String())
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("uploaded package was not installed")
	}
}

func TestUpgradeUploadHandlerRejectsUnsignedUploadsByDefault(t *testing.T) {
	t.Setenv("ALLOW_UNSIGNED_UPGRADE_UPLOAD", "")

	request := httptest.NewRequest(http.MethodPost, "/api/upgrade/upload", bytes.NewBufferString("not-used"))
	response := httptest.NewRecorder()
	app := NewApp(Config{})

	app.upgradeUploadHandler().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
}
