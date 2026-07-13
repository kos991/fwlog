package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxUploadPackageBytes = maxUpgradePackageBytes

type UpgradeUploadResponse struct {
	UpgradeStatus
	SHA256 string `json:"sha256"`
}

func packageFormatFromFilename(name string) (upgradePackageFormat, bool) {
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "fwlog-upgrade") {
		return "", false
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".rpm":
		if !strings.HasSuffix(strings.ToLower(base), ".x86_64.rpm") {
			return "", false
		}
		return upgradePackageRPM, true
	case ".deb":
		if !strings.HasSuffix(strings.ToLower(base), "_amd64.deb") {
			return "", false
		}
		return upgradePackageDEB, true
	default:
		return "", false
	}
}

func (a *App) upgradeUploadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadPackageBytes+1024*1024)
		if err := r.ParseMultipartForm(maxUploadPackageBytes); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_upload", "message": "升级包过大或上传格式无效"})
			return
		}
		file, header, err := r.FormFile("package")
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "missing_package", "message": "请选择升级包"})
			return
		}
		defer file.Close()
		format, ok := packageFormatFromFilename(header.Filename)
		if !ok {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_package_name", "message": "只允许 fwlog-upgrade RPM/DEB 包"})
			return
		}
		if err := os.MkdirAll(upgradeTempRoot, 0o755); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "prepare_upload_failed", "message": "无法准备升级目录"})
			return
		}
		dir, err := os.MkdirTemp(upgradeTempRoot, "fwlog-upload-*")
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "prepare_upload_failed", "message": "无法准备升级目录"})
			return
		}
		path := filepath.Join(dir, "fwlog-upgrade."+string(format))
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			os.RemoveAll(dir)
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "save_upload_failed", "message": "无法保存升级包"})
			return
		}
		hash := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(file, maxUploadPackageBytes+1))
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil {
			os.RemoveAll(dir)
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "save_upload_failed", "message": "无法保存升级包"})
			return
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 || info.Size() > maxUploadPackageBytes {
			os.RemoveAll(dir)
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_package_size", "message": "升级包为空或超过大小限制"})
			return
		}
		pkg := upgradePackage{Format: format, Name: filepath.Base(header.Filename), Path: path}
		if err := validateUpgradePackageContents(r.Context(), pkg); err != nil {
			os.RemoveAll(dir)
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_package", "message": "升级包内容校验失败"})
			return
		}
		status, started := a.startUploadedUpgrade(pkg, dir)
		if !started {
			os.RemoveAll(dir)
			writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "upgrade_running", "message": "已有升级任务正在执行"})
			return
		}
		writeJSONStatus(w, http.StatusAccepted, UpgradeUploadResponse{UpgradeStatus: status, SHA256: hex.EncodeToString(hash.Sum(nil))})
	})
}

func (a *App) startUploadedUpgrade(pkg upgradePackage, tempDir string) (UpgradeStatus, bool) {
	a.upgradeMu.Lock()
	if a.upgradeStatus.State == UpgradeStateRunning {
		status := a.upgradeStatus
		a.upgradeMu.Unlock()
		return status, false
	}
	status := UpgradeStatus{State: UpgradeStateRunning, CurrentVersion: a.currentAppVersion(), TargetVersion: pkg.Name, Message: "正在安装上传的升级包", StartedAt: time.Now()}
	a.upgradeStatus = status
	a.upgradeMu.Unlock()
	go func() {
		defer os.RemoveAll(tempDir)
		err := installUpgradePackage(context.Background(), pkg)
		a.upgradeMu.Lock()
		defer a.upgradeMu.Unlock()
		result := status
		result.FinishedAt = time.Now()
		if err != nil {
			result.State = UpgradeStateFailed
			result.Error = fmt.Sprintf("安装上传升级包失败: %v", err)
			result.Message = "升级失败"
		} else {
			result.State = UpgradeStateSucceeded
			result.CurrentVersion = a.currentAppVersion()
			result.Message = "升级安装完成"
		}
		a.upgradeStatus = result
	}()
	return status, true
}
