package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledVersionInfoReadsPackageMetadata(t *testing.T) {
	dir := t.TempDir()
	oldAppPath, oldRuntimePath := appVersionFile, runtimeVersionFile
	appVersionFile = filepath.Join(dir, "VERSION")
	runtimeVersionFile = filepath.Join(dir, "RUNTIME_VERSION")
	defer func() { appVersionFile, runtimeVersionFile = oldAppPath, oldRuntimePath }()
	if err := os.WriteFile(appVersionFile, []byte("VERSION=v2.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeVersionFile, []byte("RUNTIME_VERSION=clickhouse-25.8.27.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info := installedVersionInfo()
	if info.AppVersion != "v2.0.0" || info.RuntimeVersion != "clickhouse-25.8.27.1" {
		t.Fatalf("version info = %#v", info)
	}
}

func TestInstalledVersionInfoFallsBackWhenFilesAreMissing(t *testing.T) {
	dir := t.TempDir()
	oldAppPath, oldRuntimePath := appVersionFile, runtimeVersionFile
	appVersionFile = filepath.Join(dir, "missing-version")
	runtimeVersionFile = filepath.Join(dir, "missing-runtime")
	defer func() { appVersionFile, runtimeVersionFile = oldAppPath, oldRuntimePath }()
	info := installedVersionInfo()
	if info.AppVersion != appVersion || info.RuntimeVersion != "unknown" {
		t.Fatalf("fallback version info = %#v", info)
	}
}
