//go:build legacy

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 日志记录结构
type LogRecord struct {
	IP       string
	Date     string
	FilePath string
	Port     string
	Protocol string
}

// 提取日志中的IP、端口、协议
func extractLogFields(line, date, filePath string) []LogRecord {
	var records []LogRecord
	
	// 提取所有IP地址
	ipRegex := regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	ips := ipRegex.FindAllString(line, -1)
	
	// 提取源端口
	port := ""
	if match := regexp.MustCompile(`源端口:(\d{1,5})`).FindStringSubmatch(line); len(match) > 1 {
		port = match[1]
	}
	
	// 提取协议
	protocol := ""
	if match := regexp.MustCompile(`协议:(\d+)`).FindStringSubmatch(line); len(match) > 1 {
		protocol = match[1]
	}
	
	// 为每个IP创建记录
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
			})
		}
	}
	
	return records
}

// 处理单个日志文件
func processLogFile(filePath string, output chan<- string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// 从文件名提取日期 (格式: xxx_2026-04-23.log)
	basename := filepath.Base(filePath)
	dateStr := ""
	if match := regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`).FindStringSubmatch(basename); len(match) > 3 {
		dateStr = match[1] + match[2] + match[3] // 20260423
	}
	
	scanner := bufio.NewScanner(file)
	// 增大缓冲区以处理长行
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	
	for scanner.Scan() {
		line := scanner.Text()
		records := extractLogFields(line, dateStr, filePath)
		for _, rec := range records {
			// 输出格式: IP|日期|文件|端口|协议
			output <- fmt.Sprintf("%s|%s|%s|%s|%s",
				rec.IP, rec.Date, rec.FilePath, rec.Port, rec.Protocol)
		}
	}
	
	return scanner.Err()
}

// 构建索引
func buildIndex(logDir, outputFile string, workers int) error {
	// 查找所有日志文件
	pattern := filepath.Join(logDir, "*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	
	if len(files) == 0 {
		return fmt.Errorf("未找到日志文件: %s", pattern)
	}
	
	fmt.Printf("找到 %d 个日志文件\n", len(files))
	
	// 创建输出文件
	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()
	
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	
	// 创建工作通道
	fileChan := make(chan string, len(files))
	outputChan := make(chan string, 10000)
	
	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				if err := processLogFile(file, outputChan); err != nil {
					fmt.Fprintf(os.Stderr, "处理文件失败 %s: %v\n", file, err)
				}
			}
		}()
	}
	
	// 写入协程
	done := make(chan bool)
	recordCount := 0
	go func() {
		for line := range outputChan {
			writer.WriteString(line + "\n")
			recordCount++
			if recordCount%10000 == 0 {
				fmt.Printf("\r已处理: %d 条记录", recordCount)
			}
		}
		done <- true
	}()
	
	// 分发文件任务
	startTime := time.Now()
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)
	
	// 等待所有工作完成
	wg.Wait()
	close(outputChan)
	<-done
	
	elapsed := time.Since(startTime)
	fmt.Printf("\n索引构建完成: %d 条记录, 耗时: %.2f 秒\n", recordCount, elapsed.Seconds())
	
	return nil
}

// 查询索引
func queryIndex(indexFile, ip, date string) error {
	file, err := os.Open(indexFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		
		// 匹配IP和日期
		if (ip == "" || parts[0] == ip) && (date == "" || parts[1] == date) {
			fmt.Println(line)
			count++
		}
	}
	
	if count == 0 {
		fmt.Println("未找到匹配记录")
	} else {
		fmt.Printf("\n共找到 %d 条记录\n", count)
	}
	
	return scanner.Err()
}

func main() {
	// 命令行参数
	mode := flag.String("mode", "build", "模式: build(构建索引) 或 query(查询)")
	logDir := flag.String("logdir", "/data/sangfor_fw_log", "日志目录")
	indexFile := flag.String("index", "data/index/sangfor_fw_log_index.txt", "索引文件路径")
	workers := flag.Int("workers", runtime.NumCPU(), "并行工作数")
	ip := flag.String("ip", "", "查询IP地址")
	date := flag.String("date", "", "查询日期(格式: 20260423)")
	
	flag.Parse()
	
	switch *mode {
	case "build":
		if err := buildIndex(*logDir, *indexFile, *workers); err != nil {
			fmt.Fprintf(os.Stderr, "构建索引失败: %v\n", err)
			os.Exit(1)
		}
	case "query":
		if err := queryIndex(*indexFile, *ip, *date); err != nil {
			fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "未知模式: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}
}
