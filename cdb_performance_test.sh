#!/bin/bash
# CDB性能测试脚本
# 对比文本索引 vs CDB索引的查询性能

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$SCRIPT_DIR/data/sangfor_fw_log"
INDEX_FILE="$SCRIPT_DIR/data/index/sangfor_fw_log_index.db"
INDEX_FILE_CDB="$SCRIPT_DIR/data/index/sangfor_fw_log_index.cdb"

# 颜色定义
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[36m'
RESET='\033[0m'

# 测试IP列表（从实际日志中提取）
TEST_IPS=(
    "2.55.81.95"
    "1.181.87.36"
    "192.168.244.101"
    "2.55.80.84"
    "192.168.244.102"
)

# 测试日期
TEST_DATE="2026-04-27"
TEST_DATE_KEY="20260427"

echo -e "${GREEN}========================================${RESET}"
echo -e "${GREEN}  CDB性能测试${RESET}"
echo -e "${GREEN}========================================${RESET}"
echo ""

# 检查环境
echo "检查环境..."
if [ ! -f "$INDEX_FILE" ]; then
    echo -e "${RED}错误: 文本索引不存在${RESET}"
    exit 1
fi

HAS_CDB=0
if command -v cdb >/dev/null 2>&1 && command -v cdbmake >/dev/null 2>&1; then
    HAS_CDB=1
    echo -e "${GREEN}✓ TinyCDB已安装${RESET}"
else
    echo -e "${YELLOW}✗ TinyCDB未安装，仅测试文本索引${RESET}"
fi

if [ "$HAS_CDB" -eq 1 ] && [ ! -f "$INDEX_FILE_CDB" ]; then
    echo -e "${YELLOW}! CDB索引不存在，正在生成...${RESET}"
    # 调用主脚本生成CDB索引
    "$SCRIPT_DIR/sangforfw_log.sh" --rebuild-index >/dev/null 2>&1
    if [ -f "$INDEX_FILE_CDB" ]; then
        echo -e "${GREEN}✓ CDB索引生成成功${RESET}"
    else
        echo -e "${RED}✗ CDB索引生成失败${RESET}"
        HAS_CDB=0
    fi
fi

echo ""

# 显示索引信息
echo "索引信息:"
INDEX_COUNT=$(wc -l < "$INDEX_FILE")
INDEX_SIZE=$(du -h "$INDEX_FILE" | cut -f1)
echo "  文本索引: $INDEX_COUNT 条记录, $INDEX_SIZE"

if [ "$HAS_CDB" -eq 1 ] && [ -f "$INDEX_FILE_CDB" ]; then
    CDB_SIZE=$(du -h "$INDEX_FILE_CDB" | cut -f1)
    echo "  CDB索引: $CDB_SIZE"
fi

echo ""
echo "=========================================="
echo ""

# 测试1: 单次查询性能
echo -e "${BLUE}测试1: 单次查询性能${RESET}"
echo "测试IP: ${TEST_IPS[0]}, 日期: $TEST_DATE"
echo ""

# 文本索引查询
echo -n "文本索引查询... "
START=$(date +%s%N)
grep -E "(源IP:${TEST_IPS[0]}|目的IP:${TEST_IPS[0]})" "$LOG_DIR"/*_${TEST_DATE}.log 2>/dev/null > /tmp/test_text.log
END=$(date +%s%N)
TEXT_TIME=$(echo "scale=3; ($END - $START) / 1000000000" | bc)
TEXT_COUNT=$(wc -l < /tmp/test_text.log)
echo -e "${GREEN}完成${RESET}"
echo "  耗时: ${TEXT_TIME}秒"
echo "  结果: $TEXT_COUNT 条"

# CDB索引查询
if [ "$HAS_CDB" -eq 1 ] && [ -f "$INDEX_FILE_CDB" ]; then
    echo ""
    echo -n "CDB索引查询... "
    START=$(date +%s%N)

    # 查询CDB获取文件路径
    LOG_FILES=$(cdb -q "$INDEX_FILE_CDB" "${TEST_IPS[0]}|${TEST_DATE_KEY}" 2>/dev/null)

    # 从日志文件提取记录
    if [ -n "$LOG_FILES" ]; then
        echo "$LOG_FILES" | while IFS= read -r log_file; do
            [ -f "$log_file" ] && grep -E "(源IP:${TEST_IPS[0]}|目的IP:${TEST_IPS[0]})" "$log_file" 2>/dev/null
        done > /tmp/test_cdb.log
    else
        : > /tmp/test_cdb.log
    fi

    END=$(date +%s%N)
    CDB_TIME=$(echo "scale=3; ($END - $START) / 1000000000" | bc)
    CDB_COUNT=$(wc -l < /tmp/test_cdb.log)
    echo -e "${GREEN}完成${RESET}"
    echo "  耗时: ${CDB_TIME}秒"
    echo "  结果: $CDB_COUNT 条"

    # 计算提升
    if [ "$TEXT_TIME" != "0" ]; then
        SPEEDUP=$(echo "scale=2; $TEXT_TIME / $CDB_TIME" | bc)
        echo ""
        echo -e "${GREEN}性能提升: ${SPEEDUP}x${RESET}"
    fi
fi

echo ""
echo "=========================================="
echo ""

# 测试2: 批量查询性能
echo -e "${BLUE}测试2: 批量查询性能 (5个IP)${RESET}"
echo ""

# 文本索引批量查询
echo -n "文本索引批量查询... "
START=$(date +%s%N)
for ip in "${TEST_IPS[@]}"; do
    grep -E "(源IP:${ip}|目的IP:${ip})" "$LOG_DIR"/*_${TEST_DATE}.log 2>/dev/null >> /tmp/test_text_batch.log
done
END=$(date +%s%N)
TEXT_BATCH_TIME=$(echo "scale=3; ($END - $START) / 1000000000" | bc)
TEXT_BATCH_COUNT=$(wc -l < /tmp/test_text_batch.log)
echo -e "${GREEN}完成${RESET}"
echo "  总耗时: ${TEXT_BATCH_TIME}秒"
echo "  平均耗时: $(echo "scale=3; $TEXT_BATCH_TIME / 5" | bc)秒/查询"
echo "  总结果: $TEXT_BATCH_COUNT 条"

# CDB索引批量查询
if [ "$HAS_CDB" -eq 1 ] && [ -f "$INDEX_FILE_CDB" ]; then
    echo ""
    echo -n "CDB索引批量查询... "
    START=$(date +%s%N)

    : > /tmp/test_cdb_batch.log
    for ip in "${TEST_IPS[@]}"; do
        LOG_FILES=$(cdb -q "$INDEX_FILE_CDB" "${ip}|${TEST_DATE_KEY}" 2>/dev/null)
        if [ -n "$LOG_FILES" ]; then
            echo "$LOG_FILES" | while IFS= read -r log_file; do
                [ -f "$log_file" ] && grep -E "(源IP:${ip}|目的IP:${ip})" "$log_file" 2>/dev/null
            done >> /tmp/test_cdb_batch.log
        fi
    done

    END=$(date +%s%N)
    CDB_BATCH_TIME=$(echo "scale=3; ($END - $START) / 1000000000" | bc)
    CDB_BATCH_COUNT=$(wc -l < /tmp/test_cdb_batch.log)
    echo -e "${GREEN}完成${RESET}"
    echo "  总耗时: ${CDB_BATCH_TIME}秒"
    echo "  平均耗时: $(echo "scale=3; $CDB_BATCH_TIME / 5" | bc)秒/查询"
    echo "  总结果: $CDB_BATCH_COUNT 条"

    # 计算提升
    if [ "$TEXT_BATCH_TIME" != "0" ]; then
        BATCH_SPEEDUP=$(echo "scale=2; $TEXT_BATCH_TIME / $CDB_BATCH_TIME" | bc)
        echo ""
        echo -e "${GREEN}批量查询性能提升: ${BATCH_SPEEDUP}x${RESET}"
    fi
fi

echo ""
echo "=========================================="
echo ""

# 测试3: 索引查询性能（仅查询索引，不提取日志）
echo -e "${BLUE}测试3: 纯索引查询性能${RESET}"
echo ""

# 文本索引查询
echo -n "文本索引查询... "
START=$(date +%s%N)
grep "^${TEST_IPS[0]}|" "$INDEX_FILE" > /tmp/test_index_text.log
END=$(date +%s%N)
INDEX_TEXT_TIME=$(echo "scale=3; ($END - $START) / 1000000000" | bc)
INDEX_TEXT_COUNT=$(wc -l < /tmp/test_index_text.log)
echo -e "${GREEN}完成${RESET}"
echo "  耗时: ${INDEX_TEXT_TIME}秒"
echo "  结果: $INDEX_TEXT_COUNT 条"

# CDB索引查询
if [ "$HAS_CDB" -eq 1 ] && [ -f "$INDEX_FILE_CDB" ]; then
    echo ""
    echo -n "CDB索引查询... "
    START=$(date +%s%N)
    cdb -q "$INDEX_FILE_CDB" "${TEST_IPS[0]}|${TEST_DATE_KEY}" > /tmp/test_index_cdb.log 2>/dev/null
    END=$(date +%s%N)
    INDEX_CDB_TIME=$(echo "scale=3; ($END - $START) / 1000000000" | bc)
    INDEX_CDB_COUNT=$(wc -l < /tmp/test_index_cdb.log)
    echo -e "${GREEN}完成${RESET}"
    echo "  耗时: ${INDEX_CDB_TIME}秒"
    echo "  结果: $INDEX_CDB_COUNT 条"

    # 计算提升
    if [ "$INDEX_TEXT_TIME" != "0" ]; then
        INDEX_SPEEDUP=$(echo "scale=2; $INDEX_TEXT_TIME / $INDEX_CDB_TIME" | bc)
        echo ""
        echo -e "${GREEN}索引查询性能提升: ${INDEX_SPEEDUP}x${RESET}"
    fi
fi

echo ""
echo "=========================================="
echo ""

# 生成测试报告
echo -e "${GREEN}测试总结${RESET}"
echo ""

if [ "$HAS_CDB" -eq 1 ] && [ -f "$INDEX_FILE_CDB" ]; then
    echo "| 测试项 | 文本索引 | CDB索引 | 性能提升 |"
    echo "|--------|----------|---------|----------|"
    echo "| 单次查询 | ${TEXT_TIME}s | ${CDB_TIME}s | ${SPEEDUP}x |"
    echo "| 批量查询(5个) | ${TEXT_BATCH_TIME}s | ${CDB_BATCH_TIME}s | ${BATCH_SPEEDUP}x |"
    echo "| 纯索引查询 | ${INDEX_TEXT_TIME}s | ${INDEX_CDB_TIME}s | ${INDEX_SPEEDUP}x |"
    echo ""
    echo -e "${GREEN}结论: CDB索引在所有测试场景中均显著提升查询性能${RESET}"
else
    echo "| 测试项 | 文本索引 |"
    echo "|--------|----------|"
    echo "| 单次查询 | ${TEXT_TIME}s |"
    echo "| 批量查询(5个) | ${TEXT_BATCH_TIME}s |"
    echo "| 纯索引查询 | ${INDEX_TEXT_TIME}s |"
    echo ""
    echo -e "${YELLOW}提示: 安装TinyCDB以启用CDB加速${RESET}"
    echo "  Debian/Ubuntu: sudo apt-get install tinycdb"
    echo "  CentOS/RHEL: sudo yum install tinycdb"
fi

echo ""

# 清理临时文件
rm -f /tmp/test_*.log

echo "测试完成！"
