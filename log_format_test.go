package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSupportedLogFileMatchesSangforRotatedLogs(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"10.10.10.1_2026-05-06.log", false},
		{"10.10.10.1_2026-05-05.log-20260506", true},
		{"10.10.10.1_2026-04-29.log-20260430.gz", true},
		{"read me.txt", false},
		{"nat.log.tmp", false},
	}

	for _, test := range tests {
		if got := isSupportedLogFile(test.name); got != test.want {
			t.Fatalf("isSupportedLogFile(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestProcessLogFileRangeReadsGzipSangforLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "10.10.10.1_2026-04-29.log-20260430.gz")
	line := "Apr 29 00:00:09 localhost nat: 日志类型:NAT日志, NAT类型:snat, 源IP:192.168.0.101, 源端口:34165, 目的IP:114.114.114.114, 目的端口:53, 协议:17, 转换后的IP:58.216.48.6, 转换后的端口:34165\n"

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create gzip log: %v", err)
	}
	gz := gzip.NewWriter(file)
	if _, err := gz.Write([]byte(line)); err != nil {
		t.Fatalf("write gzip log: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close gzip file: %v", err)
	}

	var out bytes.Buffer
	total := 0
	if err := processLogFileRange(path, 0, int64(len(line)), &out, &total); err != nil {
		t.Fatalf("process gzip log: %v", err)
	}

	if total != 1 {
		t.Fatalf("total parsed lines = %d, want 1", total)
	}
	got := out.String()
	if !strings.Contains(got, "192.168.0.101|34165|114.114.114.114|53|UDP|58.216.48.6|34165") {
		t.Fatalf("parsed CSV line missing expected fields: %q", got)
	}
}

func TestRequiresFullRebuildWhenGzipArchiveSizeChanges(t *testing.T) {
	stored := []LogFileSnapshot{{Path: "/data/sangfor_fw_log/fw.log-20260506.gz", Size: 100}}
	current := []LogFileSnapshot{{Path: "/data/sangfor_fw_log/fw.log-20260506.gz", Size: 120}}

	if !requiresFullRebuild(stored, current) {
		t.Fatal("changed gzip archive must trigger full rebuild")
	}
}
