package app

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type ClickHouseStore struct {
	conn clickhouse.Conn
}

type QuerySortMode string

const (
	QuerySortFast     QuerySortMode = "fast"
	QuerySortTimeDesc QuerySortMode = "time_desc"
)

func OpenClickHouse(ctx context.Context, cfg Config) (*ClickHouseStore, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.ClickHouseAddr},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouseDatabase,
			Username: cfg.ClickHouseUser,
			Password: cfg.ClickHousePassword,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	return &ClickHouseStore{conn: conn}, nil
}

func (s *ClickHouseStore) EnsureTables(ctx context.Context) error {
	for _, statement := range ClickHouseDDL() {
		if err := s.conn.Exec(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}

func (s *ClickHouseStore) ListDateStates(ctx context.Context, since time.Time) ([]DateIngestState, error) {
	if since.IsZero() {
		since = time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	}

	rows, err := s.conn.Query(ctx, DateStateListQuery(), dateKey(startOfDay(since)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make([]DateIngestState, 0)
	for rows.Next() {
		var state DateIngestState
		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *ClickHouseStore) QueryNATLogs(ctx context.Context, baseSQL string, args []any, options QueryPageOptions, sortMode QuerySortMode) ([]map[string]any, bool, error) {
	sql, queryArgs := queryNATLogsPageSQL(baseSQL, args, options, sortMode)

	rows, err := s.conn.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	pageSize := normalizeQueryPageSize(options.PageSize)
	limit := pageSize
	if usesCursorPagination(options) {
		limit++
	}
	records := make([]map[string]any, 0, limit)
	for rows.Next() {
		var (
			sourceID     string
			logTag       string
			logDate      time.Time
			timestamp    time.Time
			srcIP        net.IP
			srcPort      uint16
			dstIP        net.IP
			dstPort      uint16
			natIP        net.IP
			natPort      uint16
			protocol     string
			action       string
			sourceFile   string
			sourceOffset uint64
			batchID      string
			ingestedAt   time.Time
		)

		if err := rows.Scan(
			&sourceID,
			&logTag,
			&logDate,
			&timestamp,
			&srcIP,
			&srcPort,
			&dstIP,
			&dstPort,
			&natIP,
			&natPort,
			&protocol,
			&action,
			&sourceFile,
			&sourceOffset,
			&batchID,
			&ingestedAt,
		); err != nil {
			return nil, false, err
		}

		records = append(records, map[string]any{
			"id":            fmt.Sprintf("%s-%s-%d", sourceID, sourceFile, sourceOffset),
			"source_id":     sourceID,
			"log_tag":       logTag,
			"log_date":      formatDate(logDate),
			"timestamp":     formatDateTime(timestamp),
			"src_ip":        ipString(srcIP),
			"src_port":      srcPort,
			"dst_ip":        ipString(dstIP),
			"dst_port":      dstPort,
			"nat_ip":        ipString(natIP),
			"nat_port":      natPort,
			"protocol":      normalizeProtocol(protocol),
			"action":        action,
			"source_file":   sourceFile,
			"source_offset": sourceOffset,
			"batch_id":      batchID,
			"ingested_at":   formatDateTime(ingestedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := usesCursorPagination(options) && len(records) > pageSize
	if hasMore {
		records = records[:pageSize]
	}
	return records, hasMore, nil
}

func queryNATLogsPageSQL(baseSQL string, args []any, options QueryPageOptions, sortMode QuerySortMode) (string, []any) {
	page := normalizeQueryPage(options.Page)
	pageSize := normalizeQueryPageSize(options.PageSize)

	offset := (page - 1) * pageSize
	sql := baseSQL
	queryArgs := append([]any{}, args...)
	if options.Cursor != nil {
		sql += " AND (timestamp < ? OR (timestamp = ? AND (source_id, source_file, source_offset) < (?, ?, ?)))"
		queryArgs = append(queryArgs, options.Cursor.Timestamp, options.Cursor.Timestamp, options.Cursor.SourceID, options.Cursor.SourceFile, options.Cursor.SourceOffset)
	}
	if sortMode == QuerySortTimeDesc {
		sql += " ORDER BY timestamp DESC, source_id DESC, source_file DESC, source_offset DESC"
	}
	if usesCursorPagination(options) {
		sql += " LIMIT ?"
		queryArgs = append(queryArgs, pageSize+1)
		return sql, queryArgs
	}
	sql += " LIMIT ? OFFSET ?"
	queryArgs = append(queryArgs, pageSize, offset)
	return sql, queryArgs
}

func normalizeQueryPage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizeQueryPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 50
	}
	if pageSize > 500 {
		return 500
	}
	return pageSize
}

func usesCursorPagination(options QueryPageOptions) bool {
	return options.Cursor != nil || normalizeQueryPage(options.Page) <= 1
}

func (s *ClickHouseStore) DashboardMetrics(ctx context.Context, distributionSince time.Time, includeDistributions bool) (DashboardMetrics, error) {
	var metrics DashboardMetrics

	var err error
	metrics.ClickHouseDiskUsedBytes, err = s.clickHouseDiskUsedBytes(ctx)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.TodayRows, err = s.countRowsForDate(ctx, time.Now())
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.YesterdayRows, err = s.countRowsForDate(ctx, time.Now().AddDate(0, 0, -1))
	if err != nil {
		return DashboardMetrics{}, err
	}

	if !includeDistributions {
		return metrics, nil
	}

	metrics.TopSourceIPs, err = s.distribution(ctx, "src_ip", distributionSince)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.TopDestinationIPs, err = s.distribution(ctx, "dst_ip", distributionSince)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.LogTagDistribution, err = s.distribution(ctx, "log_tag", distributionSince)
	if err != nil {
		return DashboardMetrics{}, err
	}

	return metrics, nil
}

func (s *ClickHouseStore) clickHouseDiskUsedBytes(ctx context.Context) (uint64, error) {
	var bytes uint64
	err := s.conn.QueryRow(ctx, ClickHouseDiskUsageSQL()).Scan(&bytes)
	return bytes, err
}

func ClickHouseDiskUsageSQL() string {
	return `SELECT toUInt64(coalesce(sum(bytes_on_disk), 0))
FROM system.parts
WHERE active
  AND database = currentDatabase()
  AND table IN ('nat_logs', 'ingest_dates', 'ingest_files', 'app_settings', 'log_sources')`
}

func (s *ClickHouseStore) countRowsForDate(ctx context.Context, date time.Time) (uint64, error) {
	var count uint64
	err := s.conn.QueryRow(ctx, "SELECT count() FROM nat_logs WHERE log_date = ?", startOfDay(date)).Scan(&count)
	return count, err
}

func (s *ClickHouseStore) distribution(ctx context.Context, column string, since time.Time) ([]DistributionItem, error) {
	sql, args, err := distributionSQL(column, since)
	if err != nil {
		return nil, err
	}

	rows, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]DistributionItem, 0, 10)
	for rows.Next() {
		var item DistributionItem
		if err := rows.Scan(&item.Name, &item.Value); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func distributionSQL(column string, since time.Time) (string, []any, error) {
	switch column {
	case "src_ip", "dst_ip", "nat_ip", "log_tag":
	default:
		return "", nil, fmt.Errorf("unsupported distribution column %q", column)
	}

	args := make([]any, 0, 1)
	where := ""
	if !since.IsZero() {
		where = " WHERE log_date >= ?"
		args = append(args, startOfDay(since))
	}

	sql := fmt.Sprintf("SELECT toString(%s) AS name, count() AS value FROM nat_logs%s GROUP BY %s ORDER BY value DESC LIMIT 10", column, where, column)
	return sql, args, nil
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	text := ip.String()
	if strings.HasPrefix(text, "::ffff:") {
		return strings.TrimPrefix(text, "::ffff:")
	}
	return text
}

func ClickHouseDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS app_settings
(
    key String,
    value String,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY key
ORDER BY key`,
		`CREATE TABLE IF NOT EXISTS log_sources
(
    source_id String,
    log_dir String,
    log_tag LowCardinality(String),
    enabled UInt8 DEFAULT 1,
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY source_id
ORDER BY source_id`,
		`CREATE TABLE IF NOT EXISTS ingest_dates
(
    source_id String,
    log_tag LowCardinality(String),
    log_date Date,
    status LowCardinality(String) DEFAULT 'pending',
    files_total UInt64 DEFAULT 0,
    files_done UInt64 DEFAULT 0,
    rows_imported UInt64 DEFAULT 0,
    bytes_total UInt64 DEFAULT 0,
    bytes_done UInt64 DEFAULT 0,
    current_file String DEFAULT '',
    progress_pct Float64 DEFAULT 0,
    max_visible_timestamp DateTime DEFAULT toDateTime(0),
    retry_count UInt8 DEFAULT 0,
    next_retry_at DateTime DEFAULT toDateTime(0),
    started_at DateTime DEFAULT toDateTime(0),
    finished_at DateTime DEFAULT toDateTime(0),
    error String DEFAULT '',
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY (source_id, log_date)
ORDER BY (source_id, log_date)`,
		`CREATE TABLE IF NOT EXISTS ingest_files
(
    path String,
    source_id String,
    log_tag LowCardinality(String),
    log_date Date,
    size_bytes UInt64 DEFAULT 0,
    mtime DateTime DEFAULT toDateTime(0),
    status LowCardinality(String) DEFAULT 'pending',
    rows_imported UInt64 DEFAULT 0,
    bytes_total UInt64 DEFAULT 0,
    bytes_done UInt64 DEFAULT 0,
    progress_pct Float64 DEFAULT 0,
    stable_seen_count UInt8 DEFAULT 0,
    first_seen_at DateTime DEFAULT toDateTime(0),
    retry_count UInt8 DEFAULT 0,
    next_retry_at DateTime DEFAULT toDateTime(0),
    started_at DateTime DEFAULT toDateTime(0),
    finished_at DateTime DEFAULT toDateTime(0),
    error String DEFAULT '',
    updated_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
PRIMARY KEY path
ORDER BY path`,
		`CREATE TABLE IF NOT EXISTS nat_logs
(
    source_id String CODEC(ZSTD(3)),
    log_tag LowCardinality(String) CODEC(ZSTD(3)),
    log_date Date CODEC(DoubleDelta, ZSTD(3)),
    timestamp DateTime CODEC(DoubleDelta, ZSTD(3)),
    src_ip IPv4 CODEC(ZSTD(3)),
    src_port UInt16 CODEC(T64, ZSTD(3)),
    dst_ip IPv4 CODEC(ZSTD(3)),
    dst_port UInt16 CODEC(T64, ZSTD(3)),
    nat_ip IPv4 CODEC(ZSTD(3)),
    nat_port UInt16 CODEC(T64, ZSTD(3)),
    protocol LowCardinality(String) CODEC(ZSTD(3)),
    action LowCardinality(String) DEFAULT 'ALLOW' CODEC(ZSTD(3)),
    source_file LowCardinality(String) CODEC(ZSTD(3)),
    source_offset UInt64 CODEC(Delta, ZSTD(3)),
    batch_id String DEFAULT '' CODEC(ZSTD(3)),
    ingested_at DateTime DEFAULT now() CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = MergeTree
PARTITION BY log_date
ORDER BY (log_date, source_id, src_ip, timestamp)
SETTINGS index_granularity = 8192`,
	}
}
