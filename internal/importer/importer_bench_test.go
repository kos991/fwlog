package importer

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ClickHouse/clickhouse-go/v2/lib/proto"
)

// blockBatch appends into a real driver block so benchmarks include column conversion cost.
type blockBatch struct{ block *proto.Block }

func newBlockBatch(b *testing.B) *blockBatch {
	block := &proto.Block{}
	for _, c := range [][2]string{{"source_id", "String"}, {"log_tag", "LowCardinality(String)"}, {"log_date", "Date"}, {"timestamp", "DateTime"}, {"src_ip", "IPv6"}, {"src_port", "UInt16"}, {"dst_ip", "IPv6"}, {"dst_port", "UInt16"}, {"nat_ip", "IPv6"}, {"nat_port", "UInt16"}, {"protocol", "LowCardinality(String)"}, {"action", "LowCardinality(String)"}, {"source_file", "LowCardinality(String)"}, {"source_offset", "UInt64"}, {"batch_id", "String"}} {
		if err := block.AddColumn(c[0], column.Type(c[1])); err != nil {
			b.Fatal(err)
		}
	}
	return &blockBatch{block: block}
}

func (f *blockBatch) Abort() error                  { return nil }
func (f *blockBatch) Append(v ...any) error         { return f.block.Append(v...) }
func (f *blockBatch) AppendStruct(any) error        { return nil }
func (f *blockBatch) Column(int) driver.BatchColumn { return nil }
func (f *blockBatch) Flush() error                  { return nil }
func (f *blockBatch) Send() error                   { return nil }
func (f *blockBatch) IsSent() bool                  { return false }
func (f *blockBatch) Rows() int                     { return f.block.Rows() }
func (f *blockBatch) Columns() []column.Interface   { return f.block.Columns }

type blockWriter struct{ b *testing.B }

func (w blockWriter) PrepareBatch(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
	return newBlockBatch(w.b), nil
}
func (w blockWriter) Exec(context.Context, string, ...any) error { return nil }

func BenchmarkAppendBatchBlockEncoding(b *testing.B) {
	const n = 20000
	row := NATLogRow{SourceID: "src_1", LogTag: "tag", LogDate: time.Now(), Timestamp: time.Now(), SrcIP: netip.MustParseAddr("10.0.0.1"), SrcPort: 12345, DstIP: netip.MustParseAddr("8.8.8.8"), DstPort: 443, NATIP: netip.MustParseAddr("1.1.1.1"), NATPort: 50000, Protocol: "TCP", Action: "ALLOW", SourceFile: "a.log", BatchID: "src_1-20260101"}
	rows := make([]NATLogRow, n)
	for i := range rows {
		rows[i] = row
		rows[i].SourceOffset = uint64(i)
	}
	importer := &Importer{writer: blockWriter{b}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := importer.AppendBatch(context.Background(), rows); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/row")
}

func BenchmarkImportFilePlainText(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "edge.log-20260628")
	var sb strings.Builder
	for i := 0; i < 100000; i++ {
		fmt.Fprintf(&sb, "2026 Jun 28 01:17:18 源IP:192.168.1.%d 源端口:%d 目的IP:8.8.8.8 目的端口:443 协议:6 转换后的IP:10.0.0.1 转换后的端口:50000 动作:允许\n", i%250, 1024+i%60000)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	importer := &Importer{writer: blockWriter{b}}
	source := LogSource{SourceID: "src_1", LogTag: "tag"}
	date := time.Date(2026, 6, 28, 0, 0, 0, 0, time.Local)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := importer.importFile(context.Background(), source, date, LogFileSnapshot{Path: path}); err != nil {
			b.Fatal(err)
		}
	}
}
