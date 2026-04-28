#!/bin/bash
# 部署前检查脚本

echo "🔍 防火墙日志查询系统 - 部署检查"
echo "===================================="
echo ""

PASS=0
FAIL=0

# 检查函数
check() {
    if [ $? -eq 0 ]; then
        echo "  ✅ $1"
        ((PASS++))
    else
        echo "  ❌ $1"
        ((FAIL++))
    fi
}

# 1. 检查文件完整性
echo "📦 检查文件完整性..."
[ -f "main.go" ]; check "main.go"
[ -f "setup.sh" ]; check "setup.sh"
[ -f "Makefile" ]; check "Makefile"
[ -f "nat-query-service.service" ]; check "nat-query-service.service"
[ -f "go.mod" ]; check "go.mod"
[ -f "README_OPTIMIZED.md" ]; check "README_OPTIMIZED.md"
[ -f "QUICKSTART.md" ]; check "QUICKSTART.md"
[ -f "COMPARISON.md" ]; check "COMPARISON.md"
echo ""

# 2. 检查Go环境
echo "🔧 检查Go环境..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    echo "  ✅ Go已安装: $GO_VERSION"
    ((PASS++))
else
    echo "  ❌ Go未安装"
    ((FAIL++))
fi
echo ""

# 3. 检查目录结构
echo "📁 检查目录结构..."
[ -d "data" ]; check "data目录"
[ -d "data/sangfor_fw_log" ]; check "日志目录"
[ -d "data/index" ]; check "索引目录"
echo ""

# 4. 检查日志文件
echo "📄 检查日志文件..."
LOG_COUNT=$(ls -1 data/sangfor_fw_log/*.log 2>/dev/null | wc -l)
if [ "$LOG_COUNT" -gt 0 ]; then
    echo "  ✅ 找到 $LOG_COUNT 个日志文件"
    ((PASS++))

    # 显示日志文件信息
    for log in data/sangfor_fw_log/*.log; do
        SIZE=$(du -h "$log" | cut -f1)
        LINES=$(wc -l < "$log" 2>/dev/null || echo "0")
        echo "     - $(basename $log): $SIZE ($LINES 行)"
    done
else
    echo "  ⚠️  未找到日志文件"
    echo "     请将日志文件放入: data/sangfor_fw_log/"
fi
echo ""

# 5. 检查脚本权限
echo "🔐 检查脚本权限..."
[ -x "setup.sh" ]; check "setup.sh可执行"
[ -x "quickstart.sh" ]; check "quickstart.sh可执行"
[ -x "test_performance.sh" ]; check "test_performance.sh可执行"
echo ""

# 6. 检查端口占用
echo "🌐 检查端口占用..."
if command -v netstat &> /dev/null; then
    if netstat -tuln 2>/dev/null | grep -q ":8080 "; then
        echo "  ⚠️  端口8080已被占用"
        echo "     请使用 -port 参数指定其他端口"
    else
        echo "  ✅ 端口8080可用"
        ((PASS++))
    fi
elif command -v ss &> /dev/null; then
    if ss -tuln 2>/dev/null | grep -q ":8080 "; then
        echo "  ⚠️  端口8080已被占用"
    else
        echo "  ✅ 端口8080可用"
        ((PASS++))
    fi
else
    echo "  ⚠️  无法检查端口（netstat/ss未安装）"
fi
echo ""

# 7. 检查磁盘空间
echo "💾 检查磁盘空间..."
if command -v df &> /dev/null; then
    AVAILABLE=$(df -BG . | tail -1 | awk '{print $4}' | sed 's/G//')
    if [ "$AVAILABLE" -gt 1 ]; then
        echo "  ✅ 可用空间: ${AVAILABLE}GB"
        ((PASS++))
    else
        echo "  ⚠️  可用空间不足: ${AVAILABLE}GB"
        echo "     建议至少保留2GB空间"
    fi
else
    echo "  ⚠️  无法检查磁盘空间"
fi
echo ""

# 8. 检查系统信息
echo "💻 系统信息..."
echo "  操作系统: $(uname -s)"
echo "  内核版本: $(uname -r)"
echo "  架构: $(uname -m)"
if command -v nproc &> /dev/null; then
    echo "  CPU核心: $(nproc)"
fi
if command -v free &> /dev/null; then
    TOTAL_MEM=$(free -h | grep Mem | awk '{print $2}')
    echo "  总内存: $TOTAL_MEM"
fi
echo ""

# 9. 生成部署建议
echo "===================================="
echo "📊 检查结果: ✅ $PASS 项通过, ❌ $FAIL 项失败"
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo "🎉 所有检查通过！可以开始部署"
    echo ""
    echo "📋 部署步骤:"
    echo ""
    echo "  【Linux/Debian】"
    echo "    1. chmod +x setup.sh"
    echo "    2. sudo ./setup.sh"
    echo "    3. sudo systemctl start nat-query-service"
    echo "    4. 访问 http://服务器IP:8080"
    echo ""
    echo "  【Windows】"
    echo "    1. chmod +x quickstart.sh"
    echo "    2. ./quickstart.sh"
    echo "    3. 访问 http://localhost:8080"
    echo ""
    echo "  【手动编译】"
    echo "    1. go build -ldflags=\"-s -w\" -o nat-query-service main.go"
    echo "    2. LOG_DIR=./data/sangfor_fw_log DB_FILE=./data/index/nat_logs.duckdb ./nat-query-service"
    echo ""
else
    echo "⚠️  发现 $FAIL 个问题，请先解决后再部署"
    echo ""

    if ! command -v go &> /dev/null; then
        echo "  🔧 安装Go:"
        echo "     https://go.dev/dl/"
        echo ""
    fi

    if [ "$LOG_COUNT" -eq 0 ]; then
        echo "  📄 添加日志文件:"
        echo "     cp /path/to/*.log data/sangfor_fw_log/"
        echo ""
    fi
fi

echo "📖 查看完整文档:"
echo "   - 快速开始: cat QUICKSTART.md"
echo "   - 完整文档: cat README_OPTIMIZED.md"
echo "   - 性能对比: cat COMPARISON.md"
echo ""
