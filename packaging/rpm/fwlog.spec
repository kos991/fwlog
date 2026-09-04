%global package_name %{?fwlog_package_name}%{!?fwlog_package_name:fwlog}
%global package_summary %{?fwlog_package_summary}%{!?fwlog_package_summary:FWLog NAT query service with embedded ClickHouse}
%global package_description %{?fwlog_package_description}%{!?fwlog_package_description:FWLog NAT query service bundled with a private ClickHouse runtime for server installation.}
%global include_clickhouse 0%{?fwlog_include_clickhouse}

Name: %{package_name}
Version: %{fwlog_version}
Release: 1
Summary: %{package_summary}
License: Proprietary
URL: https://github.com/kos991/fwlog
Source0: fwlog-root.tar.gz
BuildArch: x86_64
Requires: systemd
Provides: fwlog = %{version}-%{release}
Obsoletes: fwlog < %{version}-%{release}
AutoReqProv: no

%description
%{package_description}

%prep
%setup -q -n fwlog-root

%build

%install
mkdir -p %{buildroot}
cp -a . %{buildroot}/

%pre
if [ "$1" -ge 1 ]; then
    backup="/data/fwlog/backups/app_settings-before-package.tsv"
    client=""
%if %{include_clickhouse}
    client="/opt/fwlog/clickhouse/bin/clickhouse"
%else
    for candidate in /opt/fwlog/clickhouse/bin/clickhouse /opt/nat-query/clickhouse/bin/clickhouse /usr/bin/clickhouse /usr/bin/clickhouse-client; do
        if [ -x "$candidate" ]; then client="$candidate"; break; fi
    done
%endif
    clickhouse_query() {
        case "$client" in
            */clickhouse-client) "$client" --query "$1" ;;
            *) "$client" client --query "$1" ;;
        esac
    }
    mkdir -p "$(dirname "$backup")"
    chmod 700 "$(dirname "$backup")" || true
    if [ -x "$client" ]; then
        if clickhouse_query "SELECT key, value, now() FROM app_settings FINAL FORMAT TabSeparated" > "$backup.tmp" 2>/tmp/fwlog-pre-backup.err; then
            mv "$backup.tmp" "$backup"
            chmod 600 "$backup"
        else
            cat /tmp/fwlog-pre-backup.err >> /data/fwlog/backups/backup-failed.log 2>/dev/null || true
            rm -f "$backup.tmp"
            echo "app_settings 备份失败，已终止升级（如需跳过请先停止 ClickHouse 后重试）" >&2
            exit 1
        fi
    fi
fi
exit 0

%post
if [ ! -f /etc/fwlog/admin-password ]; then
    mkdir -p /etc/fwlog
    chmod 700 /etc/fwlog
    umask 077
    od -An -N24 -tx1 /dev/urandom | tr -d '[:space:]' > /etc/fwlog/admin-password
    chmod 600 /etc/fwlog/admin-password
    echo "FWLog 初始管理员密码已写入 /etc/fwlog/admin-password（仅 root 可读）" >&2
fi
if command -v systemctl >/dev/null 2>&1; then
    clickhouse_query() {
        case "$client" in
            */clickhouse-client) "$client" --query "$1" ;;
            *) "$client" client --query "$1" ;;
        esac
    }
    systemctl daemon-reload || true
%if %{include_clickhouse}
    systemctl enable fwlog-clickhouse.service fwlog.service || true
%else
    systemctl enable fwlog.service || true
%endif
    if [ -d /run/systemd/system ]; then
        embedded_clickhouse=false
        if systemctl cat fwlog-clickhouse.service >/dev/null 2>&1; then
            embedded_clickhouse=true
        fi
        if [ "$embedded_clickhouse" = "true" ]; then
            systemctl start fwlog-clickhouse.service
            client=""
            for candidate in /opt/fwlog/clickhouse/bin/clickhouse /opt/nat-query/clickhouse/bin/clickhouse; do
                if [ -x "$candidate" ]; then client="$candidate"; break; fi
            done
            if [ -z "$client" ]; then
                echo "ClickHouse client not found after starting fwlog-clickhouse.service" >&2
                exit 1
            fi
            for i in $(seq 1 60); do
                clickhouse_query "SELECT 1" >/dev/null 2>&1 && break
                sleep 1
            done
            if ! clickhouse_query "SELECT 1" >/dev/null 2>&1; then
                echo "ClickHouse did not become ready after package installation" >&2
                exit 1
            fi
        else
            client=""
            for candidate in /opt/fwlog/clickhouse/bin/clickhouse /opt/nat-query/clickhouse/bin/clickhouse /usr/bin/clickhouse /usr/bin/clickhouse-client; do
                if [ -x "$candidate" ]; then client="$candidate"; break; fi
            done
        fi
        backup="/data/fwlog/backups/app_settings-before-package.tsv"
        if [ -s "$backup" ] && [ -x "$client" ]; then
            if ! clickhouse_query "INSERT INTO app_settings (key, value, updated_at) FORMAT TabSeparated" < "$backup" >/dev/null 2>&1; then
                echo "app_settings 恢复失败，备份文件保留在 $backup" >&2
            fi
        fi
        if [ -x "$client" ]; then
            has_nat_logs="$(clickhouse_query "SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = 'nat_logs' FORMAT TSV")"
            if [ "$has_nat_logs" = "1" ]; then
                BACKFILL_CHECKPOINT=/data/fwlog/backups/dashboard-backfill-checkpoint.tsv \
                    bash /opt/fwlog/backfill-dashboard-aggregates.sh \
                    >> /data/fwlog/backups/dashboard-backfill.log 2>&1
            else
                systemctl --job-mode=ignore-dependencies stop fwlog.service || true
            fi
        else
            systemctl --job-mode=ignore-dependencies stop fwlog.service || true
        fi
        systemctl start fwlog.service
    fi
fi

%preun
if [ "$1" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop fwlog.service || true
%if %{include_clickhouse}
    systemctl stop fwlog-clickhouse.service || true
    systemctl disable fwlog.service fwlog-clickhouse.service || true
%else
    systemctl disable fwlog.service || true
%endif
fi

%postun
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

%files
%dir /opt/fwlog
/opt/fwlog/fwlog
/opt/fwlog/backfill-dashboard-aggregates.sh
/opt/fwlog/VERSION
%if %{include_clickhouse}
/opt/fwlog/RUNTIME_VERSION
%dir /opt/fwlog/clickhouse
%dir /opt/fwlog/clickhouse/bin
/opt/fwlog/clickhouse/bin/clickhouse
%dir /opt/fwlog/clickhouse/etc
%config(noreplace) /opt/fwlog/clickhouse/etc/config.xml
%config(noreplace) /opt/fwlog/clickhouse/etc/users.xml
%dir /opt/fwlog/clickhouse/data
%dir /opt/fwlog/clickhouse/tmp
%dir /opt/fwlog/clickhouse/user_files
%dir /opt/fwlog/clickhouse/format_schemas
%dir /opt/fwlog/clickhouse/log
/etc/systemd/system/fwlog-clickhouse.service
%endif
/etc/systemd/system/fwlog.service
%dir /data
%dir /data/sangfor_fw_log
%dir /data/index
/data/index/GeoLite2-City.mmdb
%dir /data/export
