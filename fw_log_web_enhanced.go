//go:build legacy

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 日志记录结构
type LogRecord struct {
	IP       string `json:"ip"`
	Date     string `json:"date"`
	FilePath string `json:"file_path"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	LineNum  string `json:"line_num"`
}

// 查询结果
type QueryResult struct {
	Records []LogRecord `json:"records"`
	Count   int         `json:"count"`
	Time    float64     `json:"time"`
}

// 统计信息
type IndexStats struct {
	TotalRecords int       `json:"total_records"`
	TotalFiles   int       `json:"total_files"`
	IndexSize    int64     `json:"index_size"`
	LastUpdate   time.Time `json:"last_update"`
	BuildTime    float64   `json:"build_time"`
}

var (
	logDir     string
	indexFile  string
	port       int
	workers    int
	rebuild    bool
	indexStats IndexStats
	statsMutex sync.RWMutex
)

func main() {
	flag.StringVar(&logDir, "logdir", "/data/sangfor_fw_log", "日志目录")
	flag.StringVar(&indexFile, "index", "sangfor_fw_log_index.db", "索引文件路径")
	flag.IntVar(&port, "port", 8080, "Web服务端口")
	flag.IntVar(&workers, "workers", 4, "并行工作数")
	flag.BoolVar(&rebuild, "rebuild", false, "启动时重建索引")
	flag.Parse()

	log.Printf("防火墙日志查询系统启动中...")
	log.Printf("日志目录: %s", logDir)
	log.Printf("索引文件: %s", indexFile)
	log.Printf("Web端口: %d", port)

	// 检查日志目录
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		log.Fatalf("日志目录不存在: %s", logDir)
	}

	// 启动时构建或加载索引
	if rebuild || !fileExists(indexFile) {
		log.Println("开始构建索引...")
		if err := buildIndex(); err != nil {
			log.Fatalf("构建索引失败: %v", err)
		}
	} else {
		loadIndexStats()
	}

	// 注册HTTP处理器
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/rebuild", rebuildHandler)
	http.HandleFunc("/stats", statsHandler)
	http.HandleFunc("/api/query", apiQueryHandler)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Web服务已启动: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// 构建索引
func buildIndex() error {
	startTime := time.Now()

	files, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("未找到日志文件")
	}

	log.Printf("找到 %d 个日志文件", len(files))

	// 创建临时文件
	tmpFile := indexFile + ".tmp"
	outFile, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	// 并发处理文件
	fileChan := make(chan string, len(files))
	resultChan := make(chan string, 1000)
	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				processLogFile(file, resultChan)
			}
		}()
	}

	// 写入协程
	var writeWg sync.WaitGroup
	writeWg.Add(1)
	totalRecords := 0
	go func() {
		defer writeWg.Done()
		for line := range resultChan {
			writer.WriteString(line + "\n")
			totalRecords++
			if totalRecords%10000 == 0 {
				log.Printf("已处理 %d 条记录...", totalRecords)
			}
		}
	}()

	// 发送文件到处理队列
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	wg.Wait()
	close(resultChan)
	writeWg.Wait()

	writer.Flush()
	outFile.Close()

	// 替换旧索引
	os.Remove(indexFile)
	if err := os.Rename(tmpFile, indexFile); err != nil {
		return err
	}

	buildTime := time.Since(startTime).Seconds()

	// 更新统计信息
	statsMutex.Lock()
	indexStats = IndexStats{
		TotalRecords: totalRecords,
		TotalFiles:   len(files),
		LastUpdate:   time.Now(),
		BuildTime:    buildTime,
	}
	if info, err := os.Stat(indexFile); err == nil {
		indexStats.IndexSize = info.Size()
	}
	statsMutex.Unlock()

	log.Printf("索引构建完成: %d 条记录, 耗时 %.2f 秒", totalRecords, buildTime)
	return nil
}

// 处理单个日志文件
func processLogFile(filePath string, resultChan chan<- string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("打开文件失败 %s: %v", filePath, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 提取字段
		srcIP := extractField(line, "src=")
		dstIP := extractField(line, "dst=")
		srcPort := extractField(line, "sport=")
		dstPort := extractField(line, "dport=")
		proto := extractField(line, "proto=")
		logDate := extractDate(line)

		if srcIP == "" && dstIP == "" {
			continue
		}

		// 生成索引记录
		if srcIP != "" {
			record := fmt.Sprintf("%s|%s|%s|%s|%s|%d",
				srcIP, logDate, filePath, srcPort, proto, lineNum)
			resultChan <- record
		}

		if dstIP != "" && dstIP != srcIP {
			record := fmt.Sprintf("%s|%s|%s|%s|%s|%d",
				dstIP, logDate, filePath, dstPort, proto, lineNum)
			resultChan <- record
		}
	}
}

// 提取字段值
func extractField(line, prefix string) string {
	idx := strings.Index(line, prefix)
	if idx == -1 {
		return ""
	}

	start := idx + len(prefix)
	end := start

	for end < len(line) && line[end] != ' ' && line[end] != '\t' {
		end++
	}

	return line[start:end]
}

// 提取日期
func extractDate(line string) string {
	if len(line) < 15 {
		return ""
	}

	// 尝试提取时间戳格式: 2026-04-27 或 20260427
	parts := strings.Fields(line)
	if len(parts) > 0 {
		dateStr := parts[0]
		dateStr = strings.ReplaceAll(dateStr, "-", "")
		if len(dateStr) >= 8 {
			return dateStr[:8]
		}
	}

	return ""
}

// 查询索引
func queryIndex(ip, date string, limit int) (*QueryResult, error) {
	startTime := time.Now()

	file, err := os.Open(indexFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var records []LogRecord
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		if (ip == "" || parts[0] == ip) && (date == "" || parts[1] == date) {
			records = append(records, LogRecord{
				IP:       parts[0],
				Date:     parts[1],
				FilePath: parts[2],
				Port:     parts[3],
				Protocol: parts[4],
				LineNum:  parts[5],
			})

			if limit > 0 && len(records) >= limit {
				break
			}
		}
	}

	elapsed := time.Since(startTime).Seconds()

	return &QueryResult{
		Records: records,
		Count:   len(records),
		Time:    elapsed,
	}, scanner.Err()
}

// Web处理器
func indexHandler(w http.ResponseWriter, r *http.Request) {
	statsMutex.RLock()
	stats := indexStats
	statsMutex.RUnlock()

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>防火墙日志查询系统 - 增强版</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Segoe UI', Arial, sans-serif; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; padding: 20px; }
        .container { max-width: 1400px; margin: 0 auto; background: white; border-radius: 12px; box-shadow: 0 10px 40px rgba(0,0,0,0.2); overflow: hidden; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
        .header h1 { font-size: 32px; margin-bottom: 10px; }
        .header p { opacity: 0.9; font-size: 14px; }
        .content { padding: 30px; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%); color: white; padding: 20px; border-radius: 8px; text-align: center; }
        .stat-card.blue { background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%); }
        .stat-card.green { background: linear-gradient(135deg, #43e97b 0%, #38f9d7 100%); }
        .stat-card.orange { background: linear-gradient(135deg, #fa709a 0%, #fee140 100%); }
        .stat-card h3 { font-size: 14px; opacity: 0.9; margin-bottom: 10px; }
        .stat-card .value { font-size: 28px; font-weight: bold; }
        .query-panel { background: #f8f9fa; padding: 25px; border-radius: 8px; margin-bottom: 30px; }
        .form-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 15px; margin-bottom: 20px; }
        .form-group label { display: block; font-weight: 600; margin-bottom: 8px; color: #333; }
        .form-group input { width: 100%; padding: 12px; border: 2px solid #e0e0e0; border-radius: 6px; font-size: 14px; transition: border-color 0.3s; }
        .form-group input:focus { outline: none; border-color: #667eea; }
        .btn-group { display: flex; gap: 10px; }
        button { padding: 12px 30px; border: none; border-radius: 6px; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.3s; }
        .btn-primary { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; }
        .btn-primary:hover { transform: translateY(-2px); box-shadow: 0 5px 15px rgba(102, 126, 234, 0.4); }
        .btn-secondary { background: #6c757d; color: white; }
        .btn-secondary:hover { background: #5a6268; }
        .btn-success { background: #28a745; color: white; }
        .btn-success:hover { background: #218838; }
        .results-panel { margin-top: 30px; }
        .results-header { background: #667eea; color: white; padding: 15px 20px; border-radius: 8px 8px 0 0; display: flex; justify-content: space-between; align-items: center; }
        table { width: 100%; border-collapse: collapse; background: white; }
        th, td { padding: 15px; text-align: left; border-bottom: 1px solid #e0e0e0; }
        th { background: #f8f9fa; font-weight: 600; color: #333; }
        tr:hover { background: #f8f9fa; }
        .badge { display: inline-block; padding: 4px 12px; border-radius: 12px; font-size: 12px; font-weight: 600; }
        .badge-success { background: #d4edda; color: #155724; }
        .badge-info { background: #d1ecf1; color: #0c5460; }
        .badge-warning { background: #fff3cd; color: #856404; }
        .no-data { text-align: center; padding: 60px; color: #999; }
        .loading { text-align: center; padding: 40px; }
        .spinner { border: 4px solid #f3f3f3; border-top: 4px solid #667eea; border-radius: 50%; width: 40px; height: 40px; animation: spin 1s linear infinite; margin: 0 auto; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
        .footer { text-align: center; padding: 20px; color: #999; font-size: 12px; border-top: 1px solid #e0e0e0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔍 防火墙日志查询系统</h1>
            <p>高性能日志检索与分析平台 - Enhanced Edition</p>
        </div>

        <div class="content">
            <div class="stats-grid">
                <div class="stat-card">
                    <h3>📊 索引记录数</h3>
                    <div class="value">` + fmt.Sprintf("%d", stats.TotalRecords) + `</div>
                </div>
                <div class="stat-card blue">
                    <h3>📁 日志文件数</h3>
                    <div class="value">` + fmt.Sprintf("%d", stats.TotalFiles) + `</div>
                </div>
                <div class="stat-card green">
                    <h3>💾 索引大小</h3>
                    <div class="value">` + formatBytes(stats.IndexSize) + `</div>
                </div>
                <div class="stat-card orange">
                    <h3>🕐 最后更新</h3>
                    <div class="value" style="font-size: 16px;">` + stats.LastUpdate.Format("15:04:05") + `</div>
                </div>
            </div>

            <div class="query-panel">
                <form method="GET" action="/query" id="queryForm">
                    <div class="form-row">
                        <div class="form-group">
                            <label>🌐 IP地址</label>
                            <input type="text" name="ip" placeholder="例如: 192.168.1.1" id="ipInput">
                        </div>
                        <div class="form-group">
                            <label>📅 日期</label>
                            <input type="text" name="date" placeholder="格式: 20260427" id="dateInput">
                        </div>
                        <div class="form-group">
                            <label>🔢 限制条数</label>
                            <input type="text" name="limit" placeholder="默认1000" value="1000">
                        </div>
                    </div>
                    <div class="btn-group">
                        <button type="submit" class="btn-primary">🔍 查询</button>
                        <button type="button" class="btn-secondary" onclick="clearForm()">🔄 清空</button>
                        <button type="button" class="btn-success" onclick="rebuildIndex()">⚡ 重建索引</button>
                    </div>
                </form>
            </div>

            <div id="results"></div>
        </div>

        <div class="footer">
            © 2026 防火墙日志查询系统 | Build Time: ` + fmt.Sprintf("%.2fs", stats.BuildTime) + ` | Powered by Go
        </div>
    </div>

    <script>
        function clearForm() {
            document.getElementById('ipInput').value = '';
            document.getElementById('dateInput').value = '';
            document.getElementById('results').innerHTML = '';
        }

        function rebuildIndex() {
            if (!confirm('确定要重建索引吗？这可能需要几分钟时间。')) return;

            document.getElementById('results').innerHTML = '<div class="loading"><div class="spinner"></div><p>正在重建索引，请稍候...</p></div>';

            fetch('/rebuild')
                .then(response => response.json())
                .then(data => {
                    alert('索引重建成功！\n记录数: ' + data.records + '\n耗时: ' + data.time.toFixed(2) + '秒');
                    location.reload();
                })
                .catch(error => {
                    alert('索引重建失败: ' + error);
                    document.getElementById('results').innerHTML = '';
                });
        }

        // 自动填充今天日期
        document.addEventListener('DOMContentLoaded', function() {
            const today = new Date();
            const dateStr = today.getFullYear() +
                           String(today.getMonth() + 1).padStart(2, '0') +
                           String(today.getDate()).padStart(2, '0');
            document.getElementById('dateInput').placeholder = '例如: ' + dateStr;
        });
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, tmpl)
}

// 查询处理器
func queryHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	date := r.URL.Query().Get("date")
	limit := 1000
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)

	result, err := queryIndex(ip, date, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>查询结果</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Segoe UI', Arial, sans-serif; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; padding: 20px; }
        .container { max-width: 1400px; margin: 0 auto; background: white; border-radius: 12px; box-shadow: 0 10px 40px rgba(0,0,0,0.2); overflow: hidden; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 20px 30px; display: flex; justify-content: space-between; align-items: center; }
        .header h1 { font-size: 24px; }
        .btn-back { background: rgba(255,255,255,0.2); color: white; padding: 10px 20px; border: none; border-radius: 6px; cursor: pointer; text-decoration: none; display: inline-block; }
        .btn-back:hover { background: rgba(255,255,255,0.3); }
        .stats-bar { background: #f8f9fa; padding: 20px 30px; border-bottom: 1px solid #e0e0e0; display: flex; gap: 30px; }
        .stat-item { display: flex; align-items: center; gap: 10px; }
        .stat-item .icon { font-size: 24px; }
        .stat-item .text { font-size: 14px; color: #666; }
        .stat-item .value { font-size: 20px; font-weight: bold; color: #333; }
        .content { padding: 30px; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 15px; text-align: left; border-bottom: 1px solid #e0e0e0; }
        th { background: #f8f9fa; font-weight: 600; color: #333; position: sticky; top: 0; }
        tr:hover { background: #f8f9fa; }
        .badge { display: inline-block; padding: 4px 12px; border-radius: 12px; font-size: 12px; font-weight: 600; }
        .badge-success { background: #d4edda; color: #155724; }
        .badge-info { background: #d1ecf1; color: #0c5460; }
        .no-data { text-align: center; padding: 60px; color: #999; font-size: 18px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📋 查询结果</h1>
            <a href="/" class="btn-back">← 返回首页</a>
        </div>

        <div class="stats-bar">
            <div class="stat-item">
                <span class="icon">📊</span>
                <div>
                    <div class="text">找到记录</div>
                    <div class="value">` + fmt.Sprintf("%d", result.Count) + ` 条</div>
                </div>
            </div>
            <div class="stat-item">
                <span class="icon">⏱️</span>
                <div>
                    <div class="text">查询耗时</div>
                    <div class="value">` + fmt.Sprintf("%.3f", result.Time) + ` 秒</div>
                </div>
            </div>
            <div class="stat-item">
                <span class="icon">🔍</span>
                <div>
                    <div class="text">查询条件</div>
                    <div class="value" style="font-size: 14px;">IP: ` + ip + ` | 日期: ` + date + `</div>
                </div>
            </div>
        </div>

        <div class="content">`

	if len(result.Records) == 0 {
		tmpl += `<div class="no-data">😔 未找到匹配的记录</div>`
	} else {
		tmpl += `<table>
            <thead>
                <tr>
                    <th>序号</th>
                    <th>IP地址</th>
                    <th>日期</th>
                    <th>端口</th>
                    <th>协议</th>
                    <th>文件路径</th>
                    <th>行号</th>
                </tr>
            </thead>
            <tbody>`

		for i, record := range result.Records {
			tmpl += fmt.Sprintf(`<tr>
                <td>%d</td>
                <td><span class="badge badge-info">%s</span></td>
                <td>%s</td>
                <td>%s</td>
                <td><span class="badge badge-success">%s</span></td>
                <td style="font-size: 12px;">%s</td>
                <td>%s</td>
            </tr>`, i+1, record.IP, record.Date, record.Port, record.Protocol, record.FilePath, record.LineNum)
		}

		tmpl += `</tbody></table>`
	}

	tmpl += `</div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, tmpl)
}

// 重建索引处理器
func rebuildHandler(w http.ResponseWriter, r *http.Request) {
	go buildIndex()

	time.Sleep(100 * time.Millisecond)

	statsMutex.RLock()
	stats := indexStats
	statsMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"records": stats.TotalRecords,
		"time":    stats.BuildTime,
	})
}

// 统计信息处理器
func statsHandler(w http.ResponseWriter, r *http.Request) {
	statsMutex.RLock()
	stats := indexStats
	statsMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// API查询处理器
func apiQueryHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	date := r.URL.Query().Get("date")
	limit := 1000
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)

	result, err := queryIndex(ip, date, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// 工具函数
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadIndexStats() {
	if info, err := os.Stat(indexFile); err == nil {
		statsMutex.Lock()
		indexStats.IndexSize = info.Size()
		indexStats.LastUpdate = info.ModTime()
		statsMutex.Unlock()
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
