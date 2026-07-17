#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version=""
binary=""
output_dir="$repo_root/dist"
arch="amd64"
mode="${PACKAGING_MODE:-full}"

usage() {
    cat <<'USAGE'
usage: packaging/build-server-packages.sh --version <vX.Y.Z|X.Y.Z> --binary <path> [--output <dir>] [--arch amd64] [--mode full|upgrade]

Environment:
  PACKAGING_MODE        Package mode: full or upgrade. Default: full.
  CLICKHOUSE_VERSION    ClickHouse version to bundle. Default: 25.8.27.1
  CLICKHOUSE_TGZ_URL    Override ClickHouse common-static tarball URL.
  CLICKHOUSE_BINARY     Use an existing ClickHouse binary instead of downloading.
  CLICKHOUSE_CACHE_DIR  Download cache directory. Default: .cache/clickhouse
  GEOIP_DB_PATH         Use an existing GeoLite2-City.mmdb instead of downloading.
  GEOIP_DB_URL          Override GeoLite2-City.mmdb URL. Default: P3TERX latest release.
  GEOIP_CACHE_DIR       Download cache directory. Default: .cache/geoip
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)
            version="${2:-}"
            shift 2
            ;;
        --binary)
            binary="${2:-}"
            shift 2
            ;;
        --output)
            output_dir="${2:-}"
            shift 2
            ;;
        --arch)
            arch="${2:-}"
            shift 2
            ;;
        --mode)
            mode="${2:-}"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ -z "$version" || -z "$binary" ]]; then
    usage >&2
    exit 2
fi

case "$arch" in
    amd64)
        rpm_arch="x86_64"
        deb_arch="amd64"
        clickhouse_arch="amd64"
        ;;
    *)
        echo "unsupported arch: $arch" >&2
        exit 2
        ;;
esac

case "$mode" in
    full|upgrade)
        ;;
    *)
        echo "unsupported PACKAGING_MODE: $mode (want full|upgrade)" >&2
        exit 2
        ;;
esac

pkg_version="${version#v}"
if [[ ! "$pkg_version" =~ ^[0-9]+(\.[0-9]+)*$ ]]; then
    echo "package version must be X.Y.Z style, got: $version" >&2
    exit 2
fi

binary_path="$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")"
if [[ ! -f "$binary_path" ]]; then
    echo "binary not found: $binary" >&2
    exit 1
fi

mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

include_clickhouse=true
package_name="fwlog-full"
rpm_output_name="fwlog-full-v${pkg_version}.${rpm_arch}.rpm"
deb_output_name="fwlog-full_${pkg_version}_${deb_arch}.deb"
package_summary="FWLog offline full installer with bundled ClickHouse runtime"
package_description="FWLog NAT query service bundled with a private ClickHouse runtime for first-time offline server installation."

if [[ "$mode" == "upgrade" ]]; then
    include_clickhouse=false
    package_name="fwlog-upgrade"
    rpm_output_name="fwlog-upgrade-v${pkg_version}.${rpm_arch}.rpm"
    deb_output_name="fwlog-upgrade_${pkg_version}_${deb_arch}.deb"
    package_summary="FWLog application upgrade package"
    package_description="FWLog NAT query service application upgrade package without ClickHouse runtime."
fi

clickhouse_binary() {
    if [[ -n "${CLICKHOUSE_BINARY:-}" ]]; then
        if [[ ! -f "$CLICKHOUSE_BINARY" ]]; then
            echo "ClickHouse did not become ready after package installation" >&2
            exit 1
        fi
        echo "$CLICKHOUSE_BINARY"
        return
    fi

    local clickhouse_version="${CLICKHOUSE_VERSION:-25.8.27.1}"
    local cache_dir="${CLICKHOUSE_CACHE_DIR:-$repo_root/.cache/clickhouse}"
    local archive="$cache_dir/clickhouse-common-static-${clickhouse_version}-${clickhouse_arch}.tgz"
    local url="${CLICKHOUSE_TGZ_URL:-https://packages.clickhouse.com/tgz/lts/clickhouse-common-static-${clickhouse_version}-${clickhouse_arch}.tgz}"

    mkdir -p "$cache_dir"
    if [[ ! -f "$archive" ]]; then
        curl -fL --retry 3 --retry-delay 5 "$url" -o "$archive"
    fi

    local extract_dir="$work_dir/clickhouse"
    mkdir -p "$extract_dir"
    tar -xzf "$archive" -C "$extract_dir"
    local found
    found="$(find "$extract_dir" -type f -name clickhouse -perm /111 | head -n 1 || true)"
    if [[ -z "$found" ]]; then
        echo "ClickHouse did not become ready after package installation" >&2
        exit 1
    fi
    echo "$found"
}

geoip_database() {
    if [[ -n "${GEOIP_DB_PATH:-}" ]]; then
        if [[ ! -f "$GEOIP_DB_PATH" ]]; then
            echo "GEOIP_DB_PATH not found: $GEOIP_DB_PATH" >&2
            exit 1
        fi
        echo "$GEOIP_DB_PATH"
        return
    fi

    local cache_dir="${GEOIP_CACHE_DIR:-$repo_root/.cache/geoip}"
    local db_path="$cache_dir/GeoLite2-City.mmdb"
    local url="${GEOIP_DB_URL:-https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-City.mmdb}"

    mkdir -p "$cache_dir"
    if [[ ! -f "$db_path" ]]; then
        curl -fL --retry 3 --retry-delay 5 "$url" -o "$db_path"
    fi
    if [[ ! -s "$db_path" ]]; then
        echo "GeoLite2-City.mmdb is empty: $db_path" >&2
        exit 1
    fi
    echo "$db_path"
}

stage_rootfs() {
    local rootfs="$1"
    local ch_bin="$2"
    local geoip_db="$3"

    install -d "$rootfs/opt/fwlog" \
        "$rootfs/etc/systemd/system" \
        "$rootfs/data/sangfor_fw_log" \
        "$rootfs/data/index" \
        "$rootfs/data/export"

    install -m 0755 "$binary_path" "$rootfs/opt/fwlog/fwlog"
    printf 'VERSION=v%s\n' "$pkg_version" > "$rootfs/opt/fwlog/VERSION"
    local service_unit="$repo_root/fwlog.service"
    if [[ "$include_clickhouse" != "true" ]]; then
        service_unit="$repo_root/packaging/systemd/fwlog.service"
    fi
    install -m 0644 "$service_unit" "$rootfs/etc/systemd/system/fwlog.service"
    install -m 0644 "$geoip_db" "$rootfs/data/index/GeoLite2-City.mmdb"

    if [[ "$include_clickhouse" == "true" ]]; then
        printf 'RUNTIME_VERSION=clickhouse-%s\n' "${CLICKHOUSE_VERSION:-25.8.27.1}" > "$rootfs/opt/fwlog/RUNTIME_VERSION"
        install -d "$rootfs/opt/fwlog/clickhouse/bin" \
            "$rootfs/opt/fwlog/clickhouse/etc" \
            "$rootfs/opt/fwlog/clickhouse/data" \
            "$rootfs/opt/fwlog/clickhouse/tmp" \
            "$rootfs/opt/fwlog/clickhouse/user_files" \
            "$rootfs/opt/fwlog/clickhouse/format_schemas" \
            "$rootfs/opt/fwlog/clickhouse/log"

        install -m 0755 "$ch_bin" "$rootfs/opt/fwlog/clickhouse/bin/clickhouse"
        install -m 0644 "$repo_root/packaging/systemd/fwlog-clickhouse.service" "$rootfs/etc/systemd/system/fwlog-clickhouse.service"
        install -m 0644 "$repo_root/packaging/clickhouse/config.xml" "$rootfs/opt/fwlog/clickhouse/etc/config.xml"
        install -m 0644 "$repo_root/packaging/clickhouse/users.xml" "$rootfs/opt/fwlog/clickhouse/etc/users.xml"
    fi
}

build_deb() {
    local rootfs="$1"
    local debroot="$work_dir/debroot"
    local package_path="$output_dir/$deb_output_name"
    cp -a "$rootfs" "$debroot"
    install -d "$debroot/DEBIAN"
    local installed_size
    installed_size="$(du -sk "$debroot" | awk '{print $1}')"
    local deb_replaces="fwlog, fwlog-full"
    if [[ "$include_clickhouse" == "true" ]]; then
        deb_replaces="fwlog, fwlog-upgrade"
    fi

    cat > "$debroot/DEBIAN/control" <<EOF
Package: $package_name
Version: $pkg_version
Section: net
Priority: optional
Architecture: $deb_arch
Depends: systemd
Provides: fwlog
Replaces: $deb_replaces
Installed-Size: $installed_size
Maintainer: fwlog <noreply@example.invalid>
Description: $package_summary
 $package_description
EOF

    cat > "$debroot/DEBIAN/preinst" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "install" ] || [ "$1" = "upgrade" ]; then
    backup="/data/fwlog/backups/app_settings-before-package.tsv"
    client=""
    for candidate in /opt/fwlog/clickhouse/bin/clickhouse /opt/nat-query/clickhouse/bin/clickhouse /usr/bin/clickhouse /usr/bin/clickhouse-client; do
        if [ -x "$candidate" ]; then client="$candidate"; break; fi
    done
    clickhouse_query() {
        case "$client" in
            */clickhouse-client) "$client" --query "$1" ;;
            *) "$client" client --query "$1" ;;
        esac
    }
    mkdir -p "$(dirname "$backup")"
    chmod 700 "$(dirname "$backup")" || true
    if [ -x "$client" ]; then
        if clickhouse_query "SELECT key, value, now() FROM app_settings FINAL FORMAT TabSeparated" > "$backup.tmp" 2>/tmp/fwlog-preinst-backup.err; then
            mv "$backup.tmp" "$backup"
            chmod 600 "$backup"
        else
            cat /tmp/fwlog-preinst-backup.err >> /data/fwlog/backups/backup-failed.log 2>/dev/null || true
            rm -f "$backup.tmp"
            echo "app_settings 澶囦唤澶辫触锛屽凡涓鍗囩骇锛堝闇€璺宠繃璇峰厛鍋滄 ClickHouse 鍚庨噸璇曪級" >&2
            exit 1
        fi
    fi
fi
exit 0
EOF

    cat > "$debroot/DEBIAN/postinst" <<EOF
#!/bin/sh
set -e
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
        case "\$client" in
            */clickhouse-client) "\$client" --query "\$1" ;;
            *) "\$client" client --query "\$1" ;;
        esac
    }
    systemctl daemon-reload || true
    if [ "$include_clickhouse" = "true" ]; then
        systemctl enable fwlog-clickhouse.service fwlog.service || true
    else
        systemctl enable fwlog.service || true
    fi
    if [ -d /run/systemd/system ]; then
        systemctl stop fwlog.service
        embedded_clickhouse=false
        if [ "$include_clickhouse" = "true" ] || systemctl cat fwlog-clickhouse.service >/dev/null 2>&1; then
            embedded_clickhouse=true
        fi
        if [ "\$embedded_clickhouse" = "true" ]; then
            systemctl start fwlog-clickhouse.service
            client=""
            for candidate in /opt/fwlog/clickhouse/bin/clickhouse /opt/nat-query/clickhouse/bin/clickhouse; do
                if [ -x "\$candidate" ]; then client="\$candidate"; break; fi
            done
            if [ -z "\$client" ]; then
                echo "ClickHouse client not found after starting fwlog-clickhouse.service" >&2
                exit 1
            fi
            i=0
            while [ "\$i" -lt 60 ]; do
                if clickhouse_query "SELECT 1" >/dev/null 2>&1; then
                    break
                fi
                i=\$((i + 1))
                sleep 1
            done
            if ! clickhouse_query "SELECT 1" >/dev/null 2>&1; then
                echo "ClickHouse did not become ready after package installation" >&2
                exit 1
            fi
        else
            client=""
            for candidate in /opt/fwlog/clickhouse/bin/clickhouse /opt/nat-query/clickhouse/bin/clickhouse /usr/bin/clickhouse /usr/bin/clickhouse-client; do
                if [ -x "\$candidate" ]; then client="\$candidate"; break; fi
            done
        fi
        backup="/data/fwlog/backups/app_settings-before-package.tsv"
        if [ -s "\$backup" ] && [ -x "\$client" ]; then
            if ! clickhouse_query "INSERT INTO app_settings (key, value, updated_at) FORMAT TabSeparated" < "\$backup" >/dev/null 2>&1; then
                echo "app_settings 鎭㈠澶辫触锛屽浠芥枃浠朵繚鐣欏湪 \$backup" >&2
            fi
        fi
        systemctl start fwlog.service
    fi
fi
exit 0
EOF

    cat > "$debroot/DEBIAN/prerm" <<EOF
#!/bin/sh
set -e
if [ "\$1" = "remove" ] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop fwlog.service || true
    if [ "$include_clickhouse" = "true" ]; then
        systemctl stop fwlog-clickhouse.service || true
        systemctl disable fwlog.service fwlog-clickhouse.service || true
    else
        systemctl disable fwlog.service || true
    fi
fi
exit 0
EOF

    cat > "$debroot/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi
exit 0
EOF

    chmod 0755 "$debroot/DEBIAN/preinst" "$debroot/DEBIAN/postinst" "$debroot/DEBIAN/prerm" "$debroot/DEBIAN/postrm"
    dpkg-deb --build --root-owner-group "$debroot" "$package_path"
}

build_rpm() {
    local rootfs="$1"
    if ! command -v rpmbuild >/dev/null 2>&1; then
        echo "rpmbuild is required to build the Kylin RPM package" >&2
        exit 1
    fi

    local rpm_top="$work_dir/rpmbuild"
    local source_dir="$work_dir/fwlog-root"
    mkdir -p "$rpm_top/BUILD" "$rpm_top/RPMS" "$rpm_top/SOURCES" "$rpm_top/SPECS" "$rpm_top/SRPMS" "$source_dir"
    cp -a "$rootfs"/. "$source_dir"/
    tar -C "$work_dir" -czf "$rpm_top/SOURCES/fwlog-root.tar.gz" fwlog-root
    rpmbuild -bb \
        --define "_topdir $rpm_top" \
        --define "fwlog_version $pkg_version" \
        --define "fwlog_package_name $package_name" \
        --define "fwlog_package_summary $package_summary" \
        --define "fwlog_package_description $package_description" \
        --define "fwlog_include_clickhouse $([[ "$include_clickhouse" == "true" ]] && echo 1 || echo 0)" \
        "$repo_root/packaging/rpm/fwlog.spec"
    cp "$rpm_top/RPMS/$rpm_arch/${package_name}-${pkg_version}-1.$rpm_arch.rpm" \
        "$output_dir/$rpm_output_name"
}

ch_bin=""
if [[ "$include_clickhouse" == "true" ]]; then
    ch_bin="$(clickhouse_binary)"
fi
geoip_db="$(geoip_database)"
rootfs="$work_dir/rootfs"
stage_rootfs "$rootfs" "$ch_bin" "$geoip_db"
build_deb "$rootfs"
build_rpm "$rootfs"

checksums_file="$output_dir/checksums.txt"
if [[ "$mode" == "full" ]]; then
    : > "$checksums_file"
fi
for artifact in "$output_dir/$rpm_output_name" "$output_dir/$deb_output_name"; do
    if [[ -f "$artifact" ]]; then
        artifact_name="$(basename "$artifact")"
        (cd "$output_dir" && sha256sum "$artifact_name" >> "$checksums_file")
    fi
done

if [[ "$mode" == "full" ]]; then
    bundle_name="fwlog-full-v${pkg_version}-amd64"
    bundle_dir="$work_dir/$bundle_name"
    mkdir -p "$bundle_dir/packages"
    cp "$output_dir/$rpm_output_name" "$output_dir/$deb_output_name" "$bundle_dir/packages/"
    cp "$checksums_file" "$bundle_dir/checksums.txt"
    cat > "$bundle_dir/install.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
bundle_dir="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
cd "\$bundle_dir"
cd "\$bundle_dir/packages"
sha256sum -c ../checksums.txt --ignore-missing
if command -v rpm >/dev/null 2>&1 && [[ -f "$rpm_output_name" ]]; then
    rpm -Uvh --replacepkgs "$rpm_output_name"
elif command -v dpkg >/dev/null 2>&1 && [[ -f "$deb_output_name" ]]; then
    dpkg -i "$deb_output_name"
else
    echo "unsupported system: rpm or dpkg is required" >&2
    exit 1
fi
systemctl is-active --quiet fwlog.service
EOF
    chmod 0755 "$bundle_dir/install.sh"
    tar -C "$work_dir" -czf "$output_dir/$bundle_name.tar.gz" "$bundle_name"
fi

ls -lh "$output_dir/$rpm_output_name" "$output_dir/$deb_output_name" "$checksums_file" "$output_dir/fwlog-full-v${pkg_version}-amd64.tar.gz" 2>/dev/null || true
