package app

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	defaultIPv4Addr    = netip.MustParseAddr("0.0.0.0")
	natLineTimePattern = regexp.MustCompile(`^(\d{4}\s+[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)
)

const (
	natLabelSrcIP    = "源IP:"
	natLabelSrcPort  = "源端口:"
	natLabelDstIP    = "目的IP:"
	natLabelDstPort  = "目的端口:"
	natLabelProtocol = "协议:"
	natLabelNATIP    = "转换后的IP:"
	natLabelNATPort  = "转换后的端口:"
	natLabelAction   = "动作:"
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

	srcIPRaw := extractField(line, natLabelSrcIP)
	srcPortRaw := extractField(line, natLabelSrcPort)
	dstIPRaw := extractField(line, natLabelDstIP)
	dstPortRaw := extractField(line, natLabelDstPort)
	protocolRaw := extractField(line, natLabelProtocol)
	natIPRaw := extractField(line, natLabelNATIP)
	natPortRaw := extractField(line, natLabelNATPort)
	actionRaw := extractField(line, natLabelAction)

	if srcIPRaw == "" && srcPortRaw == "" && dstIPRaw == "" && dstPortRaw == "" &&
		protocolRaw == "" && natIPRaw == "" && natPortRaw == "" && actionRaw == "" {
		return NATLogRow{}, false
	}

	if ts, ok := parseNATTimestamp(line, meta.LogDate); ok {
		row.Timestamp = ts
	}
	if srcIP, ok := parseIPv4(srcIPRaw); ok {
		row.SrcIP = srcIP
	}
	if dstIP, ok := parseIPv4(dstIPRaw); ok {
		row.DstIP = dstIP
	}
	if natIP, ok := parseIPv4(natIPRaw); ok {
		row.NATIP = natIP
	}

	row.SrcPort = parseUint16(srcPortRaw)
	row.DstPort = parseUint16(dstPortRaw)
	row.NATPort = parseUint16(natPortRaw)
	row.Protocol = normalizeProtocol(protocolRaw)
	if actionRaw != "" {
		row.Action = actionRaw
	}

	return row, true
}

func extractField(line, label string) string {
	_, after, found := strings.Cut(line, label)
	if !found {
		return ""
	}

	after = strings.TrimLeft(after, " \t")
	value, _, _ := strings.Cut(after, " ")
	return strings.Trim(value, ",;\r\n\t ")
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

func normalizeProtocol(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, ",;")
	switch strings.ToUpper(value) {
	case "6", "TCP":
		return "TCP"
	case "17", "UDP":
		return "UDP"
	case "1", "ICMP":
		return "ICMP"
	default:
		return strings.ToUpper(value)
	}
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
