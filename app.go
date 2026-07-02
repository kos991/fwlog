package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	cfg          Config
	store        *ClickHouseStore
	mu           sync.RWMutex
	settings     map[string]string
	ipEngine     *IPEngine
	ipStatus     IPDataStatus
	passwordHash string
	sessionToken string
}

func NewApp(cfg Config) *App {
	passwordHash, err := HashPassword(loadAdminPassword())
	if err != nil {
		panic(err)
	}

	return &App{
		cfg:          cfg,
		settings:     defaultSettings(cfg),
		ipEngine:     NewIPEngine(),
		ipStatus:     defaultIPDataStatus(cfg),
		passwordHash: passwordHash,
	}
}

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()

	queryHandler := NewQueryHandler(appQueryService{app: a})
	dashboardHandler := NewHealthDashboardHandler(appDashboardService{app: a})
	progressHandler := NewIngestProgressHandler(appDashboardService{app: a})
	passwordHandler := NewPasswordHandler(appSecurityService{app: a})
	ipReloadHandler := NewIPDataReloadHandler(appSecurityService{app: a})

	mux.Handle("/api/query", methodHandler(http.MethodGet, queryHandler))
	mux.Handle("/api/health-dashboard", methodHandler(http.MethodGet, dashboardHandler))
	mux.Handle("/api/ingest-progress", methodHandler(http.MethodGet, progressHandler))
	mux.Handle("/api/password", methodHandler(http.MethodPost, passwordHandler))
	mux.Handle("/api/ip-data/reload", methodHandler(http.MethodPost, ipReloadHandler))
	mux.Handle("/api/settings", settingsHandler(a))
	mux.Handle("/api/session", methodHandler(http.MethodGet, a.sessionHandler()))
	mux.Handle("/api/login", methodHandler(http.MethodPost, a.loginHandler()))
	mux.Handle("/api/logout", methodHandler(http.MethodPost, a.logoutHandler()))
	mux.Handle("/api/sync", methodHandler(http.MethodPost, a.importHandler(false)))
	mux.Handle("/api/rebuild", methodHandler(http.MethodPost, a.importHandler(true)))
	mux.Handle("/api/export", placeholderHandler(http.MethodPost, "export endpoint is not implemented yet"))

	mux.Handle("/", newStaticHandler())
	return mux
}

func (a *App) Run() error {
	if err := a.Connect(context.Background()); err != nil {
		return err
	}

	addr := fmt.Sprintf(":%d", a.cfg.Port)
	return http.ListenAndServe(addr, a.Router())
}

func (a *App) Connect(ctx context.Context) error {
	store, err := OpenClickHouse(ctx, a.cfg)
	if err != nil {
		return fmt.Errorf("open clickhouse: %w", err)
	}
	if err := store.EnsureTables(ctx); err != nil {
		return fmt.Errorf("ensure clickhouse tables: %w", err)
	}

	a.mu.Lock()
	a.store = store
	a.mu.Unlock()
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

func methodHandler(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"error":   "method_not_allowed",
				"message": fmt.Sprintf("%s %s is not allowed", r.Method, r.URL.Path),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

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
			app.updateSettings(payload)
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

func placeholderHandler(method, message string) http.Handler {
	return methodHandler(method, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONStatus(w, http.StatusNotImplemented, map[string]any{
			"error":   "not_implemented",
			"message": message,
		})
	}))
}

func (a *App) importHandler(rebuild bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store := a.currentStore()
		if store == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "clickhouse_not_connected",
				"message": "ClickHouse 尚未连接，无法执行入库",
			})
			return
		}

		source := a.currentLogSource()
		imported, skipped, err := importArchivedDates(r.Context(), store, source, rebuild)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
				"error":          "import_failed",
				"message":        err.Error(),
				"imported_dates": imported,
				"skipped_dates":  skipped,
			})
			return
		}

		writeJSON(w, map[string]any{
			"status":         string(StatusSucceeded),
			"imported_dates": imported,
			"skipped_dates":  skipped,
			"rebuild":        rebuild,
		})
	})
}

func (a *App) getSettings() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	settings := make(map[string]string, len(a.settings)+7)
	for key, value := range a.settings {
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

func (a *App) currentStore() *ClickHouseStore {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store
}

func (a *App) currentLogSource() LogSource {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return LogSource{
		SourceID:  settingOrFallback(a.settings, "source_id", "default"),
		LogDir:    settingOrFallback(a.settings, "log_dir", a.cfg.LogDir),
		LogTag:    settingOrFallback(a.settings, "log_tag", a.cfg.LogTag),
		Enabled:   true,
		UpdatedAt: time.Now(),
	}
}

func (a *App) updateSettings(payload map[string]any) {
	if len(payload) == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			a.settings[key] = typed
		case bool:
			a.settings[key] = fmt.Sprintf("%t", typed)
		case float64:
			a.settings[key] = fmt.Sprintf("%.0f", typed)
		default:
			a.settings[key] = fmt.Sprint(typed)
		}
	}
}

func (a *App) currentIPDataConfig() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cfg := a.cfg
	cfg.CustomIPMapPath = settingOrFallback(a.settings, "custom_ip_map_path", cfg.CustomIPMapPath)
	cfg.GeoIPDBPath = settingOrFallback(a.settings, "geoip_db_path", cfg.GeoIPDBPath)
	cfg.IPMapEnabled = settingBoolOrFallback(a.settings, "ip_map_enabled", cfg.IPMapEnabled)
	cfg.GeoIPEnabled = settingBoolOrFallback(a.settings, "geoip_enabled", cfg.GeoIPEnabled)
	return cfg
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

func importArchivedDates(ctx context.Context, store *ClickHouseStore, source LogSource, rebuild bool) ([]string, []string, error) {
	files, err := ScanArchivedLogFiles(source.LogDir, time.Now())
	if err != nil {
		return nil, nil, err
	}

	dateSet := make(map[string]time.Time)
	for _, file := range files {
		dateSet[dateKey(file.LogDate)] = startOfDay(file.LogDate)
	}

	dates := make([]time.Time, 0, len(dateSet))
	for _, date := range dateSet {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})

	importer := &Importer{
		store:  store,
		writer: store.conn,
		states: store,
	}

	imported := make([]string, 0, len(dates))
	skipped := make([]string, 0)
	for _, date := range dates {
		if !rebuild {
			state, found, err := store.LatestDateState(ctx, source.SourceID, date)
			if err != nil {
				return imported, skipped, err
			}
			if found && state.Status == StatusReady {
				skipped = append(skipped, formatDate(date))
				continue
			}
		}

		if err := importer.ImportDate(ctx, source, date); err != nil {
			return imported, skipped, err
		}
		imported = append(imported, formatDate(date))
	}

	return imported, skipped, nil
}

type appQueryService struct {
	app *App
}

func (s appQueryService) Query(r *http.Request) (QueryResponse, error) {
	store := s.appStore()
	if store == nil {
		return emptyQueryResponse(), nil
	}

	req, page, pageSize, err := parseQueryRequest(r)
	if err != nil {
		return QueryResponse{}, err
	}

	states, err := store.ListDateStates(r.Context(), startOfDay(req.Start))
	if err != nil {
		return QueryResponse{}, err
	}

	visibility := BuildVisibleRanges(req.Start, req.End, states)
	querySQL, args, err := BuildQuerySQL(req, visibility)
	if err != nil {
		return QueryResponse{
			Records:     []map[string]any{},
			Total:       0,
			Page:        page,
			PageSize:    pageSize,
			QueryTimeMS: 0,
			Visibility:  visibility,
		}, nil
	}

	startedAt := time.Now()
	records, err := store.QueryNATLogs(r.Context(), querySQL, args, page, pageSize)
	if err != nil {
		return QueryResponse{}, err
	}

	return QueryResponse{
		Records:     records,
		Total:       len(records),
		Page:        page,
		PageSize:    pageSize,
		QueryTimeMS: time.Since(startedAt).Milliseconds(),
		Visibility:  visibility,
	}, nil
}

func (s appQueryService) appStore() *ClickHouseStore {
	if s.app == nil {
		return nil
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.store
}

type appDashboardService struct {
	app *App
}

func (s appDashboardService) HealthDashboard(r *http.Request) (HealthDashboardResponse, error) {
	store := s.appStore()
	if store == nil {
		return BuildHealthDashboard(nil, DashboardMetrics{}), nil
	}

	states, err := store.ListDateStates(r.Context(), dashboardSince(r))
	if err != nil {
		return HealthDashboardResponse{}, err
	}

	metrics, err := store.DashboardMetrics(r.Context())
	if err != nil {
		return HealthDashboardResponse{}, err
	}
	metrics.GeoIPLoaded = s.geoIPLoaded()
	metrics.GeoIPStatus = s.geoIPStatus()

	return BuildHealthDashboard(states, metrics), nil
}

func (s appDashboardService) IngestProgress(r *http.Request) (IngestProgressResponse, error) {
	store := s.appStore()
	includeReady := parseBoolQuery(r, "include_ready", false)
	if store == nil {
		return BuildIngestProgress(nil, includeReady), nil
	}

	states, err := store.ListDateStates(r.Context(), time.Now().AddDate(0, 0, -30))
	if err != nil {
		return IngestProgressResponse{}, err
	}
	return BuildIngestProgress(states, includeReady), nil
}

func (s appDashboardService) appStore() *ClickHouseStore {
	if s.app == nil {
		return nil
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.store
}

func (s appDashboardService) geoIPLoaded() bool {
	if s.app == nil {
		return false
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.ipStatus.Loaded && s.app.ipStatus.GeoIPEnabled
}

func (s appDashboardService) geoIPStatus() string {
	if s.app == nil {
		return ""
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.ipStatus.Error
}

func emptyQueryResponse() QueryResponse {
	return QueryResponse{
		Records:  []map[string]any{},
		Total:    0,
		Page:     1,
		PageSize: 50,
		Visibility: QueryVisibility{
			QueriedRanges: []VisibleRange{},
			SkippedDates:  []SkippedLogDate{},
		},
	}
}

func parseQueryRequest(r *http.Request) (QueryRequest, int, int, error) {
	values := r.URL.Query()
	now := time.Now()

	start, err := parseTimeQuery(values.Get("start"), now.AddDate(0, 0, -7))
	if err != nil {
		return QueryRequest{}, 0, 0, err
	}
	end, err := parseTimeQuery(values.Get("end"), now)
	if err != nil {
		return QueryRequest{}, 0, 0, err
	}
	if end.Before(start) {
		return QueryRequest{}, 0, 0, fmt.Errorf("end must be after start")
	}

	req := QueryRequest{
		Start:    start,
		End:      end,
		IP:       strings.TrimSpace(values.Get("ip")),
		SrcIP:    strings.TrimSpace(values.Get("src_ip")),
		DstIP:    strings.TrimSpace(values.Get("dst_ip")),
		NATIP:    strings.TrimSpace(values.Get("nat_ip")),
		Protocol: strings.TrimSpace(values.Get("protocol")),
		Action:   strings.TrimSpace(values.Get("action")),
		LogTag:   strings.TrimSpace(values.Get("log_tag")),
	}

	var errPort error
	req.Port, errPort = parseUint16Query(values.Get("port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}
	req.SrcPort, errPort = parseUint16Query(values.Get("src_port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}
	req.DstPort, errPort = parseUint16Query(values.Get("dst_port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}
	req.NATPort, errPort = parseUint16Query(values.Get("nat_port"))
	if errPort != nil {
		return QueryRequest{}, 0, 0, errPort
	}

	page := parsePositiveInt(values.Get("page"), 1)
	pageSize := parsePositiveInt(values.Get("page_size"), 50)
	if pageSize > 500 {
		pageSize = 500
	}

	return req, page, pageSize, nil
}

func parseTimeQuery(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02",
	} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", value)
}

func parseUint16Query(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return uint16(parsed), nil
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseBoolQuery(r *http.Request, key string, fallback bool) bool {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func dashboardSince(r *http.Request) time.Time {
	switch r.URL.Query().Get("range") {
	case "today":
		return startOfDay(time.Now())
	case "yesterday":
		return startOfDay(time.Now().AddDate(0, 0, -1))
	case "30d":
		return time.Now().AddDate(0, 0, -30)
	case "all":
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	default:
		return time.Now().AddDate(0, 0, -7)
	}
}

type appSecurityService struct {
	app *App
}

func (s appSecurityService) ChangePassword(r *http.Request) (SessionResponse, error) {
	if !s.app.isAuthenticated(r) {
		return SessionResponse{Authenticated: false}, nil
	}

	payload, err := decodePasswordChangeRequest(r)
	if err != nil {
		return SessionResponse{Authenticated: true}, err
	}
	if !s.app.verifyPassword(payload.CurrentPassword) {
		return SessionResponse{Authenticated: true}, fmt.Errorf("当前管理员密码错误")
	}

	passwordHash, err := HashPassword(payload.NewPassword)
	if err != nil {
		return SessionResponse{Authenticated: true}, err
	}

	s.app.mu.Lock()
	s.app.passwordHash = passwordHash
	authenticated := s.app.sessionToken != ""
	s.app.mu.Unlock()

	return SessionResponse{Authenticated: authenticated}, nil
}

func (s appSecurityService) ReloadIPData(_ *http.Request) (IPDataStatus, error) {
	cfg := s.app.currentIPDataConfig()

	s.app.mu.RLock()
	oldEngine := s.app.ipEngine
	s.app.mu.RUnlock()

	nextEngine, status := ReloadIPEngine(cfg, oldEngine)

	s.app.mu.Lock()
	s.app.ipEngine = nextEngine
	s.app.ipStatus = status
	s.app.mu.Unlock()

	return status, nil
}
