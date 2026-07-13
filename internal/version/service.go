package version

import (
	"bufio"
	"os"
	"strings"
)

type Info struct {
	AppVersion     string `json:"app_version"`
	RuntimeVersion string `json:"runtime_version"`
	GitCommit      string `json:"git_commit,omitempty"`
	BuildTime      string `json:"build_time,omitempty"`
}

type Service interface {
	Installed() Info
}

func ReadValue(path, key, fallback string) string {
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

func InstalledInfo(appVersionFile, runtimeVersionFile, fallbackAppVersion string) Info {
	return Info{
		AppVersion:     ReadValue(appVersionFile, "VERSION", fallbackAppVersion),
		RuntimeVersion: ReadValue(runtimeVersionFile, "RUNTIME_VERSION", "unknown"),
	}
}
