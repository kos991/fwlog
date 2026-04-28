#!/bin/bash
# Debian服务器部署脚本 - 在Debian服务器上运行

set -e

echo "╔═══════════════════════════════════════════════════════════════════╗"
echo "║   🚀 防火墙日志查询系统 - Debian服务器部署                       ║"
echo "╚═══════════════════════════════════════════════════════════════════╝"
echo ""

# 检查是否为root
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用root权限运行此脚本"
    echo "   sudo ./deploy_debian.sh"
    exit 1
fi

echo "📋 系统信息:"
echo "  操作系统: $(lsb_release -d | cut -f2)"
echo "  内核版本: $(uname -r)"
echo "  架构: $(uname -m)"
echo ""

# 1. 更新系统
echo "📦 更新软件包列表..."
apt-get update -qq

# 2. 安装基础依赖
echo "📥 安装基础依赖..."
apt-get install -y -qq \
    wget \
    curl \
    git \
    build-essential \
    unzip \
    ca-certificates

# 3. 安装Go
if ! command -v go &> /dev/null; then
    echo "📥 安装Go 1.22.2..."
    GO_VERSION="1.22.2"
    wget -q https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
    rm go${GO_VERSION}.linux-amd64.tar.gz

    # 设置环境变量
    if ! grep -q "/usr/local/go/bin" /etc/profile; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi
    export PATH=$PATH:/usr/local/go/bin

    echo "✅ Go安装完成: $(go version)"
else
    echo "✅ Go已安装: $(go version)"
fi

# 4. 安装DuckDB库
echo "📥 安装DuckDB开发库..."
DUCKDB_VERSION="0.10.2"
if [ ! -f "/usr/local/lib/libduckdb.so" ]; then
    wget -q https://github.com/duckdb/duckdb/releases/download/v${DUCKDB_VERSION}/libduckdb-linux-amd64.zip -O /tmp/duckdb.zip
    unzip -q /tmp/duckdb.zip -d /tmp/duckdb
    cp /tmp/duckdb/libduckdb.so /usr/local/lib/
    cp /tmp/duckdb/duckdb.h /usr/local/include/
    ldconfig
    rm -rf /tmp/duckdb /tmp/duckdb.zip
    echo "✅ DuckDB库安装完成"
else
    echo "✅ DuckDB库已安装"
fi

# 5. 创建工作目录
echo "📁 创建工作目录..."
mkdir -p /opt/nat-query
mkdir -p /data/sangfor_fw_log
mkdir -p /data/index

# 6. 编译程序
echo "🔨 编译程序..."
cd "$(dirname "$0")"

# 初始化Go模块
if [ ! -f "go.mod" ]; then
    go mod init nat-query-service
fi

# 下载依赖
echo "📥 下载Go依赖..."
go get github.com/gin-gonic/gin
go get github.com/marcboeker/go-duckdb

# 编译（优化二进制大小）
echo "🔨 编译中..."
CGO_ENABLED=1 go build -ldflags="-s -w" -o /opt/nat-query/nat-query-service main.go

if [ $? -eq 0 ]; then
    BINARY_SIZE=$(du -h /opt/nat-query/nat-query-service | cut -f1)
    echo "✅ 编译完成，二进制大小: $BINARY_SIZE"
else
    echo "❌ 编译失败"
    exit 1
fi

# 7. 安装systemd服务
echo "⚙️  安装systemd服务..."
cp nat-query-service.service /etc/systemd/system/

# 重载systemd
systemctl daemon-reload

echo ""
echo "╔═══════════════════════════════════════════════════════════════════╗"
echo "║   ✅ 安装完成！                                                   ║"
echo "╚═══════════════════════════════════════════════════════════════════╝"
echo ""
echo "📋 使用说明:"
echo ""
echo "  1️⃣  将日志文件放入: /data/sangfor_fw_log/"
echo "     cp /path/to/*.log /data/sangfor_fw_log/"
echo ""
echo "  2️⃣  启动服务:"
echo "     systemctl start nat-query-service"
echo ""
echo "  3️⃣  查看状态:"
echo "     systemctl status nat-query-service"
echo ""
echo "  4️⃣  开机自启:"
echo "     systemctl enable nat-query-service"
echo ""
echo "  5️⃣  访问Web界面:"
echo "     http://$(hostname -I | awk '{print $1}'):8080"
echo ""
echo "  6️⃣  查看日志:"
echo "     journalctl -u nat-query-service -f"
echo ""
echo "🔧 手动运行（测试）:"
echo "  LOG_DIR=/data/sangfor_fw_log DB_FILE=/data/index/nat_logs.duckdb /opt/nat-query/nat-query-service"
echo ""
echo "📊 性能测试:"
echo "  如果上传了test_performance.sh，运行: ./test_performance.sh"
echo ""
