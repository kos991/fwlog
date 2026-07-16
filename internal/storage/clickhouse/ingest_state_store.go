package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const ingestStatusPrioritySQL = "multiIf(status = 'failed', 4, status IN ('ready', 'succeeded', 'no_data'), 3, status = 'importing', 1, 0)"
const ingestDateStatusPrioritySQL = "multiIf(ingest_dates.status = 'failed', 4, ingest_dates.status IN ('ready', 'succeeded', 'no_data'), 3, ingest_dates.status = 'importing', 1, 0)"
const ingestDateStateVersionSQL = "tuple(ingest_dates.updated_at, " + ingestDateStatusPrioritySQL + ")"

func (s *ClickHouseStore) WriteDateState(ctx context.Context, state DateIngestState) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("clickhouse connection is not initialized")
	}
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
	if s == nil || s.conn == nil {
		return fmt.Errorf("clickhouse connection is not initialized")
	}
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
	if s == nil || s.conn == nil {
		return DateIngestState{}, false, fmt.Errorf("clickhouse connection is not initialized")
	}
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
	if s == nil || s.conn == nil {
		return FileIngestState{}, false, fmt.Errorf("clickhouse connection is not initialized")
	}
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
	return fmt.Sprintf(`SELECT
    source_id, log_tag, log_date, status, files_total, files_done, rows_imported, bytes_total, bytes_done,
    current_file, progress_pct, max_visible_timestamp, retry_count, next_retry_at, error, updated_at
FROM ingest_dates
WHERE source_id = ? AND log_date = ?
ORDER BY updated_at DESC, %s DESC
LIMIT 1`, ingestStatusPrioritySQL)
}

func FileStatePointQuery() string {
	return fmt.Sprintf(`SELECT
    path, source_id, log_tag, log_date, status, rows_imported, bytes_total, bytes_done,
    progress_pct, retry_count, next_retry_at, error, updated_at
FROM ingest_files
WHERE path = ?
ORDER BY updated_at DESC, %s DESC
LIMIT 1`, ingestStatusPrioritySQL)
}

func DateStateListQuery() string {
	return fmt.Sprintf(`SELECT
    source_id,
    argMax(log_tag, %[1]s) AS log_tag,
    log_date,
    argMax(status, %[1]s) AS status,
    argMax(files_total, %[1]s) AS files_total,
    argMax(files_done, %[1]s) AS files_done,
    argMax(rows_imported, %[1]s) AS rows_imported,
    argMax(bytes_total, %[1]s) AS bytes_total,
    argMax(bytes_done, %[1]s) AS bytes_done,
    argMax(current_file, %[1]s) AS current_file,
    argMax(progress_pct, %[1]s) AS progress_pct,
    argMax(max_visible_timestamp, %[1]s) AS max_visible_timestamp,
    argMax(retry_count, %[1]s) AS retry_count,
    argMax(next_retry_at, %[1]s) AS next_retry_at,
    argMax(error, %[1]s) AS error,
    max(updated_at) AS latest_updated_at
FROM ingest_dates
WHERE log_date >= toDate(?)
GROUP BY source_id, log_date
ORDER BY log_date DESC, source_id ASC`, ingestDateStateVersionSQL)
}

func IngestStateMigrationSQL() []string {
	return []string{
		fmt.Sprintf(`INSERT INTO ingest_dates (
    source_id, log_tag, log_date, status, files_total, files_done, rows_imported,
    bytes_total, bytes_done, current_file, progress_pct, max_visible_timestamp,
    retry_count, next_retry_at, error, updated_at
)
SELECT
    source_id,
    argMax(log_tag, %[1]s),
    log_date,
    'ready',
    argMax(files_total, %[1]s),
    argMax(files_done, %[1]s),
    argMax(rows_imported, %[1]s),
    argMax(bytes_total, %[1]s),
    argMax(bytes_done, %[1]s),
    argMax(current_file, %[1]s),
    100,
    argMax(max_visible_timestamp, %[1]s),
    0,
    toDateTime(0),
    '',
    addSeconds(now(), 1)
FROM ingest_dates
GROUP BY source_id, log_date
HAVING maxIf(updated_at, status = 'importing' AND progress_pct >= 100
        AND files_total > 0 AND files_done >= files_total) = max(updated_at)
    AND maxIf(updated_at, status = 'importing' AND progress_pct >= 100
        AND files_total > 0 AND files_done >= files_total) > maxIf(updated_at, status = 'failed')`, ingestDateStateVersionSQL),
		fmt.Sprintf(`INSERT INTO ingest_dates (
    source_id, log_tag, log_date, status, files_total, files_done, rows_imported,
    bytes_total, bytes_done, current_file, progress_pct, max_visible_timestamp,
    retry_count, next_retry_at, error, updated_at
)
SELECT
    source_id,
    argMax(log_tag, %[1]s),
    log_date,
    'no_data',
    argMax(files_total, %[1]s),
    argMax(files_done, %[1]s),
    0,
    argMax(bytes_total, %[1]s),
    argMax(bytes_done, %[1]s),
    argMax(current_file, %[1]s),
    100,
    toDateTime(0),
    0,
    toDateTime(0),
    '',
    addSeconds(max(updated_at), 1)
FROM ingest_dates
GROUP BY source_id, log_date
HAVING argMax(status, %[1]s) IN ('ready', 'succeeded')
    AND argMax(rows_imported, %[1]s) = 0
    AND argMax(files_total, %[1]s) > 0`, ingestDateStateVersionSQL),
		fmt.Sprintf(`INSERT INTO ingest_files (
    path, source_id, log_tag, log_date, status, rows_imported, bytes_total,
    bytes_done, progress_pct, retry_count, next_retry_at, error, updated_at
)
SELECT
    path,
    argMax(source_id, %[1]s),
    argMax(log_tag, %[1]s),
    argMax(log_date, %[1]s),
    'no_data',
    0,
    argMax(bytes_total, %[1]s),
    argMax(bytes_done, %[1]s),
    100,
    0,
    toDateTime(0),
    '',
    addSeconds(max(updated_at), 1)
FROM ingest_files
GROUP BY path
HAVING argMax(status, %[1]s) IN ('ready', 'succeeded')
    AND argMax(rows_imported, %[1]s) = 0`, ingestStatusPrioritySQL),
	}
}
