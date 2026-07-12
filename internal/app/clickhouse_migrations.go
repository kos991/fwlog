package app

import (
	"context"
	"fmt"
	"strings"
)

const (
	natLogsReplacementTable = "nat_logs_source_date"
	natLogsBackupTable      = "nat_logs_date_partition_backup"
)

func normalizePartitionKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	if strings.HasPrefix(value, "tuple(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "tuple("), ")")
	}
	if strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "("), ")")
	}
	return value
}

func natLogsMigrationSQL() []string {
	return []string{
		natLogsTableDDL(natLogsReplacementTable, false),
		"INSERT INTO " + natLogsReplacementTable + " SELECT * FROM nat_logs",
		"RENAME TABLE nat_logs TO " + natLogsBackupTable + ", " + natLogsReplacementTable + " TO nat_logs",
	}
}

func (s *ClickHouseStore) migrateNatLogsSourceDatePartition(ctx context.Context) error {
	partitionKey, err := s.tablePartitionKey(ctx, "nat_logs")
	if err != nil {
		return fmt.Errorf("inspect nat_logs partition key: %w", err)
	}
	if normalizePartitionKey(partitionKey) == "source_id,log_date" {
		return nil
	}

	backupExists, err := s.tableExists(ctx, natLogsBackupTable)
	if err != nil {
		return fmt.Errorf("inspect migration backup: %w", err)
	}
	replacementExists, err := s.tableExists(ctx, natLogsReplacementTable)
	if err != nil {
		return fmt.Errorf("inspect migration replacement: %w", err)
	}
	if backupExists || replacementExists {
		return fmt.Errorf("incomplete nat_logs partition migration detected; keep %s and %s for manual recovery", natLogsBackupTable, natLogsReplacementTable)
	}

	statements := natLogsMigrationSQL()
	if err := s.conn.Exec(ctx, statements[0]); err != nil {
		return fmt.Errorf("create source-date table: %w", err)
	}
	if err := s.conn.Exec(ctx, statements[1]); err != nil {
		return fmt.Errorf("copy nat_logs rows: %w", err)
	}
	if err := s.validateNatLogsMigration(ctx); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, statements[2]); err != nil {
		return fmt.Errorf("swap nat_logs tables: %w", err)
	}
	return nil
}

func (s *ClickHouseStore) tablePartitionKey(ctx context.Context, table string) (string, error) {
	var key string
	err := s.conn.QueryRow(ctx, `SELECT partition_key FROM system.tables WHERE database = currentDatabase() AND name = ?`, table).Scan(&key)
	return key, err
}

func (s *ClickHouseStore) tableExists(ctx context.Context, table string) (bool, error) {
	var count uint64
	err := s.conn.QueryRow(ctx, `SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = ?`, table).Scan(&count)
	return count > 0, err
}

func (s *ClickHouseStore) validateNatLogsMigration(ctx context.Context) error {
	var oldCount, newCount uint64
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM nat_logs").Scan(&oldCount); err != nil {
		return fmt.Errorf("count existing nat_logs: %w", err)
	}
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM "+natLogsReplacementTable).Scan(&newCount); err != nil {
		return fmt.Errorf("count replacement nat_logs: %w", err)
	}
	if oldCount != newCount {
		return fmt.Errorf("nat_logs migration row count mismatch: old=%d new=%d", oldCount, newCount)
	}

	query := `SELECT count() FROM
(SELECT source_id, log_date, count() AS rows_old FROM nat_logs GROUP BY source_id, log_date) AS old
FULL OUTER JOIN
(SELECT source_id, log_date, count() AS rows_new FROM nat_logs_source_date GROUP BY source_id, log_date) AS new
USING (source_id, log_date)
WHERE rows_old != rows_new`
	var mismatches uint64
	if err := s.conn.QueryRow(ctx, query).Scan(&mismatches); err != nil {
		return fmt.Errorf("validate source-date groups: %w", err)
	}
	if mismatches != 0 {
		return fmt.Errorf("nat_logs migration has %d source-date count mismatches", mismatches)
	}
	return nil
}
