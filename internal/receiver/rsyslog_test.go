package receiver

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fwlog/internal/model"
)

func TestManagerReceivesUDPSyslogToSpoolFile(t *testing.T) {
	port := freeUDPPort(t)
	spoolDir := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	manager.ApplySources([]model.LogSource{{
		SourceID:       "rsyslog-a",
		LogTag:         "核心防火墙",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "udp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		SpoolDir:       spoolDir,
	}})

	status := manager.Status()["rsyslog-a"]
	if !status.Running || status.Error != "" {
		t.Fatalf("receiver should be running: %#v", status)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", status.PortString()))
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer conn.Close()

	message := "<134>2026-07-13T10:00:00Z fw01 NAT 日志测试"
	if _, err := conn.Write([]byte(message)); err != nil {
		t.Fatalf("write syslog: %v", err)
	}

	path := filepath.Join(spoolDir, time.Now().Format("2006-01-02")+".log")
	waitForFileContains(t, path, message)
}

func TestManagerReportsPortConflictWithoutPanic(t *testing.T) {
	port := freeUDPPort(t)
	occupied, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatalf("occupy udp port: %v", err)
	}
	defer occupied.Close()

	manager := NewManager()
	t.Cleanup(manager.Close)
	manager.ApplySources([]model.LogSource{{
		SourceID:       "rsyslog-conflict",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "udp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		SpoolDir:       t.TempDir(),
	}})

	status := manager.Status()["rsyslog-conflict"]
	if status.Running || status.Error == "" {
		t.Fatalf("conflict should be reported as error status: %#v", status)
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate udp port: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func waitForFileContains(t *testing.T, path string, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(content), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	content, _ := os.ReadFile(path)
	t.Fatalf("%s does not contain %q, got %q", path, want, string(content))
}
