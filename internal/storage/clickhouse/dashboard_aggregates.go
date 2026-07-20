package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	dashboardTotalsTable  = "dashboard_daily_totals"
	dashboardIPTable      = "dashboard_daily_ip_counts"
	dashboardQuerySetting = " SETTINGS max_threads = 1"
)

// DashboardAggregateDDL creates the aggregate targets and the views that keep
// them current for new nat_logs inserts. Historical data is loaded separately.
func DashboardAggregateDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS dashboard_daily_totals
(
    log_date Date,
    source_id String,
    log_tag LowCardinality(String),
    rows UInt64
)
ENGINE = SummingMergeTree
PARTITION BY (source_id, log_date)
ORDER BY (log_date, source_id, log_tag)`,
		`CREATE TABLE IF NOT EXISTS dashboard_daily_ip_counts
(
    log_date Date,
    source_id String,
    log_tag LowCardinality(String),
    dimension LowCardinality(String),
    address IPv6,
    rows UInt64
)
ENGINE = SummingMergeTree
PARTITION BY (source_id, log_date)
ORDER BY (dimension, log_date, source_id, address, log_tag)`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS dashboard_daily_totals_mv
TO dashboard_daily_totals AS
SELECT log_date, source_id, log_tag, count() AS rows
FROM nat_logs
GROUP BY log_date, source_id, log_tag`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS dashboard_daily_src_ip_mv
TO dashboard_daily_ip_counts AS
SELECT log_date, source_id, log_tag, 'src_ip' AS dimension, src_ip AS address, count() AS rows
FROM nat_logs
GROUP BY log_date, source_id, log_tag, dimension, address`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS dashboard_daily_dst_ip_mv
TO dashboard_daily_ip_counts AS
SELECT log_date, source_id, log_tag, 'dst_ip' AS dimension, dst_ip AS address, count() AS rows
FROM nat_logs
GROUP BY log_date, source_id, log_tag, dimension, address`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS dashboard_daily_dst_subnet_mv
TO dashboard_daily_ip_counts AS
SELECT log_date, source_id, log_tag, 'dst_subnet' AS dimension, toIPv6(cutIPv6(dst_ip, 8, 1)) AS address, count() AS rows
FROM nat_logs
GROUP BY log_date, source_id, log_tag, dimension, address`,
	}
}

func DashboardTotalRowsSQL() string {
	return "SELECT coalesce(sum(rows), 0) FROM dashboard_daily_totals" + dashboardQuerySetting
}

func DashboardRowsForDateSQL() string {
	return "SELECT coalesce(sum(rows), 0) FROM dashboard_daily_totals WHERE log_date = ?" + dashboardQuerySetting
}

func dashboardRankingSQL(dimension string, since time.Time, sourceID string) (string, []any, error) {
	if dimension != "src_ip" && dimension != "dst_ip" && dimension != "dst_subnet" && dimension != "log_tag" {
		return "", nil, fmt.Errorf("unsupported dashboard ranking dimension %q", dimension)
	}
	table := dashboardIPTable
	name := "toString(address)"
	conditions := []string{"dimension = ?"}
	args := []any{dimension}
	if dimension == "log_tag" {
		table = dashboardTotalsTable
		name = "log_tag"
		conditions = conditions[:0]
		args = args[:0]
	}
	if !since.IsZero() {
		conditions = append(conditions, "log_date >= ?")
		args = append(args, startOfDay(since))
	}
	if sourceID = strings.TrimSpace(sourceID); sourceID != "" {
		conditions = append(conditions, "source_id = ?")
		args = append(args, sourceID)
	}
	limit := ""
	if dimension != "dst_subnet" {
		limit = "\nLIMIT 10"
	}
	return fmt.Sprintf("SELECT %s AS name, sum(rows) AS value FROM %s WHERE %s GROUP BY %s ORDER BY value DESC%s%s", name, table, strings.Join(conditions, " AND "), name, limit, dashboardQuerySetting), args, nil
}

func (s *ClickHouseStore) DashboardSummaryMetrics(ctx context.Context) (DashboardMetrics, error) {
	var metrics DashboardMetrics
	var err error
	metrics.ClickHouseDiskUsedBytes, err = s.clickHouseDiskUsedBytes(ctx)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.SystemHealth.Database, err = s.databaseHealth(ctx, metrics.ClickHouseDiskUsedBytes)
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
	metrics.LogTrend, err = s.dailyLogTrend(ctx, time.Now())
	if err != nil {
		return DashboardMetrics{}, err
	}
	return metrics, nil
}

func (s *ClickHouseStore) DashboardRankingMetrics(ctx context.Context, since time.Time, sourceID string) (DashboardMetrics, error) {
	var metrics DashboardMetrics
	var err error
	metrics.TopSourceIPs, err = s.dashboardDistribution(ctx, "src_ip", since, sourceID)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.TopDestinationIPs, err = s.dashboardDistribution(ctx, "dst_ip", since, sourceID)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.DestinationSubnets, err = s.dashboardDistribution(ctx, "dst_subnet", since, sourceID)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.LogTagDistribution, err = s.dashboardDistribution(ctx, "log_tag", since, sourceID)
	if err != nil {
		return DashboardMetrics{}, err
	}
	return metrics, nil
}

func (s *ClickHouseStore) dashboardDistribution(ctx context.Context, dimension string, since time.Time, sourceID string) ([]DistributionItem, error) {
	sql, args, err := dashboardRankingSQL(dimension, since, sourceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DistributionItem, 0, 10)
	if dimension == "dst_subnet" {
		items = make([]DistributionItem, 0, 1024)
	}
	for rows.Next() {
		var item DistributionItem
		if err := rows.Scan(&item.Name, &item.Value); err != nil {
			return nil, err
		}
		if dimension != "log_tag" {
			item.Name = normalizeIPAddress(item.Name)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
