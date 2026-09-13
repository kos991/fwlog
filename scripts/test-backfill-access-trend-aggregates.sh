#!/usr/bin/env bash
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/backfill-access-trend-aggregates.sh"
bash -n "$script"
for required in \
    access_trend_daily_flows_staging \
    'AggregateFunction(sum, UInt64)' \
    'argMaxState(tuple(nat_ip, nat_port)' \
    'ALTER TABLE access_trend_daily_flows_staging DROP PARTITION' \
    'sumMerge(rows_state)' \
    'refresh_changed_pairs' \
    'systemctl --job-mode=ignore-dependencies stop fwlog.service' \
    'RENAME TABLE' \
    'trap restart_on_error ERR'; do
    grep -Fq "$required" "$script" || { echo "访问趋势回填脚本缺少: $required" >&2; exit 1; }
done

echo "访问趋势回填脚本静态检查通过"
