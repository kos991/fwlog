#!/usr/bin/env bash
set -euo pipefail

repo="${GITHUB_REPOSITORY:-kos991/fwlog}"
version="${1:?usage: deploy-142-from-release.sh <version>}"
asset="${2:-fwlog_linux_amd64}"
base_url="https://github.com/${repo}/releases/download/${version}"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

install -d /opt/fwlog /data/sangfor_fw_log /data/index /data/export

if [[ ! -f /etc/fwlog/admin-password ]]; then
  : "${ADMIN_PASSWORD:?首次部署请通过 ADMIN_PASSWORD 环境变量设置管理员密码}"
  install -d -m 0700 /etc/fwlog
  printf '%s' "$ADMIN_PASSWORD" > "${work_dir}/admin-password"
fi

curl -fL --retry 3 --retry-delay 5 \
  "${base_url}/${asset}" \
  -o "${work_dir}/fwlog"
curl -fL --retry 3 --retry-delay 5 \
  "${base_url}/checksums.txt" \
  -o "${work_dir}/checksums.txt"

expected_hash="$(awk -v name="$asset" '
  {
    file = $2
    sub(/^\*/, "", file)
    count = split(file, parts, "/")
    if (parts[count] == name) {
      print $1
      exit
    }
  }
' "${work_dir}/checksums.txt")"
if [[ ! "$expected_hash" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "checksums.txt 中缺少 ${asset} 的有效 SHA256" >&2
  exit 1
fi
(cd "$work_dir" && printf '%s  fwlog\n' "$expected_hash" | sha256sum -c -)

cat > "${work_dir}/fwlog.service" <<'UNITEOF'
[Unit]
Description=NAT Query Service - High-Performance Network Log Analysis
After=network-online.target fwlog-clickhouse.service
Requires=network-online.target fwlog-clickhouse.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/fwlog
Environment="LOG_DIR=/data/sangfor_fw_log"
Environment="LOG_TAG=娣变俊鏈?NAT"
Environment="PORT=8080"
Environment="CLICKHOUSE_ADDR=127.0.0.1:9000"
Environment="CLICKHOUSE_DATABASE=default"
Environment="CUSTOM_IP_MAP=/opt/fwlog/custom_ip_map.csv"
Environment="GEOIP_DB=/data/index/GeoLite2-City.mmdb"
Environment="AUTO_SCAN_ENABLED=false"
Environment="GIN_MODE=release"
Environment="ADMIN_PASSWORD_FILE=/etc/fwlog/admin-password"
ExecStart=/opt/fwlog/fwlog
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
ReadWritePaths=/data /opt/fwlog
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
RestrictNamespaces=true
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fwlog

[Install]
WantedBy=multi-user.target
UNITEOF

if systemctl is-active --quiet fwlog 2>/dev/null; then
  systemctl stop fwlog
fi

if [[ -f "${work_dir}/admin-password" ]]; then
  install -m 0600 "${work_dir}/admin-password" /etc/fwlog/admin-password
fi

install -m 0755 "${work_dir}/fwlog" /opt/fwlog/fwlog
install -m 0644 "${work_dir}/fwlog.service" /etc/systemd/system/fwlog.service

systemctl daemon-reload
systemctl enable fwlog
systemctl restart fwlog
systemctl is-active fwlog
