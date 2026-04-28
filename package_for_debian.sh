#!/bin/bash
# 打包并传输到Debian服务器

echo "╔═══════════════════════════════════════════════════════════════════╗"
echo "║   📦 打包并传输到Debian服务器                                     ║"
echo "╚═══════════════════════════════════════════════════════════════════╝"
echo ""

# 检查必需文件
REQUIRED_FILES=(
    "main.go"
    "deploy_debian.sh"
    "Makefile"
    "nat-query-service.service"
    "go.mod"
)

echo "🔍 检查必需文件..."
MISSING=0
for file in "${REQUIRED_FILES[@]}"; do
    if [ ! -f "$file" ]; then
        echo "  ❌ 缺少文件: $file"
        MISSING=1
    else
        echo "  ✅ $file"
    fi
done

if [ $MISSING -eq 1 ]; then
    echo ""
    echo "❌ 缺少必需文件，请检查"
    exit 1
fi

echo ""
echo "📦 创建部署包..."

# 创建临时目录
PACKAGE_NAME="nat_query_service_debian_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$PACKAGE_NAME"

# 复制核心文件
cp main.go "$PACKAGE_NAME/"
cp deploy_debian.sh "$PACKAGE_NAME/"
cp Makefile "$PACKAGE_NAME/"
cp nat-query-service.service "$PACKAGE_NAME/"
cp go.mod "$PACKAGE_NAME/"

# 复制文档（可选）
for doc in README_OPTIMIZED.md QUICKSTART.md COMPARISON.md QUICK_REFERENCE.txt; do
    if [ -f "$doc" ]; then
        cp "$doc" "$PACKAGE_NAME/"
    fi
done

# 复制脚本（可选）
for script in check_deployment.sh test_performance.sh; do
    if [ -f "$script" ]; then
        cp "$script" "$PACKAGE_NAME/"
        chmod +x "$PACKAGE_NAME/$script"
    fi
done

# 设置权限
chmod +x "$PACKAGE_NAME/deploy_debian.sh"

# 打包
tar -czf "${PACKAGE_NAME}.tar.gz" "$PACKAGE_NAME"
rm -rf "$PACKAGE_NAME"

PACKAGE_SIZE=$(du -h "${PACKAGE_NAME}.tar.gz" | cut -f1)
echo "✅ 部署包创建完成: ${PACKAGE_NAME}.tar.gz ($PACKAGE_SIZE)"
echo ""

# 提示传输方式
echo "╔═══════════════════════════════════════════════════════════════════╗"
echo "║   📤 传输到Debian服务器                                           ║"
echo "╚═══════════════════════════════════════════════════════════════════╝"
echo ""
echo "请选择传输方式:"
echo ""
echo "【方式1】使用scp传输（推荐）"
echo "  scp ${PACKAGE_NAME}.tar.gz user@debian-server:/tmp/"
echo ""
echo "【方式2】使用rsync传输"
echo "  rsync -avz ${PACKAGE_NAME}.tar.gz user@debian-server:/tmp/"
echo ""
echo "【方式3】使用WinSCP/FileZilla等工具"
echo "  手动上传 ${PACKAGE_NAME}.tar.gz 到服务器"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 在Debian服务器上执行:"
echo ""
echo "  # 1. 解压部署包"
echo "  cd /tmp"
echo "  tar -xzf ${PACKAGE_NAME}.tar.gz"
echo "  cd ${PACKAGE_NAME}"
echo ""
echo "  # 2. 运行部署脚本"
echo "  chmod +x deploy_debian.sh"
echo "  sudo ./deploy_debian.sh"
echo ""
echo "  # 3. 复制日志文件（如果有）"
echo "  sudo cp /path/to/*.log /data/sangfor_fw_log/"
echo ""
echo "  # 4. 启动服务"
echo "  sudo systemctl start nat-query-service"
echo ""
echo "  # 5. 访问Web界面"
echo "  http://服务器IP:8080"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 询问是否立即传输
read -p "是否立即使用scp传输? 请输入服务器地址 (例: user@192.168.1.100) 或按回车跳过: " SERVER_ADDR

if [ -n "$SERVER_ADDR" ]; then
    echo ""
    echo "📤 开始传输..."
    scp "${PACKAGE_NAME}.tar.gz" "$SERVER_ADDR:/tmp/"

    if [ $? -eq 0 ]; then
        echo ""
        echo "✅ 传输完成！"
        echo ""
        echo "现在可以SSH到服务器执行部署:"
        echo "  ssh $SERVER_ADDR"
        echo "  cd /tmp && tar -xzf ${PACKAGE_NAME}.tar.gz"
        echo "  cd ${PACKAGE_NAME} && sudo ./deploy_debian.sh"
    else
        echo ""
        echo "❌ 传输失败，请手动传输"
    fi
fi

echo ""
echo "✅ 完成！"
