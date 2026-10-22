package importer

import (
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var defaultIPv4Addr = netip.MustParseAddr("0.0.0.0")

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
	// Later duplicates win and keys are case-insensitive, matching a lowercased key map.
	var srcIP, srcPort, transIP, natIP, transPort, natPort, dstIP, dstPort, protocol, action, dateValue, timeValue string
	for rest := line; ; {
		var token string
		token, rest = nextUnicodeField(rest)
		if token == "" {
			break
		}
		key, value, ok := strings.Cut(token, "=")
		if !ok || key == "" {
			continue
		}
		value = strings.Trim(value, "\"'")
		switch strings.ToLower(key) {
		case "srcip":
			srcIP = value
		case "srcport":
			srcPort = value
		case "transip":
			transIP = value
		case "natip":
			natIP = value
		case "transport":
			transPort = value
		case "natport":
			natPort = value
		case "dstip":
			dstIP = value
		case "dstport":
			dstPort = value
		case "protocol":
			protocol = value
		case "action":
			action = value
		case "date":
			dateValue = value
		case "time":
			timeValue = value
		}
	}
	row.SrcIP = parseIPOrDefault(srcIP, row.SrcIP)
	row.SrcPort = parseUint16(srcPort)
	row.NATIP = parseIPOrDefault(firstNonEmpty(transIP, natIP), row.NATIP)
	row.NATPort = parseUint16(firstNonEmpty(transPort, natPort))
	row.DstIP = parseIPOrDefault(dstIP, row.DstIP)
	row.DstPort = parseUint16(dstPort)
	row.Protocol = normalizeProtocol(protocol)
	if action != "" {
		row.Action = action
	}
	if dateValue != "" && timeValue != "" {
		if ts, err := time.ParseInLocation("2006-01-02 15:04:05", dateValue+" "+timeValue, time.Local); err == nil {
			row.Timestamp = ts
		}
	}
	return true
}

// nextUnicodeField returns the next field as split by strings.Fields without allocating a slice.
func nextUnicodeField(s string) (field, rest string) {
	start := -1
	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		if unicode.IsSpace(r) {
			if start >= 0 {
				return s[start:i], s[i:]
			}
		} else if start < 0 {
			start = i
		}
		i += size
	}
	if start < 0 {
		return "", ""
	}
	return s[start:], ""
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

// parseNATTimestamp recognizes, in order:
//
//	^\d{4}\s+[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}
//	^(?:<\d+>)?%?[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\s+\d{4}
//	^(?:<\d+>)?%?[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}
//
// Matching is hand-written because regexp submatching dominated per-line parse cost.
func parseNATTimestamp(line string, fallbackDate time.Time) (time.Time, bool) {
	if digitsAt(line, 0, 4) {
		if start := skipRegexpSpace(line, 4); start > 4 {
			if end := matchClock(line, start); end >= 0 {
				ts, err := time.ParseInLocation("2006 Jan 2 15:04:05", line[:end], time.Local)
				if err == nil {
					return ts, true
				}
			}
		}
	}

	start := syslogClockStart(line)
	end := -1
	if start >= 0 {
		end = matchClock(line, start)
	}
	if end >= 0 {
		clock := line[start:end]
		if yearStart := skipRegexpSpace(line, end); yearStart > end && digitsAt(line, yearStart, 4) {
			ts, err := time.ParseInLocation("Jan 2 15:04:05 2006", clock+" "+line[yearStart:yearStart+4], time.Local)
			if err == nil {
				return ts, true
			}
		}
		ts, err := parseSyslogTimestamp(clock, fallbackDate)
		if err == nil {
			return ts, true
		}
	}

	return fallbackDate, !fallbackDate.IsZero()
}

// syslogClockStart consumes the optional `(?:<\d+>)?%?` prefix.
func syslogClockStart(line string) int {
	i := 0
	if i < len(line) && line[i] == '<' {
		j := i + 1
		for j < len(line) && isASCIIDigit(line[j]) {
			j++
		}
		if j == i+1 || j >= len(line) || line[j] != '>' {
			return -1
		}
		i = j + 1
	}
	if i < len(line) && line[i] == '%' {
		i++
	}
	return i
}

// matchClock matches `[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}` at line[i:] and returns its end, or -1.
func matchClock(line string, i int) int {
	if i+3 > len(line) || !isASCIILetter(line[i]) || !isASCIILetter(line[i+1]) || !isASCIILetter(line[i+2]) {
		return -1
	}
	i += 3
	next := skipRegexpSpace(line, i)
	if next == i {
		return -1
	}
	i = next
	digits := 0
	for digits < 2 && i < len(line) && isASCIIDigit(line[i]) {
		i++
		digits++
	}
	if digits == 0 {
		return -1
	}
	next = skipRegexpSpace(line, i)
	if next == i {
		return -1
	}
	i = next
	if !digitsAt(line, i, 2) || i+8 > len(line) || line[i+2] != ':' || !digitsAt(line, i+3, 2) || line[i+5] != ':' || !digitsAt(line, i+6, 2) {
		return -1
	}
	return i + 8
}

// skipRegexpSpace skips RE2 `\s` characters ([\t\n\f\r ]).
func skipRegexpSpace(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case '\t', '\n', '\f', '\r', ' ':
			i++
		default:
			return i
		}
	}
	return i
}

func digitsAt(s string, i, n int) bool {
	if i < 0 || i+n > len(s) {
		return false
	}
	for _, c := range []byte(s[i : i+n]) {
		if !isASCIIDigit(c) {
			return false
		}
	}
	return true
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func parseSyslogTimestamp(value string, fallbackDate time.Time) (time.Time, error) {
	baseYear := fallbackDate.Year()
	if fallbackDate.IsZero() {
		baseYear = time.Now().In(time.Local).Year()
	}

	var buffer [64]byte
	stamp := appendZeroPaddedYear(buffer[:0], baseYear)
	stamp = append(stamp, ' ')
	stamp = append(stamp, value...)
	ts, err := time.ParseInLocation("2006 Jan 2 15:04:05", string(stamp), time.Local)
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

// appendZeroPaddedYear formats like fmt's %04d.
func appendZeroPaddedYear(dst []byte, year int) []byte {
	if year < 0 {
		dst = append(dst, '-')
		year = -year
		if year < 100 {
			dst = append(dst, '0')
		}
		if year < 10 {
			dst = append(dst, '0')
		}
		return strconv.AppendInt(dst, int64(year), 10)
	}
	for limit := 1000; limit > 1 && year < limit; limit /= 10 {
		dst = append(dst, '0')
	}
	return strconv.AppendInt(dst, int64(year), 10)
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
