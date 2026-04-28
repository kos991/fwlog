#!/bin/bash
# 快速启动脚本 - Windows Git Bash环境

echo "🚀 防火墙日志查询系统 - 快速启动"
echo "=================================="
echo ""

# 检查Go环境
if ! command -v go &> /dev/null; then
    echo "❌ 未安装Go语言环境"
    echo ""
    echo "请下载安装Go 1.22+:"
    echo "  https://go.dev/dl/go1.22.2.windows-amd64.msi"
    echo ""
    exit 1
fi

echo "✅ Go版本: $(go version)"
echo ""

# 检查日志文件
if [ ! -d "./data/sangfor_fw_log" ]; then
    echo "📁 创建日志目录..."
    mkdir -p ./data/sangfor_fw_log
fi

LOG_COUNT=$(ls -1 ./data/sangfor_fw_log/*.log 2>/dev/null | wc -l)
if [ "$LOG_COUNT" -eq 0 ]; then
    echo "⚠️  日志目录为空，请将日志文件放入: ./data/sangfor_fw_log/"
    echo ""
    read -p "按回车键继续（将使用示例数据）..."
fi

# 创建索引目录
mkdir -p ./data/index

# 下载依赖
echo "📥 下载依赖..."
if [ ! -f "go.mod" ]; then
    go mod init fw_log_query
fi

# 注意：Windows下DuckDB需要CGO，这里提供替代方案
echo ""
echo "⚠️  注意: Windows环境下DuckDB需要CGO支持"
echo ""
echo "选择运行模式:"
echo "  1. 使用DuckDB（需要安装MinGW/TDM-GCC）"
echo "  2. 使用SQLite（无需额外依赖，推荐）"
echo ""
read -p "请选择 [1/2]: " choice

if [ "$choice" = "2" ]; then
    echo ""
    echo "🔄 切换到SQLite版本..."

    # 修改导入
    sed -i 's/github.com\/marcboeker\/go-duckdb/modernc.org\/sqlite/g' fw_log_optimized.go
    sed -i 's/"duckdb"/"sqlite"/g' fw_log_optimized.go

    go get modernc.org/sqlite

    echo "✅ 已切换到SQLite模式"
else
    echo ""
    echo "📥 安装DuckDB驱动..."

    # 检查CGO
    if ! command -v gcc &> /dev/null; then
        echo "❌ 未找到GCC编译器"
        echo ""
        echo "请安装TDM-GCC:"
        echo "  https://jmeubank.github.io/tdm-gcc/download/"
        echo ""
        exit 1
    fi

    go get github.com/marcboeker/go-duckdb
fi

# 编译
echo ""
echo "🔨 编译程序..."
go build -o fw_log_query.exe fw_log_optimized.go

if [ $? -eq 0 ]; then
    echo "✅ 编译成功！"

    BINARY_SIZE=$(du -h fw_log_query.exe | cut -f1)
    echo "📦 程序大小: $BINARY_SIZE"
else
    echo "❌ 编译失败"
    exit 1
fi

echo ""
echo "=================================="
echo "✅ 准备完成！"
echo ""
echo "🚀 启动服务:"
echo "  ./fw_log_query.exe -logdir=./data/sangfor_fw_log -db=./data/index/fw_logs.db -port=8080"
echo ""
echo "⚡ 首次运行（建立索引）:"
echo "  ./fw_log_query.exe -logdir=./data/sangfor_fw_log -db=./data/index/fw_logs.db -rebuild"
echo ""
echo "🌐 访问Web界面:"
echo "  http://localhost:8080"
echo ""

read -p "是否立即启动服务? [Y/n]: " start_now

if [ "$start_now" != "n" ] && [ "$start_now" != "N" ]; then
    echo ""
    echo "🚀 启动服务中..."

    if [ "$LOG_COUNT" -gt 0 ]; then
        ./fw_log_query.exe -logdir=./data/sangfor_fw_log -db=./data/index/fw_logs.db -port=8080 -rebuild
    else
        echo "⚠️  无日志文件，仅启动Web服务..."
        ./fw_log_query.exe -logdir=./data/sangfor_fw_log -db=./data/index/fw_logs.db -port=8080
    fi
fi
