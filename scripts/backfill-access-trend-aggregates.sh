#!/usr/bin/env bash
set -euo pipefail

database="${CLICKHOUSE_DATABASE:-default}"
checkpoint="${BACKFILL_CHECKPOINT:-/opt/fwlog/access-trend-backfill.checkpoint}"
started_at="$(date '+%Y-%m-%d %H:%M:%S')"
start_date="${START_DATE:-$(date -d '6 days ago' '+%Y-%m-%d')}"
end_date="${END_DATE:-$(date '+%Y-%m-%d')}"
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

validate_source_id() {
    local source="$1"
    if [[ -z "$source" || "$source" == *"'"* || "$source" == *"\\"* || "$source" == *$'\n'* || "$source" == *$'\r'* || "$source" == *$'\t'* ]]; then
        echo "非法来源标识: $source" >&2
        return 1
    fi
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
    ch "DROP TABLE IF EXISTS access_trend_daily_flows_staging;
CREATE TABLE access_trend_daily_flows_staging
(
    log_date Date,
    source_id String,
    src_ip IPv6,
    src_port UInt16,
    dst_ip IPv6,
    dst_port UInt16,
    protocol LowCardinality(String),
    rows_state AggregateFunction(sum, UInt64),
    last_seen_state AggregateFunction(max, DateTime),
    nat_state AggregateFunction(argMax, Tuple(IPv6, UInt16), Tuple(DateTime, String, UInt64))
)
ENGINE = AggregatingMergeTree
PARTITION BY (source_id, log_date)
ORDER BY (log_date, source_id, src_ip, src_port, dst_ip, dst_port, protocol);"
}

backfill_pair() {
    local source="$1"
    local log_date="$2"
    validate_source_id "$source"
    [[ "$log_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "非法日期: $log_date" >&2; return 1; }

    ch "ALTER TABLE access_trend_daily_flows_staging DROP PARTITION ('$source', '$log_date');
INSERT INTO access_trend_daily_flows_staging
SELECT log_date, source_id, src_ip, src_port, dst_ip, dst_port, protocol,
       sumState(toUInt64(1)), maxState(timestamp),
       argMaxState(tuple(nat_ip, nat_port), tuple(timestamp, toString(source_file), source_offset))
FROM nat_logs
WHERE source_id = '$source' AND log_date = '$log_date'
GROUP BY log_date, source_id, src_ip, src_port, dst_ip, dst_port, protocol
SETTINGS max_threads = 1;"

    local counts raw aggregated
    counts="$(ch "SELECT
toString((SELECT count() FROM nat_logs WHERE source_id = '$source' AND log_date = '$log_date')),
toString((SELECT sumMerge(rows_state) FROM access_trend_daily_flows_staging WHERE source_id = '$source' AND log_date = '$log_date'))
FORMAT TSV")"
    IFS=$'\t' read -r raw aggregated <<<"$counts"
    if [[ "$raw" != "$aggregated" ]]; then
        echo "访问趋势聚合对账失败 source=$source date=$log_date raw=$raw aggregated=$aggregated" >&2
        return 1
    fi
    printf '%s\t%s\t%s\n' "$source" "$log_date" "$raw" | tee -a "$checkpoint"
}

backfill_all() {
    while IFS=$'\t' read -r source log_date; do
        [[ -n "$source" ]] && backfill_pair "$source" "$log_date"
    done < <(ch "SELECT source_id, toString(log_date)
FROM nat_logs
WHERE log_date >= '$start_date' AND log_date <= '$end_date'
GROUP BY source_id, log_date
ORDER BY log_date, source_id
FORMAT TSV")
}

refresh_changed_pairs() {
    while IFS=$'\t' read -r source log_date; do
        [[ -n "$source" ]] && backfill_pair "$source" "$log_date"
    done < <(ch "SELECT source_id, toString(log_date)
FROM nat_logs
WHERE ingested_at >= toDateTime('$started_at')
  AND log_date >= '$start_date' AND log_date <= '$end_date'
GROUP BY source_id, log_date
ORDER BY log_date, source_id
FORMAT TSV")
}

create_view() {
    ch "CREATE MATERIALIZED VIEW access_trend_daily_flows_mv
TO access_trend_daily_flows AS
SELECT log_date, source_id, src_ip, src_port, dst_ip, dst_port, protocol,
       sumState(toUInt64(1)) AS rows_state,
       maxState(timestamp) AS last_seen_state,
       argMaxState(tuple(nat_ip, nat_port), tuple(timestamp, toString(source_file), source_offset)) AS nat_state
FROM nat_logs
GROUP BY log_date, source_id, src_ip, src_port, dst_ip, dst_port, protocol;"
}

switch_tables() {
    systemctl --job-mode=ignore-dependencies stop fwlog.service
    service_stopped=1
    refresh_changed_pairs

    ch "DROP TABLE IF EXISTS access_trend_daily_flows_mv;"
    local suffix exists
    suffix="$(date '+%Y%m%d%H%M%S')"
    exists="$(ch "SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = 'access_trend_daily_flows' FORMAT TSV")"
    if [[ "$exists" == "1" ]]; then
        ch "RENAME TABLE
access_trend_daily_flows TO access_trend_daily_flows_backup_$suffix,
access_trend_daily_flows_staging TO access_trend_daily_flows;"
    else
        ch "RENAME TABLE access_trend_daily_flows_staging TO access_trend_daily_flows;"
    fi
    create_view
    ch "SELECT throwIf(
(SELECT count() FROM nat_logs WHERE log_date >= '$start_date' AND log_date <= '$end_date') !=
(SELECT sumMerge(rows_state) FROM access_trend_daily_flows WHERE log_date >= '$start_date' AND log_date <= '$end_date'),
'access trend totals mismatch after switch')"
    if [[ "${START_SERVICE_AFTER_SWITCH:-1}" == "1" ]]; then
        systemctl start fwlog.service
        service_stopped=0
    fi
    trap - ERR
}

mkdir -p "$(dirname "$checkpoint")"
touch "$checkpoint"
create_staging
backfill_all
switch_tables
echo "访问趋势聚合回填完成: $start_date 至 $end_date"
