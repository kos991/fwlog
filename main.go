package main

import (
	"bufio"
	"database/sql"
	"embed"
	"encoding/csv"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/marcboeker/go-duckdb"
)

//go:embed assets
var assets embed.FS

type Config struct {
	LogDir              string
	DBFile              string
	Port                int
	Workers             int
	AutoScanEnabled     bool
	AutoScanIntervalSec int
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	SrcIP     string `json:"src_ip"`
	SrcPort   int    `json:"src_port"`
	DstIP     string `json:"dst_ip"`
	DstPort   int    `json:"dst_port"`
	Protocol  string `json:"protocol"`
	NatIP     string `json:"nat_ip"`
	NatPort   int    `json:"nat_port"`
	Action    string `json:"action"`
	SrcTag    IPTag  `json:"src_tag"`
	DstTag    IPTag  `json:"dst_tag"`
}

type QueryResult struct {
	Records     []LogEntry `json:"records"`
	Total       int        `json:"total"`
	Page        int        `json:"page"`
	PageSize    int        `json:"page_size"`
	QueryTimeMs float64    `json:"query_time_ms"`
}

type DashboardStats struct {
	TotalRecords   int64   `json:"total_records"`
	TotalFiles     int     `json:"total_files"`
	ActiveSessions int64   `json:"active_sessions"`
	DBSizeMB       float64 `json:"db_size_mb"`
	RawSizeMB      float64 `json:"raw_size_mb"`
	CompressionPct float64 `json:"compression_pct"`
	LastUpdate     string  `json:"last_update"`
	AvgQueryTimeMs float64 `json:"avg_query_time_ms"`
}

type DashboardData struct {
	Trend     []TrendPoint   `json:"trend"`
	TopIPs    []IPStats      `json:"top_ips"`
	Protocols []ProtocolStat `json:"protocols"`
	GovNetPct float64        `json:"gov_net_pct"`
}

type TrendPoint struct {
	Time  string `json:"time"`
	Count int    `json:"count"`
}

type ProtocolStat struct {
	Protocol string `json:"protocol"`
	Count    int    `json:"count"`
}

type IPStats struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
}

type SearchFilters struct {
	Keyword   string `json:"keyword"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	PortScope string `json:"port_scope"`
	Range     string `json:"range"`
	Protocol  string `json:"protocol"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
}

type SettingsUpdateRequest struct {
	LogDir              string `json:"log_dir"`
	AutoScanEnabled     *bool  `json:"auto_scan_enabled"`
	AutoScanIntervalSec int    `json:"auto_scan_interval_sec"`
}

type SettingsResponse struct {
	LogDir             string `json:"log_dir"`
	DBFile             string `json:"db_file"`
	ExportDir          string `json:"export_dir"`
	AutoScanEnabled    bool   `json:"auto_scan_enabled"`
	AutoScanInterval   int    `json:"auto_scan_interval_sec"`
	RebuildStatus      string `json:"rebuild_status"`
	RebuildStartedAt   string `json:"rebuild_started_at"`
	RebuildFinishedAt  string `json:"rebuild_finished_at"`
	RebuildError       string `json:"rebuild_error"`
	RebuildMode        string `json:"rebuild_mode"`
	RebuildCurrentFile string `json:"rebuild_current_file"`
	RebuildFilesTotal  int    `json:"rebuild_files_total"`
	RebuildFilesDone   int    `json:"rebuild_files_done"`
	RebuildBytesTotal  int64  `json:"rebuild_bytes_total"`
	RebuildBytesDone   int64  `json:"rebuild_bytes_done"`
	RebuildElapsedSec  int64  `json:"rebuild_elapsed_sec"`
	RebuildEtaSec      int64  `json:"rebuild_eta_sec"`
}

type ExportResponse struct {
	FileName      string `json:"file_name"`
	Category      string `json:"category"`
	ExportedCount int    `json:"exported_count"`
	DownloadPath  string `json:"download_path"`
}

type RebuildState struct {
	Status      string
	Mode        string
	StartedAt   time.Time
	FinishedAt  time.Time
	Error       string
	CurrentFile string
	FilesTotal  int
	FilesDone   int
	BytesTotal  int64
	BytesDone   int64
}

type LogFileSnapshot struct {
	Path string
	Size int64
}

type LogFileRange struct {
	Path  string
	Start int64
	End   int64
}

const (
	defaultLogDir      = "/data/sangfor_fw_log"
	defaultDBFile      = "/data/index/nat_logs.duckdb"
	defaultLocalLogDir = "./data/sangfor_fw_log"
	defaultLocalDBFile = "./data/index/nat_logs.duckdb"
	defaultPort        = 8080
	defaultWorkers     = 4
	defaultAutoScanSec = 30

	defaultPageSize = 25
	maxPageSize     = 200
)

var (
	config    Config
	configMux sync.RWMutex

	db *sql.DB

	ipEngine *IPEngine

	rebuildMux   sync.Mutex
	rebuildState = RebuildState{Status: "idle"}

	natRegex = regexp.MustCompile(`源IP:([0-9.]+).*?源端口:(\d+).*?目的IP:([0-9.]+).*?目的端口:(\d+).*?协议:(\d+).*?转换后的IP:([0-9.]+).*?转换后的端口:(\d+)`)
)

func main() {
	config = loadConfig()
	ipEngine = NewIPEngine()

	customMap := filepath.Join(filepath.Dir(currentConfig().DBFile), "custom_ip_map.csv")
	if err := ipEngine.LoadCustomMap(customMap); err == nil {
		log.Printf("加载自定义 IP 映射: %s", customMap)
	}

	geoDB := filepath.Join(filepath.Dir(currentConfig().DBFile), "GeoLite2-City.mmdb")
	if err := ipEngine.LoadGeoDB(geoDB); err == nil {
		log.Printf("加载 GeoIP 数据库: %s", geoDB)
	} else {
		embeddedGeoDB, readErr := assets.ReadFile("assets/vendor/GeoLite2-City.mmdb")
		if readErr != nil {
			log.Printf("未加载离线 GeoIP 数据库 (GeoLite2-City.mmdb): %v", err)
		} else if loadErr := ipEngine.LoadGeoDBBytes(embeddedGeoDB); loadErr != nil {
			log.Printf("加载内置 GeoIP 数据库失败: %v", loadErr)
		} else {
			log.Printf("加载内置 GeoIP 数据库: assets/vendor/GeoLite2-City.mmdb")
		}
	}

	log.Printf("NAT日志查询系统启动中...")
	log.Printf("日志目录: %s", currentConfig().LogDir)
	log.Printf("数据库: %s", currentConfig().DBFile)
	log.Printf("端口: %d", currentConfig().Port)

	os.MkdirAll(filepath.Dir(currentConfig().DBFile), 0755)
	os.MkdirAll(exportBaseDir(currentConfig()), 0755)

	var err error
	db, err = sql.Open("duckdb", currentConfig().DBFile)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer db.Close()

	db.Exec("SET memory_limit='2GB'")
	db.Exec(fmt.Sprintf("SET threads=%d", currentConfig().Workers))

	if err := ensureRuntimeTables(); err != nil {
		log.Fatalf("初始化运行时数据表失败: %v", err)
	}
	if err := loadPersistedSettings(); err != nil {
		log.Printf("加载持久化设置失败，继续使用当前配置: %v", err)
	}

	if !tableExists() {
		setRebuildRunning("full_rebuild")
		if err := buildIndex(); err != nil {
			setRebuildFinished(err)
			log.Fatalf("构建索引失败: %v", err)
		}
		setRebuildFinished(nil)
	}

	go autoSyncLoop()

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	staticAssets, err := fs.Sub(assets, "assets")
	if err != nil {
		log.Fatalf("初始化静态资源失败: %v", err)
	}

	r.GET("/", serveIndex)
	r.StaticFS("/assets", http.FS(staticAssets))
	r.GET("/api/query", handleQuery)
	r.GET("/api/stats", handleStats)
	r.GET("/api/top-ips", handleTopIPs)
	r.GET("/api/settings", handleSettings)
	r.POST("/api/settings/log-dir", handleSetLogDir)
	r.POST("/api/rebuild", handleRebuild)
	r.POST("/api/sync", handleSync)
	r.POST("/api/export", handleExport)
	r.GET("/api/dashboard", handleDashboardData)
	r.GET("/api/exports/*filepath", handleExportDownload)
	r.NoRoute(handleNotFound)

	addr := fmt.Sprintf(":%d", currentConfig().Port)
	log.Printf("服务已启动: http://0.0.0.0%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

func loadConfig() Config {
	defaults := defaultConfig()

	return Config{
		LogDir:              getEnv("LOG_DIR", defaults.LogDir),
		DBFile:              getEnv("DB_FILE", defaults.DBFile),
		Port:                getEnvInt("PORT", defaults.Port),
		Workers:             getEnvInt("WORKERS", defaults.Workers),
		AutoScanEnabled:     getEnvBool("AUTO_SCAN_ENABLED", defaults.AutoScanEnabled),
		AutoScanIntervalSec: getEnvInt("AUTO_SCAN_INTERVAL_SEC", defaults.AutoScanIntervalSec),
	}
}

func defaultConfig() Config {
	if pathExists(defaultLocalLogDir) {
		return Config{
			LogDir:              defaultLocalLogDir,
			DBFile:              defaultLocalDBFile,
			Port:                defaultPort,
			Workers:             defaultWorkers,
			AutoScanEnabled:     false,
			AutoScanIntervalSec: defaultAutoScanSec,
		}
	}

	return Config{
		LogDir:              defaultLogDir,
		DBFile:              defaultDBFile,
		Port:                defaultPort,
		Workers:             defaultWorkers,
		AutoScanEnabled:     false,
		AutoScanIntervalSec: defaultAutoScanSec,
	}
}

func currentConfig() Config {
	configMux.RLock()
	defer configMux.RUnlock()
	return config
}

func setLogDir(logDir string) {
	configMux.Lock()
	defer configMux.Unlock()
	config.LogDir = logDir
}

func updateSettings(logDir string, autoScanEnabled bool, autoScanIntervalSec int) {
	configMux.Lock()
	defer configMux.Unlock()
	config.LogDir = logDir
	config.AutoScanEnabled = autoScanEnabled
	config.AutoScanIntervalSec = autoScanIntervalSec
}

func ensureRuntimeTables() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_settings (
			key VARCHAR PRIMARY KEY,
			value VARCHAR
		);
		CREATE TABLE IF NOT EXISTS ingest_files (
			path VARCHAR PRIMARY KEY,
			size_bytes BIGINT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

func loadPersistedSettings() error {
	rows, err := db.Query(`SELECT key, value FROM app_settings WHERE key IN ('log_dir', 'auto_scan_enabled', 'auto_scan_interval_sec')`)
	if err != nil {
		return err
	}
	defer rows.Close()

	cfg := currentConfig()
	logDir := cfg.LogDir
	autoScanEnabled := cfg.AutoScanEnabled
	autoScanIntervalSec := cfg.AutoScanIntervalSec

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		switch key {
		case "log_dir":
			if strings.TrimSpace(value) != "" {
				logDir = value
			}
		case "auto_scan_enabled":
			autoScanEnabled = strings.EqualFold(value, "true") || value == "1"
		case "auto_scan_interval_sec":
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				autoScanIntervalSec = parsed
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	updateSettings(logDir, autoScanEnabled, autoScanIntervalSec)
	return nil
}

func persistSettings(cfg Config) error {
	values := map[string]string{
		"log_dir":                cfg.LogDir,
		"auto_scan_enabled":      strconv.FormatBool(cfg.AutoScanEnabled),
		"auto_scan_interval_sec": strconv.Itoa(cfg.AutoScanIntervalSec),
	}
	for key, value := range values {
		if _, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func exportBaseDir(cfg Config) string {
	dbDir := filepath.Dir(cfg.DBFile)
	parent := filepath.Dir(dbDir)
	if parent == "." || parent == "" {
		return filepath.Join("data", "export")
	}
	return filepath.Join(parent, "export")
}

func currentRebuildState() RebuildState {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	return rebuildState
}

func setRebuildRunning(mode string) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState = RebuildState{
		Status:    "running",
		Mode:      mode,
		StartedAt: time.Now(),
	}
}

func setRebuildFinished(err error) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.FinishedAt = time.Now()
	rebuildState.CurrentFile = ""
	if err != nil {
		rebuildState.Status = "failed"
		rebuildState.Error = err.Error()
		return
	}
	rebuildState.Status = "succeeded"
	rebuildState.Error = ""
}

func setRebuildTotals(filesTotal int, bytesTotal int64) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.FilesTotal = filesTotal
	rebuildState.BytesTotal = bytesTotal
}

func addRebuildTotals(files int, bytes int64) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.FilesTotal += files
	rebuildState.BytesTotal += bytes
}

func setRebuildCurrentFile(path string) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.CurrentFile = path
}

func setRebuildMode(mode string) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.Mode = mode
}

func advanceRebuildProgress(filesDone int, bytesDone int64) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.FilesDone += filesDone
	rebuildState.BytesDone += bytesDone
}

func rebuildStateMetrics(state RebuildState) (int64, int64) {
	if state.StartedAt.IsZero() {
		return 0, 0
	}
	endTime := time.Now()
	if !state.FinishedAt.IsZero() {
		endTime = state.FinishedAt
	}
	elapsed := int64(endTime.Sub(state.StartedAt).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	if state.BytesDone <= 0 || state.BytesTotal <= state.BytesDone || elapsed == 0 {
		return elapsed, 0
	}
	remainingBytes := state.BytesTotal - state.BytesDone
	eta := int64(float64(elapsed) / float64(state.BytesDone) * float64(remainingBytes))
	if eta < 0 {
		eta = 0
	}
	return elapsed, eta
}

func startRebuild() error {
	rebuildMux.Lock()
	if rebuildState.Status == "running" {
		rebuildMux.Unlock()
		return fmt.Errorf("索引正在重建中")
	}
	rebuildState = RebuildState{
		Status:    "running",
		Mode:      "full_rebuild",
		StartedAt: time.Now(),
	}
	rebuildMux.Unlock()

	go func() {
		err := buildIndex()
		setRebuildFinished(err)
		if err != nil {
			log.Printf("索引重建失败: %v", err)
			return
		}
		log.Printf("索引重建完成")
	}()

	return nil
}

func startIncrementalSync() error {
	rebuildMux.Lock()
	if rebuildState.Status == "running" {
		rebuildMux.Unlock()
		return fmt.Errorf("当前已有任务在运行")
	}
	rebuildState = RebuildState{
		Status:    "running",
		Mode:      "incremental_sync",
		StartedAt: time.Now(),
	}
	rebuildMux.Unlock()

	go func() {
		err := runIncrementalSync()
		setRebuildFinished(err)
		if err != nil {
			log.Printf("增量同步失败: %v", err)
			return
		}
		log.Printf("增量同步完成")
	}()

	return nil
}

func tableExists() bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_name='nat_logs'").Scan(&count)
	return err == nil && count > 0
}

func hasSourceMetadataColumns() bool {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_name='nat_logs'
		  AND column_name IN ('source_file', 'source_offset')
	`).Scan(&count)
	return err == nil && count == 2
}

func buildIndex() error {
	startTime := time.Now()
	cfg := currentConfig()
	setRebuildMode("full_rebuild")

	snapshots, err := snapshotLogFiles(cfg.LogDir)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 {
		log.Printf("未找到日志文件，创建空索引表")
		return createEmptyIndex()
	}

	setRebuildTotals(len(snapshots), sumSnapshotBytes(snapshots))
	log.Printf("找到 %d 个日志文件", len(snapshots))

	_, err = db.Exec(`
		DROP TABLE IF EXISTS nat_logs_next;
		CREATE TABLE nat_logs_next (
			timestamp VARCHAR,
			src_ip VARCHAR,
			src_port INTEGER,
			dst_ip VARCHAR,
			dst_port INTEGER,
			protocol VARCHAR,
			nat_ip VARCHAR,
			nat_port INTEGER,
			action VARCHAR,
			source_file VARCHAR,
			source_offset BIGINT
		);
	`)
	if err != nil {
		return fmt.Errorf("创建表失败: %v", err)
	}

	tmpCSV := filepath.Join(filepath.Dir(cfg.DBFile), "tmp_import.csv")
	csvFile, err := os.Create(tmpCSV)
	if err != nil {
		return err
	}
	defer os.Remove(tmpCSV)

	writer := bufio.NewWriter(csvFile)
	totalLines := 0

	for _, snapshot := range snapshots {
		setRebuildCurrentFile(snapshot.Path)
		log.Printf("处理: %s", filepath.Base(snapshot.Path))
		if err := processLogFileRange(snapshot.Path, 0, snapshot.Size, writer, &totalLines); err != nil {
			log.Printf("处理文件失败 %s: %v", snapshot.Path, err)
			return fmt.Errorf("build snapshot file %s: %w", snapshot.Path, err)
		}
		advanceRebuildProgress(1, snapshot.Size)
	}

	if err := writer.Flush(); err != nil {
		return err
	}
	if err := csvFile.Close(); err != nil {
		return err
	}

	log.Printf("导入数据库...")
	importStart := time.Now()
	_, err = db.Exec(fmt.Sprintf("COPY nat_logs_next FROM '%s' (DELIMITER '|', HEADER false);", strings.ReplaceAll(tmpCSV, "\\", "/")))
	if err != nil {
		return fmt.Errorf("导入失败: %v", err)
	}

	log.Printf("导入完成，耗时 %.2f 秒", time.Since(importStart).Seconds())

	_, err = db.Exec(`
		DROP TABLE IF EXISTS nat_logs;
		ALTER TABLE nat_logs_next RENAME TO nat_logs;
	`)
	if err != nil {
		return fmt.Errorf("切换索引表失败: %v", err)
	}

	log.Println("创建索引...")
	currentSnapshots, err := snapshotLogFiles(cfg.LogDir)
	if err != nil {
		return err
	}
	catchUpRanges := discoverCatchUpRanges(snapshots, currentSnapshots)
	addRebuildTotals(len(catchUpRanges), sumRangeBytes(catchUpRanges))
	if err := appendCatchUpDataWithProgress(catchUpRanges, cfg.DBFile); err != nil {
		return err
	}
	if err := saveIngestSnapshots(currentSnapshots); err != nil {
		return err
	}

	db.Exec("CREATE INDEX idx_src_ip ON nat_logs(src_ip)")
	db.Exec("CREATE INDEX idx_dst_ip ON nat_logs(dst_ip)")
	db.Exec("CREATE INDEX idx_nat_ip ON nat_logs(nat_ip)")
	db.Exec("CREATE INDEX idx_src_port ON nat_logs(src_port)")
	db.Exec("CREATE INDEX idx_dst_port ON nat_logs(dst_port)")
	db.Exec("CREATE INDEX idx_nat_port ON nat_logs(nat_port)")
	db.Exec("CREATE INDEX idx_protocol ON nat_logs(protocol)")
	db.Exec("CREATE INDEX idx_timestamp ON nat_logs(timestamp)")
	db.Exec("CHECKPOINT")

	log.Printf("索引构建完成: %d 条记录, 总耗时 %.2f 秒", totalLines, time.Since(startTime).Seconds())
	return nil
}

func createEmptyIndex() error {
	setRebuildTotals(0, 0)
	_, err := db.Exec(`
		DROP TABLE IF EXISTS nat_logs_next;
		DROP TABLE IF EXISTS nat_logs;
		CREATE TABLE nat_logs (
			timestamp VARCHAR,
			src_ip VARCHAR,
			src_port INTEGER,
			dst_ip VARCHAR,
			dst_port INTEGER,
			protocol VARCHAR,
			nat_ip VARCHAR,
			nat_port INTEGER,
			action VARCHAR,
			source_file VARCHAR,
			source_offset BIGINT
		);
		DELETE FROM ingest_files;
		CHECKPOINT;
	`)
	if err != nil {
		return fmt.Errorf("创建空索引表失败: %v", err)
	}
	return nil
}

func snapshotLogFiles(logDir string) ([]LogFileSnapshot, error) {
	files, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		return nil, err
	}

	snapshots := make([]LogFileSnapshot, 0, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		snapshots = append(snapshots, LogFileSnapshot{
			Path: file,
			Size: info.Size(),
		})
	}

	return snapshots, nil
}

func discoverCatchUpRanges(initial []LogFileSnapshot, current []LogFileSnapshot) []LogFileRange {
	initialByPath := make(map[string]int64, len(initial))
	for _, snapshot := range initial {
		initialByPath[snapshot.Path] = snapshot.Size
	}

	ranges := make([]LogFileRange, 0)
	for _, snapshot := range current {
		start, ok := initialByPath[snapshot.Path]
		if !ok {
			if snapshot.Size > 0 {
				ranges = append(ranges, LogFileRange{Path: snapshot.Path, Start: 0, End: snapshot.Size})
			}
			continue
		}
		if snapshot.Size > start {
			ranges = append(ranges, LogFileRange{Path: snapshot.Path, Start: start, End: snapshot.Size})
		}
	}

	return ranges
}

func requiresFullRebuild(stored []LogFileSnapshot, current []LogFileSnapshot) bool {
	currentByPath := make(map[string]int64, len(current))
	for _, snapshot := range current {
		currentByPath[snapshot.Path] = snapshot.Size
	}

	for _, snapshot := range stored {
		size, ok := currentByPath[snapshot.Path]
		if !ok || size < snapshot.Size {
			return true
		}
	}

	return false
}

func appendCatchUpData(ranges []LogFileRange, dbFile string, totalLines *int) error {
	if len(ranges) == 0 {
		return nil
	}

	tmpCSV := filepath.Join(filepath.Dir(dbFile), "tmp_import_catchup.csv")
	csvFile, err := os.Create(tmpCSV)
	if err != nil {
		return err
	}
	defer os.Remove(tmpCSV)

	writer := bufio.NewWriter(csvFile)
	addedLines := 0
	for _, fileRange := range ranges {
		if err := processLogFileRange(fileRange.Path, fileRange.Start, fileRange.End, writer, &addedLines); err != nil {
			return err
		}
	}

	if err := writer.Flush(); err != nil {
		return err
	}
	if err := csvFile.Close(); err != nil {
		return err
	}
	if addedLines == 0 {
		return nil
	}

	if err := importCatchUpCSV(tmpCSV, ranges); err != nil {
		return err
	}

	*totalLines += addedLines
	return nil
}

func importCatchUpCSV(tmpCSV string, ranges []LogFileRange) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, fileRange := range ranges {
		if _, err := tx.Exec(`DELETE FROM nat_logs WHERE source_file = ? AND source_offset >= ? AND source_offset < ?`, fileRange.Path, fileRange.Start, fileRange.End); err != nil {
			return fmt.Errorf("delete overlapping range %s[%d,%d): %w", fileRange.Path, fileRange.Start, fileRange.End, err)
		}
	}

	copyPath := strings.ReplaceAll(tmpCSV, "\\", "/")
	if _, err := tx.Exec(fmt.Sprintf("COPY nat_logs FROM '%s' (DELIMITER '|', HEADER false);", copyPath)); err != nil {
		return fmt.Errorf("copy catch-up data: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func sumSnapshotBytes(snapshots []LogFileSnapshot) int64 {
	var total int64
	for _, snapshot := range snapshots {
		total += snapshot.Size
	}
	return total
}

func sumRangeBytes(ranges []LogFileRange) int64 {
	var total int64
	for _, fileRange := range ranges {
		if fileRange.End > fileRange.Start {
			total += fileRange.End - fileRange.Start
		}
	}
	return total
}

func loadIngestSnapshots() ([]LogFileSnapshot, error) {
	rows, err := db.Query(`SELECT path, size_bytes FROM ingest_files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := make([]LogFileSnapshot, 0)
	for rows.Next() {
		var snapshot LogFileSnapshot
		if err := rows.Scan(&snapshot.Path, &snapshot.Size); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func saveIngestSnapshots(snapshots []LogFileSnapshot) error {
	if _, err := db.Exec(`DELETE FROM ingest_files`); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if _, err := db.Exec(`INSERT INTO ingest_files (path, size_bytes, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, snapshot.Path, snapshot.Size); err != nil {
			return err
		}
	}
	return nil
}

func runIncrementalSync() error {
	cfg := currentConfig()
	setRebuildMode("incremental_sync")
	storedSnapshots, err := loadIngestSnapshots()
	if err != nil {
		return err
	}

	currentSnapshots, err := snapshotLogFiles(cfg.LogDir)
	if err != nil {
		return err
	}

	if !hasSourceMetadataColumns() {
		return buildIndex()
	}

	if len(storedSnapshots) == 0 {
		return buildIndex()
	}

	if requiresFullRebuild(storedSnapshots, currentSnapshots) {
		return buildIndex()
	}

	catchUpRanges := discoverCatchUpRanges(storedSnapshots, currentSnapshots)
	setRebuildTotals(len(catchUpRanges), sumRangeBytes(catchUpRanges))
	if err := appendCatchUpDataWithProgress(catchUpRanges, cfg.DBFile); err != nil {
		return err
	}

	if err := saveIngestSnapshots(currentSnapshots); err != nil {
		return err
	}

	db.Exec("CHECKPOINT")
	return nil
}

func appendCatchUpDataWithProgress(ranges []LogFileRange, dbFile string) error {
	if len(ranges) == 0 {
		return nil
	}

	tmpCSV := filepath.Join(filepath.Dir(dbFile), "tmp_import_catchup.csv")
	csvFile, err := os.Create(tmpCSV)
	if err != nil {
		return err
	}
	defer os.Remove(tmpCSV)

	writer := bufio.NewWriter(csvFile)
	addedLines := 0
	for _, fileRange := range ranges {
		setRebuildCurrentFile(fileRange.Path)
		if err := processLogFileRange(fileRange.Path, fileRange.Start, fileRange.End, writer, &addedLines); err != nil {
			return err
		}
		advanceRebuildProgress(1, fileRange.End-fileRange.Start)
	}

	if err := writer.Flush(); err != nil {
		return err
	}
	if err := csvFile.Close(); err != nil {
		return err
	}
	if addedLines == 0 {
		return nil
	}

	return importCatchUpCSV(tmpCSV, ranges)
}

func autoSyncLoop() {
	for {
		cfg := currentConfig()
		interval := cfg.AutoScanIntervalSec
		if interval <= 0 {
			interval = defaultAutoScanSec
		}
		time.Sleep(time.Duration(interval) * time.Second)

		cfg = currentConfig()
		if !cfg.AutoScanEnabled {
			continue
		}
		if state := currentRebuildState(); state.Status == "running" {
			continue
		}
		if err := startIncrementalSync(); err != nil {
			log.Printf("跳过自动增量同步: %v", err)
		}
	}
}

func processLogFile(filePath string, writer io.Writer, totalLines *int) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	return processLogReaderWithOffsets(filePath, file, 0, info.Size(), writer, totalLines)
}

func processLogFileRange(filePath string, startOffset, endOffset int64, writer io.Writer, totalLines *int) error {
	if endOffset <= startOffset {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return err
	}

	return processLogReaderWithOffsets(filePath, file, startOffset, endOffset-startOffset, writer, totalLines)
}

func processLogReaderWithLimit(reader io.Reader, limit int64, writer io.Writer, totalLines *int) error {
	limitedReader := reader
	if limit > 0 {
		limitedReader = io.LimitReader(reader, limit)
	}

	scanner := bufio.NewScanner(limitedReader)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		matches := natRegex.FindStringSubmatch(line)
		if len(matches) < 8 {
			continue
		}

		timestamp := extractTimestamp(line)
		protocol := mapProtocol(matches[5])
		action := "ACCEPT"

		fmt.Fprintf(writer, "%s|%s|%s|%s|%s|%s|%s|%s|%s\n",
			timestamp, matches[1], matches[2], matches[3], matches[4],
			protocol, matches[6], matches[7], action)

		*totalLines++
		if *totalLines%100000 == 0 {
			log.Printf("已处理 %d 行...", *totalLines)
		}
	}

	return scanner.Err()
}

func processLogReaderWithOffsets(filePath string, reader io.Reader, baseOffset int64, limit int64, writer io.Writer, totalLines *int) error {
	limitedReader := reader
	if limit > 0 {
		limitedReader = io.LimitReader(reader, limit)
	}

	buffered := bufio.NewReaderSize(limitedReader, 1024*1024)
	currentOffset := baseOffset

	for {
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			recordOffset := currentOffset
			currentOffset += int64(len(line))

			trimmed := strings.TrimRight(line, "\r\n")
			matches := natRegex.FindStringSubmatch(trimmed)
			if len(matches) >= 8 {
				timestamp := extractTimestamp(trimmed)
				protocol := mapProtocol(matches[5])
				action := "ACCEPT"

				fmt.Fprintf(writer, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%d\n",
					timestamp, matches[1], matches[2], matches[3], matches[4],
					protocol, matches[6], matches[7], action, filePath, recordOffset)

				*totalLines++
				if *totalLines%100000 == 0 {
					log.Printf("processed %d lines", *totalLines)
				}
			}
		}

		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func extractTimestamp(line string) string {
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return fmt.Sprintf("%s %s %s", parts[0], parts[1], parts[2])
	}
	return time.Now().Format("Jan 02 15:04:05")
}

func mapProtocol(protoNum string) string {
	switch protoNum {
	case "6":
		return "TCP"
	case "17":
		return "UDP"
	case "1":
		return "ICMP"
	default:
		return "OTHER"
	}
}

func parseFiltersFromQuery(c *gin.Context) (SearchFilters, error) {
	filters := SearchFilters{
		Keyword:   strings.TrimSpace(c.Query("keyword")),
		IP:        strings.TrimSpace(c.Query("ip")),
		Range:     strings.TrimSpace(c.Query("range")),
		Protocol:  strings.TrimSpace(c.Query("protocol")),
		PortScope: strings.TrimSpace(c.Query("port_scope")),
		Page:      1,
		PageSize:  defaultPageSize,
	}

	if c.Query("page") != "" {
		fmt.Sscanf(c.Query("page"), "%d", &filters.Page)
	}
	if c.Query("page_size") != "" {
		fmt.Sscanf(c.Query("page_size"), "%d", &filters.PageSize)
	}
	if portValue := strings.TrimSpace(c.Query("port")); portValue != "" {
		port, err := strconv.Atoi(portValue)
		if err != nil || port <= 0 {
			return filters, fmt.Errorf("端口必须是正整数")
		}
		filters.Port = port
	}

	return normalizeFilters(filters)
}

func parseFiltersFromJSON(c *gin.Context) (SearchFilters, error) {
	var filters SearchFilters
	if err := c.ShouldBindJSON(&filters); err != nil {
		return filters, fmt.Errorf("请求参数格式错误")
	}
	return normalizeFilters(filters)
}

func normalizeFilters(filters SearchFilters) (SearchFilters, error) {
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	filters.IP = strings.TrimSpace(filters.IP)
	filters.Range = strings.TrimSpace(filters.Range)
	filters.Protocol = strings.ToUpper(strings.TrimSpace(filters.Protocol))
	filters.PortScope = strings.ToLower(strings.TrimSpace(filters.PortScope))

	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 {
		filters.PageSize = defaultPageSize
	}
	if filters.PageSize > maxPageSize {
		filters.PageSize = maxPageSize
	}
	if filters.PortScope == "" {
		filters.PortScope = "any"
	}
	switch filters.PortScope {
	case "any", "src", "dst", "nat":
	default:
		return filters, fmt.Errorf("端口范围必须是 any、src、dst 或 nat")
	}
	if filters.Port < 0 {
		return filters, fmt.Errorf("端口必须是正整数")
	}

	return filters, nil
}

func buildSearchQueries(filters SearchFilters, paginate bool) (string, string, []interface{}) {
	baseSelect := "SELECT timestamp, src_ip, src_port, dst_ip, dst_port, protocol, nat_ip, nat_port, action FROM nat_logs"
	whereParts := []string{"1=1"}
	args := make([]interface{}, 0)

	if filters.Keyword != "" {
		pattern := "%" + strings.ToLower(filters.Keyword) + "%"
		whereParts = append(whereParts, "(LOWER(timestamp) LIKE ? OR LOWER(src_ip) LIKE ? OR CAST(src_port AS VARCHAR) LIKE ? OR LOWER(dst_ip) LIKE ? OR CAST(dst_port AS VARCHAR) LIKE ? OR LOWER(protocol) LIKE ? OR LOWER(nat_ip) LIKE ? OR CAST(nat_port AS VARCHAR) LIKE ? OR LOWER(action) LIKE ?)")
		for i := 0; i < 9; i++ {
			args = append(args, pattern)
		}
	}

	if filters.IP != "" {
		whereParts = append(whereParts, "(src_ip = ? OR dst_ip = ? OR nat_ip = ?)")
		args = append(args, filters.IP, filters.IP, filters.IP)
	}

	if filters.Range != "" {
		timeFilter := getTimeFilter(filters.Range)
		if timeFilter != "" {
			whereParts = append(whereParts, timeFilter)
		}
	}

	if filters.Protocol != "" {
		whereParts = append(whereParts, "protocol = ?")
		args = append(args, filters.Protocol)
	}

	if filters.Port > 0 {
		switch filters.PortScope {
		case "src":
			whereParts = append(whereParts, "src_port = ?")
			args = append(args, filters.Port)
		case "dst":
			whereParts = append(whereParts, "dst_port = ?")
			args = append(args, filters.Port)
		case "nat":
			whereParts = append(whereParts, "nat_port = ?")
			args = append(args, filters.Port)
		default:
			whereParts = append(whereParts, "(src_port = ? OR dst_port = ? OR nat_port = ?)")
			args = append(args, filters.Port, filters.Port, filters.Port)
		}
	}

	whereSQL := strings.Join(whereParts, " AND ")
	selectSQL := baseSelect + " WHERE " + whereSQL + " ORDER BY timestamp DESC"
	countSQL := "SELECT COUNT(*) FROM nat_logs WHERE " + whereSQL

	if paginate {
		selectSQL += fmt.Sprintf(" LIMIT %d OFFSET %d", filters.PageSize, (filters.Page-1)*filters.PageSize)
	}

	return selectSQL, countSQL, args
}

func handleQuery(c *gin.Context) {
	if state := currentRebuildState(); state.Status == "running" {
		c.JSON(409, gin.H{"error": "索引重建中，请稍后再试"})
		return
	}

	startTime := time.Now()

	filters, err := parseFiltersFromQuery(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	query, countQuery, args := buildSearchQueries(filters, true)

	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var records []LogEntry
	for rows.Next() {
		var record LogEntry
		if err := rows.Scan(&record.Timestamp, &record.SrcIP, &record.SrcPort, &record.DstIP, &record.DstPort, &record.Protocol, &record.NatIP, &record.NatPort, &record.Action); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if ipEngine != nil {
			record.SrcTag = ipEngine.GetTag(record.SrcIP)
			record.DstTag = ipEngine.GetTag(record.DstIP)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, QueryResult{
		Records:     records,
		Total:       total,
		Page:        filters.Page,
		PageSize:    filters.PageSize,
		QueryTimeMs: time.Since(startTime).Seconds() * 1000,
	})
}

func getTimeFilter(timeRange string) string {
	now := time.Now()
	var cutoff time.Time

	switch timeRange {
	case "1h":
		cutoff = now.Add(-1 * time.Hour)
	case "6h":
		cutoff = now.Add(-6 * time.Hour)
	case "24h":
		cutoff = now.Add(-24 * time.Hour)
	case "7d":
		cutoff = now.Add(-7 * 24 * time.Hour)
	default:
		return ""
	}

	return fmt.Sprintf("timestamp >= '%s'", cutoff.Format("Jan 02 15:04:05"))
}

func handleStats(c *gin.Context) {
	cfg := currentConfig()
	state := currentRebuildState()

	var stats DashboardStats
	if state.Status != "running" && tableExists() {
		_ = db.QueryRow("SELECT COUNT(*) FROM nat_logs").Scan(&stats.TotalRecords)
		_ = db.QueryRow("SELECT COUNT(DISTINCT src_ip || ':' || CAST(src_port AS VARCHAR) || '|' || dst_ip || ':' || CAST(dst_port AS VARCHAR) || '|' || nat_ip || ':' || CAST(nat_port AS VARCHAR)) FROM nat_logs").Scan(&stats.ActiveSessions)
	}

	files, _ := filepath.Glob(filepath.Join(cfg.LogDir, "*.log"))
	stats.TotalFiles = len(files)

	if info, err := os.Stat(cfg.DBFile); err == nil {
		stats.DBSizeMB = float64(info.Size()) / 1024 / 1024
		stats.LastUpdate = info.ModTime().Format("2006-01-02 15:04:05")
	}

	var rawSize int64
	for _, file := range files {
		if info, err := os.Stat(file); err == nil {
			rawSize += info.Size()
		}
	}
	stats.RawSizeMB = float64(rawSize) / 1024 / 1024
	if stats.RawSizeMB > 0 {
		stats.CompressionPct = (1 - stats.DBSizeMB/stats.RawSizeMB) * 100
	}
	stats.AvgQueryTimeMs = 35

	c.JSON(200, stats)
}

func handleTopIPs(c *gin.Context) {
	if state := currentRebuildState(); state.Status == "running" || !tableExists() {
		c.JSON(200, []IPStats{})
		return
	}

	rows, err := db.Query(`
		SELECT src_ip, COUNT(*) as cnt
		FROM nat_logs
		WHERE src_ip LIKE '10.%' OR src_ip LIKE '172.%' OR src_ip LIKE '192.168.%'
		GROUP BY src_ip
		ORDER BY cnt DESC
		LIMIT 5
	`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var topIPs []IPStats
	for rows.Next() {
		var ip IPStats
		if err := rows.Scan(&ip.IP, &ip.Count); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		topIPs = append(topIPs, ip)
	}
	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, topIPs)
}

func handleDashboardData(c *gin.Context) {
	if state := currentRebuildState(); state.Status == "running" || !tableExists() {
		c.JSON(200, DashboardData{
			Trend:     []TrendPoint{},
			TopIPs:    []IPStats{},
			Protocols: []ProtocolStat{},
		})
		return
	}

	var data DashboardData

	rows, err := db.Query(`
		SELECT src_ip, COUNT(*) as cnt
		FROM nat_logs
		GROUP BY src_ip
		ORDER BY cnt DESC
		LIMIT 10
	`)
	if err == nil {
		for rows.Next() {
			var ip IPStats
			_ = rows.Scan(&ip.IP, &ip.Count)
			data.TopIPs = append(data.TopIPs, ip)
		}
		rows.Close()
	}

	rows, err = db.Query(`
		SELECT protocol, COUNT(*) as cnt
		FROM nat_logs
		GROUP BY protocol
		ORDER BY cnt DESC
	`)
	if err == nil {
		for rows.Next() {
			var protocol ProtocolStat
			_ = rows.Scan(&protocol.Protocol, &protocol.Count)
			data.Protocols = append(data.Protocols, protocol)
		}
		rows.Close()
	}

	rows, err = db.Query(`
		SELECT substr(timestamp, 1, 9) as t, COUNT(*) as cnt
		FROM nat_logs
		GROUP BY t
		ORDER BY t ASC
		LIMIT 24
	`)
	if err == nil {
		for rows.Next() {
			var trend TrendPoint
			_ = rows.Scan(&trend.Time, &trend.Count)
			data.Trend = append(data.Trend, trend)
		}
		rows.Close()
	}

	var govCount int64
	_ = db.QueryRow("SELECT COUNT(*) FROM nat_logs WHERE src_ip LIKE '172.18.%' OR src_ip LIKE '172.28.%' OR src_ip LIKE '2.%'").Scan(&govCount)
	var totalRecords int64
	_ = db.QueryRow("SELECT COUNT(*) FROM nat_logs").Scan(&totalRecords)
	if totalRecords > 0 {
		data.GovNetPct = float64(govCount) / float64(totalRecords) * 100
	}

	c.JSON(200, data)
}

func handleSettings(c *gin.Context) {
	cfg := currentConfig()
	state := currentRebuildState()

	response := SettingsResponse{
		LogDir:             cfg.LogDir,
		DBFile:             cfg.DBFile,
		ExportDir:          exportBaseDir(cfg),
		AutoScanEnabled:    cfg.AutoScanEnabled,
		AutoScanInterval:   cfg.AutoScanIntervalSec,
		RebuildStatus:      state.Status,
		RebuildError:       state.Error,
		RebuildMode:        state.Mode,
		RebuildCurrentFile: state.CurrentFile,
		RebuildFilesTotal:  state.FilesTotal,
		RebuildFilesDone:   state.FilesDone,
		RebuildBytesTotal:  state.BytesTotal,
		RebuildBytesDone:   state.BytesDone,
	}
	response.RebuildElapsedSec, response.RebuildEtaSec = rebuildStateMetrics(state)
	if !state.StartedAt.IsZero() {
		response.RebuildStartedAt = state.StartedAt.Format("2006-01-02 15:04:05")
	}
	if !state.FinishedAt.IsZero() {
		response.RebuildFinishedAt = state.FinishedAt.Format("2006-01-02 15:04:05")
	}

	c.JSON(200, response)
}

func handleSetLogDir(c *gin.Context) {
	var request SettingsUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "invalid request payload"})
		return
	}

	cfg := currentConfig()
	logDir := strings.TrimSpace(request.LogDir)
	if logDir == "" {
		logDir = cfg.LogDir
	}

	pathChanged := logDir != cfg.LogDir
	if pathChanged {
		info, err := os.Stat(logDir)
		if err != nil {
			c.JSON(400, gin.H{"error": "log path does not exist"})
			return
		}
		if !info.IsDir() {
			c.JSON(400, gin.H{"error": "log path must be a directory"})
			return
		}
	}

	if state := currentRebuildState(); state.Status == "running" {
		c.JSON(409, gin.H{"error": "index rebuild is running, please try again later"})
		return
	}

	autoScanEnabled := cfg.AutoScanEnabled
	if request.AutoScanEnabled != nil {
		autoScanEnabled = *request.AutoScanEnabled
	}
	autoScanIntervalSec := cfg.AutoScanIntervalSec
	if request.AutoScanIntervalSec > 0 {
		autoScanIntervalSec = request.AutoScanIntervalSec
	}

	updateSettings(logDir, autoScanEnabled, autoScanIntervalSec)
	if err := persistSettings(currentConfig()); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if !pathChanged {
		c.JSON(200, gin.H{
			"status":                 "updated",
			"log_dir":                logDir,
			"auto_scan_enabled":      autoScanEnabled,
			"auto_scan_interval_sec": autoScanIntervalSec,
		})
		return
	}
	if err := startRebuild(); err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}

	c.JSON(202, gin.H{
		"status":                 "started",
		"log_dir":                logDir,
		"auto_scan_enabled":      autoScanEnabled,
		"auto_scan_interval_sec": autoScanIntervalSec,
	})
}

func handleRebuild(c *gin.Context) {
	if err := startRebuild(); err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"status": "started"})
}

func handleSync(c *gin.Context) {
	if err := startIncrementalSync(); err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"status": "started"})
}

func handleExport(c *gin.Context) {
	if state := currentRebuildState(); state.Status == "running" {
		c.JSON(409, gin.H{"error": "索引重建中，暂时无法导出"})
		return
	}

	filters, err := parseFiltersFromJSON(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	query, _, args := buildSearchQueries(filters, false)
	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	cfg := currentConfig()
	category := resolveExportCategory(filters)
	fileName := buildExportFileName(category, filters)
	exportDir := filepath.Join(exportBaseDir(cfg), category)
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	filePath := filepath.Join(exportDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"时间", "源IP", "源端口", "目标IP", "目标端口", "协议", "NAT IP", "NAT端口", "动作"}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	exportedCount := 0
	for rows.Next() {
		var record LogEntry
		if err := rows.Scan(&record.Timestamp, &record.SrcIP, &record.SrcPort, &record.DstIP, &record.DstPort, &record.Protocol, &record.NatIP, &record.NatPort, &record.Action); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if err := writer.Write([]string{
			record.Timestamp,
			record.SrcIP,
			strconv.Itoa(record.SrcPort),
			record.DstIP,
			strconv.Itoa(record.DstPort),
			record.Protocol,
			record.NatIP,
			strconv.Itoa(record.NatPort),
			record.Action,
		}); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		exportedCount++
	}
	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if exportedCount == 0 {
		_ = os.Remove(filePath)
		c.JSON(400, gin.H{"error": "当前筛选条件没有可导出的结果"})
		return
	}

	c.JSON(200, ExportResponse{
		FileName:      fileName,
		Category:      category,
		ExportedCount: exportedCount,
		DownloadPath:  "/api/exports/" + category + "/" + fileName,
	})
}

func resolveExportCategory(filters SearchFilters) string {
	switch {
	case filters.IP != "":
		return "by_ip"
	case filters.Port > 0:
		return "by_port"
	case filters.Protocol != "":
		return "by_protocol"
	case filters.Range != "" && filters.Keyword == "":
		return "by_date"
	default:
		return "by_query"
	}
}

func buildExportFileName(category string, filters SearchFilters) string {
	timestamp := time.Now().Format("20060102_150405")
	switch category {
	case "by_ip":
		return fmt.Sprintf("%s_ip_%s.csv", timestamp, sanitizeFileComponent(filters.IP))
	case "by_port":
		return fmt.Sprintf("%s_port_%d.csv", timestamp, filters.Port)
	case "by_protocol":
		return fmt.Sprintf("%s_protocol_%s.csv", timestamp, strings.ToLower(sanitizeFileComponent(filters.Protocol)))
	case "by_date":
		return fmt.Sprintf("%s_date_%s.csv", timestamp, sanitizeFileComponent(filters.Range))
	default:
		return fmt.Sprintf("%s_query_custom.csv", timestamp)
	}
}

func sanitizeFileComponent(value string) string {
	replacer := strings.NewReplacer(":", "-", "/", "-", "\\", "-", " ", "_")
	return replacer.Replace(value)
}

func handleExportDownload(c *gin.Context) {
	relativePath := strings.TrimPrefix(c.Param("filepath"), "/")
	if relativePath == "" {
		c.JSON(400, gin.H{"error": "文件路径不能为空"})
		return
	}

	root := exportBaseDir(currentConfig())
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	targetPath := filepath.Join(rootAbs, filepath.Clean(relativePath))
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	rootPrefix := rootAbs + string(os.PathSeparator)
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootPrefix) {
		c.JSON(400, gin.H{"error": "非法文件路径"})
		return
	}
	if _, err := os.Stat(targetAbs); err != nil {
		c.JSON(404, gin.H{"error": "导出文件不存在"})
		return
	}

	c.FileAttachment(targetAbs, filepath.Base(targetAbs))
}

func serveIndex(c *gin.Context) {
	content, err := assets.ReadFile("assets/index.html")
	if err != nil {
		c.String(500, "Internal Server Error: index.html not found")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, string(content))
}

func handleNotFound(c *gin.Context) {
	if c.Request.Method == http.MethodGet && !strings.HasPrefix(c.Request.URL.Path, "/api/") {
		serveIndex(c)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   fmt.Sprintf("Cannot %s %s", c.Request.Method, c.Request.URL.Path),
	})
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
