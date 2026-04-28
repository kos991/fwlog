#!/bin/bash
# 编译tinycdb并部署到服务器
# 使用方法：./compile_and_deploy_tinycdb.sh root@your-server

set -e

RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[36m'
RESET='\033[0m'

if [ $# -eq 0 ]; then
    echo -e "${RED}错误: 请提供服务器地址${RESET}"
    echo "使用方法: $0 root@your-server"
    exit 1
fi

SERVER="$1"
REMOTE_DIR="/data/sangfor_fw_log_chaxun"

echo -e "${GREEN}========================================${RESET}"
echo -e "${GREEN}  编译并部署TinyCDB${RESET}"
echo -e "${GREEN}========================================${RESET}"
echo ""

# 步骤1: 解压源码
echo -e "${BLUE}[1/5] 解压tinycdb源码...${RESET}"
cd /d/sql
if [ -d tinycdb-0.78 ]; then
    rm -rf tinycdb-0.78
fi
tar xzf tinycdb-0.78.tar.gz
echo -e "${GREEN}✓ 解压完成${RESET}"

# 步骤2: 编译
echo ""
echo -e "${BLUE}[2/5] 编译tinycdb...${RESET}"
cd tinycdb-0.78

# 使用make编译
if make 2>&1 | grep -q "error"; then
    echo -e "${RED}✗ 编译失败${RESET}"
    exit 1
fi

# 检查生成的文件
if [ -f cdb ] && [ -f cdbmake ]; then
    echo -e "${GREEN}✓ 编译成功${RESET}"
    ls -lh cdb cdbmake
else
    echo -e "${RED}✗ 未找到编译产物${RESET}"
    exit 1
fi

# 步骤3: 测试本地功能
echo ""
echo -e "${BLUE}[3/5] 测试本地功能...${RESET}"
echo "+3,5:abc->12345" | ./cdbmake /tmp/test.cdb /tmp/test.cdb.tmp 2>&1
if [ -f /tmp/test.cdb ]; then
    RESULT=$(./cdb -q /tmp/test.cdb abc 2>&1)
    if [ "$RESULT" = "12345" ]; then
        echo -e "${GREEN}✓ 功能测试通过${RESET}"
    else
        echo -e "${RED}✗ 查询结果错误: $RESULT${RESET}"
        exit 1
    fi
    rm -f /tmp/test.cdb /tmp/test.cdb.tmp
else
    echo -e "${RED}✗ 测试失败${RESET}"
    exit 1
fi

# 步骤4: 上传到服务器
echo ""
echo -e "${BLUE}[4/5] 上传到服务器...${RESET}"

# 创建bin目录
ssh "$SERVER" "mkdir -p $REMOTE_DIR/bin"

# 上传二进制文件
echo -n "  上传 cdb... "
if scp -q cdb "$SERVER:$REMOTE_DIR/bin/"; then
    echo -e "${GREEN}✓${RESET}"
else
    echo -e "${RED}✗${RESET}"
    exit 1
fi

echo -n "  上传 cdbmake... "
if scp -q cdbmake "$SERVER:$REMOTE_DIR/bin/"; then
    echo -e "${GREEN}✓${RESET}"
else
    echo -e "${RED}✗${RESET}"
    exit 1
fi

# 步骤5: 在服务器上配置
echo ""
echo -e "${BLUE}[5/5] 服务器配置...${RESET}"
ssh "$SERVER" << 'EOF'
cd /data/sangfor_fw_log_chaxun

# 设置执行权限
chmod +x bin/cdb bin/cdbmake

# 测试功能
echo -n "  测试cdb... "
if bin/cdb -h >/dev/null 2>&1; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

echo -n "  测试cdbmake... "
if bin/cdbmake -h >/dev/null 2>&1; then
    echo "✓"
else
    echo "✗"
    exit 1
fi

# 功能测试
echo -n "  功能测试... "
echo "+3,5:abc->12345" | bin/cdbmake /tmp/test.cdb /tmp/test.cdb.tmp 2>&1
RESULT=$(bin/cdb -q /tmp/test.cdb abc 2>&1)
if [ "$RESULT" = "12345" ]; then
    echo "✓"
    rm -f /tmp/test.cdb /tmp/test.cdb.tmp
else
    echo "✗ 结果: $RESULT"
    exit 1
fi

# 创建软链接到PATH（可选）
echo -n "  创建软链接... "
sudo ln -sf /data/sangfor_fw_log_chaxun/bin/cdb /usr/local/bin/cdb 2>/dev/null || true
sudo ln -sf /data/sangfor_fw_log_chaxun/bin/cdbmake /usr/local/bin/cdbmake 2>/dev/null || true
echo "✓"

echo ""
echo "TinyCDB路径："
echo "  cdb: $(pwd)/bin/cdb"
echo "  cdbmake: $(pwd)/bin/cdbmake"
EOF

# 清理本地编译文件
cd /d/sql
rm -rf tinycdb-0.78

echo ""
echo -e "${GREEN}========================================${RESET}"
echo -e "${GREEN}  部署完成！${RESET}"
echo -e "${GREEN}========================================${RESET}"
echo ""
echo "下一步："
echo "  1. 上传新版本脚本: scp sangforfw_log.sh $SERVER:$REMOTE_DIR/"
echo "  2. SSH登录: ssh $SERVER"
echo "  3. 生成CDB索引: cd $REMOTE_DIR && ./sangforfw_log.sh"
echo "     选择: 3. 更新索引"
echo ""
