package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type App struct {
	cfg           Config
	store         *ClickHouseStore
	mu            sync.RWMutex
	settings      map[string]string
	ipEngine      *IPEngine
	ipStatus      IPDataStatus
	passwordHash  string
	sessionToken  string
	importRunner  importRunnerFunc
	importMu      sync.Mutex
	importing     bool
	querySem      chan struct{}
	upgradeMu     sync.Mutex
	upgradeStatus UpgradeStatus
	upgradeRunner upgradeRunnerFunc
}

type importRunnerFunc func(context.Context, *ClickHouseStore, LogSource, bool) ([]string, []string, error)

func NewApp(cfg Config) *App {
	passwordHash, err := HashPassword(loadAdminPassword())
	if err != nil {
		panic(err)
	}

	return &App{
		cfg:           cfg,
		settings:      defaultSettings(cfg),
		ipEngine:      NewIPEngine(),
		ipStatus:      defaultIPDataStatus(cfg),
		passwordHash:  passwordHash,
		importRunner:  importArchivedDates,
		querySem:      make(chan struct{}, 4),
		upgradeStatus: defaultUpgradeStatus(),
	}
}

func (a *App) Connect(ctx context.Context) error {
	store, err := OpenClickHouse(ctx, a.cfg)
	if err != nil {
		return fmt.Errorf("open clickhouse: %w", err)
	}
	if err := store.EnsureTables(ctx); err != nil {
		return fmt.Errorf("ensure clickhouse tables: %w", err)
	}
	savedSettings, err := store.LoadSettings(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	a.mu.Lock()
	a.store = store
	for key, value := range savedSettings {
		a.settings[key] = value
	}
	a.mu.Unlock()
	a.reloadIPDataFromSettings()
	return nil
}

func loadAdminPassword() string {
	password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	if password == "" {
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
