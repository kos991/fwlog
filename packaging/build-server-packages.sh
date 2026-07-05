#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version=""
binary=""
output_dir="$repo_root/dist"
arch="amd64"

usage() {
    cat <<'USAGE'
usage: packaging/build-server-packages.sh --version <vX.Y.Z|X.Y.Z> --binary <path> [--output <dir>] [--arch amd64]

Environment:
  CLICKHOUSE_VERSION    ClickHouse version to bundle. Default: 25.8.27.1
  CLICKHOUSE_TGZ_URL    Override ClickHouse common-static tarball URL.
  CLICKHOUSE_BINARY     Use an existing ClickHouse binary instead of downloading.
  CLICKHOUSE_CACHE_DIR  Download cache directory. Default: .cache/clickhouse
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

clickhouse_binary() {
    if [[ -n "${CLICKHOUSE_BINARY:-}" ]]; then
        if [[ ! -f "$CLICKHOUSE_BINARY" ]]; then
            echo "CLICKHOUSE_BINARY not found: $CLICKHOUSE_BINARY" >&2
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
        echo "clickhouse executable not found in $archive" >&2
        exit 1
    fi
    echo "$found"
}

stage_rootfs() {
    local rootfs="$1"
    local ch_bin="$2"

    install -d "$rootfs/opt/nat-query/clickhouse/bin" \
        "$rootfs/opt/nat-query/clickhouse/etc" \
        "$rootfs/opt/nat-query/clickhouse/data" \
        "$rootfs/opt/nat-query/clickhouse/tmp" \
        "$rootfs/opt/nat-query/clickhouse/user_files" \
        "$rootfs/opt/nat-query/clickhouse/format_schemas" \
        "$rootfs/opt/nat-query/clickhouse/log" \
        "$rootfs/etc/systemd/system" \
        "$rootfs/data/sangfor_fw_log" \
        "$rootfs/data/index" \
        "$rootfs/data/export"

    install -m 0755 "$binary_path" "$rootfs/opt/nat-query/nat-query-service"
    install -m 0755 "$ch_bin" "$rootfs/opt/nat-query/clickhouse/bin/clickhouse"
    install -m 0644 "$repo_root/nat-query-service.service" "$rootfs/etc/systemd/system/nat-query-service.service"
    install -m 0644 "$repo_root/packaging/systemd/fwlog-clickhouse.service" "$rootfs/etc/systemd/system/fwlog-clickhouse.service"
    install -m 0644 "$repo_root/packaging/clickhouse/config.xml" "$rootfs/opt/nat-query/clickhouse/etc/config.xml"
    install -m 0644 "$repo_root/packaging/clickhouse/users.xml" "$rootfs/opt/nat-query/clickhouse/etc/users.xml"
}

build_deb() {
    local rootfs="$1"
    local debroot="$work_dir/debroot"
    local package_path="$output_dir/nat-query-service_debian-server_${deb_arch}.deb"
    cp -a "$rootfs" "$debroot"
    install -d "$debroot/DEBIAN"
    local installed_size
    installed_size="$(du -sk "$debroot" | awk '{print $1}')"

    cat > "$debroot/DEBIAN/control" <<EOF
Package: nat-query-service
Version: $pkg_version
Section: net
Priority: optional
Architecture: $deb_arch
Depends: systemd
Installed-Size: $installed_size
Maintainer: fwlog <noreply@example.invalid>
Description: FWLog NAT query service with embedded ClickHouse
 FWLog NAT query service bundled with a private ClickHouse runtime for server installation.
EOF

    cat > "$debroot/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable fwlog-clickhouse.service nat-query-service.service || true
    if [ -d /run/systemd/system ]; then
        systemctl restart fwlog-clickhouse.service
        systemctl restart nat-query-service.service
    fi
fi
exit 0
EOF

    cat > "$debroot/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "remove" ] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop nat-query-service.service || true
    systemctl stop fwlog-clickhouse.service || true
    systemctl disable nat-query-service.service fwlog-clickhouse.service || true
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

    chmod 0755 "$debroot/DEBIAN/postinst" "$debroot/DEBIAN/prerm" "$debroot/DEBIAN/postrm"
    dpkg-deb --build --root-owner-group "$debroot" "$package_path"
}

build_rpm() {
    local rootfs="$1"
    if ! command -v rpmbuild >/dev/null 2>&1; then
        echo "rpmbuild is required to build the Kylin RPM package" >&2
        exit 1
    fi

    local rpm_top="$work_dir/rpmbuild"
    local source_dir="$work_dir/nat-query-service-root"
    mkdir -p "$rpm_top/BUILD" "$rpm_top/RPMS" "$rpm_top/SOURCES" "$rpm_top/SPECS" "$rpm_top/SRPMS" "$source_dir"
    cp -a "$rootfs"/. "$source_dir"/
    tar -C "$work_dir" -czf "$rpm_top/SOURCES/nat-query-service-root.tar.gz" nat-query-service-root
    rpmbuild -bb \
        --define "_topdir $rpm_top" \
        --define "fwlog_version $pkg_version" \
        "$repo_root/packaging/rpm/nat-query-service.spec"
    cp "$rpm_top/RPMS/$rpm_arch/nat-query-service-${pkg_version}-1.$rpm_arch.rpm" \
        "$output_dir/nat-query-service_kylin-server_${arch}.rpm"
}

ch_bin="$(clickhouse_binary)"
rootfs="$work_dir/rootfs"
stage_rootfs "$rootfs" "$ch_bin"
build_deb "$rootfs"
build_rpm "$rootfs"

ls -lh "$output_dir"/nat-query-service_*_"$arch".rpm "$output_dir"/nat-query-service_*_"$deb_arch".deb 2>/dev/null || true
