#!/usr/bin/env bash
set -euo pipefail

duration="${1:-600}"
output="${2:-/tmp/fwlog-dashboard-cpu-$(date '+%Y%m%d-%H%M%S').csv}"
cores="$(nproc)"

read_cpu() {
    read -r _ user nice system idle iowait irq softirq steal _ < /proc/stat
    echo "$((user + nice + system + idle + iowait + irq + softirq + steal)) $((idle + iowait))"
}

process_ticks() {
    local name="$1"
    local total=0
    while read -r pid; do
        [[ -r "/proc/$pid/stat" ]] || continue
        read -r user system < <(awk '{print $14, $15}' "/proc/$pid/stat")
        total=$((total + user + system))
    done < <(pgrep -x "$name" || true)
    echo "$total"
}

read -r previous_total previous_idle < <(read_cpu)
previous_fwlog="$(process_ticks fwlog)"
previous_clickhouse="$(process_ticks clickhouse)"
echo "timestamp,system_cpu,fwlog_cpu,clickhouse_cpu" > "$output"
for ((second = 0; second < duration; second++)); do
    sleep 1
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
    read -r total idle < <(read_cpu)
    fwlog="$(process_ticks fwlog)"
    clickhouse="$(process_ticks clickhouse)"
    delta_total=$((total - previous_total))
    delta_idle=$((idle - previous_idle))
    if ((delta_total > 0)); then
        system_cpu="$(awk -v busy="$((delta_total - delta_idle))" -v all="$delta_total" 'BEGIN {printf "%.2f", busy * 100 / all}')"
        fwlog_cpu="$(awk -v ticks="$((fwlog - previous_fwlog))" -v all="$delta_total" -v n="$cores" 'BEGIN {printf "%.2f", ticks * n * 100 / all}')"
        clickhouse_cpu="$(awk -v ticks="$((clickhouse - previous_clickhouse))" -v all="$delta_total" -v n="$cores" 'BEGIN {printf "%.2f", ticks * n * 100 / all}')"
    else
        system_cpu="0.00"; fwlog_cpu="0.00"; clickhouse_cpu="0.00"
    fi
    printf '%s,%s,%s,%s\n' "$timestamp" "$system_cpu" "$fwlog_cpu" "$clickhouse_cpu" | tee -a "$output"
    previous_total="$total"; previous_idle="$idle"
    previous_fwlog="$fwlog"; previous_clickhouse="$clickhouse"
done
echo "$output"

