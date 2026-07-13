package server

import (
	"net/http"

	"fwlog/internal/version"
)

var appVersionFile = "/opt/fwlog/VERSION"
var runtimeVersionFile = "/opt/fwlog/RUNTIME_VERSION"

type VersionInfo = version.Info

func installedVersionInfo() VersionInfo {
	return version.InstalledInfo(appVersionFile, runtimeVersionFile, appVersion)
}

func currentAppVersion() string {
	return installedVersionInfo().AppVersion
}

func (a *App) currentAppVersion() string {
	if a.versionInfo.AppVersion != "" {
		return a.versionInfo.AppVersion
	}
	return appVersion
}

func versionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, installedVersionInfo())
	})
}
