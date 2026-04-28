//go:build legacy

package main

import (
	"bufio"
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

//go:embed static/*
var staticFiles embed.FS

type Config struct {
	LogDir    string
	DBFile    string
	Port      int
	Workers   int
}

type LogRecord struct {
	Timestamp string `json:"timestamp"`
	SrcIP     string `json:"src_ip"`
	SrcPort   int    `json:"src_port"`
	DstIP     string `json:"dst_ip"`
	DstPort   int    `json:"dst_port"`
	Protocol  string `json:"protocol"`
	NatIP     string `json:"nat_ip"`
	NatPort   int    `json:"nat_port"`
	FilePath  string `json:"file_path"`
	LineNum   int    `json:"line_num"`
}

type QueryResult struct {
	Records     []LogRecord `json:"records"`
	Total       int         `json:"total"`
	Page        int         `json:"page"`
	PageSize    int         `json:"page_size"`
	QueryTimeMs float64     `json:"query_time_ms"`
}

type IndexStats struct {
	TotalRecords   int64   `json:"total_records"`
	TotalFiles     int     `json:"total_files"`
	DBSizeMB       float64 `json:"db_size_mb"`
	RawSizeMB      float64 `json:"raw_size_mb"`
	CompressionPct float64 `json:"compression_pct"`
	LastUpdate     string  `json:"last_update"`
	BuildTimeMs    float64 `json:"build_time_ms"`
}

var (
	config Config
	db     *sql.DB
	dbMux  sync.RWMutex
)

func main() {
	flag.StringVar(&config.LogDir, "logdir", "./data/sangfor_fw_log", "日志目录")
	flag.StringVar(&config.DBFile, "db", "./data/index/fw_logs.duckdb", "DuckDB数据库文件")
	flag.IntVar(&config.Port, "port", 8080, "Web服务端口")
	flag.IntVar(&config.Workers, "workers", 4, "并行工作数")
	rebuild := flag.Bool("rebuild", false, "重建索引")
	flag.Parse()

	log.Printf("🚀 防火墙日志查询系统启动中...")
	log.Printf("📁 日志目录: %s", config.LogDir)
	log.Printf("💾 数据库: %s", config.DBFile)
	log.Printf("🌐 Web端口: %d", config.Port)

	// 确保目录存在
	os.MkdirAll(filepath.Dir(config.DBFile), 0755)

	// 初始化数据库
	var err error
	db, err = sql.Open("duckdb", config.DBFile)
	if err != nil {
		log.Fatalf("❌ 打开数据库失败: %v", err)
	}
	defer db.Close()

	// 设置DuckDB优化参数
	db.Exec("SET memory_limit='2GB'")
	db.Exec("SET threads=4")

	if *rebuild || !tableExists() {
		log.Println("🔨 开始构建索引...")
		if err := buildIndex(); err != nil {
			log.Fatalf("❌ 构建索引失败: %v", err)
		}
	}

	// 注册路由
	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/api/query", handleQuery)
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/rebuild", handleRebuild)

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("✅ Web服务已启动: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func tableExists() bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_name='logs'").Scan(&count)
	return err == nil && count > 0
}

func buildIndex() error {
	startTime := time.Now()

	// 创建表结构（列式存储，自动压缩）
	_, err := db.Exec(`
		DROP TABLE IF EXISTS logs;
		CREATE TABLE logs (
			timestamp VARCHAR,
			src_ip VARCHAR,
			src_port INTEGER,
			dst_ip VARCHAR,
			dst_port INTEGER,
			protocol VARCHAR,
			nat_ip VARCHAR,
			nat_port INTEGER,
			file_path VARCHAR,
			line_num INTEGER
		);
	`)
	if err != nil {
		return fmt.Errorf("创建表失败: %v", err)
	}

	// 查找所有日志文件
	files, err := filepath.Glob(filepath.Join(config.LogDir, "*.log"))
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("未找到日志文件")
	}

	log.Printf("📂 找到 %d 个日志文件", len(files))

	// 创建临时CSV文件用于COPY导入（超快速）
	tmpCSV := filepath.Join(filepath.Dir(config.DBFile), "tmp_import.csv")
	csvFile, err := os.Create(tmpCSV)
	if err != nil {
		return err
	}
	defer os.Remove(tmpCSV)

	writer := bufio.NewWriter(csvFile)
	totalLines := 0

	// 正则表达式提取字段
	natRegex := regexp.MustCompile(`源IP:([0-9.]+).*?源端口:(\d+).*?目的IP:([0-9.]+).*?目的端口:(\d+).*?协议:(\d+).*?转换后的IP:([0-9.]+).*?转换后的端口:(\d+)`)

	// 解析所有日志文件
	for _, file := range files {
		log.Printf("📖 处理: %s", filepath.Base(file))
		f, err := os.Open(file)
		if err != nil {
			log.Printf("⚠️  打开文件失败: %v", err)
			continue
		}

		scanner := bufio.NewScanner(f)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			matches := natRegex.FindStringSubmatch(line)
			if len(matches) < 8 {
				continue
			}

			// 提取时间戳
			timestamp := extractTimestamp(line)

			// 协议映射
			protoNum := matches[5]
			protocol := "OTHER"
			switch protoNum {
			case "6":
				protocol = "TCP"
			case "17":
				protocol = "UDP"
			case "1":
				protocol = "ICMP"
			}

			// 写入CSV（使用|分隔符避免冲突）
			fmt.Fprintf(writer, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%d\n",
				timestamp, matches[1], matches[2], matches[3], matches[4],
				protocol, matches[6], matches[7], file, lineNum)

			totalLines++
			if totalLines%100000 == 0 {
				log.Printf("⏳ 已处理 %d 行...", totalLines)
			}
		}
		f.Close()
	}

	writer.Flush()
	csvFile.Close()

	log.Printf("💾 开始导入数据库（使用COPY命令）...")

	// 使用DuckDB的COPY命令超快速导入
	importStart := time.Now()
	_, err = db.Exec(fmt.Sprintf(`
		COPY logs FROM '%s' (DELIMITER '|', HEADER false);
	`, strings.ReplaceAll(tmpCSV, "\\", "/")))
	if err != nil {
		return fmt.Errorf("导入数据失败: %v", err)
	}

	log.Printf("⚡ 导入完成，耗时 %.2f 秒", time.Since(importStart).Seconds())

	// 创建索引加速查询
	log.Println("🔍 创建索引...")
	db.Exec("CREATE INDEX idx_src_ip ON logs(src_ip)")
	db.Exec("CREATE INDEX idx_dst_ip ON logs(dst_ip)")
	db.Exec("CREATE INDEX idx_timestamp ON logs(timestamp)")

	// 优化存储
	log.Println("🗜️  优化数据库...")
	db.Exec("CHECKPOINT")

	buildTime := time.Since(startTime).Seconds()
	log.Printf("✅ 索引构建完成: %d 条记录, 总耗时 %.2f 秒", totalLines, buildTime)

	return nil
}

func extractTimestamp(line string) string {
	// 提取时间戳: Apr 27 00:00:29
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return fmt.Sprintf("%s %s %s", parts[0], parts[1], parts[2])
	}
	return ""
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// 解析参数
	ip := r.URL.Query().Get("ip")
	date := r.URL.Query().Get("date")
	protocol := r.URL.Query().Get("protocol")
	page := 1
	pageSize := 100

	fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
	fmt.Sscanf(r.URL.Query().Get("page_size"), "%d", &pageSize)

	if page < 1 {
		page = 1
	}
	if pageSize < 10 || pageSize > 1000 {
		pageSize = 100
	}

	// 构建查询
	query := "SELECT * FROM logs WHERE 1=1"
	args := []interface{}{}

	if ip != "" {
		query += " AND (src_ip = ? OR dst_ip = ?)"
		args = append(args, ip, ip)
	}
	if date != "" {
		query += " AND timestamp LIKE ?"
		args = append(args, "%"+date+"%")
	}
	if protocol != "" {
		query += " AND protocol = ?"
		args = append(args, strings.ToUpper(protocol))
	}

	// 获取总数
	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS t"
	var total int
	db.QueryRow(countQuery, args...).Scan(&total)

	// 分页查询
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []LogRecord
	for rows.Next() {
		var r LogRecord
		rows.Scan(&r.Timestamp, &r.SrcIP, &r.SrcPort, &r.DstIP, &r.DstPort,
			&r.Protocol, &r.NatIP, &r.NatPort, &r.FilePath, &r.LineNum)
		records = append(records, r)
	}

	queryTime := time.Since(startTime).Seconds() * 1000

	result := QueryResult{
		Records:     records,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
		QueryTimeMs: queryTime,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	var stats IndexStats

	// 获取记录数
	db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&stats.TotalRecords)

	// 获取文件数
	db.QueryRow("SELECT COUNT(DISTINCT file_path) FROM logs").Scan(&stats.TotalFiles)

	// 获取数据库大小
	if info, err := os.Stat(config.DBFile); err == nil {
		stats.DBSizeMB = float64(info.Size()) / 1024 / 1024
		stats.LastUpdate = info.ModTime().Format("2006-01-02 15:04:05")
	}

	// 计算原始日志大小
	files, _ := filepath.Glob(filepath.Join(config.LogDir, "*.log"))
	var rawSize int64
	for _, f := range files {
		if info, err := os.Stat(f); err == nil {
			rawSize += info.Size()
		}
	}
	stats.RawSizeMB = float64(rawSize) / 1024 / 1024

	// 压缩率
	if stats.RawSizeMB > 0 {
		stats.CompressionPct = (1 - stats.DBSizeMB/stats.RawSizeMB) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleRebuild(w http.ResponseWriter, r *http.Request) {
	go buildIndex()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>防火墙日志查询系统 - DuckDB优化版</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        @keyframes fadeIn { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
        .fade-in { animation: fadeIn 0.3s ease-out; }
        .gradient-bg { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }
        .card { background: white; border-radius: 12px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
    </style>
</head>
<body class="bg-gray-100">
    <div class="min-h-screen">
        <!-- Header -->
        <div class="gradient-bg text-white py-8 shadow-lg">
            <div class="container mx-auto px-4">
                <h1 class="text-4xl font-bold mb-2">🔍 防火墙日志查询系统</h1>
                <p class="text-gray-200">DuckDB列式存储 | 毫秒级响应 | 实时搜索</p>
            </div>
        </div>

        <div class="container mx-auto px-4 py-8">
            <!-- Stats Cards -->
            <div id="stats" class="grid grid-cols-1 md:grid-cols-4 gap-6 mb-8"></div>

            <!-- Search Panel -->
            <div class="card p-6 mb-8">
                <h2 class="text-2xl font-bold mb-4">🔎 实时搜索</h2>
                <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
                    <input type="text" id="ipInput" placeholder="IP地址 (例: 192.168.1.1)"
                           class="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent">
                    <input type="text" id="dateInput" placeholder="日期关键字 (例: Apr 27)"
                           class="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent">
                    <select id="protocolInput" class="px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-purple-500">
                        <option value="">所有协议</option>
                        <option value="TCP">TCP</option>
                        <option value="UDP">UDP</option>
                        <option value="ICMP">ICMP</option>
                    </select>
                </div>
                <div class="flex gap-4">
                    <button onclick="search()" class="gradient-bg text-white px-6 py-2 rounded-lg font-semibold hover:opacity-90 transition">
                        🔍 查询
                    </button>
                    <button onclick="clearSearch()" class="bg-gray-500 text-white px-6 py-2 rounded-lg font-semibold hover:bg-gray-600 transition">
                        🔄 清空
                    </button>
                    <button onclick="rebuild()" class="bg-green-500 text-white px-6 py-2 rounded-lg font-semibold hover:bg-green-600 transition">
                        ⚡ 重建索引
                    </button>
                </div>
            </div>

            <!-- Results -->
            <div id="results" class="card p-6"></div>
        </div>
    </div>

    <script>
        let currentPage = 1;
        let searchTimeout;

        // 加载统计信息
        async function loadStats() {
            const res = await fetch('/api/stats');
            const stats = await res.json();

            document.getElementById('stats').innerHTML = \`
                <div class="card p-6 bg-gradient-to-br from-purple-500 to-pink-500 text-white fade-in">
                    <div class="text-sm opacity-90 mb-2">📊 索引记录</div>
                    <div class="text-3xl font-bold">\${stats.total_records.toLocaleString()}</div>
                </div>
                <div class="card p-6 bg-gradient-to-br from-blue-500 to-cyan-500 text-white fade-in">
                    <div class="text-sm opacity-90 mb-2">💾 数据库大小</div>
                    <div class="text-3xl font-bold">\${stats.db_size_mb.toFixed(1)} MB</div>
                </div>
                <div class="card p-6 bg-gradient-to-br from-green-500 to-teal-500 text-white fade-in">
                    <div class="text-sm opacity-90 mb-2">🗜️ 压缩率</div>
                    <div class="text-3xl font-bold">\${stats.compression_pct.toFixed(1)}%</div>
                </div>
                <div class="card p-6 bg-gradient-to-br from-orange-500 to-red-500 text-white fade-in">
                    <div class="text-sm opacity-90 mb-2">📁 日志文件</div>
                    <div class="text-3xl font-bold">\${stats.total_files}</div>
                </div>
            \`;
        }

        // 实时搜索（防抖）
        function setupRealTimeSearch() {
            ['ipInput', 'dateInput', 'protocolInput'].forEach(id => {
                document.getElementById(id).addEventListener('input', () => {
                    clearTimeout(searchTimeout);
                    searchTimeout = setTimeout(() => search(), 500);
                });
            });
        }

        // 查询
        async function search(page = 1) {
            currentPage = page;
            const ip = document.getElementById('ipInput').value;
            const date = document.getElementById('dateInput').value;
            const protocol = document.getElementById('protocolInput').value;

            const params = new URLSearchParams({ ip, date, protocol, page, page_size: 100 });

            document.getElementById('results').innerHTML = '<div class="text-center py-12"><div class="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-purple-500"></div></div>';

            const res = await fetch('/api/query?' + params);
            const data = await res.json();

            renderResults(data);
        }

        // 渲染结果
        function renderResults(data) {
            const totalPages = Math.ceil(data.total / data.page_size);

            let html = \`
                <div class="flex justify-between items-center mb-4">
                    <div class="text-lg font-semibold">
                        找到 <span class="text-purple-600">\${data.total}</span> 条记录
                        <span class="text-sm text-gray-500 ml-4">⚡ 查询耗时: \${data.query_time_ms.toFixed(2)} ms</span>
                    </div>
                    <div class="flex gap-2">
                        <button onclick="search(\${data.page - 1})" \${data.page <= 1 ? 'disabled' : ''}
                                class="px-4 py-2 bg-gray-200 rounded hover:bg-gray-300 disabled:opacity-50">上一页</button>
                        <span class="px-4 py-2">\${data.page} / \${totalPages}</span>
                        <button onclick="search(\${data.page + 1})" \${data.page >= totalPages ? 'disabled' : ''}
                                class="px-4 py-2 bg-gray-200 rounded hover:bg-gray-300 disabled:opacity-50">下一页</button>
                    </div>
                </div>
            \`;

            if (data.records.length === 0) {
                html += '<div class="text-center py-12 text-gray-500">😔 未找到匹配记录</div>';
            } else {
                html += '<div class="overflow-x-auto"><table class="w-full"><thead class="bg-gray-50"><tr>';
                html += '<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">时间</th>';
                html += '<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">源IP:端口</th>';
                html += '<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">目的IP:端口</th>';
                html += '<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">协议</th>';
                html += '<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">NAT IP:端口</th>';
                html += '</tr></thead><tbody class="bg-white divide-y divide-gray-200">';

                data.records.forEach(r => {
                    html += \`<tr class="hover:bg-gray-50 fade-in">
                        <td class="px-4 py-3 text-sm">\${r.timestamp}</td>
                        <td class="px-4 py-3 text-sm"><span class="bg-blue-100 text-blue-800 px-2 py-1 rounded">\${r.src_ip}:\${r.src_port}</span></td>
                        <td class="px-4 py-3 text-sm"><span class="bg-green-100 text-green-800 px-2 py-1 rounded">\${r.dst_ip}:\${r.dst_port}</span></td>
                        <td class="px-4 py-3 text-sm"><span class="bg-purple-100 text-purple-800 px-2 py-1 rounded">\${r.protocol}</span></td>
                        <td class="px-4 py-3 text-sm">\${r.nat_ip}:\${r.nat_port}</td>
                    </tr>\`;
                });

                html += '</tbody></table></div>';
            }

            document.getElementById('results').innerHTML = html;
        }

        function clearSearch() {
            document.getElementById('ipInput').value = '';
            document.getElementById('dateInput').value = '';
            document.getElementById('protocolInput').value = '';
            document.getElementById('results').innerHTML = '';
        }

        async function rebuild() {
            if (!confirm('确定要重建索引吗？')) return;
            await fetch('/api/rebuild');
            alert('索引重建已启动，请稍后刷新页面查看进度');
        }

        // 初始化
        loadStats();
        setupRealTimeSearch();
    </script>
</body>
</html>`
