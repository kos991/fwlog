package app

import (
	"bufio"
	"net/http"
	"os"
	"strings"
)

var appVersionFile = "/opt/nat-query/VERSION"
var runtimeVersionFile = "/opt/nat-query/RUNTIME_VERSION"

type VersionInfo struct {
	AppVersion     string `json:"app_version"`
	RuntimeVersion string `json:"runtime_version"`
}

func readVersionValue(path, key, fallback string) string {
	file, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer file.Close()
	prefix := key + "="
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, prefix) {
			if value := strings.TrimSpace(strings.TrimPrefix(line, prefix)); value != "" {
				return value
			}
		}
	}
	return fallback
}

func installedVersionInfo() VersionInfo {
	return VersionInfo{
		AppVersion:     readVersionValue(appVersionFile, "VERSION", appVersion),
		RuntimeVersion: readVersionValue(runtimeVersionFile, "RUNTIME_VERSION", "unknown"),
	}
}

func currentAppVersion() string {
	return installedVersionInfo().AppVersion
}

func versionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, installedVersionInfo())
	})
}
