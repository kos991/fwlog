package main

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
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

	rebuildMux   sync.Mutex
	rebuildState = RebuildState{Status: "idle"}

	natRegex = regexp.MustCompile(`源IP:([0-9.]+).*?源端口:(\d+).*?目的IP:([0-9.]+).*?目的端口:(\d+).*?协议:(\d+).*?转换后的IP:([0-9.]+).*?转换后的端口:(\d+)`)
)

func main() {
	config = loadConfig()

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

	r.GET("/", serveIndex)
	r.GET("/api/query", handleQuery)
	r.GET("/api/stats", handleStats)
	r.GET("/api/top-ips", handleTopIPs)
	r.GET("/api/settings", handleSettings)
	r.POST("/api/settings/log-dir", handleSetLogDir)
	r.POST("/api/rebuild", handleRebuild)
	r.POST("/api/sync", handleSync)
	r.POST("/api/export", handleExport)
	r.GET("/api/exports/*filepath", handleExportDownload)

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
			AutoScanEnabled:     true,
			AutoScanIntervalSec: defaultAutoScanSec,
		}
	}

	return Config{
		LogDir:              defaultLogDir,
		DBFile:              defaultDBFile,
		Port:                defaultPort,
		Workers:             defaultWorkers,
		AutoScanEnabled:     true,
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

func buildIndex() error {
	startTime := time.Now()
	cfg := currentConfig()
	setRebuildMode("full_rebuild")

	snapshots, err := snapshotLogFiles(cfg.LogDir)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 {
		return fmt.Errorf("未找到日志文件")
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
			action VARCHAR
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
			continue
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

	_, err = db.Exec(fmt.Sprintf("COPY nat_logs FROM '%s' (DELIMITER '|', HEADER false);", strings.ReplaceAll(tmpCSV, "\\", "/")))
	if err != nil {
		return fmt.Errorf("补扫增量导入失败: %v", err)
	}

	*totalLines += addedLines
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

	if len(storedSnapshots) == 0 {
		if err := saveIngestSnapshots(currentSnapshots); err != nil {
			return err
		}
		return nil
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

	_, err = db.Exec(fmt.Sprintf("COPY nat_logs FROM '%s' (DELIMITER '|', HEADER false);", strings.ReplaceAll(tmpCSV, "\\", "/")))
	if err != nil {
		return fmt.Errorf("增量导入失败: %v", err)
	}
	return nil
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

	return processLogReaderWithLimit(file, info.Size(), writer, totalLines)
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

	return processLogReaderWithLimit(file, endOffset-startOffset, writer, totalLines)
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
		c.JSON(400, gin.H{"error": "请求参数格式错误"})
		return
	}

	cfg := currentConfig()
	logDir := strings.TrimSpace(request.LogDir)
	if logDir == "" {
		logDir = cfg.LogDir
	}

	info, err := os.Stat(logDir)
	if err != nil {
		c.JSON(400, gin.H{"error": "日志路径不存在"})
		return
	}
	if !info.IsDir() {
		c.JSON(400, gin.H{"error": "日志路径必须是目录"})
		return
	}

	if state := currentRebuildState(); state.Status == "running" {
		c.JSON(409, gin.H{"error": "索引正在重建中，请稍后再试"})
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
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, indexHTML)
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

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>NAT 日志查询系统</title>
  <script src="https://cdn.tailwindcss.com"></script>
  <style>
    body {
      font-family: "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
      background: #ffffff;
      color: #111827;
    }
    .soft-shadow {
      box-shadow: 0 24px 48px rgba(15, 23, 42, 0.06);
    }
    .thin-shadow {
      box-shadow: 0 10px 24px rgba(15, 23, 42, 0.04);
    }
    .mask {
      background: rgba(15, 23, 42, 0.22);
    }
    .mono {
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    }
  </style>
</head>
<body class="min-h-screen">
  <div id="historyMask" class="mask fixed inset-0 z-40 hidden"></div>
  <aside id="historyDrawer" class="fixed right-0 top-0 z-50 h-full w-full max-w-md translate-x-full transform border-l border-slate-200 bg-white transition-transform duration-300 ease-out">
    <div class="flex h-full flex-col">
      <div class="flex items-center justify-between border-b border-slate-200 px-6 py-4">
        <div>
          <h2 class="text-lg font-semibold text-slate-900">查询历史</h2>
          <p class="mt-1 text-sm text-slate-500">仅保留当前浏览器会话中的历史记录</p>
        </div>
        <button id="closeHistoryBtn" class="rounded-full border border-slate-200 px-3 py-1 text-sm text-slate-600 transition hover:border-slate-300 hover:text-slate-900">关闭</button>
      </div>
      <div id="historyList" class="flex-1 space-y-3 overflow-y-auto px-6 py-5"></div>
    </div>
  </aside>

  <div class="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:px-8">
    <header class="sticky top-0 z-30 mb-8 rounded-full border border-slate-200 bg-white/90 px-4 py-3 backdrop-blur soft-shadow">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-full bg-slate-900 text-sm font-semibold text-white">NAT</div>
          <div>
            <h1 class="text-lg font-semibold text-slate-900">NAT 日志查询系统</h1>
            <p class="text-sm text-slate-500">极简单页查询台</p>
          </div>
        </div>
        <nav class="flex flex-wrap items-center gap-2 text-sm">
          <button id="navAllLogs" class="rounded-full px-4 py-2 text-slate-600 transition hover:bg-slate-100 hover:text-slate-900">所有日志</button>
          <button id="navStats" class="rounded-full px-4 py-2 text-slate-600 transition hover:bg-slate-100 hover:text-slate-900">统计</button>
          <button id="navSettings" class="rounded-full px-4 py-2 text-slate-600 transition hover:bg-slate-100 hover:text-slate-900">搜索设置</button>
          <button id="openHistoryBtn" class="rounded-full border border-slate-200 px-4 py-2 text-slate-700 transition hover:border-slate-300 hover:text-slate-900">查询历史</button>
        </nav>
      </div>
    </header>

    <main class="mx-auto max-w-5xl space-y-8">
      <section class="space-y-4 text-center">
        <div class="inline-flex items-center rounded-full border border-slate-200 bg-white px-4 py-2 text-sm text-slate-500 thin-shadow">
          <span id="statusBadge" class="mr-2 inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500"></span>
          <span id="statusText">索引就绪</span>
        </div>
        <div>
          <h2 class="text-3xl font-semibold tracking-tight text-slate-900 sm:text-4xl">搜索 NAT 日志</h2>
          <p class="mx-auto mt-3 max-w-2xl text-sm leading-6 text-slate-500 sm:text-base">单页查询、搜索路径切换、端口过滤与分类导出，保持高效但不过度拥挤的检索体验。</p>
        </div>
      </section>

      <section id="statsSection" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-6"></section>

      <section id="searchPanel" class="rounded-[28px] border border-slate-200 bg-white p-5 soft-shadow sm:p-6">
        <div class="space-y-5">
          <div class="relative overflow-hidden rounded-[28px] border border-slate-200 bg-white px-4 py-3 shadow-sm transition focus-within:border-slate-300">
            <label for="keywordInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">关键词检索</label>
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center">
              <div class="flex-1">
                <input id="keywordInput" class="w-full border-0 bg-transparent p-0 text-lg text-slate-900 placeholder:text-slate-400 focus:outline-none" placeholder="输入 IP、端口、协议、动作等关键词">
              </div>
              <div class="flex items-center gap-3 text-sm text-slate-500">
                <div><span class="text-slate-400">总日志量</span> <span id="heroTotal" class="font-semibold text-slate-900">0</span></div>
                <div class="h-5 w-px bg-slate-200"></div>
                <div><span class="text-slate-400">活跃会话</span> <span id="heroSessions" class="font-semibold text-slate-900">0</span></div>
              </div>
            </div>
          </div>

          <div class="grid gap-3 lg:grid-cols-6">
            <div class="rounded-2xl border border-slate-200 px-4 py-3">
              <label for="ipInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">IP 地址</label>
              <input id="ipInput" class="w-full border-0 bg-transparent p-0 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none" placeholder="源 / 目标 / NAT IP">
            </div>
            <div class="rounded-2xl border border-slate-200 px-4 py-3">
              <label for="portInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">端口</label>
              <input id="portInput" class="w-full border-0 bg-transparent p-0 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none" placeholder="例如 443">
            </div>
            <div class="rounded-2xl border border-slate-200 px-4 py-3">
              <label for="portScopeInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">端口范围</label>
              <select id="portScopeInput" class="w-full border-0 bg-transparent p-0 text-sm text-slate-900 focus:outline-none">
                <option value="any">任意端口</option>
                <option value="src">源端口</option>
                <option value="dst">目标端口</option>
                <option value="nat">NAT 端口</option>
              </select>
            </div>
            <div class="rounded-2xl border border-slate-200 px-4 py-3">
              <label for="protocolInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">协议</label>
              <select id="protocolInput" class="w-full border-0 bg-transparent p-0 text-sm text-slate-900 focus:outline-none">
                <option value="">全部协议</option>
                <option value="TCP">TCP</option>
                <option value="UDP">UDP</option>
                <option value="ICMP">ICMP</option>
                <option value="OTHER">OTHER</option>
              </select>
            </div>
            <div class="rounded-2xl border border-slate-200 px-4 py-3">
              <label for="rangeInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">时间范围</label>
              <select id="rangeInput" class="w-full border-0 bg-transparent p-0 text-sm text-slate-900 focus:outline-none">
                <option value="">全部时间</option>
                <option value="1h">最近 1 小时</option>
                <option value="6h">最近 6 小时</option>
                <option value="24h">最近 24 小时</option>
                <option value="7d">最近 7 天</option>
              </select>
            </div>
            <div class="flex items-end">
              <button id="searchBtn" class="w-full rounded-2xl bg-slate-900 px-4 py-3 text-sm font-medium text-white transition hover:bg-slate-800">开始查询</button>
            </div>
          </div>
        </div>
      </section>

      <section id="settingsSection" class="rounded-[28px] border border-slate-200 bg-white p-5 thin-shadow sm:p-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="space-y-1">
            <div class="text-xs font-medium uppercase tracking-wide text-slate-400">搜索设置</div>
            <h3 class="text-xl font-semibold text-slate-900">搜索路径配置</h3>
            <p class="text-sm text-slate-500">修改当前运行时日志目录，并触发索引重建。当前改动不写入磁盘配置。</p>
          </div>
          <div class="rounded-full border border-slate-200 bg-slate-50 px-4 py-2 text-sm text-slate-600">
            当前目录：<span id="currentLogDir" class="mono text-slate-900">-</span>
          </div>
        </div>
        <div class="mt-5 grid gap-3 lg:grid-cols-[1fr_auto_auto]">
          <div class="rounded-2xl border border-slate-200 px-4 py-3">
            <label for="logDirInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">日志目录路径</label>
            <input id="logDirInput" class="mono w-full border-0 bg-transparent p-0 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none" placeholder="/data/sangfor_fw_log">
          </div>
          <button id="saveLogDirBtn" class="rounded-2xl border border-slate-200 px-5 py-3 text-sm font-medium text-slate-700 transition hover:border-slate-300 hover:text-slate-900">保存并重建</button>
          <button id="manualRebuildBtn" class="rounded-2xl border border-slate-200 px-5 py-3 text-sm font-medium text-slate-700 transition hover:border-slate-300 hover:text-slate-900">仅重建索引</button>
        </div>
        <div class="mt-4 grid gap-3 lg:grid-cols-[1fr_220px_auto]">
          <label class="flex items-center gap-3 rounded-2xl border border-slate-200 px-4 py-3 text-sm text-slate-700">
            <input id="autoScanEnabledInput" type="checkbox" class="h-4 w-4 rounded border-slate-300 text-slate-900 focus:ring-slate-400">
            <span>Enable scheduled incremental sync</span>
          </label>
          <div class="rounded-2xl border border-slate-200 px-4 py-3">
            <label for="autoScanIntervalInput" class="mb-2 block text-xs font-medium uppercase tracking-wide text-slate-400">Sync interval (sec)</label>
            <input id="autoScanIntervalInput" type="number" min="5" step="5" class="w-full border-0 bg-transparent p-0 text-sm text-slate-900 focus:outline-none" value="30">
          </div>
          <button id="manualSyncBtn" class="rounded-2xl border border-slate-200 px-5 py-3 text-sm font-medium text-slate-700 transition hover:border-slate-300 hover:text-slate-900">Run incremental sync</button>
        </div>
        <div class="mt-4 rounded-3xl border border-slate-200 bg-slate-50 p-4">
          <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <div class="text-xs font-medium uppercase tracking-wide text-slate-400">Index Progress</div>
              <div id="rebuildProgressText" class="mt-1 text-sm font-medium text-slate-900">Idle</div>
            </div>
            <div id="rebuildProgressMeta" class="text-sm text-slate-500">No active task</div>
          </div>
          <div class="mt-3 h-2 overflow-hidden rounded-full bg-slate-200">
            <div id="rebuildProgressBar" class="h-full w-0 rounded-full bg-slate-900 transition-all duration-500"></div>
          </div>
        </div>
        <div id="settingsMessage" class="mt-4 hidden rounded-2xl border px-4 py-3 text-sm"></div>
      </section>

      <section id="resultsSection" class="rounded-[28px] border border-slate-200 bg-white p-5 soft-shadow sm:p-6">
        <div class="mb-5 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h3 class="text-xl font-semibold text-slate-900">查询结果</h3>
            <p class="mt-1 text-sm text-slate-500">支持分页、高亮字段和分类导出。</p>
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <button id="exportBtn" class="rounded-2xl border border-slate-200 px-4 py-2.5 text-sm font-medium text-slate-700 transition hover:border-slate-300 hover:text-slate-900">导出当前结果</button>
            <div id="exportFeedback" class="text-sm text-slate-500"></div>
          </div>
        </div>
        <div id="results"></div>
      </section>
    </main>
  </div>

  <script>
    let currentPage = 1;
    let searchTimeout = null;
    let latestResults = null;
    let currentStatus = 'idle';
    let queryHistory = [];

    function escapeHtml(value) {
      return String(value || '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
    }

    function indicator(label, value, sub) {
      return '<div class="rounded-3xl border border-slate-200 bg-white p-4 thin-shadow">' +
        '<div class="text-xs uppercase tracking-wide text-slate-400">' + label + '</div>' +
        '<div class="mt-2 text-2xl font-semibold text-slate-900">' + value + '</div>' +
        '<div class="mt-1 text-xs text-slate-500">' + sub + '</div>' +
      '</div>';
    }

    function renderStats(stats) {
      document.getElementById('heroTotal').textContent = Number(stats.total_records || 0).toLocaleString();
      document.getElementById('heroSessions').textContent = Number(stats.active_sessions || 0).toLocaleString();
      document.getElementById('statsSection').innerHTML = [
        indicator('总日志量', Number(stats.total_records || 0).toLocaleString(), '结构化 NAT 记录'),
        indicator('活跃会话数', Number(stats.active_sessions || 0).toLocaleString(), '按连接组合估算'),
        indicator('数据库大小', (stats.db_size_mb || 0).toFixed(1) + ' MB', 'DuckDB 当前文件'),
        indicator('压缩率', (stats.compression_pct || 0).toFixed(1) + '%', '原始日志 vs 索引库'),
        indicator('日志文件数', Number(stats.total_files || 0).toLocaleString(), '当前搜索路径下'),
        indicator('平均查询耗时', (stats.avg_query_time_ms || 0).toFixed(0) + ' ms', '当前预估值')
      ].join('');
    }

    function formatDuration(totalSeconds) {
      const seconds = Math.max(0, Number(totalSeconds || 0));
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);
      const secs = Math.floor(seconds % 60);
      if (hours > 0) {
        return hours + 'h ' + minutes + 'm ' + secs + 's';
      }
      if (minutes > 0) {
        return minutes + 'm ' + secs + 's';
      }
      return secs + 's';
    }

    function formatBytes(value) {
      const bytes = Math.max(0, Number(value || 0));
      if (bytes >= 1024 * 1024 * 1024) {
        return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
      }
      if (bytes >= 1024 * 1024) {
        return (bytes / 1024 / 1024).toFixed(1) + ' MB';
      }
      if (bytes >= 1024) {
        return (bytes / 1024).toFixed(1) + ' KB';
      }
      return bytes + ' B';
    }

    function renderRebuildProgress(settings) {
      const total = Number(settings.rebuild_bytes_total || 0);
      const done = Number(settings.rebuild_bytes_done || 0);
      const percent = settings.rebuild_status === 'succeeded'
        ? 100
        : (total > 0 ? Math.min(100, Math.round(done / total * 100)) : 0);
      const progressBar = document.getElementById('rebuildProgressBar');
      const progressText = document.getElementById('rebuildProgressText');
      const progressMeta = document.getElementById('rebuildProgressMeta');

      progressBar.style.width = percent + '%';
      if (settings.rebuild_status === 'running') {
        progressText.textContent = (settings.rebuild_mode || 'task') + ' · ' + percent + '%';
      } else if (settings.rebuild_status === 'failed') {
        progressText.textContent = 'Failed';
      } else if (settings.rebuild_status === 'succeeded') {
        progressText.textContent = 'Completed';
      } else {
        progressText.textContent = 'Idle';
      }

      const metaParts = [];
      if (settings.rebuild_current_file) {
        metaParts.push(settings.rebuild_current_file.split(/[\\/]/).pop());
      }
      if (Number(settings.rebuild_files_total || 0) > 0) {
        metaParts.push((settings.rebuild_files_done || 0) + '/' + (settings.rebuild_files_total || 0) + ' files');
      }
      if (total > 0) {
        metaParts.push(formatBytes(done) + ' / ' + formatBytes(total));
      }
      if (Number(settings.rebuild_elapsed_sec || 0) > 0) {
        metaParts.push('elapsed ' + formatDuration(settings.rebuild_elapsed_sec));
      }
      if (Number(settings.rebuild_eta_sec || 0) > 0 && settings.rebuild_status === 'running') {
        metaParts.push('eta ' + formatDuration(settings.rebuild_eta_sec));
      }
      progressMeta.textContent = metaParts.join(' · ') || 'No active task';
    }

    function setStatus(status, message) {
      currentStatus = status || 'idle';
      const badge = document.getElementById('statusBadge');
      const text = document.getElementById('statusText');
      badge.className = 'mr-2 inline-flex h-2.5 w-2.5 rounded-full';

      if (status === 'running') {
        badge.className += ' bg-blue-500 animate-pulse';
        text.textContent = message || '索引重建中';
      } else if (status === 'failed') {
        badge.className += ' bg-rose-500';
        text.textContent = message || '索引重建失败';
      } else if (status === 'succeeded') {
        badge.className += ' bg-emerald-500';
        text.textContent = message || '索引已更新';
      } else {
        badge.className += ' bg-emerald-500';
        text.textContent = message || '索引就绪';
      }

      const disabled = status === 'running';
      document.getElementById('searchBtn').disabled = disabled;
      document.getElementById('saveLogDirBtn').disabled = disabled;
      document.getElementById('manualRebuildBtn').disabled = disabled;
      document.getElementById('manualSyncBtn').disabled = disabled;
      document.getElementById('autoScanEnabledInput').disabled = disabled;
      document.getElementById('autoScanIntervalInput').disabled = disabled;
      document.getElementById('exportBtn').disabled = disabled || !latestResults || !latestResults.records || latestResults.records.length === 0;
    }

    function showSettingsMessage(kind, message) {
      const node = document.getElementById('settingsMessage');
      node.className = 'mt-4 rounded-2xl border px-4 py-3 text-sm';
      if (kind === 'error') {
        node.className += ' border-rose-200 bg-rose-50 text-rose-700';
      } else if (kind === 'success') {
        node.className += ' border-emerald-200 bg-emerald-50 text-emerald-700';
      } else {
        node.className += ' border-blue-200 bg-blue-50 text-blue-700';
      }
      node.textContent = message;
      node.classList.remove('hidden');
    }

    function clearSettingsMessage() {
      document.getElementById('settingsMessage').classList.add('hidden');
    }

    async function loadStats() {
      const res = await fetch('/api/stats');
      const data = await res.json();
      renderStats(data);
    }

    async function loadTopIPs() {
      const res = await fetch('/api/top-ips');
      const data = await res.json();
      const existing = document.getElementById('topIpWrap');
      if (!Array.isArray(data) || data.length === 0) {
        if (existing) {
          existing.remove();
        }
        return;
      }
      const text = data.map(function (item, index) {
        return '<span class="rounded-full border border-slate-200 px-3 py-1 text-xs text-slate-600">#' + (index + 1) + ' ' + escapeHtml(item.ip) + ' · ' + Number(item.count || 0).toLocaleString() + '</span>';
      }).join(' ');
      const statsSection = document.getElementById('statsSection');
      const wrapper = document.createElement('div');
      wrapper.className = 'sm:col-span-2 xl:col-span-6';
      wrapper.innerHTML = '<div class="rounded-3xl border border-slate-200 bg-white p-4 text-sm text-slate-600 thin-shadow"><div class="mb-3 text-xs uppercase tracking-wide text-slate-400">活跃内网源 IP</div><div class="flex flex-wrap gap-2">' + text + '</div></div>';
      if (existing) {
        existing.replaceWith(wrapper);
      } else {
        wrapper.id = 'topIpWrap';
        statsSection.appendChild(wrapper);
      }
      wrapper.id = 'topIpWrap';
    }

    async function loadSettings() {
      const res = await fetch('/api/settings');
      const settings = await res.json();
      document.getElementById('currentLogDir').textContent = settings.log_dir || '-';
      if (document.activeElement !== document.getElementById('logDirInput')) {
        document.getElementById('logDirInput').value = settings.log_dir || '';
      }
      document.getElementById('autoScanEnabledInput').checked = !!settings.auto_scan_enabled;
      if (document.activeElement !== document.getElementById('autoScanIntervalInput')) {
        document.getElementById('autoScanIntervalInput').value = settings.auto_scan_interval_sec || 30;
      }

      let message = '索引就绪';
      if (settings.rebuild_status === 'running') {
        message = 'Index rebuilding';
        if (settings.rebuild_eta_sec) {
          message += ' · ETA ' + formatDuration(settings.rebuild_eta_sec);
        }
      } else if (settings.rebuild_status === 'failed') {
        message = settings.rebuild_error ? '索引重建失败：' + settings.rebuild_error : '索引重建失败';
      } else if (settings.rebuild_status === 'succeeded') {
        message = settings.rebuild_finished_at ? '索引已更新 · ' + settings.rebuild_finished_at : '索引已更新';
      }
      setStatus(settings.rebuild_status, message);
      renderRebuildProgress(settings);
    }

    function spinner() {
      return '<div class="flex items-center gap-3 rounded-3xl border border-slate-200 bg-slate-50 px-4 py-6 text-sm text-slate-500">' +
        '<svg class="h-5 w-5 animate-spin text-slate-500" viewBox="0 0 24 24" fill="none">' +
        '<circle cx="12" cy="12" r="10" stroke="currentColor" stroke-opacity="0.15" stroke-width="4"></circle>' +
        '<path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" stroke-width="4" stroke-linecap="round"></path>' +
        '</svg> 正在加载结果...' +
      '</div>';
    }

    function collectFilters(page) {
      return {
        keyword: document.getElementById('keywordInput').value.trim(),
        ip: document.getElementById('ipInput').value.trim(),
        port: document.getElementById('portInput').value.trim(),
        port_scope: document.getElementById('portScopeInput').value,
        protocol: document.getElementById('protocolInput').value,
        range: document.getElementById('rangeInput').value,
        page: page || 1,
        page_size: 25
      };
    }

    function toSearchParams(filters) {
      const params = new URLSearchParams();
      Object.keys(filters).forEach(function (key) {
        const value = filters[key];
        if (value !== '' && value !== null && value !== undefined) {
          params.append(key, value);
        }
      });
      return params;
    }

    function pushHistory(filters, total) {
      queryHistory.unshift({
        time: new Date().toLocaleString('zh-CN'),
        keyword: filters.keyword || '无关键词',
        summary: [
          filters.ip ? 'IP ' + filters.ip : '',
          filters.port ? '端口 ' + filters.port + ' (' + filters.port_scope + ')' : '',
          filters.protocol ? '协议 ' + filters.protocol : '',
          filters.range ? '范围 ' + filters.range : ''
        ].filter(Boolean).join(' · ') || '全部条件',
        total: total
      });
      queryHistory = queryHistory.slice(0, 20);
      renderHistory();
    }

    function renderHistory() {
      const list = document.getElementById('historyList');
      if (queryHistory.length === 0) {
        list.innerHTML = '<div class="rounded-3xl border border-dashed border-slate-200 px-5 py-8 text-center text-sm text-slate-500">暂时还没有查询历史</div>';
        return;
      }
      list.innerHTML = queryHistory.map(function (item) {
        return '<div class="rounded-3xl border border-slate-200 p-4">' +
          '<div class="flex items-center justify-between gap-4"><div class="text-sm font-medium text-slate-900">' + escapeHtml(item.keyword) + '</div><div class="text-xs text-slate-400">' + escapeHtml(item.time) + '</div></div>' +
          '<div class="mt-2 text-sm text-slate-500">' + escapeHtml(item.summary) + '</div>' +
          '<div class="mt-3 text-xs text-slate-400">命中 ' + Number(item.total || 0).toLocaleString() + ' 条</div>' +
        '</div>';
      }).join('');
    }

    function toggleHistory(open) {
      const mask = document.getElementById('historyMask');
      const drawer = document.getElementById('historyDrawer');
      if (open) {
        mask.classList.remove('hidden');
        drawer.classList.remove('translate-x-full');
      } else {
        mask.classList.add('hidden');
        drawer.classList.add('translate-x-full');
      }
    }

    async function search(page) {
      if (currentStatus === 'running') {
        document.getElementById('results').innerHTML = '<div class="rounded-3xl border border-blue-200 bg-blue-50 px-4 py-6 text-sm text-blue-700">索引重建中，请稍后再查询。</div>';
        return;
      }

      currentPage = page || 1;
      const filters = collectFilters(currentPage);
      const params = toSearchParams(filters);
      document.getElementById('results').innerHTML = spinner();

      const res = await fetch('/api/query?' + params.toString());
      const data = await res.json();
      if (!res.ok) {
        latestResults = null;
        document.getElementById('results').innerHTML = '<div class="rounded-3xl border border-rose-200 bg-rose-50 px-4 py-6 text-sm text-rose-700">' + escapeHtml(data.error || '查询失败') + '</div>';
        setStatus(currentStatus);
        return;
      }

      latestResults = data;
      setStatus(currentStatus);
      pushHistory(filters, data.total);
      renderResults(data);
    }

    function tag(text, tone) {
      const base = 'inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium ';
      if (tone === 'blue') {
        return '<span class="' + base + 'bg-blue-50 text-blue-700 border border-blue-100">' + escapeHtml(text) + '</span>';
      }
      if (tone === 'green') {
        return '<span class="' + base + 'bg-emerald-50 text-emerald-700 border border-emerald-100">' + escapeHtml(text) + '</span>';
      }
      return '<span class="' + base + 'bg-slate-100 text-slate-700 border border-slate-200">' + escapeHtml(text) + '</span>';
    }

    function renderResults(data) {
      const totalPages = Math.max(1, Math.ceil((data.total || 0) / (data.page_size || 1)));
      let html = '' +
        '<div class="flex flex-col gap-4 border-b border-slate-100 pb-4 lg:flex-row lg:items-center lg:justify-between">' +
          '<div class="space-y-1">' +
            '<div class="text-sm text-slate-500">共检索到 <span class="font-semibold text-slate-900">' + Number(data.total || 0).toLocaleString() + '</span> 条记录</div>' +
            '<div class="text-xs text-slate-400">查询耗时 ' + Number(data.query_time_ms || 0).toFixed(2) + ' ms</div>' +
          '</div>' +
          '<div class="flex items-center gap-2 text-sm">' +
            '<button class="rounded-full border border-slate-200 px-4 py-2 text-slate-600 transition hover:border-slate-300 hover:text-slate-900 ' + (data.page <= 1 ? 'opacity-40 cursor-not-allowed' : '') + '" onclick="search(' + (data.page - 1) + ')" ' + (data.page <= 1 ? 'disabled' : '') + '>上一页</button>' +
            '<span class="rounded-full bg-slate-100 px-4 py-2 text-slate-700">' + data.page + ' / ' + totalPages + '</span>' +
            '<button class="rounded-full border border-slate-200 px-4 py-2 text-slate-600 transition hover:border-slate-300 hover:text-slate-900 ' + (data.page >= totalPages ? 'opacity-40 cursor-not-allowed' : '') + '" onclick="search(' + (data.page + 1) + ')" ' + (data.page >= totalPages ? 'disabled' : '') + '>下一页</button>' +
          '</div>' +
        '</div>';

      if (!data.records || data.records.length === 0) {
        document.getElementById('results').innerHTML = html + '<div class="py-12 text-center text-sm text-slate-500">没有匹配结果，请调整搜索条件后重试。</div>';
        return;
      }

      html += '<div class="mt-5 overflow-hidden rounded-[24px] border border-slate-200">' +
        '<div class="overflow-x-auto">' +
          '<table class="min-w-full divide-y divide-slate-200 text-sm">' +
            '<thead class="bg-slate-50">' +
              '<tr class="text-left text-xs font-semibold uppercase tracking-wide text-slate-500">' +
                '<th class="px-4 py-3">时间</th>' +
                '<th class="px-4 py-3">源</th>' +
                '<th class="px-4 py-3">目标</th>' +
                '<th class="px-4 py-3">协议</th>' +
                '<th class="px-4 py-3">NAT</th>' +
                '<th class="px-4 py-3">动作</th>' +
              '</tr>' +
            '</thead>' +
            '<tbody class="divide-y divide-slate-100 bg-white">';

      data.records.forEach(function (r) {
        html += '<tr class="transition hover:bg-slate-50">' +
          '<td class="px-4 py-4 text-sm text-slate-600">' + escapeHtml(r.timestamp) + '</td>' +
          '<td class="px-4 py-4"><div class="space-y-1">' + tag(r.src_ip, 'blue') + '<div class="mono text-xs text-slate-500">端口 ' + escapeHtml(r.src_port) + '</div></div></td>' +
          '<td class="px-4 py-4"><div class="space-y-1">' + tag(r.dst_ip, 'green') + '<div class="mono text-xs text-slate-500">端口 ' + escapeHtml(r.dst_port) + '</div></div></td>' +
          '<td class="px-4 py-4">' + tag(r.protocol, 'default') + '</td>' +
          '<td class="px-4 py-4"><div class="space-y-1">' + tag(r.nat_ip, 'default') + '<div class="mono text-xs text-slate-500">端口 ' + escapeHtml(r.nat_port) + '</div></div></td>' +
          '<td class="px-4 py-4 text-sm text-slate-500">' + escapeHtml(r.action) + '</td>' +
        '</tr>';
      });

      html += '</tbody></table></div></div>';
      document.getElementById('results').innerHTML = html;
    }

    async function saveLogDir() {
      clearSettingsMessage();
      const logDir = document.getElementById('logDirInput').value.trim();
      const autoScanEnabled = document.getElementById('autoScanEnabledInput').checked;
      const autoScanIntervalSec = Number(document.getElementById('autoScanIntervalInput').value || 30);
      const res = await fetch('/api/settings/log-dir', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          log_dir: logDir,
          auto_scan_enabled: autoScanEnabled,
          auto_scan_interval_sec: autoScanIntervalSec
        })
      });
      const data = await res.json();
      if (!res.ok) {
        showSettingsMessage('error', data.error || '保存失败');
        return;
      }
      showSettingsMessage('info', '新的日志路径已接收，后台正在重建索引。');
      await loadSettings();
    }

    async function triggerSync() {
      clearSettingsMessage();
      const res = await fetch('/api/sync', { method: 'POST' });
      const data = await res.json();
      if (!res.ok) {
        showSettingsMessage('error', data.error || 'Incremental sync failed');
        return;
      }
      showSettingsMessage('info', 'Incremental sync started.');
      await loadSettings();
    }

    async function triggerRebuild() {
      clearSettingsMessage();
      const res = await fetch('/api/rebuild', { method: 'POST' });
      const data = await res.json();
      if (!res.ok) {
        showSettingsMessage('error', data.error || '重建失败');
        return;
      }
      showSettingsMessage('info', '已开始重建索引，请稍后。');
      await loadSettings();
    }

    async function exportCurrentResults() {
      if (!latestResults || !latestResults.records || latestResults.records.length === 0) {
        return;
      }
      const feedback = document.getElementById('exportFeedback');
      feedback.textContent = '正在导出...';

      const filters = collectFilters(currentPage);
      const payload = {
        keyword: filters.keyword,
        ip: filters.ip,
        port: filters.port ? Number(filters.port) : 0,
        port_scope: filters.port_scope,
        protocol: filters.protocol,
        range: filters.range
      };

      const res = await fetch('/api/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (!res.ok) {
        feedback.textContent = data.error || '导出失败';
        return;
      }

      feedback.innerHTML = '已导出 ' + Number(data.exported_count || 0).toLocaleString() + ' 条 · <a class="text-slate-900 underline underline-offset-4" href="' + escapeHtml(data.download_path) + '">下载 ' + escapeHtml(data.file_name) + '</a>';
    }

    function setupEvents() {
      document.getElementById('searchBtn').addEventListener('click', function () {
        search(1);
      });
      document.getElementById('saveLogDirBtn').addEventListener('click', saveLogDir);
      document.getElementById('manualRebuildBtn').addEventListener('click', triggerRebuild);
      document.getElementById('manualSyncBtn').addEventListener('click', triggerSync);
      document.getElementById('exportBtn').addEventListener('click', exportCurrentResults);
      document.getElementById('openHistoryBtn').addEventListener('click', function () { toggleHistory(true); });
      document.getElementById('closeHistoryBtn').addEventListener('click', function () { toggleHistory(false); });
      document.getElementById('historyMask').addEventListener('click', function () { toggleHistory(false); });
      document.getElementById('navStats').addEventListener('click', function () { document.getElementById('statsSection').scrollIntoView({ behavior: 'smooth', block: 'start' }); });
      document.getElementById('navSettings').addEventListener('click', function () { document.getElementById('settingsSection').scrollIntoView({ behavior: 'smooth', block: 'start' }); });
      document.getElementById('navAllLogs').addEventListener('click', function () {
        document.getElementById('keywordInput').value = '';
        document.getElementById('ipInput').value = '';
        document.getElementById('portInput').value = '';
        document.getElementById('portScopeInput').value = 'any';
        document.getElementById('protocolInput').value = '';
        document.getElementById('rangeInput').value = '';
        search(1);
      });

      ['keywordInput', 'ipInput', 'portInput', 'rangeInput', 'protocolInput', 'portScopeInput'].forEach(function (id) {
        const node = document.getElementById(id);
        const eventName = node.tagName === 'SELECT' ? 'change' : 'input';
        node.addEventListener(eventName, function () {
          clearTimeout(searchTimeout);
          searchTimeout = setTimeout(function () {
            search(1);
          }, 450);
        });
      });
    }

	async function boot() {
		setupEvents();
		renderHistory();
		await loadStats();
		await Promise.all([loadSettings(), loadTopIPs()]);
		setInterval(loadStats, 30000);
		setInterval(loadSettings, 5000);
		setInterval(loadTopIPs, 60000);
      document.getElementById('results').innerHTML = '<div class="rounded-3xl border border-dashed border-slate-200 px-4 py-12 text-center text-sm text-slate-500">输入关键词、IP、端口或协议后即可开始查询。</div>';
    }

    boot();
  </script>
</body>
</html>`
