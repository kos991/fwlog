package main

import (
	"bufio"
	"database/sql"
	"embed"
	"encoding/csv"
	"fmt"
	"io"
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

//go:embed assets/*
var assets embed.FS

type Config struct {
	LogDir  string
	DBFile  string
	Port    int
	Workers int
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

type LogDirRequest struct {
	LogDir string `json:"log_dir"`
}

type SettingsResponse struct {
	LogDir            string `json:"log_dir"`
	DBFile            string `json:"db_file"`
	ExportDir         string `json:"export_dir"`
	RebuildStatus     string `json:"rebuild_status"`
	RebuildStartedAt  string `json:"rebuild_started_at"`
	RebuildFinishedAt string `json:"rebuild_finished_at"`
	RebuildError      string `json:"rebuild_error"`
}

type ExportResponse struct {
	FileName      string `json:"file_name"`
	Category      string `json:"category"`
	ExportedCount int    `json:"exported_count"`
	DownloadPath  string `json:"download_path"`
}

type RebuildState struct {
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	Error      string
}

const (
	defaultLogDir      = "/data/sangfor_fw_log"
	defaultDBFile      = "/data/index/nat_logs.duckdb"
	defaultLocalLogDir = "./data/sangfor_fw_log"
	defaultLocalDBFile = "./data/index/nat_logs.duckdb"
	defaultPort        = 8080
	defaultWorkers     = 4

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

	natRegex = regexp.MustCompile(`婧怚P:([0-9.]+).*?婧愮鍙?(\d+).*?鐩殑IP:([0-9.]+).*?鐩殑绔彛:(\d+).*?鍗忚:(\d+).*?杞崲鍚庣殑IP:([0-9.]+).*?杞崲鍚庣殑绔彛:(\d+)`)
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
		log.Printf("未加载离线 GeoIP 数据库 (GeoLite2-City.mmdb): %v", err)
	}

	log.Printf("NAT鏃ュ織鏌ヨ绯荤粺鍚姩涓?..")
	log.Printf("鏃ュ織鐩綍: %s", currentConfig().LogDir)
	log.Printf("鏁版嵁搴? %s", currentConfig().DBFile)
	log.Printf("绔彛: %d", currentConfig().Port)

	os.MkdirAll(filepath.Dir(currentConfig().DBFile), 0755)
	os.MkdirAll(exportBaseDir(currentConfig()), 0755)

	var err error
	db, err = sql.Open("duckdb", currentConfig().DBFile)
	if err != nil {
		log.Fatalf("鏁版嵁搴撹繛鎺ュけ璐? %v", err)
	}
	defer db.Close()

	db.Exec("SET memory_limit='2GB'")
	db.Exec(fmt.Sprintf("SET threads=%d", currentConfig().Workers))

	if !tableExists() {
		setRebuildRunning()
		if err := buildIndex(); err != nil {
			setRebuildFinished(err)
			log.Fatalf("鏋勫缓绱㈠紩澶辫触: %v", err)
		}
		setRebuildFinished(nil)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.GET("/", serveIndex)
	r.StaticFS("/assets", http.FS(assets))
	r.GET("/api/query", handleQuery)
	r.GET("/api/stats", handleStats)
	r.GET("/api/top-ips", handleTopIPs)
	r.GET("/api/settings", handleSettings)
	r.POST("/api/settings/log-dir", handleSetLogDir)
	r.POST("/api/rebuild", handleRebuild)
	r.POST("/api/export", handleExport)
	r.GET("/api/dashboard", handleDashboardData)
	r.GET("/api/exports/*filepath", handleExportDownload)

	addr := fmt.Sprintf(":%d", currentConfig().Port)
	log.Printf("鏈嶅姟宸插惎鍔? http://0.0.0.0%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("鏈嶅姟鍚姩澶辫触: %v", err)
	}
}

func loadConfig() Config {
	defaults := defaultConfig()

	return Config{
		LogDir:  getEnv("LOG_DIR", defaults.LogDir),
		DBFile:  getEnv("DB_FILE", defaults.DBFile),
		Port:    getEnvInt("PORT", defaults.Port),
		Workers: getEnvInt("WORKERS", defaults.Workers),
	}
}

func defaultConfig() Config {
	if pathExists(defaultLocalLogDir) {
		return Config{
			LogDir:  defaultLocalLogDir,
			DBFile:  defaultLocalDBFile,
			Port:    defaultPort,
			Workers: defaultWorkers,
		}
	}

	return Config{
		LogDir:  defaultLogDir,
		DBFile:  defaultDBFile,
		Port:    defaultPort,
		Workers: defaultWorkers,
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

func setRebuildRunning() {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState = RebuildState{
		Status:    "running",
		StartedAt: time.Now(),
	}
}

func setRebuildFinished(err error) {
	rebuildMux.Lock()
	defer rebuildMux.Unlock()
	rebuildState.FinishedAt = time.Now()
	if err != nil {
		rebuildState.Status = "failed"
		rebuildState.Error = err.Error()
		return
	}
	rebuildState.Status = "succeeded"
	rebuildState.Error = ""
}

func startRebuild() error {
	rebuildMux.Lock()
	if rebuildState.Status == "running" {
		rebuildMux.Unlock()
		return fmt.Errorf("索引正在重建中")
	}
	rebuildState = RebuildState{
		Status:    "running",
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

func tableExists() bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_name='nat_logs'").Scan(&count)
	return err == nil && count > 0
}

func buildIndex() error {
	startTime := time.Now()
	cfg := currentConfig()

	files, err := filepath.Glob(filepath.Join(cfg.LogDir, "*.log"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("未找到日志文件")
	}

	log.Printf("找到 %d 个日志文件", len(files))

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

	for _, file := range files {
		log.Printf("处理: %s", filepath.Base(file))
		if err := processLogFile(file, writer, &totalLines); err != nil {
			log.Printf("处理文件失败 %s: %v", file, err)
			continue
		}
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

func processLogFile(filePath string, writer io.Writer, totalLines *int) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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
			return filters, fmt.Errorf("绔彛蹇呴』鏄鏁存暟")
		}
		filters.Port = port
	}

	return normalizeFilters(filters)
}

func parseFiltersFromJSON(c *gin.Context) (SearchFilters, error) {
	var filters SearchFilters
	if err := c.ShouldBindJSON(&filters); err != nil {
		return filters, fmt.Errorf("璇锋眰鍙傛暟鏍煎紡閿欒")
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
		return filters, fmt.Errorf("绔彛鑼冨洿蹇呴』鏄?any銆乻rc銆乨st 鎴?nat")
	}
	if filters.Port < 0 {
		return filters, fmt.Errorf("绔彛蹇呴』鏄鏁存暟")
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
		c.JSON(409, gin.H{"error": "绱㈠紩閲嶅缓涓紝璇风◢鍚庡啀璇?})
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
		record.SrcTag = ipEngine.GetTag(record.SrcIP)
		record.DstTag = ipEngine.GetTag(record.DstIP)
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

	// 1. Top 10 IPs (重用逻辑)
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
			rows.Scan(&ip.IP, &ip.Count)
			data.TopIPs = append(data.TopIPs, ip)
		}
		rows.Close()
	}

	// 2. 鍗忚鍒嗗竷
	rows, err = db.Query(`
		SELECT protocol, COUNT(*) as cnt
		FROM nat_logs
		GROUP BY protocol
		ORDER BY cnt DESC
	`)
	if err == nil {
		for rows.Next() {
			var p ProtocolStat
			rows.Scan(&p.Protocol, &p.Count)
			data.Protocols = append(data.Protocols, p)
		}
		rows.Close()
	}

	// 3. 娴侀噺瓒嬪娍 (绠€鍗曟瘡灏忔椂缁熻)
	rows, err = db.Query(`
		SELECT substr(timestamp, 1, 9) as t, COUNT(*) as cnt
		FROM nat_logs
		GROUP BY t
		ORDER BY t ASC
		LIMIT 24
	`)
	if err == nil {
		for rows.Next() {
			var tp TrendPoint
			rows.Scan(&tp.Time, &tp.Count)
			data.Trend = append(data.Trend, tp)
		}
		rows.Close()
	}

	// 4. 鏀垮姟缃戝崰姣?	var govCount int64
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
		LogDir:        cfg.LogDir,
		DBFile:        cfg.DBFile,
		ExportDir:     exportBaseDir(cfg),
		RebuildStatus: state.Status,
		RebuildError:  state.Error,
	}
	if !state.StartedAt.IsZero() {
		response.RebuildStartedAt = state.StartedAt.Format("2006-01-02 15:04:05")
	}
	if !state.FinishedAt.IsZero() {
		response.RebuildFinishedAt = state.FinishedAt.Format("2006-01-02 15:04:05")
	}

	c.JSON(200, response)
}

func handleSetLogDir(c *gin.Context) {
	var request LogDirRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "璇锋眰鍙傛暟鏍煎紡閿欒"})
		return
	}

	logDir := strings.TrimSpace(request.LogDir)
	if logDir == "" {
		c.JSON(400, gin.H{"error": "鏃ュ織璺緞涓嶈兘涓虹┖"})
		return
	}

	info, err := os.Stat(logDir)
	if err != nil {
		c.JSON(400, gin.H{"error": "鏃ュ織璺緞涓嶅瓨鍦?})
		return
	}
	if !info.IsDir() {
		c.JSON(400, gin.H{"error": "鏃ュ織璺緞蹇呴』鏄洰褰?})
		return
	}

	if state := currentRebuildState(); state.Status == "running" {
		c.JSON(409, gin.H{"error": "绱㈠紩姝ｅ湪閲嶅缓涓紝璇风◢鍚庡啀璇?})
		return
	}

	setLogDir(logDir)
	if err := startRebuild(); err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}

	c.JSON(202, gin.H{
		"status":  "started",
		"log_dir": logDir,
	})
}

func handleRebuild(c *gin.Context) {
	if err := startRebuild(); err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"status": "started"})
}

func handleExport(c *gin.Context) {
	if state := currentRebuildState(); state.Status == "running" {
		c.JSON(409, gin.H{"error": "绱㈠紩閲嶅缓涓紝鏆傛椂鏃犳硶瀵煎嚭"})
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
	if err := writer.Write([]string{"鏃堕棿", "婧怚P", "婧愮鍙?, "鐩爣IP", "鐩爣绔彛", "鍗忚", "NAT IP", "NAT绔彛", "鍔ㄤ綔"}); err != nil {
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
		c.JSON(400, gin.H{"error": "褰撳墠绛涢€夋潯浠舵病鏈夊彲瀵煎嚭鐨勭粨鏋?})
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
		c.JSON(400, gin.H{"error": "鏂囦欢璺緞涓嶈兘涓虹┖"})
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
		c.JSON(400, gin.H{"error": "闈炴硶鏂囦欢璺緞"})
		return
	}
	if _, err := os.Stat(targetAbs); err != nil {
		c.JSON(404, gin.H{"error": "瀵煎嚭鏂囦欢涓嶅瓨鍦?})
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
