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

curl -fL --retry 3 --retry-delay 5 \
  "${base_url}/nat-query-service.service" \
  -o "${work_dir}/nat-query-service.service"

install -m 0755 "${work_dir}/nat-query-service" /opt/nat-query/nat-query-service
install -m 0644 "${work_dir}/nat-query-service.service" /etc/systemd/system/nat-query-service.service

systemctl daemon-reload
systemctl enable nat-query-service
systemctl restart nat-query-service
systemctl is-active nat-query-service
