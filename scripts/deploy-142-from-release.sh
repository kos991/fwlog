#!/usr/bin/env bash
set -euo pipefail

repo="${GITHUB_REPOSITORY:-kos991/fwlog}"
version="${1:?usage: deploy-142-from-release.sh <version>}"
asset="${2:-nat-query-service_linux_amd64}"
base_url="https://github.com/${repo}/releases/download/${version}"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

install -d /opt/nat-query /data/sangfor_fw_log /data/index /data/export

curl -fL --retry 3 --retry-delay 5 \
  "${base_url}/${asset}" \
  -o "${work_dir}/nat-query-service"

cat > "${work_dir}/nat-query-service.service" <<'UNITEOF'
[Unit]
Description=NAT Query Service - High-Performance Network Log Analysis
After=network-online.target fwlog-clickhouse.service
Requires=network-online.target fwlog-clickhouse.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/nat-query
Environment="LOG_DIR=/data/sangfor_fw_log"
Environment="LOG_TAG=深信服 NAT"
Environment="PORT=8080"
Environment="CLICKHOUSE_ADDR=127.0.0.1:9000"
Environment="CLICKHOUSE_DATABASE=default"
Environment="CUSTOM_IP_MAP=/opt/nat-query/custom_ip_map.csv"
Environment="GEOIP_DB=/data/index/GeoLite2-City.mmdb"
Environment="AUTO_SCAN_ENABLED=false"
Environment="GIN_MODE=release"
ExecStart=/opt/nat-query/nat-query-service
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
StartLimitInterval=60s
StartLimitBurst=3
LimitNOFILE=65536
LimitNPROC=4096
LimitMEMLOCK=infinity
Nice=-5
IOSchedulingClass=best-effort
IOSchedulingPriority=2
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/data /opt/nat-query
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
RestrictNamespaces=true
StandardOutput=journal
StandardError=journal
SyslogIdentifier=nat-query-service

[Install]
WantedBy=multi-user.target
UNITEOF

if systemctl is-active --quiet nat-query-service 2>/dev/null; then
  systemctl stop nat-query-service
fi

install -m 0755 "${work_dir}/nat-query-service" /opt/nat-query/nat-query-service
install -m 0644 "${work_dir}/nat-query-service.service" /etc/systemd/system/nat-query-service.service

systemctl daemon-reload
systemctl enable nat-query-service
systemctl restart nat-query-service
systemctl is-active nat-query-service
