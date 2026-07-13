package server

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var logSourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateLogSources(sources []LogSource) error {
	ids := make(map[string]struct{}, len(sources))
	routes := make(map[string]map[string]string)

	for _, source := range sources {
		if !logSourceIDPattern.MatchString(source.SourceID) {
			return fmt.Errorf("设备 ID %q 只允许字母、数字、点、下划线和连字符", source.SourceID)
		}
		if _, exists := ids[source.SourceID]; exists {
			return fmt.Errorf("设备 ID %q 重复", source.SourceID)
		}
		ids[source.SourceID] = struct{}{}

		if source.SourceType != "rsyslog" {
			continue
		}
		if source.ListenProtocol != "udp" && source.ListenProtocol != "tcp" {
			return fmt.Errorf("接收协议 %q 无效", source.ListenProtocol)
		}
		if source.ListenPort < 1 || source.ListenPort > 65535 {
			return fmt.Errorf("监听端口必须为 1-65535")
		}
		routeKey, err := normalizeClientRouteKey(source.ClientIP)
		if err != nil {
			return err
		}
		if !isAbsoluteLogPath(source.SpoolDir) {
			return fmt.Errorf("落盘目录 %q 必须为绝对路径", source.SpoolDir)
		}
		if source.ArchiveDir != "" && !isAbsoluteLogPath(source.ArchiveDir) {
			return fmt.Errorf("归档目录 %q 必须为绝对路径", source.ArchiveDir)
		}
		if source.ArchiveRetentionDays < 0 || source.ArchiveRetentionDays > 3650 {
			return fmt.Errorf("归档保留天数必须为 0-3650")
		}

		endpoint := strings.Join([]string{source.ListenProtocol, source.ListenHost, strconv.Itoa(source.ListenPort)}, "|")
		if routes[endpoint] == nil {
			routes[endpoint] = make(map[string]string)
		}
		if previous, exists := routes[endpoint][routeKey]; exists {
			return fmt.Errorf("日志源 %q 和 %q 在同一监听端点使用了重复的客户端 IP %q", previous, source.SourceID, source.ClientIP)
		}
		routes[endpoint][routeKey] = source.SourceID
	}
	return nil
}

func normalizeClientRouteKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "*", nil
	}
	if ip := net.ParseIP(value); ip != nil {
		bits := 128
		if ip.To4() != nil {
			ip = ip.To4()
			bits = 32
		}
		return fmt.Sprintf("%s/%d", ip.String(), bits), nil
	}
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		return "", fmt.Errorf("客户端 IP/网段 %q 无效", value)
	}
	return network.String(), nil
}

func isAbsoluteLogPath(path string) bool {
	path = strings.TrimSpace(path)
	return path != "" && (filepath.IsAbs(path) || strings.HasPrefix(filepath.ToSlash(path), "/"))
}
