package importer

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenLogReaderRejectsGzipBeyondDecompressedBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.log.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := io.Copy(writer, bytes.NewBufferString(strings.Repeat("a", 1024))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	handle, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	reader, closeReader, err := openLogReader(handle, path, 128)
	if err != nil {
		t.Fatal(err)
	}
	defer closeReader()
	_, err = io.ReadAll(reader)
	if !errors.Is(err, errDecompressedLogTooLarge) {
		t.Fatalf("read error = %v, want %v", err, errDecompressedLogTooLarge)
	}
}

func TestImportFileStopsWhenContextIsCanceled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026-07-15.log")
	if err := os.WriteFile(path, []byte("line that should not be parsed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	importer := &Importer{writer: &fakeClickHouseWriter{batch: &fakeBatch{}}}
	_, err = importer.importFile(ctx, LogSource{SourceID: "fw-a"}, time.Now(), LogFileSnapshot{Path: path, Size: info.Size()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("importFile error = %v, want context.Canceled", err)
	}
}
