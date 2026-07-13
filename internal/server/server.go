package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	receiverpkg "fwlog/internal/receiver"
)

const minPasswordHashIterations = 10000

type App struct {
	cfg           Config
	store         *ClickHouseStore
	mu            sync.RWMutex
	settings      map[string]string
	ipEngine      *IPEngine
	ipStatus      IPDataStatus
	passwordHash  string
	sessionToken  string
	loginLimiter  loginLimiter
	importRunner  importRunnerFunc
	imports       *ImportCoordinator
	receiver      *receiverpkg.Manager
	querySem      chan struct{}
	upgradeMu     sync.Mutex
	upgradeStatus UpgradeStatus
	upgradeRunner upgradeRunnerFunc
	versionInfo   VersionInfo
	logger        *slog.Logger
	settingsSaver func(context.Context, map[string]string) error
}

type importRunnerFunc func(context.Context, *ClickHouseStore, LogSource, bool) ([]string, []string, error)

const adminPasswordHashSettingKey = "admin_password_hash"

var (
	openClickHouse       = OpenClickHouse
	connectRetryAttempts = 30
	connectRetryDelay    = 2 * time.Second
)

func NewApp(cfg Config) *App {
	passwordHash, err := HashPassword(loadAdminPassword())
	if err != nil {
		panic(err)
	}

	logger := initLogger()
	versionInfo := installedVersionInfo()

	logger.Info("initializing application",
		"app_version", versionInfo.AppVersion,
		"clickhouse_addr", cfg.ClickHouseAddr,
		"workers", cfg.Workers,
	)

	return &App{
		cfg:           cfg,
		settings:      defaultSettings(cfg),
		ipEngine:      NewIPEngine(),
		ipStatus:      defaultIPDataStatus(cfg),
		passwordHash:  passwordHash,
		imports:       NewImportCoordinator(cfg.Workers, defaultConcurrentWrites),
		receiver:      receiverpkg.NewManager(),
		querySem:      make(chan struct{}, 4),
		upgradeStatus: defaultUpgradeStatus(),
		versionInfo:   versionInfo,
		logger:        logger,
	}
}

func (a *App) Connect(ctx context.Context) error {
	a.logger.Info("connecting to clickhouse", "addr", a.cfg.ClickHouseAddr)

	store, err := a.openClickHouseWithRetry(ctx)
	if err != nil {
		a.logger.Error("clickhouse connection failed", "error", err)
		return fmt.Errorf("open clickhouse: %w", err)
	}

	a.logger.Info("clickhouse connected successfully")

	if err := store.EnsureTables(ctx); err != nil {
		a.logger.Error("ensure tables failed", "error", err)
		return fmt.Errorf("ensure clickhouse tables: %w", err)
	}

	a.logger.Info("clickhouse tables verified")

	savedSettings, err := store.LoadSettings(ctx)
	if err != nil {
		a.logger.Error("load settings failed", "error", err)
		return fmt.Errorf("load settings: %w", err)
	}

	a.mu.Lock()
	a.store = store
	a.applySavedSettingsLocked(savedSettings)
	a.mu.Unlock()
	a.reloadIPDataFromSettings()
	a.applyReceiverFromSettings()

	a.logger.Info("application ready", "settings_count", len(savedSettings))
	return nil
}

func (a *App) applySavedSettings(savedSettings map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applySavedSettingsLocked(savedSettings)
}

func (a *App) applySavedSettingsLocked(savedSettings map[string]string) {
	for key, value := range savedSettings {
		if key == adminPasswordHashSettingKey {
			if looksLikePasswordHash(value) {
				a.passwordHash = value
			}
			continue
		}
		a.settings[key] = value
	}
}

func (a *App) openClickHouseWithRetry(ctx context.Context) (*ClickHouseStore, error) {
	attempts := connectRetryAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		store, err := openClickHouse(ctx, a.cfg)
		if err == nil {
			return store, nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}

		a.logger.Warn("clickhouse connection attempt failed, retrying",
			"attempt", attempt,
			"max_attempts", attempts,
			"retry_delay", connectRetryDelay,
			"error", err,
		)

		timer := time.NewTimer(connectRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, fmt.Errorf("clickhouse not ready after %d attempts: %w", attempts, lastErr)
}

func loadAdminPassword() string {
	password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	if password == "" {
		fmt.Fprintln(os.Stderr, "WARNING: ADMIN_PASSWORD 未设置，使用默认密码 admin，请在部署后立即修改")
		return "admin"
	}
	return password
}

func defaultSettings(cfg Config) map[string]string {
	return map[string]string{
		"source_id":              "default",
		"log_dir":                cfg.LogDir,
		"log_tag":                cfg.LogTag,
		"custom_ip_map_path":     cfg.CustomIPMapPath,
		"geoip_db_path":          cfg.GeoIPDBPath,
		"ip_map_enabled":         fmt.Sprintf("%t", cfg.IPMapEnabled),
		"geoip_enabled":          fmt.Sprintf("%t", cfg.GeoIPEnabled),
		"auto_scan_enabled":      fmt.Sprintf("%t", cfg.AutoScanEnabled),
		"auto_scan_mode":         cfg.AutoScanMode,
		"auto_scan_times":        cfg.AutoScanTimes,
		"auto_scan_timezone":     cfg.AutoScanTimezone,
		"auto_scan_jitter_sec":   fmt.Sprintf("%d", cfg.AutoScanJitterSec),
		"auto_scan_interval_sec": fmt.Sprintf("%d", cfg.AutoScanIntervalSec),
		"export_dir":             cfg.ExportDir(),
	}
}

func defaultIPDataStatus(cfg Config) IPDataStatus {
	return IPDataStatus{
		CustomMapPath: cfg.CustomIPMapPath,
		GeoIPDBPath:   cfg.GeoIPDBPath,
		IPMapEnabled:  cfg.IPMapEnabled,
		GeoIPEnabled:  cfg.GeoIPEnabled,
		UpdatedAt:     time.Now(),
	}
}
