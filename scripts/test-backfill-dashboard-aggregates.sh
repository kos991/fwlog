#!/usr/bin/env bash
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/backfill-dashboard-aggregates.sh"
bash -n "$script"
for required in \
    dashboard_daily_totals_staging \
    dashboard_daily_ip_counts_staging \
    'SETTINGS max_threads = 1' \
    'ALTER TABLE dashboard_daily_totals_staging DROP PARTITION' \
    'refresh_changed_pairs' \
    'systemctl stop fwlog.service' \
    'RENAME TABLE' \
    'throwIf(' \
    'trap restart_on_error ERR'; do
    grep -Fq "$required" "$script" || { echo "回填脚本缺少: $required" >&2; exit 1; }
done
echo "回填脚本静态检查通过"

