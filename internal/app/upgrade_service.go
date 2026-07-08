package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	githubRepoOwner       = "kos991"
	githubRepoName        = "fwlog"
	linuxUpgradeAssetName = "nat-query-service_linux_amd64"
	installedBinaryPath   = "/opt/nat-query/nat-query-service"
)

const (
	upgradeHTTPTimeout      = 10 * time.Minute
	maxUpgradePackageBytes  = 512 * 1024 * 1024
	checksumsAssetName      = "checksums.txt"
)

var appVersion = "dev"
var lookPath = exec.LookPath
var upgradeBackupDir = "/data/nat-query/backups"
var osReleasePath = "/etc/os-release"
var upgradeTempRoot = "/opt/nat-query/tmp"
var upgradeHTTPClient = &http.Client{Timeout: upgradeHTTPTimeout}
var runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

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
	LegacyBinaryURL string
	UpgradeRPMURL   string
	UpgradeDEBURL   string
}

type upgradePackageFormat string

const (
	upgradePackageRPM upgradePackageFormat = "rpm"
	upgradePackageDEB upgradePackageFormat = "deb"
)

type upgradePackage struct {
	Format upgradePackageFormat
	URL    string
	Path   string
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
	return regexp.MustCompile(`^v\d+(\.\d+){0,2}$`).MatchString(strings.TrimSpace(version))
}

func releaseUpgradeAssets(release githubRelease) (upgradeAssets, []string) {
	found := map[string]string{}
	for _, asset := range release.Assets {
		found[asset.Name] = asset.BrowserDownloadURL
	}

	upgradeRPMAssetName, upgradeDEBAssetName := upgradePackageAssetNames(release.TagName)
	missing := make([]string, 0)
	for _, name := range []string{linuxUpgradeAssetName, upgradeRPMAssetName, upgradeDEBAssetName} {
		if strings.TrimSpace(found[name]) == "" {
			missing = append(missing, name)
		}
	}

	return upgradeAssets{
		LegacyBinaryURL: found[linuxUpgradeAssetName],
		UpgradeRPMURL:   found[upgradeRPMAssetName],
		UpgradeDEBURL:   found[upgradeDEBAssetName],
	}, missing
}

func upgradePackageAssetNames(version string) (string, string) {
	pkgVersion := strings.TrimPrefix(strings.TrimSpace(version), "v")
	return fmt.Sprintf("fwlog-upgrade-v%s.x86_64.rpm", pkgVersion), fmt.Sprintf("fwlog-upgrade_%s_amd64.deb", pkgVersion)
}

func selectUpgradePackage(assets upgradeAssets) (upgradePackage, error) {
	preferred := preferredUpgradePackageFormat()
	if preferred == upgradePackageDEB {
		if pkg, ok := availableUpgradePackage(upgradePackageDEB, assets.UpgradeDEBURL); ok {
			return pkg, nil
		}
		if pkg, ok := availableUpgradePackage(upgradePackageRPM, assets.UpgradeRPMURL); ok {
			return pkg, nil
		}
	}
	if preferred == upgradePackageRPM {
		if pkg, ok := availableUpgradePackage(upgradePackageRPM, assets.UpgradeRPMURL); ok {
			return pkg, nil
		}
		if pkg, ok := availableUpgradePackage(upgradePackageDEB, assets.UpgradeDEBURL); ok {
			return pkg, nil
		}
	}
	if pkg, ok := availableUpgradePackage(upgradePackageDEB, assets.UpgradeDEBURL); ok {
		return pkg, nil
	}
	if pkg, ok := availableUpgradePackage(upgradePackageRPM, assets.UpgradeRPMURL); ok {
		return pkg, nil
	}
	return upgradePackage{}, errors.New("未找到可用的 rpm/dpkg 包管理器，或 Release 缺少对应的 fwlog-upgrade 包")
}

func availableUpgradePackage(format upgradePackageFormat, assetURL string) (upgradePackage, bool) {
	if strings.TrimSpace(assetURL) == "" {
		return upgradePackage{}, false
	}
	switch format {
	case upgradePackageRPM:
		if _, err := lookPath("rpm"); err == nil {
			return upgradePackage{Format: upgradePackageRPM, URL: assetURL}, true
		}
	case upgradePackageDEB:
		if _, err := lookPath("dpkg"); err == nil {
			return upgradePackage{Format: upgradePackageDEB, URL: assetURL}, true
		}
	}
	return upgradePackage{}, false
}

func preferredUpgradePackageFormat() upgradePackageFormat {
	content, err := os.ReadFile(osReleasePath)
	if err != nil {
		return ""
	}
	release := strings.ToLower(string(content))
	if strings.Contains(release, "debian") || strings.Contains(release, "ubuntu") {
		return upgradePackageDEB
	}
	if strings.Contains(release, "rhel") || strings.Contains(release, "fedora") || strings.Contains(release, "centos") || strings.Contains(release, "kylin") {
		return upgradePackageRPM
	}
	return ""
}

func validateUpgradePackageContents(ctx context.Context, pkg upgradePackage) error {
	var output []byte
	var err error
	switch pkg.Format {
	case upgradePackageRPM:
		output, err = runCommand(ctx, "rpm", "-qpl", pkg.Path)
	case upgradePackageDEB:
		output, err = runCommand(ctx, "dpkg-deb", "--contents", pkg.Path)
	default:
		return fmt.Errorf("unsupported_upgrade_package: %s", pkg.Format)
	}
	if err != nil {
		return fmt.Errorf("检查 fwlog-upgrade 包内容失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	contents := string(output)
	for _, forbidden := range []string{
		"/opt/nat-query/clickhouse",
		"/data/clickhouse",
		"/etc/systemd/system/fwlog-clickhouse.service",
	} {
		if strings.Contains(contents, forbidden) {
			return fmt.Errorf("fwlog-upgrade 包不能包含 ClickHouse 文件: %s", forbidden)
		}
	}
	return nil
}

func installUpgradePackage(ctx context.Context, pkg upgradePackage) error {
	commandName := ""
	commandArgs := make([]string, 0, 2)
	switch pkg.Format {
	case upgradePackageRPM:
		commandName = "rpm"
		commandArgs = []string{"-Uvh", pkg.Path}
	case upgradePackageDEB:
		commandName = "dpkg"
		commandArgs = []string{"-i", pkg.Path}
	default:
		return fmt.Errorf("unsupported_upgrade_package: %s", pkg.Format)
	}
	output, err := runPackageManagerCommand(ctx, commandName, commandArgs...)
	if err != nil {
		return fmt.Errorf("安装 fwlog-upgrade 包失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runPackageManagerCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	if _, err := lookPath("systemd-run"); err == nil {
		systemdArgs := []string{
			"--wait",
			"--collect",
			"--pipe",
			"--service-type=oneshot",
			"--property=ProtectSystem=off",
			"--property=ProtectHome=off",
			"--property=PrivateTmp=no",
			"--property=ReadWritePaths=/ /var/lib/dpkg /var/lib/rpm /opt/nat-query /data",
			name,
		}
		systemdArgs = append(systemdArgs, args...)
		return runCommand(ctx, "systemd-run", systemdArgs...)
	}
	return runCommand(ctx, name, args...)
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
		_ = strings.TrimSpace(payload.Version)
		status, started := a.startUpgrade(upgradeTarget{Version: "latest"})
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
		defer func() {
			if r := recover(); r != nil {
				a.upgradeMu.Lock()
				a.upgradeStatus = UpgradeStatus{
					State:          UpgradeStateFailed,
					CurrentVersion: appVersion,
					TargetVersion:  target.Version,
					Error:          fmt.Sprintf("升级任务异常退出: %v", r),
					Message:        "升级失败",
					FinishedAt:     time.Now(),
				}
				a.upgradeMu.Unlock()
			}
		}()
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

	resp, err := upgradeHTTPClient.Do(req)
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
	_ = status
	upgradeCtx, cancel := context.WithTimeout(ctx, upgradeHTTPTimeout)
	defer cancel()

	backupPath, err := backupCurrentInstall(upgradeCtx)
	if err != nil {
		return fmt.Errorf("升级前备份失败: %w", err)
	}
	if status != nil {
		status.BackupPath = backupPath
	}

	release, err := fetchGithubRelease(upgradeCtx, target.Version)
	if err != nil {
		return err
	}
	assets, missing := releaseUpgradeAssets(release)
	if len(missing) > 0 {
		return fmt.Errorf("release_asset_missing: %s", strings.Join(missing, ", "))
	}

	if err := os.MkdirAll(upgradeTempRoot, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(upgradeTempRoot, "fwlog-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	pkg, err := selectUpgradePackage(assets)
	if err != nil {
		return err
	}
	pkg.Path = filepath.Join(tempDir, "fwlog-upgrade."+string(pkg.Format))
	if err := downloadFile(upgradeCtx, pkg.URL, pkg.Path); err != nil {
		return err
	}
	info, err := os.Stat(pkg.Path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return errors.New("下载到的 fwlog-upgrade 包为空")
	}

	if err := verifyPackageChecksum(upgradeCtx, release, pkg); err != nil {
		return err
	}

	if err := validateUpgradePackageContents(ctx, pkg); err != nil {
		return err
	}
	if err := installUpgradePackage(ctx, pkg); err != nil {
		return err
	}
	return nil
}

func downloadFile(ctx context.Context, sourceURL, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "fwlog-auto-upgrade")
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载升级资产失败，状态码 %d", resp.StatusCode)
	}

	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, io.LimitReader(resp.Body, maxUpgradePackageBytes+1))
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(targetPath); statErr == nil && info.Size() > maxUpgradePackageBytes {
		return fmt.Errorf("升级包超过最大允许大小 %d 字节", maxUpgradePackageBytes)
	}
	return nil
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

func findChecksumAssetURL(release githubRelease) string {
	for _, asset := range release.Assets {
		if asset.Name == checksumsAssetName {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func backupCurrentInstall(ctx context.Context) (string, error) {
	backupDir := upgradeBackupDir
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	target := filepath.Join(backupDir, "pre-upgrade-"+stamp)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}

	for _, item := range []struct {
		src string
		dst string
	}{
		{"/etc/systemd/system/nat-query-service.service", "nat-query-service.service"},
		{"/opt/nat-query/VERSION", "VERSION"},
	} {
		if err := copyFileForBackup(item.src, filepath.Join(target, item.dst)); err != nil && !os.IsNotExist(err) {
			return target, fmt.Errorf("备份 %s 失败: %w", item.src, err)
		}
	}
	return target, nil
}

func copyFileForBackup(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	target, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
}

func verifyPackageChecksum(ctx context.Context, release githubRelease, pkg upgradePackage) error {
	checksumURL := findChecksumAssetURL(release)
	if checksumURL == "" {
		return nil
	}

	tempDir := filepath.Dir(pkg.Path)
	checksumPath := filepath.Join(tempDir, checksumsAssetName)
	if err := downloadFile(ctx, checksumURL, checksumPath); err != nil {
		return fmt.Errorf("下载 checksums.txt 失败: %w", err)
	}

	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	pkgName := filepath.Base(pkg.Path)
	expectedHash, ok := parseChecksumEntry(string(data), pkgName)
	if !ok {
		return fmt.Errorf("checksums.txt 中未找到 %s 的校验记录", pkgName)
	}

	actualHash, err := sha256File(pkg.Path)
	if err != nil {
		return fmt.Errorf("计算升级包 sha256 失败: %w", err)
	}
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("升级包 sha256 校验失败: 期望 %s 实际 %s", expectedHash, actualHash)
	}
	return nil
}

func parseChecksumEntry(checksums, filename string) (string, bool) {
	for _, line := range strings.Split(checksums, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == filename {
			return fields[0], true
		}
	}
	return "", false
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
