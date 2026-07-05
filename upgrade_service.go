package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	githubRepoOwner              = "kos991"
	githubRepoName               = "fwlog"
	linuxUpgradeAssetName        = "nat-query-service_linux_amd64"
	kylinServerPackageAssetName  = "nat-query-service_kylin-server_amd64.rpm"
	debianServerPackageAssetName = "nat-query-service_debian-server_amd64.deb"
	installedBinaryPath          = "/opt/nat-query/nat-query-service"
)

var appVersion = "dev"

type UpgradeState string

const (
	UpgradeStateIdle      UpgradeState = "idle"
	UpgradeStateRunning   UpgradeState = "running"
	UpgradeStateSucceeded UpgradeState = "succeeded"
	UpgradeStateFailed    UpgradeState = "failed"
)

type UpgradeStatus struct {
	State          UpgradeState `json:"state"`
	CurrentVersion string       `json:"current_version"`
	TargetVersion  string       `json:"target_version,omitempty"`
	Message        string       `json:"message,omitempty"`
	Error          string       `json:"error,omitempty"`
	BackupPath     string       `json:"backup_path,omitempty"`
	StartedAt      time.Time    `json:"started_at,omitempty"`
	FinishedAt     time.Time    `json:"finished_at,omitempty"`
}

type UpgradeCheckResponse struct {
	CurrentVersion  string        `json:"current_version"`
	LatestVersion   string        `json:"latest_version"`
	UpdateAvailable bool          `json:"update_available"`
	ReleaseURL      string        `json:"release_url"`
	AssetsReady     bool          `json:"assets_ready"`
	MissingAssets   []string      `json:"missing_assets"`
	Message         string        `json:"message,omitempty"`
	Status          UpgradeStatus `json:"status"`
}

type upgradeTarget struct {
	Version string
}

type upgradeRunRequest struct {
	Version string `json:"version"`
}

type upgradeAssets struct {
	BinaryURL              string
	KylinServerPackageURL  string
	DebianServerPackageURL string
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	HTMLURL string               `json:"html_url"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type upgradeRunnerFunc func(context.Context, upgradeTarget) UpgradeStatus

func validUpgradeVersion(version string) bool {
	return regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(strings.TrimSpace(version))
}

func releaseUpgradeAssets(release githubRelease) (upgradeAssets, []string) {
	found := map[string]string{}
	for _, asset := range release.Assets {
		found[asset.Name] = asset.BrowserDownloadURL
	}

	missing := make([]string, 0)
	for _, name := range []string{linuxUpgradeAssetName, kylinServerPackageAssetName, debianServerPackageAssetName} {
		if strings.TrimSpace(found[name]) == "" {
			missing = append(missing, name)
		}
	}

	return upgradeAssets{
		BinaryURL:              found[linuxUpgradeAssetName],
		KylinServerPackageURL:  found[kylinServerPackageAssetName],
		DebianServerPackageURL: found[debianServerPackageAssetName],
	}, missing
}

func defaultUpgradeStatus() UpgradeStatus {
	return UpgradeStatus{
		State:          UpgradeStateIdle,
		CurrentVersion: appVersion,
	}
}

func (a *App) upgradeCheckHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isAuthenticated(r) {
			writeUnauthenticated(w)
			return
		}

		release, err := fetchGithubRelease(r.Context(), "latest")
		if err != nil {
			writeJSONStatus(w, http.StatusBadGateway, map[string]any{
				"error":   "release_check_failed",
				"message": err.Error(),
			})
			return
		}
		_, missing := releaseUpgradeAssets(release)
		latestVersion := strings.TrimSpace(release.TagName)
		response := UpgradeCheckResponse{
			CurrentVersion:  appVersion,
			LatestVersion:   latestVersion,
			UpdateAvailable: latestVersion != "" && latestVersion != appVersion,
			ReleaseURL:      release.HTMLURL,
			AssetsReady:     len(missing) == 0,
			MissingAssets:   missing,
			Status:          a.currentUpgradeStatus(),
		}
		if len(missing) > 0 {
			response.Message = "Release 缺少 Linux 升级资产"
		}
		writeJSON(w, response)
	})
}

func (a *App) upgradeStatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isAuthenticated(r) {
			writeUnauthenticated(w)
			return
		}
		writeJSON(w, a.currentUpgradeStatus())
	})
}

func (a *App) upgradeRunHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isAuthenticated(r) {
			writeUnauthenticated(w)
			return
		}

		var payload upgradeRunRequest
		if err := decodeJSONBody(r, &payload); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{
				"error":   "invalid_json",
				"message": err.Error(),
			})
			return
		}
		version := strings.TrimSpace(payload.Version)
		if !validUpgradeVersion(version) {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{
				"error":   "invalid_version",
				"message": "升级版本必须使用 vX.Y.Z 格式",
			})
			return
		}

		status, started := a.startUpgrade(upgradeTarget{Version: version})
		if !started {
			writeJSONStatus(w, http.StatusConflict, map[string]any{
				"error":   "upgrade_running",
				"message": "已有升级任务正在执行",
				"status":  status,
			})
			return
		}
		writeJSONStatus(w, http.StatusAccepted, status)
	})
}

func writeUnauthenticated(w http.ResponseWriter) {
	writeJSONStatus(w, http.StatusUnauthorized, map[string]any{
		"error":   "unauthenticated",
		"message": "请先登录后再执行升级操作",
	})
}

func (a *App) currentUpgradeStatus() UpgradeStatus {
	a.upgradeMu.Lock()
	defer a.upgradeMu.Unlock()
	if a.upgradeStatus.State == "" {
		a.upgradeStatus = defaultUpgradeStatus()
	}
	a.upgradeStatus.CurrentVersion = appVersion
	return a.upgradeStatus
}

func (a *App) startUpgrade(target upgradeTarget) (UpgradeStatus, bool) {
	a.upgradeMu.Lock()
	if a.upgradeStatus.State == UpgradeStateRunning {
		status := a.upgradeStatus
		a.upgradeMu.Unlock()
		return status, false
	}

	status := UpgradeStatus{
		State:          UpgradeStateRunning,
		CurrentVersion: appVersion,
		TargetVersion:  target.Version,
		Message:        "升级任务已开始",
		StartedAt:      time.Now(),
	}
	a.upgradeStatus = status
	runner := a.upgradeRunner
	if runner == nil {
		runner = runSystemUpgrade
	}
	a.upgradeMu.Unlock()

	go func() {
		result := runner(context.Background(), target)
		if result.CurrentVersion == "" {
			result.CurrentVersion = appVersion
		}
		if result.TargetVersion == "" {
			result.TargetVersion = target.Version
		}
		if result.FinishedAt.IsZero() {
			result.FinishedAt = time.Now()
		}

		a.upgradeMu.Lock()
		a.upgradeStatus = result
		a.upgradeMu.Unlock()
	}()

	return status, true
}

func fetchGithubRelease(ctx context.Context, version string) (githubRelease, error) {
	endpoint := "https://api.github.com/repos/" + githubRepoOwner + "/" + githubRepoName + "/releases/"
	if version == "latest" {
		endpoint += "latest"
	} else {
		endpoint += "tags/" + url.PathEscape(version)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "fwlog-auto-upgrade")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("GitHub Release API 返回状态码 %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func runSystemUpgrade(ctx context.Context, target upgradeTarget) UpgradeStatus {
	status := UpgradeStatus{
		State:          UpgradeStateRunning,
		CurrentVersion: appVersion,
		TargetVersion:  target.Version,
		StartedAt:      time.Now(),
	}
	if err := executeSystemUpgrade(ctx, target, &status); err != nil {
		status.State = UpgradeStateFailed
		status.Error = err.Error()
		status.Message = "升级失败"
		status.FinishedAt = time.Now()
		return status
	}
	status.State = UpgradeStateSucceeded
	status.Message = "升级完成，服务正在重启"
	status.FinishedAt = time.Now()
	return status
}

func executeSystemUpgrade(ctx context.Context, target upgradeTarget, status *UpgradeStatus) error {
	release, err := fetchGithubRelease(ctx, target.Version)
	if err != nil {
		return err
	}
	assets, missing := releaseUpgradeAssets(release)
	if len(missing) > 0 {
		return fmt.Errorf("release_asset_missing: %s", strings.Join(missing, ", "))
	}

	tempDir, err := os.MkdirTemp("", "fwlog-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	downloadPath := filepath.Join(tempDir, linuxUpgradeAssetName)
	if err := downloadFile(ctx, assets.BinaryURL, downloadPath); err != nil {
		return err
	}
	info, err := os.Stat(downloadPath)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return errors.New("下载到的升级二进制为空")
	}

	backupPath := fmt.Sprintf("%s.bak.%s", installedBinaryPath, time.Now().Format("20060102150405"))
	if err := copyFile(installedBinaryPath, backupPath); err != nil {
		return err
	}
	status.BackupPath = backupPath

	if err := replaceFileAtomic(downloadPath, installedBinaryPath, 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "systemctl", "restart", "nat-query-service")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("重启 nat-query-service 失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func downloadFile(ctx context.Context, sourceURL, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "fwlog-auto-upgrade")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载升级资产失败，状态码 %d", resp.StatusCode)
	}

	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func copyFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

func replaceFileAtomic(sourcePath, targetPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	dir := filepath.Dir(targetPath)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(targetPath)+".*.new")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(temp, source); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
