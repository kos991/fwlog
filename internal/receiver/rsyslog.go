package receiver

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fwlog/internal/model"
)

type Manager struct {
	mu        sync.Mutex
	listeners map[string]*udpListener
	statuses  map[string]Status
}

type Status struct {
	SourceID string `json:"source_id"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	SpoolDir string `json:"spool_dir"`
	Running  bool   `json:"running"`
	Error    string `json:"error"`
}

func (s Status) PortString() string {
	return intToString(s.Port)
}

type udpListener struct {
	source model.LogSource
	conn   net.PacketConn
	done   chan struct{}
}

func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string]*udpListener),
		statuses:  make(map[string]Status),
	}
}

func (m *Manager) ApplySources(sources []model.LogSource) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for sourceID, listener := range m.listeners {
		_ = listener.conn.Close()
		delete(m.listeners, sourceID)
	}
	m.statuses = make(map[string]Status)

	for _, source := range sources {
		if !shouldReceive(source) {
			continue
		}
		status := Status{
			SourceID: source.SourceID,
			Address:  net.JoinHostPort(source.ListenHost, intToString(source.ListenPort)),
			Port:     source.ListenPort,
			SpoolDir: source.SpoolDir,
		}
		if err := os.MkdirAll(source.SpoolDir, 0o755); err != nil {
			status.Error = err.Error()
			m.statuses[source.SourceID] = status
			continue
		}
		conn, err := net.ListenPacket("udp", status.Address)
		if err != nil {
			status.Error = err.Error()
			m.statuses[source.SourceID] = status
			continue
		}
		status.Running = true
		listener := &udpListener{source: source, conn: conn, done: make(chan struct{})}
		m.listeners[source.SourceID] = listener
		m.statuses[source.SourceID] = status
		go listener.serve()
	}
}

func (m *Manager) Status() map[string]Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := make(map[string]Status, len(m.statuses))
	for key, status := range m.statuses {
		statuses[key] = status
	}
	return statuses
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for sourceID, listener := range m.listeners {
		_ = listener.conn.Close()
		delete(m.listeners, sourceID)
	}
}

func (l *udpListener) serve() {
	buffer := make([]byte, 64*1024)
	for {
		n, _, err := l.conn.ReadFrom(buffer)
		if err != nil {
			return
		}
		message := strings.TrimRight(string(buffer[:n]), "\r\n")
		if message == "" {
			continue
		}
		_ = appendMessage(l.source.SpoolDir, message)
	}
}

func appendMessage(spoolDir string, message string) error {
	if err := os.MkdirAll(spoolDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(spoolDir, time.Now().Format("2006-01-02")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, message)
	return err
}

func shouldReceive(source model.LogSource) bool {
	return source.Enabled &&
		strings.EqualFold(strings.TrimSpace(source.SourceType), "rsyslog") &&
		strings.EqualFold(strings.TrimSpace(source.ListenProtocol), "udp") &&
		strings.TrimSpace(source.ListenHost) != "" &&
		source.ListenPort > 0 &&
		strings.TrimSpace(source.SpoolDir) != ""
}

func intToString(value int) string {
	return strconv.Itoa(value)
}
