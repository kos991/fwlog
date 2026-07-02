package main

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	defaultIPv4Addr     = netip.MustParseAddr("0.0.0.0")
	natLineTimePattern  = regexp.MustCompile(`^(\d{4}\s+[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)
	natFieldPatterns    = []struct {
		key   string
		regex *regexp.Regexp
	}{
		{key: "src_ip", regex: regexp.MustCompile(`源IP:\s*([0-9.]+)`)},
		{key: "src_port", regex: regexp.MustCompile(`源端口:\s*(\d+)`)},
		{key: "dst_ip", regex: regexp.MustCompile(`目的IP:\s*([0-9.]+)`)},
		{key: "dst_port", regex: regexp.MustCompile(`目的端口:\s*(\d+)`)},
		{key: "protocol", regex: regexp.MustCompile(`协议:\s*([^\s]+)`)},
		{key: "nat_ip", regex: regexp.MustCompile(`转换后的IP:\s*([0-9.]+)`)},
		{key: "nat_port", regex: regexp.MustCompile(`转换后的端口:\s*(\d+)`)},
		{key: "action", regex: regexp.MustCompile(`动作:\s*([^\s]+)`)},
	}
)

type ParseMeta struct {
	SourceID     string
	LogTag       string
	LogDate      time.Time
	SourceFile   string
	SourceOffset uint64
	BatchID      string
}

type NATLogRow struct {
	SourceID     string
	LogTag       string
	LogDate      time.Time
	Timestamp    time.Time
	SrcIP        netip.Addr
	SrcPort      uint16
	DstIP        netip.Addr
	DstPort      uint16
	NATIP        netip.Addr
	NATPort      uint16
	Protocol     string
	Action       string
	SourceFile   string
	SourceOffset uint64
	BatchID      string
}

func ParseNATLine(line string, meta ParseMeta) (NATLogRow, bool) {
	row := NATLogRow{
		SourceID:     meta.SourceID,
		LogTag:       meta.LogTag,
		LogDate:      meta.LogDate,
		Timestamp:    meta.LogDate,
		SrcIP:        defaultIPv4Addr,
		DstIP:        defaultIPv4Addr,
		NATIP:        defaultIPv4Addr,
		Protocol:     "",
		Action:       "ALLOW",
		SourceFile:   meta.SourceFile,
		SourceOffset: meta.SourceOffset,
		BatchID:      meta.BatchID,
	}

	fields := make(map[string]string, len(natFieldPatterns))
	matchedField := false
	for _, pattern := range natFieldPatterns {
		matches := pattern.regex.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}
		fields[pattern.key] = strings.TrimSpace(matches[1])
		matchedField = true
	}
	if !matchedField {
		return NATLogRow{}, false
	}

	if ts, ok := parseNATTimestamp(line, meta.LogDate); ok {
		row.Timestamp = ts
	}
	if srcIP, ok := parseIPv4(fields["src_ip"]); ok {
		row.SrcIP = srcIP
	}
	if dstIP, ok := parseIPv4(fields["dst_ip"]); ok {
		row.DstIP = dstIP
	}
	if natIP, ok := parseIPv4(fields["nat_ip"]); ok {
		row.NATIP = natIP
	}

	row.SrcPort = parseUint16(fields["src_port"])
	row.DstPort = parseUint16(fields["dst_port"])
	row.NATPort = parseUint16(fields["nat_port"])
	row.Protocol = fields["protocol"]
	if action := fields["action"]; action != "" {
		row.Action = action
	}

	return row, true
}

func parseNATTimestamp(line string, fallbackDate time.Time) (time.Time, bool) {
	matches := natLineTimePattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return fallbackDate, !fallbackDate.IsZero()
	}

	ts, err := time.ParseInLocation("2006 Jan 2 15:04:05", matches[1], time.Local)
	if err != nil {
		return fallbackDate, !fallbackDate.IsZero()
	}

	return ts, true
}

func parseIPv4(raw string) (netip.Addr, bool) {
	if raw == "" {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil || !addr.Is4() {
		return netip.Addr{}, false
	}

	return addr, true
}

func parseUint16(raw string) uint16 {
	if raw == "" {
		return 0
	}

	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0
	}

	return uint16(value)
}
