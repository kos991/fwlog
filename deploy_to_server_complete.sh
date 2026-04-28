#!/bin/bash
#================================================================
# 一键部署到服务器脚本
# 用途: 将所有文件上传到服务器并自动安装
# 使用: ./deploy_to_server_complete.sh root@your-server-ip
#================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 检查参数
if [ $# -eq 0 ]; then
    echo -e "${RED}错误: 请提供服务器地址${NC}"
    echo ""
    echo "使用方法:"
    echo "  $0 root@your-server-ip"
    echo ""
    echo "示例:"
    echo "  $0 root@192.168.1.100"
    echo "  $0 admin@server.example.com"
    exit 1
fi

SERVER="$1"
REMOTE_DIR="/data/sangfor_fw_log_chaxun"

echo -e "${BLUE}════════════════════════════════════════${NC}"
echo -e "${BLUE}  一键部署到服务器${NC}"
echo -e "${BLUE}════════════════════════════════════════${NC}"
echo ""
echo -e "目标服务器: ${GREEN}$SERVER${NC}"
echo -e "部署目录:   ${GREEN}$REMOTE_DIR${NC}"
echo ""

# 步骤1: 测试SSH连接
echo -e "${YELLOW}[1/5]${NC} 测试SSH连接..."
if ssh -o ConnectTimeout=5 "$SERVER" "echo 'SSH连接成功'" >/dev/null 2>&1; then
    echo -e "${GREEN}✓ SSH连接正常${NC}"
else
    echo -e "${RED}✗ SSH连接失败${NC}"
    echo "请检查:"
    echo "  1. 服务器地址是否正确"
    echo "  2. SSH密钥是否配置"
    echo "  3. 网络是否畅通"
    exit 1
fi

# 步骤2: 检查本地文件
echo -e "\n${YELLOW}[2/5]${NC} 检查本地文件..."
REQUIRED_FILES=(
    "sangforfw_log.sh"
    "tinycdb-0.78.tar.gz"
    "server_install_tinycdb.sh"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ ! -f "$file" ]; then
        echo -e "${RED}✗ 缺少文件: $file${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ $file${NC}"
done

# 步骤3: 创建远程目录
echo -e "\n${YELLOW}[3/5]${NC} 创建远程目录..."
ssh "$SERVER" "mkdir -p $REMOTE_DIR" || {
    echo -e "${RED}✗ 创建目录失败${NC}"
    exit 1
}
echo -e "${GREEN}✓ 目录已创建${NC}"

# 步骤4: 上传文件
echo -e "\n${YELLOW}[4/5]${NC} 上传文件到服务器..."

echo "上传主脚本..."
scp sangforfw_log.sh "$SERVER:$REMOTE_DIR/" || {
    echo -e "${RED}✗ 上传失败: sangforfw_log.sh${NC}"
    exit 1
}
echo -e "${GREEN}✓ sangforfw_log.sh${NC}"

echo "上传TinyCDB源码包..."
scp tinycdb-0.78.tar.gz "$SERVER:$REMOTE_DIR/" || {
    echo -e "${RED}✗ 上传失败: tinycdb-0.78.tar.gz${NC}"
    exit 1
}
echo -e "${GREEN}✓ tinycdb-0.78.tar.gz${NC}"

echo "上传安装脚本..."
scp server_install_tinycdb.sh "$SERVER:$REMOTE_DIR/" || {
    echo -e "${RED}✗ 上传失败: server_install_tinycdb.sh${NC}"
    exit 1
}
echo -e "${GREEN}✓ server_install_tinycdb.sh${NC}"

# 设置执行权限
echo "设置执行权限..."
ssh "$SERVER" "chmod +x $REMOTE_DIR/sangforfw_log.sh $REMOTE_DIR/server_install_tinycdb.sh" || {
    echo -e "${RED}✗ 设置权限失败${NC}"
    exit 1
}
echo -e "${GREEN}✓ 权限已设置${NC}"

# 步骤5: 在服务器上编译安装TinyCDB
echo -e "\n${YELLOW}[5/5]${NC} 在服务器上编译安装TinyCDB..."
echo ""
echo -e "${BLUE}════════════════════════════════════════${NC}"
echo -e "${BLUE}  开始远程编译...${NC}"
echo -e "${BLUE}════════════════════════════════════════${NC}"
echo ""

ssh "$SERVER" "cd $REMOTE_DIR && ./server_install_tinycdb.sh" || {
    echo ""
    echo -e "${RED}✗ TinyCDB安装失败${NC}"
    echo ""
    echo "可能的原因:"
    echo "  1. 服务器缺少编译工具 (gcc, make)"
    echo "  2. 磁盘空间不足"
    echo ""
    echo "解决方法:"
    echo "  SSH登录服务器手动安装:"
    echo "    ssh $SERVER"
    echo "    cd $REMOTE_DIR"
    echo "    ./server_install_tinycdb.sh"
    exit 1
}

# 完成
echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✓ 部署完成！${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo -e "已部署的文件:"
echo -e "  ${BLUE}$REMOTE_DIR/sangforfw_log.sh${NC}          (主脚本)"
echo -e "  ${BLUE}$REMOTE_DIR/bin/cdb${NC}                   (CDB查询工具)"
echo -e "  ${BLUE}$REMOTE_DIR/bin/cdbmake${NC}               (CDB索引生成工具)"
echo ""
echo -e "${YELLOW}下一步操作:${NC}"
echo ""
echo -e "1. SSH登录服务器:"
echo -e "   ${BLUE}ssh $SERVER${NC}"
echo ""
echo -e "2. 进入目录:"
echo -e "   ${BLUE}cd $REMOTE_DIR${NC}"
echo ""
echo -e "3. 运行脚本:"
echo -e "   ${BLUE}./sangforfw_log.sh${NC}"
echo ""
echo -e "4. 生成CDB索引:"
echo -e "   选择菜单: ${GREEN}3. 更新索引${NC}"
echo ""
echo -e "5. 验证CDB状态:"
echo -e "   主菜单应显示: ${GREEN}CDB: ✓ 已启用 (快速查询)${NC}"
echo ""
echo -e "6. 测试查询:"
echo -e "   选择菜单: ${GREEN}2. 高级查询 → 1. 单IP查询${NC}"
echo -e "   应显示: ${GREEN}[CDB加速模式]${NC}"
echo ""
echo -e "${GREEN}性能提升: 查询速度提升 10 倍！${NC}"
echo ""
