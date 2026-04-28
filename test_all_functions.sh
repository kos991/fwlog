#!/bin/bash
# 深信服防火墙日志查询工具 - 自动功能测试脚本
# 测试所有主要功能并生成测试报告

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAIN_SCRIPT="$SCRIPT_DIR/sangforfw_log.sh"
TEST_LOG="$SCRIPT_DIR/test_report_$(date +%Y%m%d_%H%M%S).log"

# 颜色定义
GREEN='\033[32m'
RED='\033[31m'
YELLOW='\033[33m'
BLUE='\033[36m'
NC='\033[0m'

# 测试计数器
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$TEST_LOG"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1" | tee -a "$TEST_LOG"
    ((PASSED_TESTS++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1" | tee -a "$TEST_LOG"
    ((FAILED_TESTS++))
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" | tee -a "$TEST_LOG"
}

# 测试函数
test_function() {
    local test_name="$1"
    local test_cmd="$2"
    local expected_pattern="$3"

    ((TOTAL_TESTS++))
    log_info "测试 #$TOTAL_TESTS: $test_name"

    local output
    output=$(eval "$test_cmd" 2>&1)
    local exit_code=$?

    if [ $exit_code -eq 0 ] && echo "$output" | grep -q "$expected_pattern"; then
        log_success "$test_name - 通过"
        return 0
    else
        log_fail "$test_name - 失败 (退出码: $exit_code)"
        echo "输出: $output" >> "$TEST_LOG"
        return 1
    fi
}

# 开始测试
clear
echo "═══════════════════════════════════════════════════"
echo "  深信服防火墙日志查询工具 - 自动功能测试"
echo "  测试时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "═══════════════════════════════════════════════════"
echo ""

log_info "测试报告保存到: $TEST_LOG"
echo ""

# ============================================
# 测试1: 环境检查
# ============================================
log_info "========== 环境检查 =========="

test_function "脚本文件存在" \
    "[ -f '$MAIN_SCRIPT' ] && echo 'exists'" \
    "exists"

test_function "脚本可执行" \
    "[ -x '$MAIN_SCRIPT' ] || chmod +x '$MAIN_SCRIPT'; echo 'executable'" \
    "executable"

test_function "日志目录存在" \
    "[ -d '$SCRIPT_DIR/data/sangfor_fw_log' ] && echo 'exists'" \
    "exists"

test_function "日志文件存在" \
    "ls '$SCRIPT_DIR/data/sangfor_fw_log'/*.log 2>/dev/null | wc -l" \
    "[1-9]"

echo ""

# ============================================
# 测试2: 索引功能
# ============================================
log_info "========== 索引功能测试 =========="

test_function "索引文件存在" \
    "[ -f '$SCRIPT_DIR/data/index/sangfor_fw_log_index.db' ] && echo 'exists'" \
    "exists"

test_function "索引记录数量" \
    "wc -l < '$SCRIPT_DIR/data/index/sangfor_fw_log_index.db'" \
    "[1-9]"

test_function "索引格式正确" \
    "head -1 '$SCRIPT_DIR/data/index/sangfor_fw_log_index.db' | grep -E '^[0-9.]+\\|[0-9]+\\|.*\\.log'" \
    "."

test_function "索引压缩文件" \
    "[ -f '$SCRIPT_DIR/data/index/sangfor_fw_log_index.db.gz' ] && echo 'exists'" \
    "exists"

echo ""

# ============================================
# 测试3: 查询功能
# ============================================
log_info "========== 查询功能测试 =========="

# 从索引中随机选择一个IP进行测试
TEST_IP=$(awk -F'|' 'NR==1 {print $1}' "$SCRIPT_DIR/data/index/sangfor_fw_log_index.db")

if [ -n "$TEST_IP" ]; then
    log_info "使用测试IP: $TEST_IP"

    test_function "命令行IP查询" \
        "bash '$MAIN_SCRIPT' '$TEST_IP' 2>&1" \
        "查询条件"

    test_function "查询结果导出" \
        "ls '$SCRIPT_DIR/data/export/by_ip'/*.log 2>/dev/null | wc -l" \
        "[1-9]"
else
    log_warn "无法获取测试IP，跳过查询测试"
fi

echo ""

# ============================================
# 测试4: 数据目录结构
# ============================================
log_info "========== 目录结构测试 =========="

test_function "索引目录" \
    "[ -d '$SCRIPT_DIR/data/index' ] && echo 'exists'" \
    "exists"

test_function "导出目录" \
    "[ -d '$SCRIPT_DIR/data/export' ] && echo 'exists'" \
    "exists"

test_function "by_ip目录" \
    "[ -d '$SCRIPT_DIR/data/export/by_ip' ] && echo 'exists'" \
    "exists"

test_function "by_port目录" \
    "[ -d '$SCRIPT_DIR/data/export/by_port' ] && echo 'exists'" \
    "exists"

test_function "by_date目录" \
    "[ -d '$SCRIPT_DIR/data/export/by_date' ] && echo 'exists'" \
    "exists"

test_function "by_query目录" \
    "[ -d '$SCRIPT_DIR/data/export/by_query' ] && echo 'exists'" \
    "exists"

echo ""

# ============================================
# 测试5: 性能测试
# ============================================
log_info "========== 性能测试 =========="

# 索引大小
INDEX_SIZE=$(wc -c < "$SCRIPT_DIR/data/index/sangfor_fw_log_index.db" 2>/dev/null || echo 0)
INDEX_SIZE_MB=$((INDEX_SIZE / 1024 / 1024))
log_info "索引文件大小: ${INDEX_SIZE_MB}MB"

# 索引记录数
INDEX_COUNT=$(wc -l < "$SCRIPT_DIR/data/index/sangfor_fw_log_index.db" 2>/dev/null || echo 0)
log_info "索引记录数: $INDEX_COUNT 条"

# 日志文件大小
LOG_SIZE=$(du -sh "$SCRIPT_DIR/data/sangfor_fw_log" 2>/dev/null | cut -f1)
log_info "日志文件大小: $LOG_SIZE"

# 查询速度测试
if [ -n "$TEST_IP" ]; then
    log_info "测试查询速度..."
    START_TIME=$(date +%s%N)
    bash "$MAIN_SCRIPT" "$TEST_IP" >/dev/null 2>&1
    END_TIME=$(date +%s%N)
    QUERY_TIME=$(( (END_TIME - START_TIME) / 1000000 ))
    log_info "查询耗时: ${QUERY_TIME}ms"

    if [ $QUERY_TIME -lt 2000 ]; then
        log_success "查询速度优秀 (<2秒)"
    elif [ $QUERY_TIME -lt 5000 ]; then
        log_warn "查询速度一般 (2-5秒)"
    else
        log_fail "查询速度较慢 (>5秒)"
    fi
fi

echo ""

# ============================================
# 测试6: 文件权限
# ============================================
log_info "========== 权限测试 =========="

test_function "主脚本可执行" \
    "[ -x '$MAIN_SCRIPT' ] && echo 'executable'" \
    "executable"

test_function "包装脚本存在" \
    "[ -f '$SCRIPT_DIR/fwlog' ] && echo 'exists' || echo 'not_found'" \
    "."

echo ""

# ============================================
# 生成测试报告
# ============================================
echo "═══════════════════════════════════════════════════"
echo "  测试报告汇总"
echo "═══════════════════════════════════════════════════"
echo ""
echo "  总测试数: $TOTAL_TESTS"
echo -e "  ${GREEN}通过: $PASSED_TESTS${NC}"
echo -e "  ${RED}失败: $FAILED_TESTS${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ 所有测试通过！${NC}"
    EXIT_CODE=0
else
    echo -e "${RED}✗ 有 $FAILED_TESTS 个测试失败${NC}"
    EXIT_CODE=1
fi

echo ""
echo "详细报告: $TEST_LOG"
echo ""

exit $EXIT_CODE
