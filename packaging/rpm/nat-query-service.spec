%global package_name %{?fwlog_package_name}%{!?fwlog_package_name:nat-query-service}
%global package_summary %{?fwlog_package_summary}%{!?fwlog_package_summary:FWLog NAT query service with embedded ClickHouse}
%global package_description %{?fwlog_package_description}%{!?fwlog_package_description:FWLog NAT query service bundled with a private ClickHouse runtime for server installation.}
%global include_clickhouse 0%{?fwlog_include_clickhouse}

Name: %{package_name}
Version: %{fwlog_version}
Release: 1
Summary: %{package_summary}
License: Proprietary
URL: https://github.com/kos991/fwlog
Source0: nat-query-service-root.tar.gz
BuildArch: x86_64
Requires: systemd
AutoReqProv: no

%description
%{package_description}

%prep
%setup -q -n nat-query-service-root

%build

%install
mkdir -p %{buildroot}
cp -a . %{buildroot}/

%pre
if [ "$1" -ge 2 ]; then
    backup="/data/nat-query/backups/app_settings-before-package.tsv"
    client="/opt/nat-query/clickhouse/bin/clickhouse"
    mkdir -p "$(dirname "$backup")"
    chmod 700 "$(dirname "$backup")" || true
    if [ -x "$client" ]; then
        if "$client" client --query "SELECT key, value, now() FROM app_settings FINAL FORMAT TabSeparated" > "$backup.tmp" 2>/tmp/fwlog-pre-backup.err; then
            mv "$backup.tmp" "$backup"
            chmod 600 "$backup"
        else
            cat /tmp/fwlog-pre-backup.err >> /data/nat-query/backups/backup-failed.log 2>/dev/null || true
            rm -f "$backup.tmp"
            echo "app_settings 备份失败，已中止升级（如需跳过请先停止 ClickHouse 后重试）" >&2
            exit 1
        fi
    fi
fi
exit 0

%post
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
%if %{include_clickhouse}
    systemctl enable fwlog-clickhouse.service nat-query-service.service || true
%else
    systemctl enable nat-query-service.service || true
%endif
    if [ -d /run/systemd/system ]; then
%if %{include_clickhouse}
        systemctl restart fwlog-clickhouse.service
        client="/opt/nat-query/clickhouse/bin/clickhouse"
        for i in $(seq 1 60); do
            "$client" client --query "SELECT 1" >/dev/null 2>&1 && break
            sleep 1
        done
        if ! "$client" client --query "SELECT 1" >/dev/null 2>&1; then
            echo "ClickHouse 启动失败，中止安装" >&2
            exit 1
        fi
%else
        client="/opt/nat-query/clickhouse/bin/clickhouse"
%endif
        backup="/data/nat-query/backups/app_settings-before-package.tsv"
        if [ -s "$backup" ] && [ -x "$client" ]; then
            if ! "$client" client --query "INSERT INTO app_settings (key, value, updated_at) FORMAT TabSeparated" < "$backup" >/dev/null 2>&1; then
                echo "app_settings 恢复失败，备份文件保留在 $backup" >&2
            fi
        fi
        systemctl restart nat-query-service.service || true
    fi
fi

%preun
if [ "$1" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop nat-query-service.service || true
%if %{include_clickhouse}
    systemctl stop fwlog-clickhouse.service || true
    systemctl disable nat-query-service.service fwlog-clickhouse.service || true
%else
    systemctl disable nat-query-service.service || true
%endif
fi

%postun
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

%files
%dir /opt/nat-query
/opt/nat-query/nat-query-service
%if %{include_clickhouse}
%dir /opt/nat-query/clickhouse
%dir /opt/nat-query/clickhouse/bin
/opt/nat-query/clickhouse/bin/clickhouse
%dir /opt/nat-query/clickhouse/etc
%config(noreplace) /opt/nat-query/clickhouse/etc/config.xml
%config(noreplace) /opt/nat-query/clickhouse/etc/users.xml
%dir /opt/nat-query/clickhouse/data
%dir /opt/nat-query/clickhouse/tmp
%dir /opt/nat-query/clickhouse/user_files
%dir /opt/nat-query/clickhouse/format_schemas
%dir /opt/nat-query/clickhouse/log
/etc/systemd/system/fwlog-clickhouse.service
%endif
/etc/systemd/system/nat-query-service.service
%dir /data
%dir /data/sangfor_fw_log
%dir /data/index
/data/index/GeoLite2-City.mmdb
%dir /data/export
