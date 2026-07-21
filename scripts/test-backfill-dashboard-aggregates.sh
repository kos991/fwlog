#!/usr/bin/env bash
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/backfill-dashboard-aggregates.sh"
bash -n "$script"
for required in \
    dashboard_daily_totals_staging \
    dashboard_daily_ip_counts_staging \
    '/opt/nat-query/clickhouse/bin/clickhouse' \
    'SETTINGS max_threads = 1' \
    'ALTER TABLE dashboard_daily_totals_staging DROP PARTITION' \
    'refresh_changed_pairs' \
    'systemctl --job-mode=ignore-dependencies stop fwlog.service' \
    'RENAME TABLE' \
    'throwIf(' \
    'trap restart_on_error ERR'; do
    grep -Fq "$required" "$script" || { echo "回填脚本缺少: $required" >&2; exit 1; }
done

validation_source="$(sed '/^client=/,$d' "$script")"
bash -c "$validation_source"$'\n''validate_source_id "$1"' _ '外网'
if bash -c "$validation_source"$'\n''validate_source_id "$1"' _ "unsafe'source"; then
    echo "来源标识校验不应接受单引号" >&2
    exit 1
fi
echo "回填脚本静态检查通过"
