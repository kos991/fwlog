//go:build legacy

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
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
	IP       string
	Date     string
	FilePath string
	Port     string
	Protocol string
	Line     string
}

// 查询结果
type QueryResult struct {
	Records []LogRecord
	Count   int
	Time    float64
}

var config Config
var indexMutex sync.RWMutex

// 提取日志字段
func extractLogFields(line, date, filePath string) []LogRecord {
	var records []LogRecord

	ipRegex := regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	ips := ipRegex.FindAllString(line, -1)

	port := ""
	if match := regexp.MustCompile(`源端口:(\d{1,5})`).FindStringSubmatch(line); len(match) > 1 {
		port = match[1]
	}

	protocol := ""
	if match := regexp.MustCompile(`协议:(\d+)`).FindStringSubmatch(line); len(match) > 1 {
		protocol = match[1]
	}

	seen := make(map[string]bool)
	for _, ip := range ips {
		if !seen[ip] {
			seen[ip] = true
			records = append(records, LogRecord{
				IP:       ip,
				Date:     date,
				FilePath: filePath,
				Port:     port,
				Protocol: protocol,
				Line:     line,
			})
		}
	}

	return records
}

// 处理日志文件
func processLogFile(filePath string, output chan<- string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	basename := filepath.Base(filePath)
	dateStr := ""
	if match := regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`).FindStringSubmatch(basename); len(match) > 3 {
		dateStr = match[1] + match[2] + match[3]
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		records := extractLogFields(line, dateStr, filePath)
		for _, rec := range records {
			output <- fmt.Sprintf("%s|%s|%s|%s|%s",
				rec.IP, rec.Date, rec.FilePath, rec.Port, rec.Protocol)
		}
	}

	return scanner.Err()
}

// 构建索引
func buildIndex() error {
	indexMutex.Lock()
	defer indexMutex.Unlock()

	pattern := filepath.Join(config.LogDir, "*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("未找到日志文件: %s", pattern)
	}

	log.Printf("找到 %d 个日志文件", len(files))

	// 确保目录存在
	indexDir := filepath.Dir(config.IndexFile)
	os.MkdirAll(indexDir, 0755)

	out, err := os.Create(config.IndexFile)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := bufio.NewWriter(out)
	defer writer.Flush()

	fileChan := make(chan string, len(files))
	outputChan := make(chan string, 10000)

	var wg sync.WaitGroup
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				if err := processLogFile(file, outputChan); err != nil {
					log.Printf("处理文件失败 %s: %v", file, err)
				}
			}
		}()
	}

	done := make(chan bool)
	recordCount := 0
	go func() {
		for line := range outputChan {
			writer.WriteString(line + "\n")
			recordCount++
		}
		done <- true
	}()

	startTime := time.Now()
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	wg.Wait()
	close(outputChan)
	<-done

	elapsed := time.Since(startTime)
	log.Printf("索引构建完成: %d 条记录, 耗时: %.2f 秒", recordCount, elapsed.Seconds())

	return nil
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
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var records []LogRecord
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		if (ip == "" || parts[0] == ip) && (date == "" || parts[1] == date) {
			records = append(records, LogRecord{
				IP:       parts[0],
				Date:     parts[1],
				FilePath: parts[2],
				Port:     parts[3],
				Protocol: parts[4],
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
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>防火墙日志查询系统</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; border-bottom: 2px solid #4CAF50; padding-bottom: 10px; }
        .form-group { margin: 15px 0; }
        label { display: inline-block; width: 100px; font-weight: bold; }
        input[type="text"] { padding: 8px; width: 300px; border: 1px solid #ddd; border-radius: 4px; }
        button { padding: 10px 20px; background: #4CAF50; color: white; border: none; border-radius: 4px; cursor: pointer; margin-right: 10px; }
        button:hover { background: #45a049; }
        .rebuild-btn { background: #2196F3; }
        .rebuild-btn:hover { background: #0b7dda; }
        .stats { background: #e8f5e9; padding: 15px; border-radius: 4px; margin: 20px 0; }
        .stats span { margin-right: 20px; font-weight: bold; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #4CAF50; color: white; }
        tr:hover { background: #f5f5f5; }
        .no-data { text-align: center; padding: 40px; color: #999; }
        .loading { text-align: center; padding: 20px; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 防火墙日志查询系统</h1>

        <form method="GET" action="/query">
            <div class="form-group">
                <label>IP地址:</label>
                <input type="text" name="ip" placeholder="例如: 192.168.1.1" value="{{.IP}}">
            </div>
            <div class="form-group">
                <label>日期:</label>
                <input type="text" name="date" placeholder="格式: 20260427" value="{{.Date}}">
            </div>
            <div class="form-group">
                <label>限制条数:</label>
                <input type="text" name="limit" placeholder="默认1000" value="{{.Limit}}" style="width:100px;">
            </div>
            <div class="form-group">
                <label></label>
                <button type="submit">查询</button>
                <button type="button" class="rebuild-btn" onclick="rebuildIndex()">重建索引</button>
            </div>
        </form>

        {{if .Result}}
        <div class="stats">
            <span>📊 找到记录: {{.Result.Count}} 条</span>
            <span>⏱️ 查询耗时: {{printf "%.3f" .Result.Time}} 秒</span>
        </div>

        {{if .Result.Records}}
        <table>
            <thead>
                <tr>
                    <th>IP地址</th>
                    <th>日期</th>
                    <th>端口</th>
                    <th>协议</th>
                    <th>文件路径</th>
                </tr>
            </thead>
            <tbody>
                {{range .Result.Records}}
                <tr>
                    <td>{{.IP}}</td>
                    <td>{{.Date}}</td>
                    <td>{{.Port}}</td>
                    <td>{{.Protocol}}</td>
                    <td>{{.FilePath}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <div class="no-data">未找到匹配记录</div>
        {{end}}
        {{end}}
    </div>

    <script>
        function rebuildIndex() {
            if (!confirm('确定要重建索引吗？这可能需要几分钟时间。')) return;

            document.body.innerHTML += '<div class="loading">正在重建索引，请稍候...</div>';

            fetch('/rebuild', {method: 'POST'})
                .then(r => r.json())
                .then(data => {
                    alert(data.message);
                    location.reload();
                })
                .catch(err => {
                    alert('重建索引失败: ' + err);
                    location.reload();
                });
        }
    </script>
</body>
</html>
`

	t, _ := template.New("index").Parse(tmpl)
	t.Execute(w, map[string]interface{}{
		"IP":     r.URL.Query().Get("ip"),
		"Date":   r.URL.Query().Get("date"),
		"Limit":  r.URL.Query().Get("limit"),
		"Result": nil,
	})
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	date := r.URL.Query().Get("date")
	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	result, err := queryIndex(ip, date, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>查询结果</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; border-bottom: 2px solid #4CAF50; padding-bottom: 10px; }
        .stats { background: #e8f5e9; padding: 15px; border-radius: 4px; margin: 20px 0; }
        .stats span { margin-right: 20px; font-weight: bold; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #4CAF50; color: white; }
        tr:hover { background: #f5f5f5; }
        .no-data { text-align: center; padding: 40px; color: #999; }
        a { color: #4CAF50; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 查询结果</h1>
        <p><a href="/">← 返回查询</a></p>

        <div class="stats">
            <span>📊 找到记录: {{.Count}} 条</span>
            <span>⏱️ 查询耗时: {{printf "%.3f" .Time}} 秒</span>
        </div>

        {{if .Records}}
        <table>
            <thead>
                <tr>
                    <th>IP地址</th>
                    <th>日期</th>
                    <th>端口</th>
                    <th>协议</th>
                    <th>文件路径</th>
                </tr>
            </thead>
            <tbody>
                {{range .Records}}
                <tr>
                    <td>{{.IP}}</td>
                    <td>{{.Date}}</td>
                    <td>{{.Port}}</td>
                    <td>{{.Protocol}}</td>
                    <td>{{.FilePath}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <div class="no-data">未找到匹配记录</div>
        {{end}}
    </div>
</body>
</html>
`

	t, _ := template.New("query").Parse(tmpl)
	t.Execute(w, result)
}

func rebuildHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		if err := buildIndex(); err != nil {
			log.Printf("重建索引失败: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "索引重建已开始，请稍候...",
	})
}

func main() {
	flag.StringVar(&config.LogDir, "logdir", "/data/sangfor_fw_log", "日志目录")
	flag.StringVar(&config.IndexFile, "index", "/data/sangfor_fw_log_chaxun/data/index/fw_log_index.txt", "索引文件")
	flag.IntVar(&config.Port, "port", 8080, "Web服务端口")
	flag.IntVar(&config.Workers, "workers", runtime.NumCPU(), "并行工作数")
	rebuild := flag.Bool("rebuild", false, "启动时重建索引")
	flag.Parse()

	log.Printf("配置: 日志目录=%s, 索引文件=%s, 端口=%d, 工作数=%d",
		config.LogDir, config.IndexFile, config.Port, config.Workers)

	if *rebuild {
		log.Println("正在重建索引...")
		if err := buildIndex(); err != nil {
			log.Fatalf("重建索引失败: %v", err)
		}
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/rebuild", rebuildHandler)

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("Web服务启动: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
