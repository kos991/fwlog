package importer

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const defaultImportBatchSize = 20000

type clickHouseWriter interface {
	PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
	Exec(ctx context.Context, query string, args ...any) error
}

type ingestStateWriter interface {
	WriteDateState(ctx context.Context, state DateIngestState) error
	WriteFileState(ctx context.Context, state FileIngestState) error
}

type ingestStateReader interface {
	LatestDateState(ctx context.Context, sourceID string, date time.Time) (DateIngestState, bool, error)
	LatestFileState(ctx context.Context, path string) (FileIngestState, bool, error)
}

type Importer struct {
	store     *ClickHouseStore
	writer    clickHouseWriter
	states    ingestStateWriter
	now       func() time.Time
	sleep     func(time.Duration)
	batchSize int
	writeGate batchWriteGate
}

type batchWriteGate interface {
	WithWriteSlot(context.Context, func() error) error
}

func NewImporter(store *ClickHouseStore, gate batchWriteGate) *Importer {
	return &Importer{
		store:     store,
		writer:    store,
		states:    store,
		writeGate: gate,
	}
}

func (i *Importer) AppendBatch(ctx context.Context, rows []NATLogRow) error {
	if len(rows) == 0 {
		return nil
	}

	writer, err := i.writerOrDefault()
	if err != nil {
		return err
	}

	write := func() error {
		batch, err := writer.PrepareBatch(ctx, `INSERT INTO nat_logs (
    source_id, log_tag, log_date, timestamp, src_ip, src_port, dst_ip, dst_port,
    nat_ip, nat_port, protocol, action, source_file, source_offset, batch_id
)`)
		if err != nil {
			return err
		}

		for _, row := range rows {
			if err := batch.Append(
				row.SourceID,
				row.LogTag,
				row.LogDate,
				row.Timestamp,
				row.SrcIP,
				row.SrcPort,
				row.DstIP,
				row.DstPort,
				row.NATIP,
				row.NATPort,
				row.Protocol,
				row.Action,
				row.SourceFile,
				row.SourceOffset,
				row.BatchID,
			); err != nil {
				return err
			}
		}
		return batch.Send()
	}
	if i.writeGate != nil {
		return i.writeGate.WithWriteSlot(ctx, write)
	}
	return write()
}

func (i *Importer) ImportDate(ctx context.Context, source LogSource, date time.Time) error {
	writer, err := i.writerOrDefault()
	if err != nil {
		return err
	}

	now := i.nowOrDefault()()
	if err := writer.Exec(ctx, dropLogSourceDatePartitionSQL(source.SourceID, date)); err != nil {
		return err
	}
	i.sleepOrDefault()(time.Second)

	files, err := ScanArchivedLogFiles(source.LogDir, now)
	if err != nil {
		return err
	}

	targetFiles := filterLogFilesByDate(files, date)
	if len(targetFiles) == 0 {
		failErr := fmt.Errorf("no archived log files for %s", date.Format("2006-01-02"))
		if markErr := i.markDateFailed(ctx, source, date, now, 0, failErr); markErr != nil {
			return errors.Join(failErr, markErr)
		}
		return failErr
	}

	var totalBytes uint64
	for _, file := range targetFiles {
		totalBytes += uint64(file.Size)
	}

	if err := i.writeDateState(ctx, DateIngestState{
		SourceID:   source.SourceID,
		LogTag:     source.LogTag,
		LogDate:    date,
		Status:     StatusImporting,
		FilesTotal: uint64(len(targetFiles)),
		BytesTotal: totalBytes,
		UpdatedAt:  now,
	}); err != nil {
		return err
	}
	dateUpdatedAt := now

	var totalRows uint64
	var filesDone uint64
	var bytesDone uint64

	for _, file := range targetFiles {
		progressUpdatedAt := i.nowOrDefault()()
		if err := i.writeDateState(ctx, DateIngestState{
			SourceID:     source.SourceID,
			LogTag:       source.LogTag,
			LogDate:      date,
			Status:       StatusImporting,
			FilesTotal:   uint64(len(targetFiles)),
			FilesDone:    filesDone,
			RowsImported: totalRows,
			BytesTotal:   totalBytes,
			BytesDone:    bytesDone,
			CurrentFile:  filepath.Base(file.Path),
			ProgressPct:  float64(filesDone) / float64(len(targetFiles)) * 100,
			UpdatedAt:    progressUpdatedAt,
		}); err != nil {
			return err
		}
		if progressUpdatedAt.After(dateUpdatedAt) {
			dateUpdatedAt = progressUpdatedAt
		}
		fileUpdatedAt := i.nowOrDefault()()
		if err := i.writeFileState(ctx, FileIngestState{
			Path:       file.Path,
			SourceID:   source.SourceID,
			LogTag:     source.LogTag,
			LogDate:    date,
			Status:     StatusImporting,
			BytesTotal: uint64(file.Size),
			UpdatedAt:  fileUpdatedAt,
		}); err != nil {
			return err
		}

		rowsImported, err := i.importFile(ctx, source, date, file)
		if err != nil {
			if markErr := i.markFileFailed(ctx, source, date, file, fileUpdatedAt, err); markErr != nil {
				return errors.Join(err, markErr)
			}
			if markErr := i.markDateFailed(ctx, source, date, dateUpdatedAt, filesDone, err); markErr != nil {
				return errors.Join(err, markErr)
			}
			return err
		}

		filesDone++
		totalRows += rowsImported
		bytesDone += uint64(file.Size)
		if err := i.writeFileState(ctx, FileIngestState{
			Path:         file.Path,
			SourceID:     source.SourceID,
			LogTag:       source.LogTag,
			LogDate:      date,
			Status:       StatusReady,
			RowsImported: rowsImported,
			BytesTotal:   uint64(file.Size),
			BytesDone:    uint64(file.Size),
			ProgressPct:  100,
			UpdatedAt:    terminalStateTimestamp(fileUpdatedAt, i.nowOrDefault()()),
		}); err != nil {
			return err
		}
		progressUpdatedAt = i.nowOrDefault()()
		if err := i.writeDateState(ctx, DateIngestState{
			SourceID:     source.SourceID,
			LogTag:       source.LogTag,
			LogDate:      date,
			Status:       StatusImporting,
			FilesTotal:   uint64(len(targetFiles)),
			FilesDone:    filesDone,
			RowsImported: totalRows,
			BytesTotal:   totalBytes,
			BytesDone:    bytesDone,
			CurrentFile:  filepath.Base(file.Path),
			ProgressPct:  float64(filesDone) / float64(len(targetFiles)) * 100,
			UpdatedAt:    progressUpdatedAt,
		}); err != nil {
			return err
		}
		if progressUpdatedAt.After(dateUpdatedAt) {
			dateUpdatedAt = progressUpdatedAt
		}
	}

	return i.writeDateState(ctx, DateIngestState{
		SourceID:     source.SourceID,
		LogTag:       source.LogTag,
		LogDate:      date,
		Status:       StatusReady,
		FilesTotal:   uint64(len(targetFiles)),
		FilesDone:    filesDone,
		RowsImported: totalRows,
		BytesTotal:   totalBytes,
		BytesDone:    totalBytes,
		ProgressPct:  100,
		UpdatedAt:    terminalStateTimestamp(dateUpdatedAt, i.nowOrDefault()()),
	})
}

func terminalStateTimestamp(previous, current time.Time) time.Time {
	minimum := previous.Truncate(time.Second).Add(time.Second)
	if current.Before(minimum) {
		return minimum
	}
	return current
}

func dropLogSourceDatePartitionSQL(sourceID string, date time.Time) string {
	sourceID = strings.ReplaceAll(sourceID, "'", "''")
	return fmt.Sprintf("ALTER TABLE nat_logs DROP PARTITION ('%s', '%s')", sourceID, date.Format("2006-01-02"))
}

func (i *Importer) importFile(ctx context.Context, source LogSource, date time.Time, file LogFileSnapshot) (uint64, error) {
	handle, err := os.Open(file.Path)
	if err != nil {
		return 0, err
	}
	defer handle.Close()

	reader, closeReader, err := openLogReader(handle, file.Path)
	if err != nil {
		return 0, err
	}
	if closeReader != nil {
		defer closeReader()
	}

	batchSize := i.batchSize
	if batchSize <= 0 {
		batchSize = defaultImportBatchSize
	}

	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 10*1024*1024)

	rows := make([]NATLogRow, 0, batchSize)
	var rowsImported uint64
	var offset uint64
	batchID := fmt.Sprintf("%s-%s", source.SourceID, date.Format("20060102"))
	for scanner.Scan() {
		line := scanner.Text()
		meta := ParseMeta{
			SourceID:     source.SourceID,
			LogTag:       source.LogTag,
			LogDate:      date,
			SourceFile:   filepath.Base(file.Path),
			SourceOffset: offset,
			BatchID:      batchID,
		}
		offset += uint64(len(scanner.Bytes())) + 1
		row, ok := ParseNATLine(line, meta)
		if !ok {
			continue
		}
		rows = append(rows, row)
		if len(rows) < batchSize {
			continue
		}
		if err := i.AppendBatch(ctx, rows); err != nil {
			return rowsImported, err
		}
		rowsImported += uint64(len(rows))
		rows = rows[:0]
	}

	if err := scanner.Err(); err != nil {
		return rowsImported, err
	}

	if len(rows) > 0 {
		if err := i.AppendBatch(ctx, rows); err != nil {
			return rowsImported, err
		}
		rowsImported += uint64(len(rows))
	}

	return rowsImported, nil
}

func openLogReader(handle *os.File, path string) (io.Reader, func() error, error) {
	if !strings.HasSuffix(path, ".gz") {
		return handle, nil, nil
	}

	reader, err := gzip.NewReader(handle)
	if err != nil {
		return nil, nil, err
	}
	return reader, reader.Close, nil
}

func filterLogFilesByDate(files []LogFileSnapshot, date time.Time) []LogFileSnapshot {
	filtered := make([]LogFileSnapshot, 0, len(files))
	for _, file := range files {
		if sameDay(file.LogDate, date) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (i *Importer) writerOrDefault() (clickHouseWriter, error) {
	if i != nil && i.writer != nil {
		return i.writer, nil
	}
	if i == nil || i.store == nil || !i.store.Ready() {
		return nil, errors.New("clickhouse store is required")
	}
	return i.store, nil
}

func (i *Importer) nowOrDefault() func() time.Time {
	if i != nil && i.now != nil {
		return i.now
	}
	return time.Now
}

func (i *Importer) sleepOrDefault() func(time.Duration) {
	if i != nil && i.sleep != nil {
		return i.sleep
	}
	return time.Sleep
}

func (i *Importer) writeDateState(ctx context.Context, state DateIngestState) error {
	writer, err := i.stateWriterOrDefault()
	if err != nil {
		return err
	}
	return writer.WriteDateState(ctx, state)
}

func (i *Importer) writeFileState(ctx context.Context, state FileIngestState) error {
	writer, err := i.stateWriterOrDefault()
	if err != nil {
		return err
	}
	return writer.WriteFileState(ctx, state)
}

func (i *Importer) markDateFailed(ctx context.Context, source LogSource, date, now time.Time, filesDone uint64, cause error) error {
	retryCount, err := i.nextDateRetryCount(ctx, source.SourceID, date)
	if err != nil {
		return err
	}
	failedAt := i.nowOrDefault()()
	return i.writeDateState(ctx, DateIngestState{
		SourceID:    source.SourceID,
		LogTag:      source.LogTag,
		LogDate:     date,
		Status:      StatusFailed,
		FilesDone:   filesDone,
		RetryCount:  retryCount,
		NextRetryAt: NextRetryAt(retryCount-1, failedAt),
		Error:       cause.Error(),
		UpdatedAt:   terminalStateTimestamp(now, failedAt),
	})
}

func (i *Importer) markFileFailed(ctx context.Context, source LogSource, date time.Time, file LogFileSnapshot, now time.Time, cause error) error {
	retryCount, err := i.nextFileRetryCount(ctx, file.Path)
	if err != nil {
		return err
	}
	failedAt := i.nowOrDefault()()
	return i.writeFileState(ctx, FileIngestState{
		Path:        file.Path,
		SourceID:    source.SourceID,
		LogTag:      source.LogTag,
		LogDate:     date,
		Status:      StatusFailed,
		BytesTotal:  uint64(file.Size),
		RetryCount:  retryCount,
		NextRetryAt: NextRetryAt(retryCount-1, failedAt),
		Error:       cause.Error(),
		UpdatedAt:   terminalStateTimestamp(now, failedAt),
	})
}

func (i *Importer) nextDateRetryCount(ctx context.Context, sourceID string, date time.Time) (uint8, error) {
	reader, ok := i.stateReader()
	if !ok {
		return 1, nil
	}
	state, found, err := reader.LatestDateState(ctx, sourceID, date)
	if err != nil {
		return 0, err
	}
	if !found {
		return 1, nil
	}
	return incrementRetryCount(state.RetryCount), nil
}

func (i *Importer) nextFileRetryCount(ctx context.Context, path string) (uint8, error) {
	reader, ok := i.stateReader()
	if !ok {
		return 1, nil
	}
	state, found, err := reader.LatestFileState(ctx, path)
	if err != nil {
		return 0, err
	}
	if !found {
		return 1, nil
	}
	return incrementRetryCount(state.RetryCount), nil
}

func (i *Importer) stateReader() (ingestStateReader, bool) {
	if i == nil {
		return nil, false
	}
	if i.states != nil {
		reader, ok := i.states.(ingestStateReader)
		return reader, ok
	}
	if i.store != nil && i.store.Ready() {
		return i.store, true
	}
	return nil, false
}

func (i *Importer) stateWriterOrDefault() (ingestStateWriter, error) {
	if i != nil && i.states != nil {
		return i.states, nil
	}
	if i != nil && i.store != nil && i.store.Ready() {
		return i.store, nil
	}
	return nil, errors.New("ingest state store is required")
}

func incrementRetryCount(current uint8) uint8 {
	if current >= 3 {
		return 3
	}
	return current + 1
}
