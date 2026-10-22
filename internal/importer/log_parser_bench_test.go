package importer

import (
	"testing"
	"time"
)

func benchmarkParseLine(b *testing.B, line string) {
	b.Helper()
	meta := ParseMeta{
		SourceID:     "src_1",
		LogTag:       "nat_tag",
		LogDate:      time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local),
		SourceFile:   "sangfor.log-20260904",
		SourceOffset: 128,
		BatchID:      "src_1-20260904",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := ParseNATLine(line, meta); !ok {
			b.Fatal("ParseNATLine failed")
		}
	}
}

func BenchmarkParseNATLineHuawei(b *testing.B) {
	benchmarkParseLine(b, "<14>Sep  4 18:15:30 2026 USG6000 %%01NAT/4/NAT_LOG(l): Zone: trust->untrust; Protocol: TCP; SrcIP: 192.168.1.100; SrcPort: 54321; NatSrcIP: 203.0.113.50; NatSrcPort: 1024; DstIP: 8.8.8.8; DstPort: 443; Action: Create;")
}

func BenchmarkParseNATLineH3C(b *testing.B) {
	benchmarkParseLine(b, "%Sep  4 18:15:32 2026 H3C NAT/6/NAT_STREAM: Protocol(6), SrcIP(192.168.1.100:54321) -> NatSrcIP(203.0.113.50:1024) -> DstIP(8.8.8.8:443) MsgType(Create)")
}

func BenchmarkParseNATLineFortiGate(b *testing.B) {
	benchmarkParseLine(b, "date=2026-09-04 time=18:15:30 devname=FG-100D action=accept protocol=6 srcip=192.168.1.100 srcport=54321 transip=203.0.113.50 transport=1024 dstip=8.8.8.8 dstport=443")
}

func BenchmarkParseNATLineSyslogSangfor(b *testing.B) {
	benchmarkParseLine(b, "Jun 13 23:59:30 localhost nat: 日志类型:NAT日志, NAT类型:snat, 源IP:10.1.1.1, 源端口:37830, 目的IP:8.8.8.8, 目的端口:443, 协议:6, 转换后的IP:1.2.3.4, 转换后的端口:37830")
}
