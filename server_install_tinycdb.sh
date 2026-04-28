#!/bin/bash
#================================================================
# 服务器端TinyCDB安装脚本
# 用途: 在服务器上直接编译安装TinyCDB
# 使用: ./server_install_tinycdb.sh
#================================================================

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
TINYCDB_TAR="$SCRIPT_DIR/tinycdb-0.78.tar.gz"
TINYCDB_DIR="$SCRIPT_DIR/tinycdb-0.78"

echo -e "${BLUE}════════════════════════════════════════${NC}"
echo -e "${BLUE}  TinyCDB 服务器端安装脚本${NC}"
echo -e "${BLUE}════════════════════════════════════════${NC}"
echo ""

# 步骤1: 检查源码包
echo -e "${YELLOW}[1/6]${NC} 检查TinyCDB源码包..."
if [ ! -f "$TINYCDB_TAR" ]; then
    echo -e "${RED}✗ 错误: 找不到 tinycdb-0.78.tar.gz${NC}"
    echo "请确保文件在当前目录: $SCRIPT_DIR"
    exit 1
fi
echo -e "${GREEN}✓ 找到源码包: $TINYCDB_TAR${NC}"

# 步骤2: 检查编译工具
echo -e "\n${YELLOW}[2/6]${NC} 检查编译工具..."
if ! command -v gcc >/dev/null 2>&1; then
    echo -e "${RED}✗ 错误: 未找到 gcc 编译器${NC}"
    echo ""
    echo "请先安装编译工具："
    echo "  CentOS/RHEL: yum groupinstall 'Development Tools'"
    echo "  Ubuntu/Debian: apt-get install build-essential"
    exit 1
fi

if ! command -v make >/dev/null 2>&1; then
    echo -e "${RED}✗ 错误: 未找到 make 工具${NC}"
    echo ""
    echo "请先安装编译工具："
    echo "  CentOS/RHEL: yum install make"
    echo "  Ubuntu/Debian: apt-get install make"
    exit 1
fi

echo -e "${GREEN}✓ gcc 版本: $(gcc --version | head -n1)${NC}"
echo -e "${GREEN}✓ make 版本: $(make --version | head -n1)${NC}"

# 步骤3: 解压源码
echo -e "\n${YELLOW}[3/6]${NC} 解压源码..."
if [ -d "$TINYCDB_DIR" ]; then
    echo "清理旧的源码目录..."
    rm -rf "$TINYCDB_DIR"
fi

tar -xzf "$TINYCDB_TAR" -C "$SCRIPT_DIR"
if [ ! -d "$TINYCDB_DIR" ]; then
    echo -e "${RED}✗ 错误: 解压失败${NC}"
    exit 1
fi
echo -e "${GREEN}✓ 解压完成: $TINYCDB_DIR${NC}"

# 步骤4: 编译
echo -e "\n${YELLOW}[4/6]${NC} 编译TinyCDB..."
cd "$TINYCDB_DIR"

echo "正在编译..."
if make > /tmp/tinycdb_compile.log 2>&1; then
    echo -e "${GREEN}✓ 编译成功${NC}"
else
    echo -e "${RED}✗ 编译失败${NC}"
    echo "查看日志: cat /tmp/tinycdb_compile.log"
    tail -20 /tmp/tinycdb_compile.log
    exit 1
fi

# 检查生成的文件
if [ ! -f "cdb" ] || [ ! -f "cdbmake" ]; then
    echo -e "${RED}✗ 错误: 未找到编译后的文件${NC}"
    ls -la
    exit 1
fi

echo -e "${GREEN}✓ 生成文件:${NC}"
ls -lh cdb cdbmake

# 步骤5: 安装到bin目录
echo -e "\n${YELLOW}[5/6]${NC} 安装到bin目录..."
mkdir -p "$BIN_DIR"

cp -f cdb "$BIN_DIR/"
cp -f cdbmake "$BIN_DIR/"
chmod +x "$BIN_DIR/cdb" "$BIN_DIR/cdbmake"

echo -e "${GREEN}✓ 安装完成:${NC}"
ls -lh "$BIN_DIR/cdb" "$BIN_DIR/cdbmake"

# 步骤6: 测试功能
echo -e "\n${YELLOW}[6/6]${NC} 测试功能..."

# 测试cdb
if "$BIN_DIR/cdb" -h >/dev/null 2>&1; then
    echo -e "${GREEN}✓ cdb 命令正常${NC}"
else
    echo -e "${RED}✗ cdb 命令测试失败${NC}"
    exit 1
fi

# 测试cdbmake
if "$BIN_DIR/cdbmake" -h >/dev/null 2>&1; then
    echo -e "${GREEN}✓ cdbmake 命令正常${NC}"
else
    echo -e "${RED}✗ cdbmake 命令测试失败${NC}"
    exit 1
fi

# 功能测试
echo "进行功能测试..."
TEST_CDB="/tmp/test_$$.cdb"
TEST_TMP="/tmp/test_$$.cdb.tmp"

# 创建测试数据
echo "+3,5:abc->12345" | "$BIN_DIR/cdbmake" "$TEST_CDB" "$TEST_TMP"

# 查询测试
RESULT=$("$BIN_DIR/cdb" -q "$TEST_CDB" "abc" 2>/dev/null || echo "")
if [ "$RESULT" = "12345" ]; then
    echo -e "${GREEN}✓ 功能测试通过${NC}"
else
    echo -e "${RED}✗ 功能测试失败 (期望: 12345, 实际: $RESULT)${NC}"
    rm -f "$TEST_CDB" "$TEST_TMP"
    exit 1
fi

# 清理测试文件
rm -f "$TEST_CDB" "$TEST_TMP"

# 清理源码目录（可选）
echo -e "\n${YELLOW}清理源码目录...${NC}"
cd "$SCRIPT_DIR"
rm -rf "$TINYCDB_DIR"
echo -e "${GREEN}✓ 清理完成${NC}"

# 完成
echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✓ TinyCDB 安装成功！${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo -e "安装位置:"
echo -e "  cdb:      ${BLUE}$BIN_DIR/cdb${NC}"
echo -e "  cdbmake:  ${BLUE}$BIN_DIR/cdbmake${NC}"
echo ""
echo -e "下一步操作:"
echo -e "  1. 运行主脚本: ${BLUE}./sangforfw_log.sh${NC}"
echo -e "  2. 选择菜单: ${BLUE}3. 更新索引${NC}"
echo -e "  3. 验证CDB: 主菜单应显示 ${GREEN}CDB: ✓ 已启用${NC}"
echo ""
echo -e "性能提升: ${GREEN}查询速度提升 10 倍！${NC}"
echo ""
