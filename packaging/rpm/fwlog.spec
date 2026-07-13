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
if [ "$1" -ge 2 ]; then
    backup="/data/fwlog/backups/app_settings-before-package.tsv"
    client=""
%if %{include_clickhouse}
    client="/opt/fwlog/clickhouse/bin/clickhouse"
%else
    for candidate in /usr/bin/clickhouse-client /usr/bin/clickhouse /opt/fwlog/clickhouse/bin/clickhouse; do
        if [ -x "$candidate" ]; then client="$candidate"; break; fi
    done
%endif
    mkdir -p "$(dirname "$backup")"
    chmod 700 "$(dirname "$backup")" || true
    if [ -x "$client" ]; then
        if "$client" client --query "SELECT key, value, now() FROM app_settings FINAL FORMAT TabSeparated" > "$backup.tmp" 2>/tmp/fwlog-pre-backup.err; then
            mv "$backup.tmp" "$backup"
            chmod 600 "$backup"
        else
            cat /tmp/fwlog-pre-backup.err >> /data/fwlog/backups/backup-failed.log 2>/dev/null || true
            rm -f "$backup.tmp"
            echo "app_settings 澶囦唤澶辫触锛屽凡涓鍗囩骇锛堝闇€璺宠繃璇峰厛鍋滄 ClickHouse 鍚庨噸璇曪級" >&2
            exit 1
        fi
    fi
fi
exit 0

%post
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
%if %{include_clickhouse}
    systemctl enable fwlog-clickhouse.service fwlog.service || true
%else
    systemctl enable fwlog.service || true
%endif
    if [ -d /run/systemd/system ]; then
%if %{include_clickhouse}
        systemctl restart fwlog-clickhouse.service
        client="/opt/fwlog/clickhouse/bin/clickhouse"
        for i in $(seq 1 60); do
            "$client" client --query "SELECT 1" >/dev/null 2>&1 && break
            sleep 1
        done
        if ! "$client" client --query "SELECT 1" >/dev/null 2>&1; then
            echo "ClickHouse did not become ready after package installation" >&2
            exit 1
        fi
%else
        client=""
        for candidate in /usr/bin/clickhouse-client /usr/bin/clickhouse /opt/fwlog/clickhouse/bin/clickhouse; do
            if [ -x "$candidate" ]; then client="$candidate"; break; fi
        done
%endif
        backup="/data/fwlog/backups/app_settings-before-package.tsv"
        if [ -s "$backup" ] && [ -x "$client" ]; then
            if ! "$client" client --query "INSERT INTO app_settings (key, value, updated_at) FORMAT TabSeparated" < "$backup" >/dev/null 2>&1; then
                echo "app_settings 鎭㈠澶辫触锛屽浠芥枃浠朵繚鐣欏湪 $backup" >&2
            fi
        fi
        systemctl restart fwlog.service
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
