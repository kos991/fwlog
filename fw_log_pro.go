//go:build legacy

package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 配置
type Config struct {
	LogDir    string
	IndexFile string
	Port      int
	Workers   int
}

// 日志记录
type LogRecord struct {
	IP       string `json:"ip"`
	Date     string `json:"date"`
	FilePath string `json:"file_path"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	SrcIP    string `json:"src_ip"`
	DstIP    string `json:"dst_ip"`
	SrcPort  string `json:"src_port"`
	DstPort  string `json:"dst_port"`
	RawLine  string `json:"raw_line"`
}

// 查询结果
type QueryResult struct {
	Records []LogRecord `json:"records"`
	Count   int         `json:"count"`
	Time    float64     `json:"time"`
}

// 统计信息
type Stats struct {
	TotalRecords int       `json:"total_records"`
	TotalFiles   int       `json:"total_files"`
	IndexSize    int64     `json:"index_size"`
	LastUpdate   time.Time `json:"last_update"`
}

var (
	config     Config
	stats      Stats
	statsMutex sync.RWMutex
	indexMutex sync.RWMutex
)

func main() {
	flag.StringVar(&config.LogDir, "logdir", "/data/sangfor_fw_log", "日志目录")
	flag.StringVar(&config.IndexFile, "index", "sangfor_fw_log_index.db", "索引文件")
	flag.IntVar(&config.Port, "port", 8080, "Web端口")
	flag.IntVar(&config.Workers, "workers", runtime.NumCPU(), "并行数")
	rebuild := flag.Bool("rebuild", false, "重建索引")
	flag.Parse()

	log.Printf("深信服防火墙NAT日志查询系统启动...")
	log.Printf("日志目录: %s", config.LogDir)
	log.Printf("Web端口: %d", config.Port)

	if *rebuild || !fileExists(config.IndexFile) {
		log.Println("开始构建索引...")
		if err := buildIndex(); err != nil {
			log.Fatalf("构建索引失败: %v", err)
		}
	} else {
		loadStats()
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/query", apiQueryHandler)
	http.HandleFunc("/api/rebuild", rebuildHandler)
	http.HandleFunc("/api/stats", statsHandler)
	http.HandleFunc("/api/export", exportHandler)

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("Web服务已启动: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// 构建索引
func buildIndex() error {
	startTime := time.Now()

	files, err := filepath.Glob(filepath.Join(config.LogDir, "*.log"))
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("未找到日志文件")
	}

	log.Printf("找到 %d 个日志文件", len(files))

	tmpFile := config.IndexFile + ".tmp"
	outFile, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	fileChan := make(chan string, len(files))
	resultChan := make(chan string, 10000)
	var wg sync.WaitGroup

	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				processLogFile(file, resultChan)
			}
		}()
	}

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

	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	wg.Wait()
	close(resultChan)
	writeWg.Wait()

	writer.Flush()
	outFile.Close()

	os.Remove(config.IndexFile)
	if err := os.Rename(tmpFile, config.IndexFile); err != nil {
		return err
	}

	statsMutex.Lock()
	stats.TotalRecords = totalRecords
	stats.TotalFiles = len(files)
	stats.LastUpdate = time.Now()
	if info, err := os.Stat(config.IndexFile); err == nil {
		stats.IndexSize = info.Size()
	}
	statsMutex.Unlock()

	elapsed := time.Since(startTime).Seconds()
	log.Printf("索引构建完成: %d 条记录, 耗时 %.2f 秒", totalRecords, elapsed)
	return nil
}

// 处理日志文件
func processLogFile(filePath string, resultChan chan<- string) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	basename := filepath.Base(filePath)
	dateStr := ""
	if match := regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`).FindStringSubmatch(basename); len(match) > 3 {
		dateStr = match[1] + match[2] + match[3]
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	ipRegex := regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)

	for scanner.Scan() {
		line := scanner.Text()

		// 提取源IP
		srcIP := extractField(line, "源IP:")
		dstIP := extractField(line, "目的IP:")
		srcPort := extractField(line, "源端口:")
		dstPort := extractField(line, "目的端口:")
		proto := extractField(line, "协议:")

		// 提取所有IP
		ips := ipRegex.FindAllString(line, -1)
		seen := make(map[string]bool)

		for _, ip := range ips {
			if !seen[ip] {
				seen[ip] = true
				record := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s",
					ip, dateStr, filePath, srcIP, dstIP, srcPort, dstPort, proto, line)
				resultChan <- record
			}
		}
	}
}

// 提取字段
func extractField(line, prefix string) string {
	idx := strings.Index(line, prefix)
	if idx == -1 {
		return ""
	}

	start := idx + len(prefix)
	end := start

	for end < len(line) && line[end] != ',' && line[end] != ' ' {
		end++
	}

	return line[start:end]
}

// 查询索引
func queryIndex(ip, date string, limit int) (*QueryResult, error) {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	startTime := time.Now()

	file, err := os.Open(config.IndexFile)
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
		if len(parts) < 9 {
			continue
		}

		if (ip == "" || parts[0] == ip || parts[3] == ip || parts[4] == ip) &&
			(date == "" || parts[1] == date) {
			records = append(records, LogRecord{
				IP:       parts[0],
				Date:     parts[1],
				FilePath: parts[2],
				SrcIP:    parts[3],
				DstIP:    parts[4],
				SrcPort:  parts[5],
				DstPort:  parts[6],
				Protocol: parts[7],
				RawLine:  parts[8],
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
	s := stats
	statsMutex.RUnlock()

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>深信服防火墙NAT日志查询系统</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/flatpickr/dist/flatpickr.min.css">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Microsoft YaHei', Arial, sans-serif; background: #f0f2f5; }
        .header { background: linear-gradient(135deg, #1e3c72 0%, #2a5298 100%); color: white; padding: 20px 30px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header h1 { font-size: 24px; margin-bottom: 5px; }
        .header p { opacity: 0.9; font-size: 13px; }
        .container { display: flex; max-width: 1600px; margin: 20px auto; gap: 20px; padding: 0 20px; }
        .left-panel { width: 350px; flex-shrink: 0; }
        .right-panel { flex: 1; }
        .panel { background: white; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); padding: 20px; margin-bottom: 20px; }
        .panel-title { font-size: 16px; font-weight: 600; margin-bottom: 15px; color: #333; border-bottom: 2px solid #1e3c72; padding-bottom: 10px; }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; font-size: 13px; font-weight: 600; margin-bottom: 5px; color: #555; }
        .form-group input, .form-group select { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; }
        .form-group input:focus { outline: none; border-color: #1e3c72; }
        .btn { width: 100%; padding: 12px; border: none; border-radius: 4px; font-size: 14px; font-weight: 600; cursor: pointer; transition: all 0.3s; }
        .btn-primary { background: #1e3c72; color: white; margin-bottom: 10px; }
        .btn-primary:hover { background: #2a5298; }
        .btn-secondary { background: #6c757d; color: white; margin-bottom: 10px; }
        .btn-secondary:hover { background: #5a6268; }
        .btn-success { background: #28a745; color: white; }
        .btn-success:hover { background: #218838; }
        .stats-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-bottom: 15px; }
        .stat-item { background: #f8f9fa; padding: 12px; border-radius: 4px; text-align: center; }
        .stat-item .label { font-size: 12px; color: #666; margin-bottom: 5px; }
        .stat-item .value { font-size: 20px; font-weight: bold; color: #1e3c72; }
        .calendar-container { background: white; border-radius: 8px; padding: 15px; }
        #calendar { width: 100%; }
        .results-header { background: #1e3c72; color: white; padding: 15px 20px; border-radius: 8px 8px 0 0; display: flex; justify-content: space-between; align-items: center; }
        .results-header h3 { font-size: 16px; }
        .results-header .actions button { background: rgba(255,255,255,0.2); color: white; border: none; padding: 8px 15px; border-radius: 4px; cursor: pointer; margin-left: 10px; }
        .results-header .actions button:hover { background: rgba(255,255,255,0.3); }
        table { width: 100%; border-collapse: collapse; background: white; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #e0e0e0; font-size: 13px; }
        th { background: #f8f9fa; font-weight: 600; position: sticky; top: 0; }
        tr:hover { background: #f8f9fa; }
        .badge { display: inline-block; padding: 3px 8px; border-radius: 3px; font-size: 11px; font-weight: 600; }
        .badge-blue { background: #cfe2ff; color: #084298; }
        .badge-green { background: #d1e7dd; color: #0f5132; }
        .badge-orange { background: #fff3cd; color: #664d03; }
        .no-data { text-align: center; padding: 60px; color: #999; background: white; border-radius: 0 0 8px 8px; }
        .loading { text-align: center; padding: 40px; background: white; border-radius: 0 0 8px 8px; }
        .spinner { border: 3px solid #f3f3f3; border-top: 3px solid #1e3c72; border-radius: 50%; width: 40px; height: 40px; animation: spin 1s linear infinite; margin: 0 auto 15px; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="header">
        <h1>🛡️ 深信服防火墙NAT日志查询系统</h1>
        <p>Sangfor Firewall NAT Log Query System - Professional Edition</p>
    </div>

    <div class="container">
        <div class="left-panel">
            <div class="panel">
                <div class="panel-title">📊 系统统计</div>
                <div class="stats-grid">
                    <div class="stat-item">
                        <div class="label">索引记录</div>
                        <div class="value">` + fmt.Sprintf("%d", s.TotalRecords) + `</div>
                    </div>
                    <div class="stat-item">
                        <div class="label">日志文件</div>
                        <div class="value">` + fmt.Sprintf("%d", s.TotalFiles) + `</div>
                    </div>
                    <div class="stat-item">
                        <div class="label">索引大小</div>
                        <div class="value" style="font-size: 16px;">` + formatBytes(s.IndexSize) + `</div>
                    </div>
                    <div class="stat-item">
                        <div class="label">最后更新</div>
                        <div class="value" style="font-size: 14px;">` + s.LastUpdate.Format("15:04") + `</div>
                    </div>
                </div>
            </div>

            <div class="panel">
                <div class="panel-title">🔍 查询参数</div>
                <form id="queryForm">
                    <div class="form-group">
                        <label>IP地址</label>
                        <input type="text" id="ipInput" placeholder="例如: 192.168.1.1">
                    </div>
                    <div class="form-group">
                        <label>日期</label>
                        <input type="text" id="dateInput" placeholder="点击选择日期" readonly>
                    </div>
                    <div class="form-group">
                        <label>限制条数</label>
                        <select id="limitInput">
                            <option value="100">100 条</option>
                            <option value="500">500 条</option>
                            <option value="1000" selected>1000 条</option>
                            <option value="5000">5000 条</option>
                            <option value="10000">10000 条</option>
                        </select>
                    </div>
                    <button type="submit" class="btn btn-primary">🔍 查询</button>
                    <button type="button" class="btn btn-secondary" onclick="clearForm()">🔄 清空</button>
                    <button type="button" class="btn btn-success" onclick="rebuildIndex()">⚡ 重建索引</button>
                </form>
            </div>

            <div class="panel">
                <div class="panel-title">📅 日期选择</div>
                <div class="calendar-container">
                    <input type="text" id="calendar" placeholder="选择日期">
                </div>
            </div>
        </div>

        <div class="right-panel">
            <div id="results"></div>
        </div>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/flatpickr"></script>
    <script src="https://cdn.jsdelivr.net/npm/flatpickr/dist/l10n/zh.js"></script>
    <script>
        let currentResults = null;

        // 初始化日历
        const calendar = flatpickr("#calendar", {
            locale: "zh",
            inline: true,
            dateFormat: "Ymd",
            onChange: function(selectedDates, dateStr) {
                document.getElementById('dateInput').value = dateStr;
            }
        });

        flatpickr("#dateInput", {
            locale: "zh",
            dateFormat: "Ymd",
            onChange: function(selectedDates, dateStr) {
                calendar.setDate(dateStr);
            }
        });

        // 查询表单
        document.getElementById('queryForm').addEventListener('submit', function(e) {
            e.preventDefault();
            performQuery();
        });

        function performQuery() {
            const ip = document.getElementById('ipInput').value;
            const date = document.getElementById('dateInput').value;
            const limit = document.getElementById('limitInput').value;

            document.getElementById('results').innerHTML = '<div class="loading"><div class="spinner"></div><p>正在查询，请稍候...</p></div>';

            fetch('/api/query?ip=' + encodeURIComponent(ip) + '&date=' + encodeURIComponent(date) + '&limit=' + limit)
                .then(response => response.json())
                .then(data => {
                    currentResults = data;
                    displayResults(data, ip, date);
                })
                .catch(error => {
                    document.getElementById('results').innerHTML = '<div class="no-data">❌ 查询失败: ' + error + '</div>';
                });
        }

        function displayResults(data, ip, date) {
            let html = '<div class="results-header">';
            html += '<div><h3>📋 查询结果</h3><p style="font-size: 12px; opacity: 0.9;">找到 ' + data.count + ' 条记录 | 耗时 ' + data.time.toFixed(3) + ' 秒</p></div>';
            html += '<div class="actions">';
            html += '<button onclick="exportCSV()">📥 导出CSV</button>';
            html += '<button onclick="exportJSON()">📥 导出JSON</button>';
            html += '</div></div>';

            if (data.records && data.records.length > 0) {
                html += '<table><thead><tr>';
                html += '<th>序号</th><th>源IP</th><th>目的IP</th><th>源端口</th><th>目的端口</th><th>协议</th><th>日期</th><th>原始日志</th>';
                html += '</tr></thead><tbody>';

                data.records.forEach((record, index) => {
                    html += '<tr>';
                    html += '<td>' + (index + 1) + '</td>';
                    html += '<td><span class="badge badge-blue">' + record.src_ip + '</span></td>';
                    html += '<td><span class="badge badge-green">' + record.dst_ip + '</span></td>';
                    html += '<td>' + record.src_port + '</td>';
                    html += '<td>' + record.dst_port + '</td>';
                    html += '<td><span class="badge badge-orange">' + record.protocol + '</span></td>';
                    html += '<td>' + record.date + '</td>';
                    html += '<td style="font-size: 11px; max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="' + record.raw_line + '">' + record.raw_line + '</td>';
                    html += '</tr>';
                });

                html += '</tbody></table>';
            } else {
                html += '<div class="no-data">😔 未找到匹配的记录</div>';
            }

            document.getElementById('results').innerHTML = html;
        }

        function clearForm() {
            document.getElementById('ipInput').value = '';
            document.getElementById('dateInput').value = '';
            calendar.clear();
            document.getElementById('results').innerHTML = '';
            currentResults = null;
        }

        function exportCSV() {
            if (!currentResults || !currentResults.records) {
                alert('没有可导出的数据');
                return;
            }

            const ip = document.getElementById('ipInput').value || 'all';
            const date = document.getElementById('dateInput').value || 'all';

            window.location.href = '/api/export?format=csv&ip=' + encodeURIComponent(ip) + '&date=' + encodeURIComponent(date) + '&limit=' + document.getElementById('limitInput').value;
        }

        function exportJSON() {
            if (!currentResults || !currentResults.records) {
                alert('没有可导出的数据');
                return;
            }

            const dataStr = JSON.stringify(currentResults, null, 2);
            const blob = new Blob([dataStr], {type: 'application/json'});
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'firewall_log_' + new Date().getTime() + '.json';
            a.click();
        }

        function rebuildIndex() {
            if (!confirm('确定要重建索引吗？这可能需要几分钟时间。')) return;

            document.getElementById('results').innerHTML = '<div class="loading"><div class="spinner"></div><p>正在重建索引，请稍候...</p></div>';

            fetch('/api/rebuild')
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
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, tmpl)
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

// 重建索引处理器
func rebuildHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	if err := buildIndex(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(startTime).Seconds()

	statsMutex.RLock()
	s := stats
	statsMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"records": s.TotalRecords,
		"time":    elapsed,
	})
}

// 统计信息处理器
func statsHandler(w http.ResponseWriter, r *http.Request) {
	statsMutex.RLock()
	s := stats
	statsMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// 导出处理器
func exportHandler(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	ip := r.URL.Query().Get("ip")
	date := r.URL.Query().Get("date")
	limit := 10000
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)

	result, err := queryIndex(ip, date, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if format == "csv" {
		filename := fmt.Sprintf("firewall_log_%s.csv", time.Now().Format("20060102_150405"))
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)

		writer := csv.NewWriter(w)
		writer.Write([]string{"序号", "源IP", "目的IP", "源端口", "目的端口", "协议", "日期", "文件路径"})

		for i, record := range result.Records {
			writer.Write([]string{
				fmt.Sprintf("%d", i+1),
				record.SrcIP,
				record.DstIP,
				record.SrcPort,
				record.DstPort,
				record.Protocol,
				record.Date,
				record.FilePath,
			})
		}

		writer.Flush()
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// 工具函数
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadStats() {
	if info, err := os.Stat(config.IndexFile); err == nil {
		statsMutex.Lock()
		stats.IndexSize = info.Size()
		stats.LastUpdate = info.ModTime()
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
