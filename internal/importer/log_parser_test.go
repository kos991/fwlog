package importer

import (
	"net/netip"
	"testing"
	"time"
)

func TestParseNATLineWritesDefaultsAndMetadata(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 源端�?12345 目的IP:222.186.177.145 目的端口:443 协议:6 转换后的IP:10.0.0.1 转换后的端口:50000"

	row, ok := ParseNATLine(line, ParseMeta{
		SourceID:     "src_1",
		LogTag:       "nat_tag",
		LogDate:      time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local),
		SourceFile:   "/logs/sangfor.log-20260628",
		SourceOffset: 128,
		BatchID:      "batch_1",
	})
	if !ok {
		t.Fatal("line should parse")
	}

	if row.SourceID != "src_1" || row.LogTag != "nat_tag" {
		t.Fatalf("metadata missing: %#v", row)
	}
	if row.LogDate != time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local) {
		t.Fatalf("log date missing: %#v", row)
	}
	if row.SourceFile != "/logs/sangfor.log-20260628" || row.SourceOffset != 128 || row.BatchID != "batch_1" {
		t.Fatalf("source metadata missing: %#v", row)
	}
	if row.SrcIP != netip.MustParseAddr("192.168.1.10") || row.DstPort != 443 {
		t.Fatalf("unexpected parsed row: %#v", row)
	}
	if row.NATIP != netip.MustParseAddr("10.0.0.1") || row.NATPort != 50000 {
		t.Fatalf("nat fields missing: %#v", row)
	}
	if row.Protocol != "TCP" || row.Action != "ALLOW" {
		t.Fatalf("protocol/action should be filled: %#v", row)
	}
}

func TestParseNATLineParsesIPv6Addresses(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:2001:db8::10 源端口:12345 目的IP:2001:db8:1::20 目的端口:443 协议:6 转换后的IP:2001:db8:2::30 转换后的端口:50000"

	row, ok := ParseNATLine(line, ParseMeta{})
	if !ok {
		t.Fatal("IPv6 NAT line should parse")
	}

	if row.SrcIP != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("source IPv6 = %v", row.SrcIP)
	}
	if row.DstIP != netip.MustParseAddr("2001:db8:1::20") {
		t.Fatalf("destination IPv6 = %v", row.DstIP)
	}
	if row.NATIP != netip.MustParseAddr("2001:db8:2::30") {
		t.Fatalf("NAT IPv6 = %v", row.NATIP)
	}
}

func TestParseNATLineParsesSyslogTimestampWithArchiveYear(t *testing.T) {
	line := "Jun 13 23:59:30 localhost nat: 日志类型:NAT日志, NAT类型:snat, 源IP:2.55.80.250, 源端�?37830, 目的IP:47.96.193.154, 目的端口:443, 协议:6, 转换后的IP:58.216.48.6, 转换后的端口:37830"

	row, ok := ParseNATLine(line, ParseMeta{
		LogDate: time.Date(2026, 6, 14, 0, 0, 0, 0, time.Local),
	})
	if !ok {
		t.Fatal("syslog NAT line should parse")
	}

	want := time.Date(2026, 6, 13, 23, 59, 30, 0, time.Local)
	if !row.Timestamp.Equal(want) {
		t.Fatalf("timestamp = %v, want %v", row.Timestamp, want)
	}
	if row.Timestamp.Equal(row.LogDate) {
		t.Fatalf("timestamp should not fall back to archive date: %#v", row)
	}
}

func TestParseNATLineInfersNearestYearForSyslogTimestamp(t *testing.T) {
	line := "Dec 31 23:59:59 localhost nat: 日志类型:NAT日志, NAT类型:snat, 源IP:2.55.80.250, 目的IP:47.96.193.154"

	row, ok := ParseNATLine(line, ParseMeta{
		LogDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local),
	})
	if !ok {
		t.Fatal("syslog NAT line should parse")
	}

	want := time.Date(2026, 12, 31, 23, 59, 59, 0, time.Local)
	if !row.Timestamp.Equal(want) {
		t.Fatalf("timestamp = %v, want %v", row.Timestamp, want)
	}
}

func TestParseNATLineUsesDefaultValuesForMissingFields(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 目的IP:222.186.177.145"

	row, ok := ParseNATLine(line, ParseMeta{})
	if !ok {
		t.Fatal("line with core fields should parse")
	}

	if row.SrcPort != 0 || row.DstPort != 0 || row.NATPort != 0 {
		t.Fatalf("missing ports should default to zero: %#v", row)
	}
	if row.Protocol != "" || row.Action != "ALLOW" {
		t.Fatalf("protocol/action defaults wrong: %#v", row)
	}
	if row.NATIP != netip.MustParseAddr("0.0.0.0") {
		t.Fatalf("missing nat ip should default to 0.0.0.0: %#v", row)
	}
	if row.SourceID != "" || row.SourceFile != "" || row.BatchID != "" {
		t.Fatalf("empty metadata should stay empty string defaults: %#v", row)
	}
}

func TestParseNATLineDefaultsMissingSourceIP(t *testing.T) {
	line := "2026 Jun 28 01:17:18 目的IP:222.186.177.145 源端�?12345 目的端口:443"

	row, ok := ParseNATLine(line, ParseMeta{})
	if !ok {
		t.Fatal("line with missing source ip should still parse")
	}
	if row.SrcIP != netip.MustParseAddr("0.0.0.0") {
		t.Fatalf("missing source ip should default to 0.0.0.0: %#v", row)
	}
	if row.DstIP != netip.MustParseAddr("222.186.177.145") {
		t.Fatalf("destination ip should still parse: %#v", row)
	}
}

func TestParseNATLineDefaultsMissingDestinationIP(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 源端�?12345 协议:6"

	row, ok := ParseNATLine(line, ParseMeta{})
	if !ok {
		t.Fatal("line with missing destination ip should still parse")
	}
	if row.DstIP != netip.MustParseAddr("0.0.0.0") {
		t.Fatalf("missing destination ip should default to 0.0.0.0: %#v", row)
	}
	if row.SrcIP != netip.MustParseAddr("192.168.1.10") {
		t.Fatalf("source ip should still parse: %#v", row)
	}
}

func TestParseNATLineRejectsNonNatLine(t *testing.T) {
	if _, ok := ParseNATLine("plain text without nat fields", ParseMeta{}); ok {
		t.Fatal("non NAT line should not parse")
	}
}

func TestParseNATLineRejectsTimestampOnlyLine(t *testing.T) {
	line := "2026 Jun 28 01:17:18 no structured nat fields here"
	if _, ok := ParseNATLine(line, ParseMeta{}); ok {
		t.Fatal("timestamp-only non NAT line should not parse")
	}
}

func TestParseNATLineParsesActionAndTrimsFieldPunctuation(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 目的IP:222.186.177.145 动作:允许,"

	row, ok := ParseNATLine(line, ParseMeta{})
	if !ok {
		t.Fatal("line with action should parse")
	}
	if row.Action != "允许" {
		t.Fatalf("action should be trimmed, got %#v", row)
	}
}

func TestParseNATLineNormalizesNumericProtocols(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "6", want: "TCP"},
		{raw: "17", want: "UDP"},
		{raw: "1", want: "ICMP"},
	}

	for _, tt := range tests {
		line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 目的IP:222.186.177.145 协议:" + tt.raw
		row, ok := ParseNATLine(line, ParseMeta{})
		if !ok {
			t.Fatalf("line with protocol %s should parse", tt.raw)
		}
		if row.Protocol != tt.want {
			t.Fatalf("protocol %s normalized to %q, want %q", tt.raw, row.Protocol, tt.want)
		}
	}
}

func TestParseNATLineDefaultsInvalidPortsToZero(t *testing.T) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 源端�?bad 目的IP:222.186.177.145 目的端口:70000 转换后的端口:-1"

	row, ok := ParseNATLine(line, ParseMeta{})
	if !ok {
		t.Fatal("line with invalid ports should still parse")
	}
	if row.SrcPort != 0 || row.DstPort != 0 || row.NATPort != 0 {
		t.Fatalf("invalid ports should default to zero: %#v", row)
	}
}

func BenchmarkParseNATLine(b *testing.B) {
	line := "2026 Jun 28 01:17:18 源IP:192.168.1.10 源端�?12345 目的IP:222.186.177.145 目的端口:443 协议:6 转换后的IP:10.0.0.1 转换后的端口:50000 动作:允许"
	meta := ParseMeta{
		SourceID:     "src_1",
		LogTag:       "nat_tag",
		LogDate:      time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local),
		SourceFile:   "/logs/sangfor.log-20260628",
		SourceOffset: 128,
		BatchID:      "batch_1",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := ParseNATLine(line, meta); !ok {
			b.Fatal("ParseNATLine failed")
		}
	}
}
