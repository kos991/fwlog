package receiver

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fwlog/internal/model"
)

type Manager struct {
	mu        sync.Mutex
	listeners map[endpointKey]*endpointListener
	statuses  map[string]Status
}

type Status struct {
	SourceID         string    `json:"source_id"`
	Protocol         string    `json:"protocol"`
	Address          string    `json:"address"`
	Port             int       `json:"port"`
	SpoolDir         string    `json:"spool_dir"`
	ClientIP         string    `json:"client_ip"`
	Running          bool      `json:"running"`
	Error            string    `json:"error"`
	LastClientIP     string    `json:"last_client_ip"`
	LastReceivedAt   time.Time `json:"last_received_at"`
	ReceivedMessages uint64    `json:"received_messages"`
	ArchiveError     string    `json:"archive_error"`
	LastArchiveAt    time.Time `json:"last_archive_at"`
}

func (s Status) PortString() string {
	return intToString(s.Port)
}

type endpointListener struct {
	key       endpointKey
	manager   *Manager
	packet    net.PacketConn
	listener  net.Listener
	routes    atomic.Value
	connMu    sync.Mutex
	conns     map[net.Conn]struct{}
	closed    bool
	closeOnce sync.Once
	lastDrop  atomic.Int64
}

func NewManager() *Manager {
	return &Manager{
		listeners: make(map[endpointKey]*endpointListener),
		statuses:  make(map[string]Status),
	}
}

func (m *Manager) ApplySources(sources []model.LogSource) error {
	grouped := make(map[endpointKey][]model.LogSource)
	ordered := make([]model.LogSource, 0, len(sources))
	for _, source := range sources {
		if !shouldReceive(source) {
			continue
		}
		source.ListenProtocol = strings.ToLower(strings.TrimSpace(source.ListenProtocol))
		source.ListenHost = strings.TrimSpace(source.ListenHost)
		source.ClientIP = strings.TrimSpace(source.ClientIP)
		source.SpoolDir = strings.TrimSpace(source.SpoolDir)
		key := endpointKey{Protocol: source.ListenProtocol, Host: source.ListenHost, Port: source.ListenPort}
		grouped[key] = append(grouped[key], source)
		ordered = append(ordered, source)
	}

	tables := make(map[endpointKey]routeTable, len(grouped))
	for key, endpointSources := range grouped {
		table, err := buildRouteTable(endpointSources)
		if err != nil {
			m.recordApplyError(endpointSources, err)
			return fmt.Errorf("构建 RSyslog 路由 %s: %w", endpointAddress(key), err)
		}
		for _, source := range endpointSources {
			if err := os.MkdirAll(source.SpoolDir, 0o755); err != nil {
				m.recordApplyError(endpointSources, err)
				return fmt.Errorf("创建日志源 %s 落盘目录: %w", source.SourceID, err)
			}
		}
		tables[key] = table
	}

	m.mu.Lock()
	created := make(map[endpointKey]*endpointListener)
	for key, table := range tables {
		if _, exists := m.listeners[key]; exists {
			continue
		}
		listener, err := newEndpointListener(m, key, table)
		if err != nil {
			for _, candidate := range created {
				_ = candidate.Close()
			}
			for _, source := range grouped[key] {
				status := statusForSource(source)
				status.Error = err.Error()
				m.statuses[source.SourceID] = status
			}
			m.mu.Unlock()
			return fmt.Errorf("监听 RSyslog 端点 %s: %w", endpointAddress(key), err)
		}
		created[key] = listener
	}

	nextListeners := make(map[endpointKey]*endpointListener, len(tables))
	toStart := make([]*endpointListener, 0, len(created))
	for key, table := range tables {
		listener, exists := m.listeners[key]
		if !exists {
			listener = created[key]
			toStart = append(toStart, listener)
		} else {
			listener.routes.Store(table)
		}
		nextListeners[key] = listener
	}

	nextStatuses := make(map[string]Status, len(ordered))
	for _, source := range ordered {
		status := statusForSource(source)
		status.Running = true
		if previous, exists := m.statuses[source.SourceID]; exists {
			status.LastClientIP = previous.LastClientIP
			status.LastReceivedAt = previous.LastReceivedAt
			status.ReceivedMessages = previous.ReceivedMessages
			status.ArchiveError = previous.ArchiveError
			status.LastArchiveAt = previous.LastArchiveAt
		}
		nextStatuses[source.SourceID] = status
	}

	removed := make([]*endpointListener, 0)
	for key, listener := range m.listeners {
		if _, keep := nextListeners[key]; !keep {
			removed = append(removed, listener)
		}
	}
	m.listeners = nextListeners
	m.statuses = nextStatuses
	m.mu.Unlock()

	for _, listener := range toStart {
		go listener.serve()
	}
	for _, listener := range removed {
		_ = listener.Close()
	}
	return nil
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
	listeners := make([]*endpointListener, 0, len(m.listeners))
	for key, listener := range m.listeners {
		listeners = append(listeners, listener)
		delete(m.listeners, key)
	}
	for sourceID, status := range m.statuses {
		status.Running = false
		m.statuses[sourceID] = status
	}
	m.mu.Unlock()

	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func newEndpointListener(manager *Manager, key endpointKey, table routeTable) (*endpointListener, error) {
	listener := &endpointListener{
		key:     key,
		manager: manager,
		conns:   make(map[net.Conn]struct{}),
	}
	listener.routes.Store(table)
	var err error
	if key.Protocol == "tcp" {
		listener.listener, err = net.Listen("tcp", endpointAddress(key))
	} else {
		listener.packet, err = net.ListenPacket("udp", endpointAddress(key))
	}
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func (l *endpointListener) serve() {
	if l.key.Protocol == "tcp" {
		l.serveTCP()
		return
	}
	l.serveUDP()
}

func (l *endpointListener) serveUDP() {
	buffer := make([]byte, 64*1024)
	for {
		n, address, err := l.packet.ReadFrom(buffer)
		if err != nil {
			return
		}
		message := strings.TrimRight(string(buffer[:n]), "\r\n")
		if message != "" {
			l.routeMessage(remoteIP(address), message)
		}
	}
}

func (l *endpointListener) serveTCP() {
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

func (l *endpointListener) serveConn(conn net.Conn) {
	defer func() {
		l.untrack(conn)
		_ = conn.Close()
	}()

	clientIP := remoteIP(conn.RemoteAddr())
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		message := strings.TrimRight(scanner.Text(), "\r\n")
		if message != "" {
			l.routeMessage(clientIP, message)
		}
	}
}

func (l *endpointListener) routeMessage(clientIP net.IP, message string) {
	table := l.routes.Load().(routeTable)
	source, ok := table.Match(clientIP)
	if !ok {
		l.logUnmatched(clientIP)
		return
	}
	if err := appendMessage(source.SpoolDir, message); err != nil {
		l.manager.recordWriteError(source.SourceID, err)
		return
	}
	l.manager.recordMessage(source.SourceID, clientIP)
}

func (l *endpointListener) logUnmatched(clientIP net.IP) {
	now := time.Now().Unix()
	previous := l.lastDrop.Load()
	if now-previous < 60 || !l.lastDrop.CompareAndSwap(previous, now) {
		return
	}
	log.Printf("RSyslog 端点 %s 拒绝未匹配客户端 %s", endpointAddress(l.key), clientIP)
}

func (l *endpointListener) track(conn net.Conn) bool {
	l.connMu.Lock()
	defer l.connMu.Unlock()
	if l.closed {
		return false
	}
	l.conns[conn] = struct{}{}
	return true
}

func (l *endpointListener) untrack(conn net.Conn) {
	l.connMu.Lock()
	defer l.connMu.Unlock()
	delete(l.conns, conn)
}

func (l *endpointListener) Close() error {
	var closeErr error
	l.closeOnce.Do(func() {
		l.connMu.Lock()
		l.closed = true
		if l.packet != nil {
			closeErr = l.packet.Close()
		}
		if l.listener != nil {
			closeErr = l.listener.Close()
		}
		for conn := range l.conns {
			_ = conn.Close()
			delete(l.conns, conn)
		}
		l.connMu.Unlock()
	})
	return closeErr
}

func (m *Manager) recordApplyError(sources []model.LogSource, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, source := range sources {
		status := statusForSource(source)
		status.Error = err.Error()
		m.statuses[source.SourceID] = status
	}
}

func (m *Manager) recordMessage(sourceID string, clientIP net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, exists := m.statuses[sourceID]
	if !exists {
		return
	}
	status.LastClientIP = clientIP.String()
	status.LastReceivedAt = time.Now()
	status.ReceivedMessages++
	status.Error = ""
	m.statuses[sourceID] = status
}

func (m *Manager) recordWriteError(sourceID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, exists := m.statuses[sourceID]
	if !exists {
		return
	}
	status.Error = err.Error()
	m.statuses[sourceID] = status
}

func statusForSource(source model.LogSource) Status {
	key := endpointKey{
		Protocol: strings.ToLower(strings.TrimSpace(source.ListenProtocol)),
		Host:     strings.TrimSpace(source.ListenHost),
		Port:     source.ListenPort,
	}
	return Status{
		SourceID: source.SourceID,
		Protocol: key.Protocol,
		Address:  endpointAddress(key),
		Port:     source.ListenPort,
		SpoolDir: source.SpoolDir,
		ClientIP: strings.TrimSpace(source.ClientIP),
	}
}

func remoteIP(address net.Addr) net.IP {
	if address == nil {
		return nil
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return nil
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func endpointAddress(key endpointKey) string {
	return net.JoinHostPort(key.Host, intToString(key.Port))
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
