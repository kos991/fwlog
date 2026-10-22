package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	receiverpkg "fwlog/internal/receiver"
	"fwlog/internal/security"
)

const minPasswordHashIterations = 10000

type App struct {
	cfg                       Config
	store                     *ClickHouseStore
	mu                        sync.RWMutex
	settings                  map[string]string
	ipEngine                  *IPEngine
	ipStatus                  IPDataStatus
	passwordHash              string
	setupMu                   sync.Mutex
	sessionToken              string
	jwtManager                *security.JWTManager
	mfaEnabled                bool
	mfaSecret                 string
	recoveryCodeHashes        []string
	mfaPendingSecret          string
	mfaChallenges             map[string]time.Time
	loginLimiter              loginLimiter
	loginSem                  chan struct{}
	importRunner              importRunnerFunc
	imports                   *ImportCoordinator
	receiver                  *receiverpkg.Manager
	threatIntelligenceService threatIntelligenceService
	querySem                  chan struct{}
	dashboardCache            *rankingCache
	accessTrendCache          *accessTrendCache
	upgradeMu                 sync.Mutex
	upgradeStatus             UpgradeStatus
	upgradeRunner             upgradeRunnerFunc
	versionInfo               VersionInfo
	logger                    *slog.Logger
	settingsSaver             func(context.Context, map[string]string) error
	archiver                  *receiverpkg.Archiver
	dateStatesLoader          func(context.Context, time.Time) ([]DateIngestState, error)
	rsyslogArchiveNow         func() time.Time
	rsyslogArchiveMu          sync.Mutex
}

type importRunnerFunc func(context.Context, *ClickHouseStore, LogSource, bool, time.Time) ([]string, []string, error)

const adminPasswordHashSettingKey = "admin_password_hash"

var (
	openClickHouse       = OpenClickHouse
	connectRetryAttempts = 30
	connectRetryDelay    = 2 * time.Second
)

func NewApp(cfg Config) *App {
	var passwordHash string
	if password := loadAdminPassword(); password != "" {
		var err error
		passwordHash, err = HashPassword(password)
		if err != nil {
			panic(err)
		}
	}

	logger := initLogger()
	versionInfo := installedVersionInfo()

	logger.Info("initializing application",
		"app_version", versionInfo.AppVersion,
		"clickhouse_addr", cfg.ClickHouseAddr,
		"workers", cfg.Workers,
	)

	jwtManager, err := security.NewEphemeralJWTManager(15 * time.Minute)
	if err != nil {
		panic(err)
	}
	app := &App{
		cfg:               cfg,
		settings:          defaultSettings(cfg),
		ipEngine:          NewIPEngine(),
		ipStatus:          defaultIPDataStatus(cfg),
		passwordHash:      passwordHash,
		jwtManager:        jwtManager,
		mfaChallenges:     make(map[string]time.Time),
		loginSem:          make(chan struct{}, maxConcurrentLogins),
		imports:           NewImportCoordinator(cfg.Workers, cfg.Workers),
		receiver:          receiverpkg.NewManager(),
		archiver:          receiverpkg.NewArchiver(),
		rsyslogArchiveNow: time.Now,
		querySem:          make(chan struct{}, 4),
		dashboardCache:    newRankingCache(dashboardRankingCacheTTL),
		accessTrendCache:  newAccessTrendCache(dashboardRankingCacheTTL),
		upgradeStatus:     defaultUpgradeStatus(),
		versionInfo:       versionInfo,
		logger:            logger,
	}
	app.threatIntelligenceService = newThreatIntelligenceService(app)
	return app
}

func (a *App) Connect(ctx context.Context) error {
	jwtManager, err := loadJWTManager(a.cfg.JWTKeyFile)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.jwtManager = jwtManager
	a.mu.Unlock()
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
	a.applySavedSettingsLocked(savedSettings)
	// 首次安装允许无密码启动，由公开初始化接口完成管理员创建。
	a.store = store
	a.mu.Unlock()
	a.reloadIPDataFromSettings()
	a.applyReceiverFromSettings()

	a.logger.Info("application ready", "settings_count", len(savedSettings))
	return nil
}

func (a *App) applySavedSettingsLocked(savedSettings map[string]string) {
	for key, value := range savedSettings {
		if key == adminPasswordHashSettingKey {
			if looksLikePasswordHash(value) {
				a.passwordHash = value
			}
			continue
		}
		if key == mfaEnabledSettingKey || key == mfaSecretSettingKey || key == mfaRecoverySettingKey {
			a.settings[key] = value
			continue
		}
		a.settings[key] = value
	}
	// GeoIP 曾随旧安装包放在 /data/index；只迁移旧默认值，保留用户明确配置的其他路径。
	if a.settings["geoip_db_path"] == "/data/index/GeoLite2-City.mmdb" {
		a.settings["geoip_db_path"] = "/opt/fwlog/GeoLite2-City.mmdb"
	}
	a.loadMFASettingsLocked()
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
	if password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD")); password != "" {
		return password
	}
	path := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_FILE"))
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		panic(fmt.Errorf("读取 ADMIN_PASSWORD_FILE: %w", err))
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		panic(fmt.Errorf("ADMIN_PASSWORD_FILE 权限过宽: %s（要求 0600）", info.Mode().Perm()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("读取 ADMIN_PASSWORD_FILE: %w", err))
	}
	return strings.TrimSpace(string(data))
}

func defaultSettings(cfg Config) map[string]string {
	defaultSource, _ := json.Marshal([]map[string]any{{
		"source_id":   "default",
		"log_dir":     cfg.LogDir,
		"log_tag":     cfg.LogTag,
		"source_type": "file",
		"enabled":     true,
	}})
	return map[string]string{
		"source_id":                  "default",
		"log_dir":                    cfg.LogDir,
		"log_tag":                    cfg.LogTag,
		"log_sources":                string(defaultSource),
		"custom_ip_map_path":         cfg.CustomIPMapPath,
		"geoip_db_path":              cfg.GeoIPDBPath,
		"ip_map_enabled":             fmt.Sprintf("%t", cfg.IPMapEnabled),
		"geoip_enabled":              fmt.Sprintf("%t", cfg.GeoIPEnabled),
		"auto_scan_enabled":          fmt.Sprintf("%t", cfg.AutoScanEnabled),
		"auto_scan_mode":             cfg.AutoScanMode,
		"auto_scan_times":            cfg.AutoScanTimes,
		"auto_scan_timezone":         cfg.AutoScanTimezone,
		"auto_scan_jitter_sec":       fmt.Sprintf("%d", cfg.AutoScanJitterSec),
		"auto_scan_interval_sec":     fmt.Sprintf("%d", cfg.AutoScanIntervalSec),
		"export_dir":                 cfg.ExportDir(),
		externalAPIEnabledSettingKey: "true",
		mcpEnabledSettingKey:         "false",
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
