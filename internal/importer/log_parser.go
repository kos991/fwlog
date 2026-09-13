package importer

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	defaultIPv4Addr    = netip.MustParseAddr("0.0.0.0")
	natLineTimePattern = regexp.MustCompile(`^(\d{4}\s+[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)
	syslogTimePattern  = regexp.MustCompile(`^(?:<\d+>)?%?([A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)
	syslogYearPattern  = regexp.MustCompile(`^(?:<\d+>)?%?([A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+(\d{4})`)
)

const (
	natLabelSrcIP    = "源IP:"
	natLabelSrcPort  = "源端�?"
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

	if parseHuaweiUSGLine(line, &row) || parseH3CNATLine(line, &row) || parseFortiGateLine(line, &row) {
		return row, true
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
	if srcIP, ok := parseIP(srcIPRaw); ok {
		row.SrcIP = srcIP
	}
	if dstIP, ok := parseIP(dstIPRaw); ok {
		row.DstIP = dstIP
	}
	if natIP, ok := parseIP(natIPRaw); ok {
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

func parseHuaweiUSGLine(line string, row *NATLogRow) bool {
	if !strings.Contains(line, "NAT_LOG") {
		return false
	}
	if ts, ok := parseNATTimestamp(line, row.LogDate); ok {
		row.Timestamp = ts
	}
	row.SrcIP = parseIPOrDefault(extractDelimitedField(line, "SrcIP:"), row.SrcIP)
	row.SrcPort = parseUint16(extractDelimitedField(line, "SrcPort:"))
	row.NATIP = parseIPOrDefault(extractDelimitedField(line, "NatSrcIP:"), row.NATIP)
	row.NATPort = parseUint16(extractDelimitedField(line, "NatSrcPort:"))
	row.DstIP = parseIPOrDefault(extractDelimitedField(line, "DstIP:"), row.DstIP)
	row.DstPort = parseUint16(extractDelimitedField(line, "DstPort:"))
	row.Protocol = normalizeProtocol(extractDelimitedField(line, "Protocol:"))
	if action := extractDelimitedField(line, "Action:"); action != "" {
		row.Action = action
	}
	return row.SrcIP != defaultIPv4Addr || row.DstIP != defaultIPv4Addr || row.NATIP != defaultIPv4Addr || row.Protocol != ""
}

func parseH3CNATLine(line string, row *NATLogRow) bool {
	if !strings.Contains(line, "NAT_STREAM") {
		return false
	}
	if ts, ok := parseNATTimestamp(line, row.LogDate); ok {
		row.Timestamp = ts
	}
	row.Protocol = normalizeProtocol(extractParenField(line, "Protocol"))
	row.SrcIP, row.SrcPort = parseEndpoint(extractParenField(line, "SrcIP"), row.SrcIP, row.SrcPort)
	row.NATIP, row.NATPort = parseEndpoint(extractParenField(line, "NatSrcIP"), row.NATIP, row.NATPort)
	row.DstIP, row.DstPort = parseEndpoint(extractParenField(line, "DstIP"), row.DstIP, row.DstPort)
	if action := extractParenField(line, "MsgType"); action != "" {
		row.Action = action
	}
	return row.Protocol != "" || row.SrcIP != defaultIPv4Addr || row.DstIP != defaultIPv4Addr
}

func parseFortiGateLine(line string, row *NATLogRow) bool {
	if !strings.Contains(line, "srcip=") || !strings.Contains(line, "dstip=") {
		return false
	}
	fields := make(map[string]string)
	for _, token := range strings.Fields(line) {
		key, value, ok := strings.Cut(token, "=")
		if ok && key != "" {
			fields[strings.ToLower(key)] = strings.Trim(value, "\"'")
		}
	}
	row.SrcIP = parseIPOrDefault(fields["srcip"], row.SrcIP)
	row.SrcPort = parseUint16(fields["srcport"])
	row.NATIP = parseIPOrDefault(firstNonEmpty(fields["transip"], fields["natip"]), row.NATIP)
	row.NATPort = parseUint16(firstNonEmpty(fields["transport"], fields["natport"]))
	row.DstIP = parseIPOrDefault(fields["dstip"], row.DstIP)
	row.DstPort = parseUint16(fields["dstport"])
	row.Protocol = normalizeProtocol(fields["protocol"])
	if action := fields["action"]; action != "" {
		row.Action = action
	}
	if dateValue, timeValue := fields["date"], fields["time"]; dateValue != "" && timeValue != "" {
		if ts, err := time.ParseInLocation("2006-01-02 15:04:05", dateValue+" "+timeValue, time.Local); err == nil {
			row.Timestamp = ts
		}
	}
	return true
}

func extractDelimitedField(line, label string) string {
	_, after, found := strings.Cut(line, label)
	if !found {
		return ""
	}
	value, _, _ := strings.Cut(after, ";")
	return strings.TrimSpace(value)
}

func extractParenField(line, name string) string {
	start := strings.Index(line, name+"(")
	if start < 0 {
		return ""
	}
	start += len(name) + 1
	end := strings.IndexByte(line[start:], ')')
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(line[start : start+end])
}

func parseEndpoint(raw string, fallbackIP netip.Addr, fallbackPort uint16) (netip.Addr, uint16) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallbackIP, fallbackPort
	}
	if endpoint, err := netip.ParseAddrPort(raw); err == nil {
		return endpoint.Addr().Unmap(), endpoint.Port()
	}
	separator := strings.LastIndexByte(raw, ':')
	if separator <= 0 {
		if ip, ok := parseIP(raw); ok {
			return ip, fallbackPort
		}
		return fallbackIP, fallbackPort
	}
	ip, ok := parseIP(raw[:separator])
	if !ok {
		return fallbackIP, fallbackPort
	}
	return ip, parseUint16(raw[separator+1:])
}

func parseIPOrDefault(raw string, fallback netip.Addr) netip.Addr {
	if ip, ok := parseIP(strings.TrimSpace(raw)); ok {
		return ip
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseNATTimestamp(line string, fallbackDate time.Time) (time.Time, bool) {
	if matches := natLineTimePattern.FindStringSubmatch(line); len(matches) == 2 {
		ts, err := time.ParseInLocation("2006 Jan 2 15:04:05", matches[1], time.Local)
		if err == nil {
			return ts, true
		}
	}

	if matches := syslogYearPattern.FindStringSubmatch(line); len(matches) == 3 {
		ts, err := time.ParseInLocation("Jan 2 15:04:05 2006", matches[1]+" "+matches[2], time.Local)
		if err == nil {
			return ts, true
		}
	}

	if matches := syslogTimePattern.FindStringSubmatch(line); len(matches) == 2 {
		ts, err := parseSyslogTimestamp(matches[1], fallbackDate)
		if err == nil {
			return ts, true
		}
	}

	return fallbackDate, !fallbackDate.IsZero()
}

func parseSyslogTimestamp(value string, fallbackDate time.Time) (time.Time, error) {
	baseYear := fallbackDate.Year()
	if fallbackDate.IsZero() {
		baseYear = time.Now().In(time.Local).Year()
	}

	ts, err := time.ParseInLocation("2006 Jan 2 15:04:05", fmt.Sprintf("%04d %s", baseYear, value), time.Local)
	if err != nil {
		return time.Time{}, err
	}
	if fallbackDate.IsZero() {
		return ts, nil
	}

	if ts.Sub(fallbackDate) > 183*24*time.Hour {
		return ts.AddDate(-1, 0, 0), nil
	}
	if fallbackDate.Sub(ts) > 183*24*time.Hour {
		return ts.AddDate(1, 0, 0), nil
	}
	return ts, nil
}

func parseIP(raw string) (netip.Addr, bool) {
	if raw == "" {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}

	return addr.Unmap(), true
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
