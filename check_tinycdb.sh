#!/bin/bash
# TinyCDB安装检查和修复脚本
# 在服务器上运行此脚本

echo "=========================================="
echo "  TinyCDB 安装检查"
echo "=========================================="
echo ""

# 检查1: 命令是否存在
echo "[1] 检查命令..."
echo -n "  cdb: "
if command -v cdb >/dev/null 2>&1; then
    echo "✓ $(which cdb)"
else
    echo "✗ 未找到"
fi

echo -n "  cdbmake: "
if command -v cdbmake >/dev/null 2>&1; then
    echo "✓ $(which cdbmake)"
else
    echo "✗ 未找到"
fi

echo ""

# 检查2: 包是否安装
echo "[2] 检查包安装状态..."
if command -v dpkg >/dev/null 2>&1; then
    # Debian/Ubuntu
    echo "  系统: Debian/Ubuntu"
    dpkg -l | grep tinycdb || echo "  ✗ tinycdb包未安装"
elif command -v rpm >/dev/null 2>&1; then
    # CentOS/RHEL
    echo "  系统: CentOS/RHEL"
    rpm -qa | grep tinycdb || echo "  ✗ tinycdb包未安装"
fi

echo ""

# 检查3: 查找可执行文件
echo "[3] 搜索可执行文件..."
echo "  查找 cdb:"
find /usr -name "cdb" -type f 2>/dev/null | head -5
echo "  查找 cdbmake:"
find /usr -name "cdbmake" -type f 2>/dev/null | head -5

echo ""

# 检查4: PATH环境变量
echo "[4] 检查PATH..."
echo "  当前PATH: $PATH"

echo ""

# 检查5: 测试安装
echo "[5] 尝试安装TinyCDB..."
if command -v apt-get >/dev/null 2>&1; then
    echo "  使用apt-get安装..."
    sudo apt-get update
    sudo apt-get install -y tinycdb
elif command -v yum >/dev/null 2>&1; then
    echo "  使用yum安装..."
    sudo yum install -y tinycdb
elif command -v dnf >/dev/null 2>&1; then
    echo "  使用dnf安装..."
    sudo dnf install -y tinycdb
else
    echo "  ✗ 无法识别包管理器"
fi

echo ""

# 检查6: 再次验证
echo "[6] 安装后验证..."
echo -n "  cdb: "
if command -v cdb >/dev/null 2>&1; then
    cdb -h 2>&1 | head -1
    echo "  ✓ 安装成功"
else
    echo "  ✗ 仍未找到"
fi

echo -n "  cdbmake: "
if command -v cdbmake >/dev/null 2>&1; then
    cdbmake -h 2>&1 | head -1
    echo "  ✓ 安装成功"
else
    echo "  ✗ 仍未找到"
fi

echo ""

# 检查7: 测试功能
echo "[7] 功能测试..."
if command -v cdb >/dev/null 2>&1 && command -v cdbmake >/dev/null 2>&1; then
    echo "  创建测试CDB..."
    echo "+3,5:abc->12345" | cdbmake /tmp/test.cdb /tmp/test.cdb.tmp 2>&1

    if [ -f /tmp/test.cdb ]; then
        echo "  ✓ CDB文件创建成功"

        echo "  测试查询..."
        RESULT=$(cdb -q /tmp/test.cdb abc 2>&1)
        if [ "$RESULT" = "12345" ]; then
            echo "  ✓ 查询功能正常"
        else
            echo "  ✗ 查询结果错误: $RESULT"
        fi

        rm -f /tmp/test.cdb /tmp/test.cdb.tmp
    else
        echo "  ✗ CDB文件创建失败"
    fi
else
    echo "  ✗ 跳过（命令不可用）"
fi

echo ""
echo "=========================================="
echo "  检查完成"
echo "=========================================="
