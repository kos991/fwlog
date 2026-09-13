package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AccessTrendRow struct {
	TargetIP string
	Count    uint64
}

func accessTrendSQL(from, until time.Time, sourceID string) (string, []any) {
	conditions := []string{"log_date >= ?", "log_date < ?"}
	args := []any{startOfDay(from), startOfDay(until)}
	if sourceID = strings.TrimSpace(sourceID); sourceID != "" {
		conditions = append(conditions, "source_id = ?")
		args = append(args, sourceID)
	}
	sql := fmt.Sprintf(`SELECT toString(address) AS target_ip, sum(rows) AS rows
FROM dashboard_daily_ip_counts
WHERE %s
  AND dimension = 'dst_subnet'
GROUP BY address
ORDER BY rows DESC%s`, strings.Join(conditions, " AND "), dashboardQuerySetting)
	return sql, args
}

func (s *ClickHouseStore) AccessTrendRows(ctx context.Context, from, until time.Time, sourceID string) ([]AccessTrendRow, error) {
	if s == nil || s.conn == nil {
		return nil, fmt.Errorf("clickhouse connection is not initialized")
	}
	sql, args := accessTrendSQL(from, until, sourceID)
	rows, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AccessTrendRow, 0)
	for rows.Next() {
		var item AccessTrendRow
		if err := rows.Scan(&item.TargetIP, &item.Count); err != nil {
			return nil, err
		}
		item.TargetIP = normalizeIPAddress(item.TargetIP)
		result = append(result, item)
	}
	return result, rows.Err()
}
