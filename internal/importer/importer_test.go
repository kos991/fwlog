package importer

import (
	"compress/gzip"
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestImportDateMarksEmptyArchiveAsNoData(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260715.gz")

	handle, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create empty gzip: %v", err)
	}
	zipWriter := gzip.NewWriter(handle)
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close empty gzip: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	modTime := logDate.Add(12 * time.Hour)
	if err := os.Chtimes(logFile, modTime, modTime); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	states := &fakeImportStateRecorder{}
	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: &fakeClickHouseWriter{batch: &fakeBatch{}},
		states: states,
		now:    func() time.Time { return logDate.Add(24 * time.Hour) },
		sleep:  func(time.Duration) {},
	}

	err = importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate)
	if err != nil {
		t.Fatalf("ImportDate returned error: %v", err)
	}
	lastDate := states.dateStates[len(states.dateStates)-1]
	if lastDate.Status != StatusNoData || lastDate.RowsImported != 0 || lastDate.ProgressPct != 100 {
		t.Fatalf("empty date terminal state = %#v", lastDate)
	}
	lastFile := states.fileStates[len(states.fileStates)-1]
	if lastFile.Status != StatusNoData || lastFile.RowsImported != 0 || lastFile.ProgressPct != 100 {
		t.Fatalf("empty file terminal state = %#v", lastFile)
	}
}

func TestImportDateFailsWhenNonEmptyArchiveHasNoRecognizedLogs(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260715")
	if err := os.WriteFile(logFile, []byte("this is not a supported NAT log\n"), 0o644); err != nil {
		t.Fatalf("write unsupported log: %v", err)
	}
	modTime := logDate.Add(12 * time.Hour)
	if err := os.Chtimes(logFile, modTime, modTime); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	states := &fakeImportStateRecorder{}
	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: &fakeClickHouseWriter{batch: &fakeBatch{}},
		states: states,
		now:    func() time.Time { return logDate.Add(24 * time.Hour) },
		sleep:  func(time.Duration) {},
	}

	err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate)
	if err == nil || !strings.Contains(err.Error(), "没有可识别的日志") {
		t.Fatalf("ImportDate error = %v, want unrecognized log error", err)
	}
	if got := states.fileStates[len(states.fileStates)-1].Status; got != StatusFailed {
		t.Fatalf("file status = %s, want failed", got)
	}
	if got := states.dateStates[len(states.dateStates)-1].Status; got != StatusFailed {
		t.Fatalf("date status = %s, want failed", got)
	}
}

func TestImportDateSkipsUnrecognizedArchiveWhenDateAlsoHasNATLogs(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 16, 0, 0, 0, 0, time.Local)
	natPath := filepath.Join(dir, "a-nat.log-20260716")
	apPath := filepath.Join(dir, "z-ap.log-20260716")
	natLine := "2026 Jul 16 12:00:01 " + natLabelSrcIP + " 10.0.0.1 " + natLabelDstIP + " 10.0.0.2 " + natLabelProtocol + " TCP"
	apLine := "Jul 16 19:24:56 AP:e05f.b9ea.78be: %LINK-3-UPDOWN: Interface Dot11Radio0, changed state to down"
	for path, content := range map[string]string{natPath: natLine, apPath: apLine} {
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatalf("write archive: %v", err)
		}
		if err := os.Chtimes(path, logDate.Add(12*time.Hour), logDate.Add(12*time.Hour)); err != nil {
			t.Fatalf("set mod time: %v", err)
		}
	}

	states := &fakeImportStateRecorder{}
	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: &fakeClickHouseWriter{batch: &fakeBatch{}},
		states: states,
		now:    func() time.Time { return logDate.Add(24 * time.Hour) },
		sleep:  func(time.Duration) {},
	}

	if err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate); err != nil {
		t.Fatalf("mixed archive import returned error: %v", err)
	}
	lastDate := states.dateStates[len(states.dateStates)-1]
	if lastDate.Status != StatusReady || lastDate.RowsImported != 1 || lastDate.FilesDone != 2 {
		t.Fatalf("mixed archive date state = %#v", lastDate)
	}
	var apState FileIngestState
	for _, state := range states.fileStates {
		if state.Path == apPath {
			apState = state
		}
	}
	if apState.Status != StatusNoData || apState.ProgressPct != 100 {
		t.Fatalf("AP archive state = %#v, want no_data", apState)
	}
}

func TestImportDateSuccessDropsSourceDatePartitionSleepsAndMarksReady(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260702")
	line := "2026 Jul 2 12:00:01 源IP: 10.0.0.1 源端�? 1234 目的IP: 10.0.0.2 目的端口: 80 协议: TCP 转换后的IP: 10.0.0.3 转换后的端口: 8080 动作: ALLOW"
	if err := os.WriteFile(logFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	modTime := logDate.Add(12 * time.Hour)
	if err := os.Chtimes(logFile, modTime, modTime); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	writer := &fakeClickHouseWriter{batch: &fakeBatch{}}
	states := &fakeImportStateRecorder{}
	var slept []time.Duration
	now := logDate.Add(24 * time.Hour)
	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: writer,
		states: states,
		now: func() time.Time {
			return now
		},
		sleep: func(d time.Duration) {
			slept = append(slept, d)
		},
		batchSize: 1,
	}

	source := LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}
	if err := importer.ImportDate(context.Background(), source, logDate); err != nil {
		t.Fatalf("ImportDate returned error: %v", err)
	}

	if len(writer.execCalls) != 3 {
		t.Fatalf("Exec calls = %d, want 3", len(writer.execCalls))
	}
	wantDrops := []string{
		"ALTER TABLE nat_logs DROP PARTITION ('fw-a', '2026-07-02')",
		"ALTER TABLE dashboard_daily_totals DROP PARTITION ('fw-a', '2026-07-02')",
		"ALTER TABLE dashboard_daily_ip_counts DROP PARTITION ('fw-a', '2026-07-02')",
	}
	for index, want := range wantDrops {
		if got := writer.execCalls[index].query; got != want {
			t.Fatalf("drop partition query %d = %q, want %q", index, got, want)
		}
	}
	if got := writer.execCalls[0].args; !reflect.DeepEqual(got, []any(nil)) {
		t.Fatalf("drop partition args = %#v", got)
	}
	if !reflect.DeepEqual(slept, []time.Duration{time.Second}) {
		t.Fatalf("sleep calls = %#v", slept)
	}
	if len(writer.prepareCalls) != 1 {
		t.Fatalf("PrepareBatch calls = %d, want 1", len(writer.prepareCalls))
	}
	wantInsert := `INSERT INTO nat_logs (
    source_id, log_tag, log_date, timestamp, src_ip, src_port, dst_ip, dst_port,
    nat_ip, nat_port, protocol, action, source_file, source_offset, batch_id
)`
	if writer.prepareCalls[0] != wantInsert {
		t.Fatalf("PrepareBatch query = %q", writer.prepareCalls[0])
	}
	if len(writer.batch.appended) != 1 {
		t.Fatalf("append rows = %d, want 1", len(writer.batch.appended))
	}
	if !writer.batch.sent {
		t.Fatal("batch Send was not called")
	}
	if len(states.dateStates) != 4 {
		t.Fatalf("date states = %d, want 4", len(states.dateStates))
	}
	if states.dateStates[0].Status != StatusImporting {
		t.Fatalf("first date status = %s", states.dateStates[0].Status)
	}
	if states.dateStates[1].Status != StatusImporting || states.dateStates[1].CurrentFile != "edge-fw.log-20260702" || states.dateStates[1].FilesDone != 0 {
		t.Fatalf("second date current file state = %#v", states.dateStates[1])
	}
	if states.dateStates[2].Status != StatusImporting || states.dateStates[2].FilesDone != 1 || states.dateStates[2].ProgressPct != 100 {
		t.Fatalf("third date progress state = %#v", states.dateStates[2])
	}
	if states.dateStates[3].Status != StatusReady {
		t.Fatalf("fourth date status = %s", states.dateStates[3].Status)
	}
	if len(states.fileStates) != 2 {
		t.Fatalf("file states = %d, want 2", len(states.fileStates))
	}
	if states.fileStates[0].Status != StatusImporting {
		t.Fatalf("first file status = %s", states.fileStates[0].Status)
	}
	if states.fileStates[1].Status != StatusReady {
		t.Fatalf("second file status = %s", states.fileStates[1].Status)
	}
	row := writer.batch.appended[0]
	if len(row) != 15 {
		t.Fatalf("append values = %d, want 15", len(row))
	}
	if got := row[0]; got != source.SourceID {
		t.Fatalf("source_id = %v", got)
	}
	if got := row[8]; got != netip.MustParseAddr("10.0.0.3") {
		t.Fatalf("nat_ip = %v", got)
	}
}

func TestDropLogSourceDatePartitionSQLSeparatesSources(t *testing.T) {
	date := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	a := dropLogSourceDatePartitionSQL("fw-a", date)
	b := dropLogSourceDatePartitionSQL("fw-b", date)
	if a == b {
		t.Fatalf("source partitions must differ: %q", a)
	}
	if strings.Contains(a, "DROP PARTITION '2026-07-02'") {
		t.Fatalf("partition drop must not be date-only: %q", a)
	}
}

func TestTerminalStateTimestampAdvancesPastSecondPrecision(t *testing.T) {
	previous := time.Date(2026, 7, 14, 10, 0, 0, 900_000_000, time.Local)
	current := previous.Add(50 * time.Millisecond)

	got := terminalStateTimestamp(previous, current)
	want := time.Date(2026, 7, 14, 10, 0, 1, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("terminal timestamp = %v, want %v", got, want)
	}

	later := want.Add(2 * time.Second)
	if got := terminalStateTimestamp(previous, later); !got.Equal(later) {
		t.Fatalf("later terminal timestamp = %v, want %v", got, later)
	}
}

func TestImportDateUpdatesDateProgressAfterEachFile(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	line := "2026 Jul 2 12:00:01 源IP: 10.0.0.1 源端�? 1234 目的IP: 10.0.0.2 目的端口: 80 协议: TCP 转换后的IP: 10.0.0.3 转换后的端口: 8080 动作: ALLOW"
	for _, name := range []string{"edge-a.log-20260702", "edge-b.log-20260702"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
			t.Fatalf("write log file %s: %v", name, err)
		}
		modTime := logDate.Add(12 * time.Hour)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("set mod time %s: %v", name, err)
		}
	}

	now := logDate.Add(24 * time.Hour)
	states := &fakeImportStateRecorder{}
	importer := &Importer{
		store:     &ClickHouseStore{},
		writer:    &fakeClickHouseWriter{batch: &fakeBatch{}},
		states:    states,
		now:       func() time.Time { return now },
		sleep:     func(time.Duration) {},
		batchSize: 1,
	}

	source := LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}
	if err := importer.ImportDate(context.Background(), source, logDate); err != nil {
		t.Fatalf("ImportDate returned error: %v", err)
	}

	var currentFileStates []DateIngestState
	for _, state := range states.dateStates {
		if state.Status == StatusImporting && state.CurrentFile != "" {
			currentFileStates = append(currentFileStates, state)
		}
	}
	if len(currentFileStates) < 2 {
		t.Fatalf("current file states = %d, want at least 2; all states = %#v", len(currentFileStates), states.dateStates)
	}
	if got := currentFileStates[0]; got.FilesDone != 0 || got.CurrentFile != "edge-a.log-20260702" {
		t.Fatalf("first current file state = %#v", got)
	}

	assertDateProgressState(t, states.dateStates, 1, "edge-a.log-20260702", 1, 50)
	assertDateProgressState(t, states.dateStates, 2, "edge-b.log-20260702", 2, 100)
}

func assertDateProgressState(t *testing.T, states []DateIngestState, filesDone uint64, currentFile string, rowsImported uint64, progressPct float64) {
	t.Helper()
	for _, state := range states {
		if state.Status != StatusImporting || state.FilesDone != filesDone {
			continue
		}
		if state.FilesTotal == 2 && state.RowsImported == rowsImported && state.ProgressPct == progressPct && state.CurrentFile == currentFile {
			return
		}
	}
	t.Fatalf("missing progress state filesDone=%d currentFile=%s rows=%d pct=%v; all states = %#v", filesDone, currentFile, rowsImported, progressPct, states)
}

func TestImportDateMarksDateFailedWhenAppendBatchFails(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260702")
	line := "2026 Jul 2 12:00:01 源IP: 10.0.0.1 源端�? 1234 目的IP: 10.0.0.2 目的端口: 80 协议: TCP 转换后的IP: 10.0.0.3 转换后的端口: 8080 动作: ALLOW"
	if err := os.WriteFile(logFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	modTime := logDate.Add(12 * time.Hour)
	if err := os.Chtimes(logFile, modTime, modTime); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	writer := &fakeClickHouseWriter{batch: &fakeBatch{sendErr: errors.New("send failed")}}
	states := &fakeImportStateRecorder{}
	now := logDate.Add(24 * time.Hour)
	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: writer,
		states: states,
		now: func() time.Time {
			return now
		},
		sleep:     func(time.Duration) {},
		batchSize: 1,
	}

	err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate)
	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("ImportDate error = %v, want send failed", err)
	}

	lastDate := states.dateStates[len(states.dateStates)-1]
	if lastDate.Status != StatusFailed {
		t.Fatalf("last date status = %s, want failed", lastDate.Status)
	}
	if lastDate.RetryCount != 1 {
		t.Fatalf("retry count = %d, want 1", lastDate.RetryCount)
	}
	if lastDate.NextRetryAt.IsZero() {
		t.Fatal("next retry at should be set")
	}
	for _, state := range states.dateStates {
		if state.Status == StatusReady {
			t.Fatal("date should not be marked ready after append failure")
		}
	}
	lastFile := states.fileStates[len(states.fileStates)-1]
	if lastFile.Status != StatusFailed {
		t.Fatalf("last file status = %s, want failed", lastFile.Status)
	}
	if lastFile.RetryCount != 1 {
		t.Fatalf("file retry count = %d, want 1", lastFile.RetryCount)
	}
	if lastFile.NextRetryAt.IsZero() {
		t.Fatal("file next retry at should be set")
	}
	if lastFile.Error == "" {
		t.Fatal("file error should be set")
	}
	if !ShouldRetryDate(DateIngestState{
		Status:      lastDate.Status,
		RetryCount:  lastDate.RetryCount,
		NextRetryAt: lastDate.NextRetryAt,
	}, now.Add(2*time.Minute)) {
		t.Fatal("failed date should remain retryable within retry budget")
	}
}

func TestImportDateWithoutFilesMarksDateFailed(t *testing.T) {
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	now := logDate.Add(24 * time.Hour)
	states := &fakeImportStateRecorder{}
	writer := &fakeClickHouseWriter{batch: &fakeBatch{}}
	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: writer,
		states: states,
		now: func() time.Time {
			return now
		},
		sleep: func(time.Duration) {},
	}

	err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: t.TempDir()}, logDate)
	if err == nil || !strings.Contains(err.Error(), "no archived log files") {
		t.Fatalf("ImportDate error = %v, want no archived log files", err)
	}
	if len(states.dateStates) == 0 {
		t.Fatal("expected date state writes")
	}
	if states.dateStates[0].Status == StatusImporting {
		t.Fatal("empty date should not be marked importing before failed")
	}
	lastDate := states.dateStates[len(states.dateStates)-1]
	if lastDate.Status != StatusFailed {
		t.Fatalf("last date status = %s, want failed", lastDate.Status)
	}
	if len(writer.execCalls) != 0 {
		t.Fatalf("missing archive must not delete existing partition: %#v", writer.execCalls)
	}
}

func TestImportDateRequiresStateStore(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260702")
	line := "2026 Jul 2 12:00:01 源IP: 10.0.0.1 目的IP: 10.0.0.2"
	if err := os.WriteFile(logFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	if err := os.Chtimes(logFile, logDate.Add(12*time.Hour), logDate.Add(12*time.Hour)); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	importer := &Importer{
		store:  &ClickHouseStore{},
		writer: &fakeClickHouseWriter{batch: &fakeBatch{}},
		now:    func() time.Time { return logDate.Add(24 * time.Hour) },
		sleep:  func(time.Duration) {},
	}
	err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate)
	if err == nil || !strings.Contains(err.Error(), "ingest state store is required") {
		t.Fatalf("ImportDate error = %v, want state store error", err)
	}
}

func TestImportDateFailureIncrementsExistingRetryCounts(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260702")
	line := "2026 Jul 2 12:00:01 源IP: 10.0.0.1 源端�? 1234 目的IP: 10.0.0.2 目的端口: 80 协议: TCP"
	if err := os.WriteFile(logFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	if err := os.Chtimes(logFile, logDate.Add(12*time.Hour), logDate.Add(12*time.Hour)); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	writer := &fakeClickHouseWriter{batch: &fakeBatch{sendErr: errors.New("send failed")}}
	states := &fakeImportStateRecorder{
		latestDate: DateIngestState{RetryCount: 1},
		dateFound:  true,
		latestFile: FileIngestState{RetryCount: 2},
		fileFound:  true,
	}
	now := logDate.Add(24 * time.Hour)
	importer := &Importer{
		store:     &ClickHouseStore{},
		writer:    writer,
		states:    states,
		now:       func() time.Time { return now },
		sleep:     func(time.Duration) {},
		batchSize: 1,
	}

	err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate)
	if err == nil {
		t.Fatal("ImportDate should fail")
	}

	lastDate := states.dateStates[len(states.dateStates)-1]
	if lastDate.RetryCount != 2 {
		t.Fatalf("date retry count = %d, want 2", lastDate.RetryCount)
	}
	if got := lastDate.NextRetryAt.Sub(now); got != 5*time.Minute {
		t.Fatalf("date retry backoff = %s, want 5m", got)
	}
	lastFile := states.fileStates[len(states.fileStates)-1]
	if lastFile.RetryCount != 3 {
		t.Fatalf("file retry count = %d, want 3", lastFile.RetryCount)
	}
	if got := lastFile.NextRetryAt.Sub(now); got != 15*time.Minute {
		t.Fatalf("file retry backoff = %s, want 15m", got)
	}
}

func TestImportDateRebuildAdvancesPastNewerFailedState(t *testing.T) {
	dir := t.TempDir()
	logDate := time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)
	logFile := filepath.Join(dir, "edge-fw.log-20260702")
	line := "2026 Jul 2 12:00:01 " + natLabelSrcIP + " 10.0.0.1 " + natLabelSrcPort + " 1234 " + natLabelDstIP + " 10.0.0.2 " + natLabelDstPort + " 80 " + natLabelProtocol + " TCP"
	if err := os.WriteFile(logFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := os.Chtimes(logFile, logDate.Add(12*time.Hour), logDate.Add(12*time.Hour)); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	failedAt := logDate.Add(24*time.Hour + 2*time.Second)
	states := &fakeImportStateRecorder{
		latestDate: DateIngestState{Status: StatusFailed, UpdatedAt: failedAt},
		dateFound:  true,
	}
	importer := &Importer{
		store:     &ClickHouseStore{},
		writer:    &fakeClickHouseWriter{batch: &fakeBatch{}},
		states:    states,
		now:       func() time.Time { return logDate.Add(24 * time.Hour) },
		sleep:     func(time.Duration) {},
		batchSize: 1,
	}

	if err := importer.ImportDate(context.Background(), LogSource{SourceID: "fw-a", LogTag: "edge", LogDir: dir}, logDate); err != nil {
		t.Fatalf("ImportDate returned error: %v", err)
	}
	lastDate := states.dateStates[len(states.dateStates)-1]
	if lastDate.Status != StatusReady {
		t.Fatalf("last date status = %s, want ready", lastDate.Status)
	}
	if !lastDate.UpdatedAt.After(failedAt) {
		t.Fatalf("rebuild timestamp = %v, must be after previous failed state %v", lastDate.UpdatedAt, failedAt)
	}
}

func TestAppendBatchWithNoRowsReturnsNil(t *testing.T) {
	importer := &Importer{}
	if err := importer.AppendBatch(context.Background(), nil); err != nil {
		t.Fatalf("AppendBatch should ignore empty rows: %v", err)
	}
}

func TestAppendBatchUsesWriteGate(t *testing.T) {
	gate := &fakeWriteGate{}
	importer := &Importer{writer: &fakeClickHouseWriter{batch: &fakeBatch{}}, writeGate: gate}
	row := NATLogRow{SourceID: "fw-a", LogDate: time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local)}
	if err := importer.AppendBatch(context.Background(), []NATLogRow{row}); err != nil {
		t.Fatalf("AppendBatch returned error: %v", err)
	}
	if gate.calls != 1 {
		t.Fatalf("write gate calls = %d, want 1", gate.calls)
	}
}

type fakeWriteGate struct{ calls int }

func (g *fakeWriteGate) WithWriteSlot(_ context.Context, write func() error) error {
	g.calls++
	return write()
}

type fakeClickHouseWriter struct {
	batch        *fakeBatch
	prepareErr   error
	execErr      error
	prepareCalls []string
	execCalls    []fakeExecCall
}

type fakeExecCall struct {
	query string
	args  []any
}

func (f *fakeClickHouseWriter) PrepareBatch(_ context.Context, query string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	f.prepareCalls = append(f.prepareCalls, query)
	if f.prepareErr != nil {
		return nil, f.prepareErr
	}
	if f.batch == nil {
		f.batch = &fakeBatch{}
	}
	return f.batch, nil
}

func (f *fakeClickHouseWriter) Exec(_ context.Context, query string, args ...any) error {
	f.execCalls = append(f.execCalls, fakeExecCall{query: query, args: append([]any(nil), args...)})
	return f.execErr
}

type fakeBatch struct {
	appended [][]any
	sent     bool
	sendErr  error
}

func (f *fakeBatch) Abort() error { return nil }
func (f *fakeBatch) Append(v ...any) error {
	f.appended = append(f.appended, append([]any(nil), v...))
	return nil
}
func (f *fakeBatch) AppendStruct(any) error        { return nil }
func (f *fakeBatch) Column(int) driver.BatchColumn { return nil }
func (f *fakeBatch) Flush() error                  { return nil }
func (f *fakeBatch) Send() error                   { f.sent = true; return f.sendErr }
func (f *fakeBatch) IsSent() bool                  { return f.sent }
func (f *fakeBatch) Rows() int                     { return len(f.appended) }
func (f *fakeBatch) Columns() []column.Interface   { return nil }

type fakeImportStateRecorder struct {
	dateStates []DateIngestState
	fileStates []FileIngestState
	latestDate DateIngestState
	dateFound  bool
	dateErr    error
	latestFile FileIngestState
	fileFound  bool
	fileErr    error
}

func (f *fakeImportStateRecorder) WriteDateState(_ context.Context, state DateIngestState) error {
	f.dateStates = append(f.dateStates, state)
	return nil
}

func (f *fakeImportStateRecorder) WriteFileState(_ context.Context, state FileIngestState) error {
	f.fileStates = append(f.fileStates, state)
	return nil
}

func (f *fakeImportStateRecorder) LatestDateState(_ context.Context, _ string, _ time.Time) (DateIngestState, bool, error) {
	return f.latestDate, f.dateFound, f.dateErr
}

func (f *fakeImportStateRecorder) LatestFileState(_ context.Context, _ string) (FileIngestState, bool, error) {
	return f.latestFile, f.fileFound, f.fileErr
}
