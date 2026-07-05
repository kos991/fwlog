Name: nat-query-service
Version: %{fwlog_version}
Release: 1
Summary: FWLog NAT query service with embedded ClickHouse
License: Proprietary
URL: https://github.com/kos991/fwlog
Source0: nat-query-service-root.tar.gz
BuildArch: x86_64
Requires: systemd
AutoReqProv: no

%description
FWLog NAT query service bundled with a private ClickHouse runtime for server installation.

%prep
%setup -q -n nat-query-service-root

%build

%install
mkdir -p %{buildroot}
cp -a . %{buildroot}/

%post
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable fwlog-clickhouse.service nat-query-service.service || true
    if [ -d /run/systemd/system ]; then
        systemctl restart fwlog-clickhouse.service
        systemctl restart nat-query-service.service
    fi
fi

%preun
if [ "$1" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop nat-query-service.service || true
    systemctl stop fwlog-clickhouse.service || true
    systemctl disable nat-query-service.service fwlog-clickhouse.service || true
fi

%postun
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

%files
%dir /opt/nat-query
/opt/nat-query/nat-query-service
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
/etc/systemd/system/nat-query-service.service
%dir /data
%dir /data/sangfor_fw_log
%dir /data/index
/data/index/GeoLite2-City.mmdb
%dir /data/export
