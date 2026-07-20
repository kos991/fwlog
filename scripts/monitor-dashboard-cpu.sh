#!/usr/bin/env bash
set -euo pipefail

duration="${1:-600}"
output="${2:-/tmp/fwlog-dashboard-cpu-$(date '+%Y%m%d-%H%M%S').csv}"
echo "timestamp,load1,fwlog_cpu,clickhouse_cpu" > "$output"
for ((second = 0; second < duration; second++)); do
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
    load1="$(awk '{print $1}' /proc/loadavg)"
    fwlog_cpu="$(ps -C fwlog -o %cpu= | awk '{sum += $1} END {printf "%.2f", sum + 0}')"
    clickhouse_cpu="$(ps -C clickhouse -o %cpu= | awk '{sum += $1} END {printf "%.2f", sum + 0}')"
    printf '%s,%s,%s,%s\n' "$timestamp" "$load1" "$fwlog_cpu" "$clickhouse_cpu" | tee -a "$output"
    sleep 1
done
echo "$output"

