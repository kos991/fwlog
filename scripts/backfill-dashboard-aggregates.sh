#!/usr/bin/env bash
set -euo pipefail

database="${CLICKHOUSE_DATABASE:-default}"
checkpoint="${BACKFILL_CHECKPOINT:-/opt/fwlog/dashboard-backfill.checkpoint}"
started_at="$(date '+%Y-%m-%d %H:%M:%S')"
service_stopped=0

find_client() {
    if [[ -n "${CLICKHOUSE_CLIENT:-}" ]]; then
        printf '%s\n' "$CLICKHOUSE_CLIENT"
        return
    fi
    for candidate in /opt/fwlog/clickhouse/bin/clickhouse /usr/bin/clickhouse /usr/bin/clickhouse-client; do
        if [[ -x "$candidate" ]]; then
            printf '%s\n' "$candidate"
            return
        fi
    done
    echo "未找到 ClickHouse 客户端" >&2
    return 1
}

client="$(find_client)"
client_args=(--database "$database")
[[ -n "${CLICKHOUSE_USER:-}" ]] && client_args+=(--user "$CLICKHOUSE_USER")
[[ -n "${CLICKHOUSE_PASSWORD:-}" ]] && client_args+=(--password "$CLICKHOUSE_PASSWORD")

ch() {
    if [[ "$(basename "$client")" == "clickhouse-client" ]]; then
        "$client" "${client_args[@]}" --multiquery --query "$1"
    else
        "$client" client "${client_args[@]}" --multiquery --query "$1"
    fi
}

restart_on_error() {
    if [[ "$service_stopped" == "1" ]]; then
        systemctl start fwlog.service || true
    fi
}
trap restart_on_error ERR

create_staging() {
    ch "CREATE TABLE IF NOT EXISTS dashboard_daily_totals_staging
(
    log_date Date, source_id String, log_tag LowCardinality(String), rows UInt64
)
ENGINE = SummingMergeTree
PARTITION BY (source_id, log_date)
ORDER BY (log_date, source_id, log_tag);
CREATE TABLE IF NOT EXISTS dashboard_daily_ip_counts_staging
(
    log_date Date, source_id String, log_tag LowCardinality(String),
    dimension LowCardinality(String), address IPv6, rows UInt64
)
ENGINE = SummingMergeTree
PARTITION BY (source_id, log_date)
ORDER BY (dimension, log_date, source_id, address, log_tag);"
}

backfill_pair() {
    local source="$1"
    local log_date="$2"
    [[ "$source" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "非法来源标识: $source" >&2; return 1; }
    [[ "$log_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "非法日期: $log_date" >&2; return 1; }

    ch "ALTER TABLE dashboard_daily_totals_staging DROP PARTITION ('$source', '$log_date');
ALTER TABLE dashboard_daily_ip_counts_staging DROP PARTITION ('$source', '$log_date');
INSERT INTO dashboard_daily_totals_staging
SELECT log_date, source_id, log_tag, count() AS rows
FROM nat_logs
WHERE source_id = '$source' AND log_date = '$log_date'
GROUP BY log_date, source_id, log_tag
SETTINGS max_threads = 1;
INSERT INTO dashboard_daily_ip_counts_staging
SELECT log_date, source_id, log_tag, 'src_ip', src_ip, count()
FROM nat_logs WHERE source_id = '$source' AND log_date = '$log_date'
GROUP BY log_date, source_id, log_tag, src_ip
UNION ALL
SELECT log_date, source_id, log_tag, 'dst_ip', dst_ip, count()
FROM nat_logs WHERE source_id = '$source' AND log_date = '$log_date'
GROUP BY log_date, source_id, log_tag, dst_ip
UNION ALL
SELECT log_date, source_id, log_tag, 'dst_subnet', toIPv6(cutIPv6(dst_ip, 8, 1)), count()
FROM nat_logs WHERE source_id = '$source' AND log_date = '$log_date'
GROUP BY log_date, source_id, log_tag, toIPv6(cutIPv6(dst_ip, 8, 1))
SETTINGS max_threads = 1;"

    local counts raw totals src dst subnet
    counts="$(ch "SELECT
toString((SELECT count() FROM nat_logs WHERE source_id = '$source' AND log_date = '$log_date')),
toString((SELECT sum(rows) FROM dashboard_daily_totals_staging WHERE source_id = '$source' AND log_date = '$log_date')),
toString((SELECT sum(rows) FROM dashboard_daily_ip_counts_staging WHERE source_id = '$source' AND log_date = '$log_date' AND dimension = 'src_ip')),
toString((SELECT sum(rows) FROM dashboard_daily_ip_counts_staging WHERE source_id = '$source' AND log_date = '$log_date' AND dimension = 'dst_ip')),
toString((SELECT sum(rows) FROM dashboard_daily_ip_counts_staging WHERE source_id = '$source' AND log_date = '$log_date' AND dimension = 'dst_subnet'))
FORMAT TSV")"
    IFS=$'\t' read -r raw totals src dst subnet <<<"$counts"
    if [[ "$raw" != "$totals" || "$raw" != "$src" || "$raw" != "$dst" || "$raw" != "$subnet" ]]; then
        echo "对账失败 source=$source date=$log_date raw=$raw totals=$totals src=$src dst=$dst subnet=$subnet" >&2
        return 1
    fi
    printf '%s\t%s\t%s\n' "$source" "$log_date" "$raw" | tee -a "$checkpoint"
}

backfill_all() {
	local where="WHERE 1"
	[[ -n "${START_DATE:-}" ]] && where="$where AND log_date >= '${START_DATE}'"
	[[ -n "${END_DATE:-}" ]] && where="$where AND log_date <= '${END_DATE}'"
    while IFS=$'\t' read -r source log_date; do
        [[ -n "$source" ]] && backfill_pair "$source" "$log_date"
    done < <(ch "SELECT source_id, toString(log_date) FROM nat_logs $where GROUP BY source_id, log_date ORDER BY log_date, source_id FORMAT TSV")
}

refresh_changed_pairs() {
    while IFS=$'\t' read -r source log_date; do
        [[ -n "$source" ]] && backfill_pair "$source" "$log_date"
    done < <(ch "SELECT source_id, toString(log_date) FROM nat_logs WHERE ingested_at >= toDateTime('$started_at') GROUP BY source_id, log_date ORDER BY log_date, source_id FORMAT TSV")
}

create_views() {
    ch "CREATE MATERIALIZED VIEW dashboard_daily_totals_mv TO dashboard_daily_totals AS
SELECT log_date, source_id, log_tag, count() AS rows FROM nat_logs GROUP BY log_date, source_id, log_tag;
CREATE MATERIALIZED VIEW dashboard_daily_src_ip_mv TO dashboard_daily_ip_counts AS
SELECT log_date, source_id, log_tag, 'src_ip' AS dimension, src_ip AS address, count() AS rows FROM nat_logs GROUP BY log_date, source_id, log_tag, dimension, address;
CREATE MATERIALIZED VIEW dashboard_daily_dst_ip_mv TO dashboard_daily_ip_counts AS
SELECT log_date, source_id, log_tag, 'dst_ip' AS dimension, dst_ip AS address, count() AS rows FROM nat_logs GROUP BY log_date, source_id, log_tag, dimension, address;
CREATE MATERIALIZED VIEW dashboard_daily_dst_subnet_mv TO dashboard_daily_ip_counts AS
SELECT log_date, source_id, log_tag, 'dst_subnet' AS dimension, toIPv6(cutIPv6(dst_ip, 8, 1)) AS address, count() AS rows FROM nat_logs GROUP BY log_date, source_id, log_tag, dimension, address;"
}

switch_tables() {
    systemctl --job-mode=ignore-dependencies stop fwlog.service
    service_stopped=1
    refresh_changed_pairs
    local raw totals
    raw="$(ch "SELECT count() FROM nat_logs FORMAT TSV")"
    totals="$(ch "SELECT sum(rows) FROM dashboard_daily_totals_staging FORMAT TSV")"
    [[ "$raw" == "$totals" ]] || { echo "全量对账失败 raw=$raw totals=$totals" >&2; return 1; }

    ch "DROP TABLE IF EXISTS dashboard_daily_totals_mv;
DROP TABLE IF EXISTS dashboard_daily_src_ip_mv;
DROP TABLE IF EXISTS dashboard_daily_dst_ip_mv;
DROP TABLE IF EXISTS dashboard_daily_dst_subnet_mv;"
    local suffix exists
    suffix="$(date '+%Y%m%d%H%M%S')"
    exists="$(ch "SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = 'dashboard_daily_totals' FORMAT TSV")"
    if [[ "$exists" == "1" ]]; then
        ch "RENAME TABLE
dashboard_daily_totals TO dashboard_daily_totals_backup_$suffix,
dashboard_daily_ip_counts TO dashboard_daily_ip_counts_backup_$suffix,
dashboard_daily_totals_staging TO dashboard_daily_totals,
dashboard_daily_ip_counts_staging TO dashboard_daily_ip_counts;"
    else
        ch "RENAME TABLE
dashboard_daily_totals_staging TO dashboard_daily_totals,
dashboard_daily_ip_counts_staging TO dashboard_daily_ip_counts;"
    fi
    create_views
    ch "SELECT throwIf(
(SELECT count() FROM nat_logs) != (SELECT sum(rows) FROM dashboard_daily_totals),
'dashboard totals mismatch after switch')"
    trap - ERR
    echo "聚合表切换完成；保持 fwlog 停止，等待部署新二进制"
}

mkdir -p "$(dirname "$checkpoint")"
touch "$checkpoint"
create_staging
backfill_all
switch_tables
