#!/bin/bash
# 性能测试脚本

echo "═══════════════════════════════════════════════════"
echo "  深信服防火墙日志查询工具 - 性能测试"
echo "═══════════════════════════════════════════════════"
echo ""

# 测试IP列表
TEST_IPS=(
    "2.55.81.95"    # 1942条记录
    "1.181.87.36"   # 1条记录
    "192.168.1.1"   # 可能不存在
)

# 测试函数
run_test() {
    local test_ip="$1"
    local test_name="$2"

    echo "测试 $test_name: $test_ip"
    echo -n "  查询中..."

    # 记录开始时间
    start=$(date +%s%N)

    # 执行查询
    result=$(bash sangforfw_log.sh "$test_ip" 2>&1 | grep "找到.*条匹配记录")

    # 记录结束时间
    end=$(date +%s%N)

    # 计算耗时（毫秒）
    elapsed=$(( (end - start) / 1000000 ))

    echo -e "\r  结果: $result"
    echo "  耗时: ${elapsed}ms"
    echo ""
}

# 检查索引状态
echo "【系统状态】"
if [ -f "data/index/sangfor_fw_log_index.db" ]; then
    INDEX_COUNT=$(wc -l < data/index/sangfor_fw_log_index.db)
    INDEX_SIZE=$(du -h data/index/sangfor_fw_log_index.db | cut -f1)
    echo "  索引记录: $INDEX_COUNT 条"
    echo "  索引大小: $INDEX_SIZE"
else
    echo "  索引状态: 未建立"
    echo "  请先运行脚本建立索引"
    exit 1
fi

LOG_SIZE=$(du -h data/sangfor_fw_log/*.log 2>/dev/null | awk '{sum+=$1} END {print sum}')
LOG_COUNT=$(ls data/sangfor_fw_log/*.log 2>/dev/null | wc -l)
echo "  日志文件: $LOG_COUNT 个"
echo "  日志大小: $(du -sh data/sangfor_fw_log 2>/dev/null | cut -f1)"
echo ""

# 运行测试
echo "【性能测试】"
echo ""

total_time=0
test_count=0

for i in "${!TEST_IPS[@]}"; do
    test_ip="${TEST_IPS[$i]}"

    # 执行3次取平均
    times=()
    for run in 1 2 3; do
        start=$(date +%s%N)
        bash sangforfw_log.sh "$test_ip" >/dev/null 2>&1
        end=$(date +%s%N)
        elapsed=$(( (end - start) / 1000000 ))
        times+=($elapsed)
    done

    # 计算平均值
    avg=$(( (${times[0]} + ${times[1]} + ${times[2]}) / 3 ))

    # 获取结果数量
    result=$(bash sangforfw_log.sh "$test_ip" 2>&1 | grep -o "找到 [0-9]* 条" || echo "找到 0 条")

    echo "测试 #$((i+1)): $test_ip"
    echo "  $result"
    echo "  第1次: ${times[0]}ms"
    echo "  第2次: ${times[1]}ms"
    echo "  第3次: ${times[2]}ms"
    echo "  平均: ${avg}ms"

    # 性能评级
    if [ $avg -lt 500 ]; then
        echo "  评级: ⭐⭐⭐⭐⭐ 优秀"
    elif [ $avg -lt 1000 ]; then
        echo "  评级: ⭐⭐⭐⭐ 良好"
    elif [ $avg -lt 2000 ]; then
        echo "  评级: ⭐⭐⭐ 一般"
    else
        echo "  评级: ⭐⭐ 需要优化"
    fi
    echo ""

    total_time=$((total_time + avg))
    test_count=$((test_count + 1))
done

# 总结
avg_total=$((total_time / test_count))
echo "═══════════════════════════════════════════════════"
echo "【测试总结】"
echo "  测试次数: $test_count 个IP × 3次"
echo "  平均耗时: ${avg_total}ms"
echo ""

# 性能建议
if [ $avg_total -gt 1000 ]; then
    echo "【优化建议】"
    echo "  当前查询速度: ${avg_total}ms"
    echo "  使用CDB优化后预期: 200-300ms"
    echo "  性能提升: 5-8倍"
    echo ""
    echo "  优化步骤:"
    echo "  1. 在银河麒麟系统上安装 tinycdb"
    echo "     sudo apt install tinycdb"
    echo "  2. 重新生成索引（自动生成CDB索引）"
    echo "  3. 查询速度将提升到 200-300ms"
else
    echo "【性能评估】"
    echo "  当前性能已经很好！"
    echo "  如需进一步优化，可考虑："
    echo "  - 使用SSD存储日志文件"
    echo "  - 安装ripgrep替代grep"
    echo "  - 增加并行查询进程数"
fi

echo "═══════════════════════════════════════════════════"
