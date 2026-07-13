package receiver

import (
	"bufio"
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
	listeners map[string]receiverListener
	statuses  map[string]Status
}

type Status struct {
	SourceID string `json:"source_id"`
	Protocol string `json:"protocol"`
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

type receiverListener interface {
	Close() error
}

type tcpListener struct {
	source   model.LogSource
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	closed   bool
}

func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string]receiverListener),
		statuses:  make(map[string]Status),
	}
}

func (m *Manager) ApplySources(sources []model.LogSource) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for sourceID, listener := range m.listeners {
		_ = listener.Close()
		delete(m.listeners, sourceID)
	}
	m.statuses = make(map[string]Status)

	for _, source := range sources {
		if !shouldReceive(source) {
			continue
		}
		status := Status{
			SourceID: source.SourceID,
			Protocol: strings.ToLower(strings.TrimSpace(source.ListenProtocol)),
			Address:  net.JoinHostPort(source.ListenHost, intToString(source.ListenPort)),
			Port:     source.ListenPort,
			SpoolDir: source.SpoolDir,
		}
		if err := os.MkdirAll(source.SpoolDir, 0o755); err != nil {
			status.Error = err.Error()
			m.statuses[source.SourceID] = status
			continue
		}
		if status.Protocol == "tcp" {
			conn, err := net.Listen("tcp", status.Address)
			if err != nil {
				status.Error = err.Error()
				m.statuses[source.SourceID] = status
				continue
			}
			status.Running = true
			listener := &tcpListener{source: source, listener: conn, conns: make(map[net.Conn]struct{})}
			m.listeners[source.SourceID] = listener
			m.statuses[source.SourceID] = status
			go listener.serve()
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
		_ = listener.Close()
		delete(m.listeners, sourceID)
	}
}

func (l *udpListener) Close() error {
	return l.conn.Close()
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

func (l *tcpListener) serve() {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			return
		}
		if !l.track(conn) {
			_ = conn.Close()
			return
		}
		go l.serveConn(conn)
	}
}

func (l *tcpListener) serveConn(conn net.Conn) {
	defer func() {
		l.untrack(conn)
		_ = conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		message := strings.TrimRight(scanner.Text(), "\r\n")
		if message != "" {
			_ = appendMessage(l.source.SpoolDir, message)
		}
	}
}

func (l *tcpListener) track(conn net.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.conns[conn] = struct{}{}
	return true
}

func (l *tcpListener) untrack(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.conns, conn)
}

func (l *tcpListener) Close() error {
	l.mu.Lock()
	l.closed = true
	err := l.listener.Close()
	for conn := range l.conns {
		_ = conn.Close()
		delete(l.conns, conn)
	}
	l.mu.Unlock()
	return err
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
	protocol := strings.ToLower(strings.TrimSpace(source.ListenProtocol))
	return source.Enabled &&
		strings.EqualFold(strings.TrimSpace(source.SourceType), "rsyslog") &&
		(protocol == "udp" || protocol == "tcp") &&
		strings.TrimSpace(source.ListenHost) != "" &&
		source.ListenPort > 0 &&
		strings.TrimSpace(source.SpoolDir) != ""
}

func intToString(value int) string {
	return strconv.Itoa(value)
}
