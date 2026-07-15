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

func TestManagerReceivesTCPSyslogToSpoolFile(t *testing.T) {
	port := freeTCPPort(t)
	spoolDir := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	manager.ApplySources([]model.LogSource{{
		SourceID:       "rsyslog-tcp",
		LogTag:         "核心防火墙",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "tcp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		SpoolDir:       spoolDir,
	}})

	status := manager.Status()["rsyslog-tcp"]
	if !status.Running || status.Error != "" || status.Protocol != "tcp" {
		t.Fatalf("tcp receiver should be running: %#v", status)
	}

	conn, err := net.Dial("tcp", status.Address)
	if err != nil {
		t.Fatalf("dial tcp receiver: %v", err)
	}
	message := "<134>2026-07-14T10:00:00Z fw01 NAT TCP 日志测试"
	if _, err := conn.Write([]byte(message + "\n")); err != nil {
		conn.Close()
		t.Fatalf("write tcp syslog: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close tcp connection: %v", err)
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

func TestManagerRoutesUDPClientsSharingOneEndpoint(t *testing.T) {
	port := freeUDPPort(t)
	localSpool := t.TempDir()
	otherSpool := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	err := manager.ApplySources([]model.LogSource{
		{
			SourceID:       "local",
			Enabled:        true,
			SourceType:     "rsyslog",
			ListenProtocol: "udp",
			ListenHost:     "127.0.0.1",
			ListenPort:     port,
			ClientIP:       "127.0.0.1/32",
			SpoolDir:       localSpool,
		},
		{
			SourceID:       "other",
			Enabled:        true,
			SourceType:     "rsyslog",
			ListenProtocol: "udp",
			ListenHost:     "127.0.0.1",
			ListenPort:     port,
			ClientIP:       "192.0.2.0/24",
			SpoolDir:       otherSpool,
		},
	})
	if err != nil {
		t.Fatalf("ApplySources: %v", err)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer conn.Close()
	message := "<134> shared UDP route"
	if _, err := conn.Write([]byte(message)); err != nil {
		t.Fatalf("write syslog: %v", err)
	}

	localPath := filepath.Join(localSpool, time.Now().Format("2006-01-02")+".log")
	waitForFileContains(t, localPath, message)
	assertFileDoesNotExist(t, filepath.Join(otherSpool, time.Now().Format("2006-01-02")+".log"))

	status := waitForReceivedMessages(t, manager, "local", 1)
	if status.ClientIP != "127.0.0.1/32" || status.LastClientIP != "127.0.0.1" || status.LastReceivedAt.IsZero() {
		t.Fatalf("local status = %#v", status)
	}
}

func TestManagerRoutesTCPClientsSharingOneEndpoint(t *testing.T) {
	port := freeTCPPort(t)
	localSpool := t.TempDir()
	otherSpool := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	err := manager.ApplySources([]model.LogSource{
		{
			SourceID:       "local",
			Enabled:        true,
			SourceType:     "rsyslog",
			ListenProtocol: "tcp",
			ListenHost:     "127.0.0.1",
			ListenPort:     port,
			ClientIP:       "127.0.0.1",
			SpoolDir:       localSpool,
		},
		{
			SourceID:       "other",
			Enabled:        true,
			SourceType:     "rsyslog",
			ListenProtocol: "tcp",
			ListenHost:     "127.0.0.1",
			ListenPort:     port,
			ClientIP:       "198.51.100.0/24",
			SpoolDir:       otherSpool,
		},
	})
	if err != nil {
		t.Fatalf("ApplySources: %v", err)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	message := "<134> shared TCP route"
	if _, err := conn.Write([]byte(message + "\n")); err != nil {
		conn.Close()
		t.Fatalf("write syslog: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close connection: %v", err)
	}

	localPath := filepath.Join(localSpool, time.Now().Format("2006-01-02")+".log")
	waitForFileContains(t, localPath, message)
	assertFileDoesNotExist(t, filepath.Join(otherSpool, time.Now().Format("2006-01-02")+".log"))

	status := waitForReceivedMessages(t, manager, "local", 1)
	if status.ClientIP != "127.0.0.1" || status.LastClientIP != "127.0.0.1" || status.LastReceivedAt.IsZero() {
		t.Fatalf("local status = %#v", status)
	}
}

func TestManagerRejectsUnmatchedUDPClient(t *testing.T) {
	port := freeUDPPort(t)
	spoolDir := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	if err := manager.ApplySources([]model.LogSource{{
		SourceID:       "remote-only",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "udp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		ClientIP:       "192.0.2.0/24",
		SpoolDir:       spoolDir,
	}}); err != nil {
		t.Fatalf("ApplySources: %v", err)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	if _, err := conn.Write([]byte("<134> rejected UDP route")); err != nil {
		conn.Close()
		t.Fatalf("write syslog: %v", err)
	}
	conn.Close()

	time.Sleep(100 * time.Millisecond)
	assertFileDoesNotExist(t, filepath.Join(spoolDir, time.Now().Format("2006-01-02")+".log"))
	if status := manager.Status()["remote-only"]; status.ReceivedMessages != 0 {
		t.Fatalf("unmatched source status = %#v", status)
	}
}

func TestManagerReloadsRoutesWithoutClosingSharedTCPConnection(t *testing.T) {
	port := freeTCPPort(t)
	firstSpool := t.TempDir()
	secondSpool := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	base := model.LogSource{
		SourceID:       "local",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "tcp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		ClientIP:       "127.0.0.1",
		SpoolDir:       firstSpool,
	}
	if err := manager.ApplySources([]model.LogSource{base}); err != nil {
		t.Fatalf("initial ApplySources: %v", err)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("before reload\n")); err != nil {
		t.Fatalf("write before reload: %v", err)
	}
	waitForFileContains(t, filepath.Join(firstSpool, time.Now().Format("2006-01-02")+".log"), "before reload")

	base.SpoolDir = secondSpool
	if err := manager.ApplySources([]model.LogSource{base}); err != nil {
		t.Fatalf("reload ApplySources: %v", err)
	}
	if _, err := conn.Write([]byte("after reload\n")); err != nil {
		t.Fatalf("write after reload: %v", err)
	}
	waitForFileContains(t, filepath.Join(secondSpool, time.Now().Format("2006-01-02")+".log"), "after reload")
}

func TestSpoolWriterReusesFileAndRotatesByDay(t *testing.T) {
	dir := t.TempDir()
	writer := newSpoolWriter(dir)
	t.Cleanup(func() { _ = writer.Close() })

	firstDay := time.Date(2026, 7, 14, 23, 59, 0, 0, time.Local)
	if err := writer.Append("first", firstDay); err != nil {
		t.Fatal(err)
	}
	firstFile := writer.file
	if err := writer.Append("second", firstDay.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if writer.file != firstFile {
		t.Fatal("same-day append reopened the spool file")
	}

	secondDay := firstDay.Add(2 * time.Minute)
	if err := writer.Append("third", secondDay); err != nil {
		t.Fatal(err)
	}
	if writer.file == firstFile {
		t.Fatal("next-day append did not rotate the spool file")
	}
	if _, err := firstFile.WriteString("closed"); err == nil {
		t.Fatal("rotated spool file is still open")
	}

	assertFileContains(t, filepath.Join(dir, "2026-07-14.log"), "first\nsecond\n")
	assertFileContains(t, filepath.Join(dir, "2026-07-15.log"), "third\n")
}

func TestManagerClosesUnusedSpoolWriterWhenSourcesChange(t *testing.T) {
	port := freeUDPPort(t)
	spoolDir := t.TempDir()
	manager := NewManager()
	t.Cleanup(manager.Close)

	if err := manager.ApplySources([]model.LogSource{{
		SourceID:       "rsyslog-a",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "udp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		SpoolDir:       spoolDir,
	}}); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("first\nsecond")); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	conn.Close()
	waitForReceivedMessages(t, manager, "rsyslog-a", 1)

	manager.writerMu.Lock()
	writer := manager.spoolWriters[filepath.Clean(spoolDir)]
	manager.writerMu.Unlock()
	if writer == nil || writer.file == nil {
		t.Fatal("manager did not retain the active spool writer")
	}
	file := writer.file

	if err := manager.ApplySources(nil); err != nil {
		t.Fatal(err)
	}
	manager.writerMu.Lock()
	remaining := len(manager.spoolWriters)
	manager.writerMu.Unlock()
	if remaining != 0 {
		t.Fatalf("unused spool writers = %d; want 0", remaining)
	}
	if _, err := file.WriteString("closed"); err == nil {
		t.Fatal("unused spool file is still open")
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

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate tcp port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
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

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("%s content = %q; want %q", path, content, want)
	}
}

func waitForReceivedMessages(t *testing.T, manager *Manager, sourceID string, want uint64) Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.Status()[sourceID]
		if status.ReceivedMessages >= want {
			return status
		}
		time.Sleep(20 * time.Millisecond)
	}
	status := manager.Status()[sourceID]
	t.Fatalf("source %s received_messages = %d; want at least %d", sourceID, status.ReceivedMessages, want)
	return Status{}
}

func assertFileDoesNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected spool file %s: %v", path, err)
	}
}
