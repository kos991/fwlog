#!/bin/bash

set -e

PACKAGE_NAME="sangfor-fw-log-query"
VERSION="2.1.0"
ARCH="all"
BUILD_DIR="build/${PACKAGE_NAME}_${VERSION}_${ARCH}"

echo "开始构建DEB包: ${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"

# 清理旧构建
rm -rf build
mkdir -p "$BUILD_DIR"

# 创建目录结构
mkdir -p "$BUILD_DIR/DEBIAN"
mkdir -p "$BUILD_DIR/usr/bin"
mkdir -p "$BUILD_DIR/usr/share/doc/$PACKAGE_NAME"
mkdir -p "$BUILD_DIR/usr/share/man/man1"
mkdir -p "$BUILD_DIR/opt/sangfor-fw-log/config"
mkdir -p "$BUILD_DIR/opt/sangfor-fw-log/data"/{sangfor_fw_log,index,export}
mkdir -p "$BUILD_DIR/opt/sangfor-fw-log/data/export"/{by_ip,by_port,by_date,by_query}
mkdir -p "$BUILD_DIR/etc/sangfor-fw-log"

# 复制主程序
cp sangforfw_log.sh "$BUILD_DIR/opt/sangfor-fw-log/"
chmod 755 "$BUILD_DIR/opt/sangfor-fw-log/sangforfw_log.sh"

# 创建包装脚本（而不是符号链接，Windows不支持）
cat > "$BUILD_DIR/usr/bin/sangfor-fw-log" << 'WRAPPER'
#!/bin/bash
exec /opt/sangfor-fw-log/sangforfw_log.sh "$@"
WRAPPER
chmod 755 "$BUILD_DIR/usr/bin/sangfor-fw-log"

# 复制配置文件
cat > "$BUILD_DIR/opt/sangfor-fw-log/config/default.conf" << 'CONF'
LOG_DIR="/opt/sangfor-fw-log/data/sangfor_fw_log"
INDEX_DIR="/opt/sangfor-fw-log/data/index"
EXPORT_DIR="/opt/sangfor-fw-log/data/export"
INDEX_TYPE="auto"
PARALLEL_JOBS=8
AUTO_COMPRESS_INDEX=1
MAX_RESULT_LINES=10000
CONF

# 创建配置文件副本（而不是符号链接）
cp "$BUILD_DIR/opt/sangfor-fw-log/config/default.conf" "$BUILD_DIR/etc/sangfor-fw-log/config.conf"

# 创建DEBIAN控制文件
cat > "$BUILD_DIR/DEBIAN/control" << CONTROL
Package: $PACKAGE_NAME
Version: $VERSION
Section: admin
Priority: optional
Architecture: $ARCH
Depends: bash (>= 4.0), gawk, grep, gzip, coreutils
Recommends: tinycdb
Suggests: ripgrep
Maintainer: QJKJ Team <support@qjkj.com>
Description: 深信服防火墙日志查询工具
 高性能的防火墙日志查询和分析工具，支持：
  - 基于CDB的高速索引查询（毫秒级响应）
  - 多维度过滤（IP、端口、日期、协议）
  - 查询结果自动导出
  - 命令行和交互式两种模式
  - 自动索引压缩和更新
CONTROL

# 创建postinst脚本
cat > "$BUILD_DIR/DEBIAN/postinst" << 'POSTINST'
#!/bin/bash
set -e
case "$1" in
    configure)
        chmod 755 /opt/sangfor-fw-log/sangforfw_log.sh
        if ! command -v cdbmake >/dev/null 2>&1; then
            echo "警告: 未检测到tinycdb，将使用文本索引模式"
            echo "建议安装: sudo apt install tinycdb"
        fi
        echo "深信服防火墙日志查询工具安装完成！"
        echo "使用方法:"
        echo "  交互模式: sangfor-fw-log"
        echo "  命令行查询: sangfor-fw-log 192.168.1.1"
        ;;
esac
exit 0
POSTINST
chmod 755 "$BUILD_DIR/DEBIAN/postinst"

# 创建prerm脚本
cat > "$BUILD_DIR/DEBIAN/prerm" << 'PRERM'
#!/bin/bash
set -e
case "$1" in
    remove|upgrade|deconfigure)
        if [ -d /opt/sangfor-fw-log/data ]; then
            echo "数据目录: /opt/sangfor-fw-log/data"
        fi
        ;;
esac
exit 0
PRERM
chmod 755 "$BUILD_DIR/DEBIAN/prerm"

# 创建postrm脚本
cat > "$BUILD_DIR/DEBIAN/postrm" << 'POSTRM'
#!/bin/bash
set -e
case "$1" in
    purge)
        rm -rf /opt/sangfor-fw-log
        rm -rf /etc/sangfor-fw-log
        echo "已删除所有数据和配置文件"
        ;;
    remove)
        echo "数据已保留在 /opt/sangfor-fw-log/data"
        ;;
esac
exit 0
POSTRM
chmod 755 "$BUILD_DIR/DEBIAN/postrm"

# 创建README
cat > "$BUILD_DIR/usr/share/doc/$PACKAGE_NAME/README.md" << 'README'
# 深信服防火墙日志查询工具 v2.1.0

## 功能特性
- 基于CDB的高速索引查询（毫秒级响应）
- 多维度过滤（IP、端口、日期、协议）
- 查询结果自动导出
- 命令行和交互式两种模式

## 使用方法

### 交互模式
```bash
sangfor-fw-log
```

### 命令行模式
```bash
# 查询单个IP
sangfor-fw-log 192.168.1.100

# 查询IP和日期
sangfor-fw-log 20260427 192.168.1.100

# 查询IP和端口
sangfor-fw-log 20260427 192.168.1.100:443
```

## 配置文件
- 主配置: /etc/sangfor-fw-log/config.conf
- 日志目录: /opt/sangfor-fw-log/data/sangfor_fw_log
- 索引目录: /opt/sangfor-fw-log/data/index
- 导出目录: /opt/sangfor-fw-log/data/export

## 性能优化
安装tinycdb可获得5-8倍查询速度提升：
```bash
sudo apt install tinycdb
```

## 版权信息
© 2026 QJKJ Team
README

# 创建man页面
cat > /tmp/sangfor-fw-log.1 << 'MAN'
.TH SANGFOR-FW-LOG 1 "2026-04-27" "2.1.0" "深信服防火墙日志查询工具"
.SH NAME
sangfor-fw-log \- 深信服防火墙日志查询和分析工具
.SH SYNOPSIS
.B sangfor-fw-log
[\fIIP\fR] [\fIDATE\fR] [\fIIP:PORT\fR]
.SH DESCRIPTION
高性能的防火墙日志查询工具，支持基于CDB的毫秒级索引查询。
.SH EXAMPLES
.TP
查询单个IP:
.B sangfor-fw-log 192.168.1.100
.TP
查询IP和日期:
.B sangfor-fw-log 20260427 192.168.1.100
.TP
查询IP和端口:
.B sangfor-fw-log 20260427 192.168.1.100:443
.SH FILES
.TP
.I /opt/sangfor-fw-log/sangforfw_log.sh
主程序脚本
.TP
.I /etc/sangfor-fw-log/config.conf
配置文件
.SH AUTHOR
QJKJ Team <support@qjkj.com>
MAN
gzip -c /tmp/sangfor-fw-log.1 > "$BUILD_DIR/usr/share/man/man1/sangfor-fw-log.1.gz"

# 计算安装大小
INSTALLED_SIZE=$(du -sk "$BUILD_DIR" | cut -f1)
echo "Installed-Size: $INSTALLED_SIZE" >> "$BUILD_DIR/DEBIAN/control"

# 构建DEB包（仅在Linux环境）
if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --build "$BUILD_DIR"
    mv "build/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb" .
    echo ""
    echo "✅ DEB包构建完成: ${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
else
    echo ""
    echo "⚠️  Windows环境检测到，已创建DEB包目录结构"
    echo "📁 目录位置: $BUILD_DIR"
    echo ""
    echo "请在Linux系统上完成打包："
    echo "  1. 将整个 build/ 目录复制到Linux系统"
    echo "  2. 运行: dpkg-deb --build $BUILD_DIR"
    echo "  3. 生成: ${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
fi

echo ""
echo "安装方法:"
echo "  sudo dpkg -i ${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
echo "  sudo apt install -f  # 安装依赖"
echo ""
echo "卸载方法:"
echo "  sudo apt remove $PACKAGE_NAME        # 保留数据"
echo "  sudo apt purge $PACKAGE_NAME         # 删除所有数据"
