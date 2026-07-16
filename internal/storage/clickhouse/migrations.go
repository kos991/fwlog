package clickhouse

import (
	"context"
	"fmt"
	"strings"
)

const (
	natLogsReplacementTable       = "nat_logs_dual_stack"
	natLogsBackupTable            = "nat_logs_ipv4_backup"
	legacyNatLogsReplacementTable = "nat_logs_source_date"
)

const natLogsColumns = `source_id, log_tag, log_date, timestamp, src_ip, src_port, dst_ip, dst_port,
    nat_ip, nat_port, protocol, action, source_file, source_offset, batch_id, ingested_at`

const migrationQuerySettings = " SETTINGS max_execution_time = 0, max_rows_to_read = 0"

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
	return natLogsMigrationSQLForSource(true)
}

func natLogsMigrationSQLForSource(ipv4Source bool) []string {
	srcIP := "src_ip"
	dstIP := "dst_ip"
	natIP := "nat_ip"
	if ipv4Source {
		srcIP = "IPv4ToIPv6(src_ip)"
		dstIP = "IPv4ToIPv6(dst_ip)"
		natIP = "IPv4ToIPv6(nat_ip)"
	}

	copySQL := fmt.Sprintf(`INSERT INTO %s (%s)
SELECT source_id, log_tag, log_date, timestamp, %s, src_port, %s, dst_port,
    %s, nat_port, protocol, action, source_file, source_offset, batch_id, ingested_at
FROM nat_logs%s`, natLogsReplacementTable, natLogsColumns, srcIP, dstIP, natIP, migrationQuerySettings)

	return []string{
		natLogsTableDDL(natLogsReplacementTable, false),
		copySQL,
		"RENAME TABLE nat_logs TO " + natLogsBackupTable + ", " + natLogsReplacementTable + " TO nat_logs",
	}
}

func (s *ClickHouseStore) migrateNatLogsDualStackSchema(ctx context.Context) error {
	partitionKey, err := s.tablePartitionKey(ctx, "nat_logs")
	if err != nil {
		return fmt.Errorf("inspect nat_logs partition key: %w", err)
	}
	addressType, err := s.natLogsAddressType(ctx)
	if err != nil {
		return fmt.Errorf("inspect nat_logs address columns: %w", err)
	}
	if normalizePartitionKey(partitionKey) == "source_id,log_date" && addressType == "IPv6" {
		return nil
	}

	legacyReplacementExists, err := s.tableExists(ctx, legacyNatLogsReplacementTable)
	if err != nil {
		return fmt.Errorf("inspect legacy migration replacement: %w", err)
	}
	if legacyReplacementExists {
		return fmt.Errorf("incomplete legacy nat_logs migration detected; keep %s for manual recovery", legacyNatLogsReplacementTable)
	}

	backupExists, err := s.tableExists(ctx, natLogsBackupTable)
	if err != nil {
		return fmt.Errorf("inspect migration backup: %w", err)
	}
	replacementExists, err := s.tableExists(ctx, natLogsReplacementTable)
	if err != nil {
		return fmt.Errorf("inspect migration replacement: %w", err)
	}
	recoverySQL, err := natLogsMigrationRecoverySQL(backupExists, replacementExists)
	if err != nil {
		return err
	}
	if recoverySQL != "" {
		if err := s.conn.Exec(ctx, recoverySQL); err != nil {
			return fmt.Errorf("clean interrupted dual-stack migration: %w", err)
		}
	}

	statements := natLogsMigrationSQLForSource(addressType == "IPv4")
	if err := s.conn.Exec(ctx, statements[0]); err != nil {
		return fmt.Errorf("create dual-stack replacement table: %w", err)
	}
	if err := s.conn.Exec(ctx, statements[1]); err != nil {
		return fmt.Errorf("copy nat_logs rows to dual-stack table: %w", err)
	}
	if err := s.validateNatLogsMigration(ctx); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, statements[2]); err != nil {
		return fmt.Errorf("swap dual-stack nat_logs tables: %w", err)
	}
	return nil
}

func natLogsMigrationRecoverySQL(backupExists, replacementExists bool) (string, error) {
	if !backupExists && replacementExists {
		return "DROP TABLE " + natLogsReplacementTable, nil
	}
	if backupExists {
		return "", fmt.Errorf("incomplete nat_logs dual-stack migration detected; keep %s and %s for manual recovery", natLogsBackupTable, natLogsReplacementTable)
	}
	return "", nil
}

func (s *ClickHouseStore) natLogsAddressType(ctx context.Context) (string, error) {
	rows, err := s.conn.Query(ctx, `SELECT name, type
FROM system.columns
WHERE database = currentDatabase()
  AND table = 'nat_logs'
  AND name IN ('src_ip', 'dst_ip', 'nat_ip')`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	columnTypes := make(map[string]string, 3)
	for rows.Next() {
		var name, columnType string
		if err := rows.Scan(&name, &columnType); err != nil {
			return "", err
		}
		columnTypes[name] = columnType
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(columnTypes) != 3 {
		return "", fmt.Errorf("expected 3 address columns, found %d", len(columnTypes))
	}

	addressType := columnTypes["src_ip"]
	if addressType != "IPv4" && addressType != "IPv6" {
		return "", fmt.Errorf("unsupported src_ip type %q", addressType)
	}
	for _, name := range []string{"dst_ip", "nat_ip"} {
		if columnTypes[name] != addressType {
			return "", fmt.Errorf("mixed address column types: src_ip=%s %s=%s", addressType, name, columnTypes[name])
		}
	}
	return addressType, nil
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
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM nat_logs"+migrationQuerySettings).Scan(&oldCount); err != nil {
		return fmt.Errorf("count existing nat_logs: %w", err)
	}
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM "+natLogsReplacementTable+migrationQuerySettings).Scan(&newCount); err != nil {
		return fmt.Errorf("count replacement nat_logs: %w", err)
	}
	if oldCount != newCount {
		return fmt.Errorf("nat_logs migration row count mismatch: old=%d new=%d", oldCount, newCount)
	}

	query := `SELECT count() FROM
(SELECT source_id, log_date, count() AS rows_old FROM nat_logs GROUP BY source_id, log_date) AS old
FULL OUTER JOIN
(SELECT source_id, log_date, count() AS rows_new FROM nat_logs_dual_stack GROUP BY source_id, log_date) AS replacement
USING (source_id, log_date)
WHERE coalesce(rows_old, 0) != coalesce(rows_new, 0)` + migrationQuerySettings
	var mismatches uint64
	if err := s.conn.QueryRow(ctx, query).Scan(&mismatches); err != nil {
		return fmt.Errorf("validate source-date groups: %w", err)
	}
	if mismatches != 0 {
		return fmt.Errorf("nat_logs migration has %d source-date count mismatches", mismatches)
	}
	return nil
}
