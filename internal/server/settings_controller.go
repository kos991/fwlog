package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"fwlog/internal/ip"
)

func settingsHandler(app *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, app.getSettings())
		case http.MethodPost:
			var payload map[string]any
			if r.Body != nil {
				defer r.Body.Close()
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err.Error() != "EOF" {
					writeJSONStatus(w, http.StatusBadRequest, map[string]any{
						"error":   "invalid_json",
						"message": err.Error(),
					})
					return
				}
			}
			updates, err := app.normalizeSettingsPayload(payload)
			if err != nil {
				writeJSONStatus(w, http.StatusBadRequest, map[string]any{
					"error":   "invalid_settings",
					"message": err.Error(),
				})
				return
			}
			if err := app.saveNormalizedSettings(r.Context(), updates); err != nil {
				writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
					"error":   "settings_save_failed",
					"message": err.Error(),
				})
				return
			}
			app.applyNormalizedSettings(updates)
			app.reloadIPDataFromSettings()
			app.applyReceiverFromSettings()
			if _, changed := updates["log_sources"]; changed {
				app.triggerRSyslogArchive()
			}
			writeJSON(w, app.getSettings())
		default:
			w.Header().Set("Allow", "GET, POST")
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"error":   "method_not_allowed",
				"message": fmt.Sprintf("%s %s is not allowed", r.Method, r.URL.Path),
			})
		}
	})
}

func (a *App) applyReceiverFromSettings() {
	if a.receiver == nil {
		return
	}
	if err := a.receiver.ApplySources(a.currentLogSources()); err != nil {
		a.logger.Error("应用 RSyslog 接收配置失败", "error", err)
	}
}

func (a *App) receiverStatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if a.receiver == nil {
			writeJSON(w, map[string]any{})
			return
		}
		writeJSON(w, a.receiver.Status())
	})
}

func (a *App) getSettings() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	settings := make(map[string]string, len(a.settings)+7)
	for key, value := range a.settings {
		if isProtectedSettingsKey(key) {
			continue
		}
		settings[key] = value
	}
	settings["rebuild_status"] = string(StatusIdle)
	settings["current_date"] = ""
	settings["current_file"] = ""
	settings["files_total"] = "0"
	settings["files_done"] = "0"
	settings["rows_imported"] = "0"
	settings["error"] = a.ipStatus.Error
	return settings
}

var protectedSettingsKeys = map[string]bool{
	adminPasswordHashSettingKey: true,
}

func (a *App) updateSettings(payload map[string]any) {
	updates, err := a.normalizeSettingsPayload(payload)
	if err != nil {
		return
	}
	a.applyNormalizedSettings(updates)
}

func (a *App) normalizeSettingsPayload(payload map[string]any) (map[string]string, error) {
	updates := make(map[string]string, len(payload))
	for key, value := range payload {
		if isProtectedSettingsKey(key) {
			continue
		}
		if key == "log_sources" {
			normalized, ok := normalizeLogSourcesSetting(value)
			if !ok {
				return nil, errors.New("日志源配置格式无效")
			}
			var sources []LogSource
			if err := json.Unmarshal([]byte(normalized), &sources); err != nil {
				return nil, fmt.Errorf("解析日志源配置: %w", err)
			}
			if err := validateLogSources(sources); err != nil {
				return nil, err
			}
			updates[key] = normalized
			continue
		}
		updates[key] = stringifySettingValue(value)
	}
	return updates, nil
}

func stringifySettingValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return fmt.Sprintf("%t", typed)
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

func (a *App) applyNormalizedSettings(updates map[string]string) {
	if len(updates) == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for key, value := range updates {
		a.settings[key] = value
	}
}

func (a *App) reloadIPDataFromSettings() IPDataStatus {
	cfg := a.currentIPDataConfig()

	a.mu.RLock()
	oldEngine := a.ipEngine
	a.mu.RUnlock()

	nextEngine, status := ip.ReloadIPEngine(cfg, oldEngine)

	a.mu.Lock()
	a.ipEngine = nextEngine
	a.ipStatus = status
	a.mu.Unlock()

	return status
}

func (a *App) saveSettings(ctx context.Context, payload map[string]any) error {
	if len(payload) == 0 {
		return nil
	}
	settings := make(map[string]string, len(payload))
	a.mu.RLock()
	for key := range payload {
		if isProtectedSettingsKey(key) {
			continue
		}
		settings[key] = a.settings[key]
	}
	a.mu.RUnlock()
	return a.saveNormalizedSettings(ctx, settings)
}

func (a *App) saveNormalizedSettings(ctx context.Context, settings map[string]string) error {
	if len(settings) == 0 {
		return nil
	}
	if a.settingsSaver != nil {
		return a.settingsSaver(ctx, settings)
	}
	store := a.currentStore()
	if store == nil {
		return errors.New("ClickHouse 尚未连接，无法持久化设置")
	}
	if !store.Ready() {
		return nil
	}
	return store.SaveSettings(ctx, settings)
}

func (a *App) saveAdminPasswordHash(ctx context.Context, passwordHash string) error {
	store := a.currentStore()
	if store == nil {
		return errors.New("ClickHouse 尚未连接，无法持久化管理员密码")
	}
	if !store.Ready() {
		return nil
	}
	return store.SaveSettings(ctx, map[string]string{
		adminPasswordHashSettingKey: passwordHash,
	})
}

func (a *App) currentIPDataConfig() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cfg := a.cfg
	cfg.CustomIPMapPath = settingOrFallback(a.settings, "custom_ip_map_path", cfg.CustomIPMapPath)
	cfg.GeoIPDBPath = settingOrFallback(a.settings, "geoip_db_path", cfg.GeoIPDBPath)
	cfg.CIDRAliases = parseCIDRAliases(a.settings["cidr_aliases"])
	cfg.IPMapEnabled = settingBoolOrFallback(a.settings, "ip_map_enabled", cfg.IPMapEnabled)
	cfg.GeoIPEnabled = settingBoolOrFallback(a.settings, "geoip_enabled", cfg.GeoIPEnabled)
	return cfg
}

func parseCIDRAliases(raw string) []CIDRAliasSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var aliases []CIDRAliasSetting
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		return nil
	}
	return aliases
}

func settingOrFallback(settings map[string]string, key, fallback string) string {
	value := strings.TrimSpace(settings[key])
	if value == "" {
		return fallback
	}
	return value
}

func settingBoolOrFallback(settings map[string]string, key string, fallback bool) bool {
	value := strings.TrimSpace(settings[key])
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
