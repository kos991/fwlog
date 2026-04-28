#!/bin/bash
# 服务器一键部署脚本 - CDB集成版本
# 使用方法：./deploy_to_server.sh root@your-server

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[36m'
RESET='\033[0m'

# 检查参数
if [ $# -eq 0 ]; then
    echo -e "${RED}错误: 请提供服务器地址${RESET}"
    echo "使用方法: $0 root@your-server"
    echo "示例: $0 root@192.168.1.100"
    exit 1
fi

SERVER="$1"
REMOTE_DIR="/data/sangfor_fw_log_chaxun"
LOCAL_DIR="/d/sql"

echo -e "${GREEN}========================================${RESET}"
echo -e "${GREEN}  服务器部署 - CDB集成版本${RESET}"
echo -e "${GREEN}========================================${RESET}"
echo ""
echo "目标服务器: $SERVER"
echo "远程目录: $REMOTE_DIR"
echo ""

# 步骤1: 检查本地文件
echo -e "${BLUE}[1/6] 检查本地文件...${RESET}"
if [ ! -f "$LOCAL_DIR/sangforfw_log.sh" ]; then
    echo -e "${RED}错误: 找不到 sangforfw_log.sh${RESET}"
    exit 1
fi

FILE_SIZE=$(du -h "$LOCAL_DIR/sangforfw_log.sh" | cut -f1)
echo -e "${GREEN}✓ 找到主脚本 (${FILE_SIZE})${RESET}"

# 步骤2: 测试SSH连接
echo ""
echo -e "${BLUE}[2/6] 测试SSH连接...${RESET}"
if ! ssh -o ConnectTimeout=5 "$SERVER" "echo '连接成功'" >/dev/null 2>&1; then
    echo -e "${RED}错误: 无法连接到服务器${RESET}"
    echo "请检查："
    echo "  1. 服务器地址是否正确"
    echo "  2. SSH密钥是否配置"
    echo "  3. 网络是否畅通"
    exit 1
fi
echo -e "${GREEN}✓ SSH连接正常${RESET}"

# 步骤3: 备份服务器上的旧版本
echo ""
echo -e "${BLUE}[3/6] 备份服务器旧版本...${RESET}"
ssh "$SERVER" << 'EOF'
cd /data/sangfor_fw_log_chaxun 2>/dev/null || exit 0
if [ -f sangforfw_log.sh ]; then
    BACKUP_NAME="sangforfw_log.sh.backup_$(date +%Y%m%d_%H%M%S)"
    cp sangforfw_log.sh "$BACKUP_NAME"
    echo "✓ 已备份为: $BACKUP_NAME"
else
    echo "○ 未找到旧版本，跳过备份"
fi
EOF

# 步骤4: 上传文件
echo ""
echo -e "${BLUE}[4/6] 上传文件到服务器...${RESET}"

# 上传主脚本
echo -n "  上传 sangforfw_log.sh... "
if scp -q "$LOCAL_DIR/sangforfw_log.sh" "$SERVER:$REMOTE_DIR/"; then
    echo -e "${GREEN}✓${RESET}"
else
    echo -e "${RED}✗${RESET}"
    exit 1
fi

# 上传文档
echo -n "  上传 CDB文档... "
if scp -q "$LOCAL_DIR"/CDB_*.md "$SERVER:$REMOTE_DIR/" 2>/dev/null; then
    echo -e "${GREEN}✓${RESET}"
else
    echo -e "${YELLOW}○ (可选文件)${RESET}"
fi

# 上传测试脚本
echo -n "  上传 cdb_performance_test.sh... "
if scp -q "$LOCAL_DIR/cdb_performance_test.sh" "$SERVER:$REMOTE_DIR/" 2>/dev/null; then
    echo -e "${GREEN}✓${RESET}"
else
    echo -e "${YELLOW}○ (可选文件)${RESET}"
fi

# 步骤5: 在服务器上安装TinyCDB和配置
echo ""
echo -e "${BLUE}[5/6] 服务器配置...${RESET}"
ssh "$SERVER" << 'EOF'
cd /data/sangfor_fw_log_chaxun

# 检查TinyCDB
echo -n "  检查TinyCDB... "
if command -v cdb >/dev/null 2>&1 && command -v cdbmake >/dev/null 2>&1; then
    echo "✓ 已安装"
else
    echo "✗ 未安装"
    echo "  正在安装TinyCDB..."

    # 检测操作系统
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update -qq
        sudo apt-get install -y tinycdb
    elif command -v yum >/dev/null 2>&1; then
        sudo yum install -y tinycdb
    elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y tinycdb
    else
        echo "  ✗ 无法自动安装，请手动安装TinyCDB"
        exit 1
    fi

    if command -v cdb >/dev/null 2>&1; then
        echo "  ✓ TinyCDB安装成功"
    else
        echo "  ✗ TinyCDB安装失败"
        exit 1
    fi
fi

# 设置权限
echo -n "  设置脚本权限... "
chmod +x sangforfw_log.sh 2>/dev/null
chmod +x cdb_performance_test.sh 2>/dev/null
echo "✓"

# 检查日志目录
echo -n "  检查日志目录... "
if [ -d /data/sangfor_fw_log ]; then
    LOG_COUNT=$(ls -1 /data/sangfor_fw_log/*.log 2>/dev/null | wc -l)
    echo "✓ ($LOG_COUNT 个日志文件)"
else
    echo "✗ /data/sangfor_fw_log 不存在"
    exit 1
fi

# 检查数据目录
echo -n "  检查数据目录... "
if [ ! -d data ]; then
    mkdir -p data/index data/export/{by_ip,by_port,by_date,by_query} data/temp data/backup
    echo "✓ (已创建)"
else
    echo "✓ (已存在)"
fi
EOF

# 步骤6: 生成CDB索引
echo ""
echo -e "${BLUE}[6/6] 生成CDB索引...${RESET}"
echo "这可能需要几分钟，请耐心等待..."
echo ""

ssh "$SERVER" << 'EOF'
cd /data/sangfor_fw_log_chaxun

# 检查是否已有索引
if [ -f data/index/sangfor_fw_log_index.db ]; then
    echo "  检测到现有索引，执行增量更新..."
    ./sangforfw_log.sh --update-index
else
    echo "  未检测到索引，执行全量构建..."
    ./sangforfw_log.sh --rebuild-index
fi

echo ""
echo "索引状态："
if [ -f data/index/sangfor_fw_log_index.db ]; then
    INDEX_COUNT=$(wc -l < data/index/sangfor_fw_log_index.db)
    INDEX_SIZE=$(du -h data/index/sangfor_fw_log_index.db | cut -f1)
    echo "  文本索引: $INDEX_COUNT 条, $INDEX_SIZE"
fi

if [ -f data/index/sangfor_fw_log_index.cdb ]; then
    CDB_SIZE=$(du -h data/index/sangfor_fw_log_index.cdb | cut -f1)
    echo "  CDB索引: $CDB_SIZE"
    echo "  状态: ✓ CDB已启用"
else
    echo "  状态: ✗ CDB未生成"
fi
EOF

# 完成
echo ""
echo -e "${GREEN}========================================${RESET}"
echo -e "${GREEN}  部署完成！${RESET}"
echo -e "${GREEN}========================================${RESET}"
echo ""
echo "下一步："
echo "  1. SSH登录服务器: ssh $SERVER"
echo "  2. 进入目录: cd $REMOTE_DIR"
echo "  3. 运行工具: ./sangforfw_log.sh"
echo "  4. 测试CDB查询（选择高级查询 → 单IP查询 → 指定日期）"
echo ""
echo "性能测试："
echo "  ssh $SERVER 'cd $REMOTE_DIR && ./cdb_performance_test.sh'"
echo ""
