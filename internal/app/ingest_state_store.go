package app

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type DateIngestState struct {
	SourceID            string       `json:"source_id"`
	LogTag              string       `json:"log_tag"`
	LogDate             time.Time    `json:"log_date"`
	Status              IngestStatus `json:"status"`
	FilesTotal          uint64       `json:"files_total"`
	FilesDone           uint64       `json:"files_done"`
	RowsImported        uint64       `json:"rows_imported"`
	BytesTotal          uint64       `json:"bytes_total"`
	BytesDone           uint64       `json:"bytes_done"`
	CurrentFile         string       `json:"current_file"`
	ProgressPct         float64      `json:"progress_pct"`
	MaxVisibleTimestamp time.Time    `json:"max_visible_timestamp"`
	RetryCount          uint8        `json:"retry_count"`
	NextRetryAt         time.Time    `json:"next_retry_at"`
	Error               string       `json:"error"`
	UpdatedAt           time.Time    `json:"updated_at"`
}

type FileIngestState struct {
	Path         string       `json:"path"`
	SourceID     string       `json:"source_id"`
	LogTag       string       `json:"log_tag"`
	LogDate      time.Time    `json:"log_date"`
	Status       IngestStatus `json:"status"`
	RowsImported uint64       `json:"rows_imported"`
	BytesTotal   uint64       `json:"bytes_total"`
	BytesDone    uint64       `json:"bytes_done"`
	ProgressPct  float64      `json:"progress_pct"`
	RetryCount   uint8        `json:"retry_count"`
	NextRetryAt  time.Time    `json:"next_retry_at"`
	Error        string       `json:"error"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

func (s *ClickHouseStore) WriteDateState(ctx context.Context, state DateIngestState) error {
	return s.conn.Exec(ctx, DateStateUpsertSQL(),
		state.SourceID,
		state.LogTag,
		state.LogDate,
		state.Status,
		state.FilesTotal,
		state.FilesDone,
		state.RowsImported,
		state.BytesTotal,
		state.BytesDone,
		state.CurrentFile,
		state.ProgressPct,
		state.MaxVisibleTimestamp,
		state.RetryCount,
		state.NextRetryAt,
		state.Error,
		state.UpdatedAt,
	)
}

func (s *ClickHouseStore) WriteFileState(ctx context.Context, state FileIngestState) error {
	return s.conn.Exec(ctx, FileStateUpsertSQL(),
		state.Path,
		state.SourceID,
		state.LogTag,
		state.LogDate,
		state.Status,
		state.RowsImported,
		state.BytesTotal,
		state.BytesDone,
		state.ProgressPct,
		state.RetryCount,
		state.NextRetryAt,
		state.Error,
		state.UpdatedAt,
	)
}

func (s *ClickHouseStore) LatestDateState(ctx context.Context, sourceID string, date time.Time) (DateIngestState, bool, error) {
	var state DateIngestState
	err := s.conn.QueryRow(ctx, DateStatePointQuery(), sourceID, date).Scan(
		&state.SourceID,
		&state.LogTag,
		&state.LogDate,
		&state.Status,
		&state.FilesTotal,
		&state.FilesDone,
		&state.RowsImported,
		&state.BytesTotal,
		&state.BytesDone,
		&state.CurrentFile,
		&state.ProgressPct,
		&state.MaxVisibleTimestamp,
		&state.RetryCount,
		&state.NextRetryAt,
		&state.Error,
		&state.UpdatedAt,
	)
	if err != nil {
		if isNoRowsError(err) {
			return DateIngestState{}, false, nil
		}
		return DateIngestState{}, false, err
	}
	return state, true, nil
}

func (s *ClickHouseStore) LatestFileState(ctx context.Context, path string) (FileIngestState, bool, error) {
	var state FileIngestState
	err := s.conn.QueryRow(ctx, FileStatePointQuery(), path).Scan(
		&state.Path,
		&state.SourceID,
		&state.LogTag,
		&state.LogDate,
		&state.Status,
		&state.RowsImported,
		&state.BytesTotal,
		&state.BytesDone,
		&state.ProgressPct,
		&state.RetryCount,
		&state.NextRetryAt,
		&state.Error,
		&state.UpdatedAt,
	)
	if err != nil {
		if isNoRowsError(err) {
			return FileIngestState{}, false, nil
		}
		return FileIngestState{}, false, err
	}
	return state, true, nil
}

func isNoRowsError(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || (err != nil && err.Error() == "clickhouse: no rows in result")
}

func DateStateUpsertSQL() string {
	return `INSERT INTO ingest_dates (
    source_id, log_tag, log_date, status, files_total, files_done, rows_imported,
    bytes_total, bytes_done, current_file, progress_pct, max_visible_timestamp,
    retry_count, next_retry_at, error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

func FileStateUpsertSQL() string {
	return `INSERT INTO ingest_files (
    path, source_id, log_tag, log_date, status, rows_imported, bytes_total,
    bytes_done, progress_pct, retry_count, next_retry_at, error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

func DateStatePointQuery() string {
	return `SELECT
    source_id, log_tag, log_date, status, files_total, files_done, rows_imported, bytes_total, bytes_done,
    current_file, progress_pct, max_visible_timestamp, retry_count, next_retry_at, error, updated_at
FROM ingest_dates
WHERE source_id = ? AND log_date = ?
ORDER BY updated_at DESC
LIMIT 1`
}

func FileStatePointQuery() string {
	return `SELECT
    path, source_id, log_tag, log_date, status, rows_imported, bytes_total, bytes_done,
    progress_pct, retry_count, next_retry_at, error, updated_at
FROM ingest_files
WHERE path = ?
ORDER BY updated_at DESC
LIMIT 1`
}

func DateStateListQuery() string {
	return `SELECT
    source_id,
    argMax(log_tag, updated_at) AS log_tag,
    log_date,
    argMax(status, updated_at) AS status,
    argMax(files_total, updated_at) AS files_total,
    argMax(files_done, updated_at) AS files_done,
    argMax(rows_imported, updated_at) AS rows_imported,
    argMax(bytes_total, updated_at) AS bytes_total,
    argMax(bytes_done, updated_at) AS bytes_done,
    argMax(current_file, updated_at) AS current_file,
    argMax(progress_pct, updated_at) AS progress_pct,
    argMax(max_visible_timestamp, updated_at) AS max_visible_timestamp,
    argMax(retry_count, updated_at) AS retry_count,
    argMax(next_retry_at, updated_at) AS next_retry_at,
    argMax(error, updated_at) AS error,
    max(updated_at) AS latest_updated_at
FROM ingest_dates
WHERE log_date >= toDate(?)
GROUP BY source_id, log_date
ORDER BY log_date DESC, source_id ASC`
}
