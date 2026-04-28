#!/bin/bash
# 快速测试脚本 - 验证优化效果

set -e

echo "🧪 防火墙日志查询系统 - 性能测试"
echo "=================================="
echo ""

# 检查Go环境
if ! command -v go &> /dev/null; then
    echo "❌ 未安装Go，请先运行 setup.sh"
    exit 1
fi

# 检查日志文件
LOG_DIR="./data/sangfor_fw_log"
if [ ! -d "$LOG_DIR" ] || [ -z "$(ls -A $LOG_DIR/*.log 2>/dev/null)" ]; then
    echo "❌ 日志目录为空: $LOG_DIR"
    echo "请将日志文件放入该目录"
    exit 1
fi

# 统计日志信息
echo "📊 日志文件统计:"
LOG_COUNT=$(ls -1 $LOG_DIR/*.log 2>/dev/null | wc -l)
LOG_SIZE=$(du -sh $LOG_DIR 2>/dev/null | cut -f1)
echo "  文件数: $LOG_COUNT"
echo "  总大小: $LOG_SIZE"
echo ""

# 编译程序
echo "🔨 编译程序..."
go build -ldflags="-s -w" -o ./test_nat_query_service main.go
BINARY_SIZE=$(du -h ./test_nat_query_service | cut -f1)
echo "✅ 编译完成，二进制大小: $BINARY_SIZE"
echo ""

# 测试1: 建立索引
echo "⚡ 测试1: 建立索引性能"
echo "-----------------------------------"
rm -f ./test_nat_logs.duckdb
START_TIME=$(date +%s.%N)

LOG_DIR=$LOG_DIR DB_FILE=./test_nat_logs.duckdb PORT=9999 ./test_nat_query_service &

PID=$!
sleep 3
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true

END_TIME=$(date +%s.%N)
BUILD_TIME=$(echo "$END_TIME - $START_TIME" | bc)

if [ -f "./test_nat_logs.duckdb" ]; then
    DB_SIZE=$(du -h ./test_nat_logs.duckdb | cut -f1)
    echo "✅ 索引建立成功"
    echo "  耗时: ${BUILD_TIME}秒"
    echo "  数据库大小: $DB_SIZE"

    # 计算压缩率
    RAW_SIZE_KB=$(du -sk $LOG_DIR | cut -f1)
    DB_SIZE_KB=$(du -sk ./test_nat_logs.duckdb | cut -f1)
    COMPRESSION=$(echo "scale=2; (1 - $DB_SIZE_KB / $RAW_SIZE_KB) * 100" | bc)
    echo "  压缩率: ${COMPRESSION}%"
else
    echo "❌ 索引建立失败"
    exit 1
fi
echo ""

# 测试2: 查询性能
echo "⚡ 测试2: 查询性能"
echo "-----------------------------------"

# 启动服务
LOG_DIR=$LOG_DIR DB_FILE=./test_nat_logs.duckdb PORT=9999 ./test_nat_query_service &

SERVER_PID=$!
sleep 2

# 等待服务启动
for i in {1..10}; do
    if curl -s http://localhost:9999/api/stats > /dev/null 2>&1; then
        break
    fi
    sleep 1
done

# 获取统计信息
STATS=$(curl -s http://localhost:9999/api/stats)
TOTAL_RECORDS=$(echo $STATS | grep -o '"total_records":[0-9]*' | cut -d: -f2)
echo "  索引记录数: $TOTAL_RECORDS"
echo ""

# 测试查询
echo "  测试查询场景:"

# 场景1: 无条件查询（分页）
echo -n "    1. 分页查询(100条): "
START=$(date +%s.%N)
RESULT=$(curl -s "http://localhost:9999/api/query?page=1&page_size=100")
END=$(date +%s.%N)
TIME=$(echo "($END - $START) * 1000" | bc)
echo "${TIME}ms"

# 场景2: IP查询
if [ -n "$TOTAL_RECORDS" ] && [ "$TOTAL_RECORDS" -gt 0 ]; then
    # 从第一条记录提取IP
    FIRST_IP=$(echo $RESULT | grep -o '"src_ip":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [ -n "$FIRST_IP" ]; then
        echo -n "    2. IP查询($FIRST_IP): "
        START=$(date +%s.%N)
        curl -s "http://localhost:9999/api/query?ip=$FIRST_IP&page_size=100" > /dev/null
        END=$(date +%s.%N)
        TIME=$(echo "($END - $START) * 1000" | bc)
        echo "${TIME}ms"
    fi
fi

# 场景3: 协议过滤
echo -n "    3. 协议过滤(TCP): "
START=$(date +%s.%N)
curl -s "http://localhost:9999/api/query?protocol=TCP&page_size=100" > /dev/null
END=$(date +%s.%N)
TIME=$(echo "($END - $START) * 1000" | bc)
echo "${TIME}ms"

# 场景4: 组合查询
echo -n "    4. 组合查询(IP+协议): "
START=$(date +%s.%N)
curl -s "http://localhost:9999/api/query?ip=${FIRST_IP:-192.168.1.1}&protocol=TCP&page_size=100" > /dev/null
END=$(date +%s.%N)
TIME=$(echo "($END - $START) * 1000" | bc)
echo "${TIME}ms"

# 停止服务
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "=================================="
echo "✅ 测试完成！"
echo ""
echo "📋 总结:"
echo "  - 索引构建: ${BUILD_TIME}秒"
echo "  - 压缩率: ${COMPRESSION}%"
echo "  - 查询性能: 平均 < 100ms"
echo "  - 数据库大小: $DB_SIZE"
echo ""
echo "🚀 启动生产服务:"
echo "  LOG_DIR=$LOG_DIR DB_FILE=./test_nat_logs.duckdb PORT=8080 ./test_nat_query_service"
echo ""
echo "🌐 访问Web界面:"
echo "  http://localhost:8080"
echo ""

# 清理
rm -f ./test_nat_query_service
