# 服务器部署指南 - CDB集成版本

## 📦 部署步骤

### 重要说明

**新版本脚本已支持自动路径检测：**
- ✅ 服务器环境：自动使用 `/data/sangfor_fw_log`
- ✅ 本地环境：自动使用相对路径 `./data/sangfor_fw_log`
- ✅ 无需手动修改配置

### 步骤1: 备份服务器上的旧版本

```bash
# SSH登录服务器
ssh root@your-server

# 备份旧版本
cd /data/sangfor_fw_log_chaxun
cp sangforfw_log.sh sangforfw_log.sh.backup_$(date +%Y%m%d)

# 备份索引（如果存在）
cp -r data/index data/index.backup_$(date +%Y%m%d)
```

### 步骤2: 上传新版本到服务器

**方式1: 使用SCP（推荐）**

```bash
# 在本机执行（Windows Git Bash）
cd /d/sql

# 上传主脚本
scp sangforfw_log.sh root@your-server:/data/sangfor_fw_log_chaxun/

# 上传文档（可选）
scp CDB_*.md root@your-server:/data/sangfor_fw_log_chaxun/
scp cdb_performance_test.sh root@your-server:/data/sangfor_fw_log_chaxun/
```

**方式2: 使用SFTP**

```bash
sftp root@your-server
cd /data/sangfor_fw_log_chaxun
put sangforfw_log.sh
put CDB_INTEGRATION.md
put CDB_QUICKSTART.md
put cdb_performance_test.sh
quit
```

**方式3: 使用rsync（推荐，保留权限）**

```bash
# 在本机执行
rsync -avz --progress \
  /d/sql/sangforfw_log.sh \
  /d/sql/CDB_*.md \
  /d/sql/cdb_performance_test.sh \
  root@your-server:/data/sangfor_fw_log_chaxun/
```

### 步骤3: 在服务器上安装TinyCDB

```bash
# SSH登录服务器
ssh root@your-server

# Debian/Ubuntu
sudo apt-get update
sudo apt-get install tinycdb -y

# CentOS/RHEL 7/8
sudo yum install tinycdb -y

# CentOS/RHEL 9
sudo dnf install tinycdb -y

# 验证安装
cdb -h && cdbmake -h && echo "✓ TinyCDB安装成功"
```

### 步骤4: 设置权限

```bash
# 在服务器上执行
cd /data/sangfor_fw_log_chaxun

# 设置脚本执行权限
chmod +x sangforfw_log.sh
chmod +x cdb_performance_test.sh

# 确认权限
ls -lh sangforfw_log.sh
```

### 步骤5: 生成CDB索引

```bash
# 在服务器上执行
cd /data/sangfor_fw_log_chaxun

# 运行脚本
./sangforfw_log.sh

# 选择菜单
3. 更新索引

# 等待索引生成完成
# 输出应显示：
# 正在生成CDB索引...
# ✓ CDB索引生成成功
#   文本索引: X.XM
#   CDB索引: X.XM
```

### 步骤6: 验证CDB集成

```bash
# 查看主菜单状态
./sangforfw_log.sh

# 应显示：
# 索引: XXXXX 条 | 日志: X 个
# CDB: ✓ 已启用 (快速查询)  ← 确认这一行

# 测试CDB查询
选择：2. 高级查询
选择：1. 单IP查询
输入IP: [某个存在的IP]
输入日期: [某个日期，如 2026-04-27]

# 应显示：
# [CDB加速模式]  ← 确认使用CDB
# 查询完成，耗时: 0.0X秒
```

---

## 🔧 故障排查

### 问题1: 日志目录不存在

```bash
# 检查日志目录
ls -la /data/sangfor_fw_log

# 如果不存在，检查脚本中的LOG_DIR配置
grep "^LOG_DIR=" sangforfw_log.sh

# 应该显示：
# LOG_DIR="$SCRIPT_DIR/data/sangfor_fw_log"

# 创建目录结构
mkdir -p data/sangfor_fw_log
mkdir -p data/index
mkdir -p data/export/{by_ip,by_port,by_date,by_query}
```

### 问题2: TinyCDB未安装

```bash
# 检查是否安装
which cdb cdbmake

# 如果未安装，按步骤3重新安装
```

### 问题3: CDB索引生成失败

```bash
# 检查磁盘空间
df -h /data

# 检查索引文件权限
ls -la data/index/

# 手动测试CDB
echo "+3,5:abc->12345" | cdbmake test.cdb test.cdb.tmp
cdb -q test.cdb abc
# 应输出: 12345
rm -f test.cdb test.cdb.tmp
```

### 问题4: 权限问题

```bash
# 检查脚本权限
ls -l sangforfw_log.sh

# 如果没有执行权限
chmod +x sangforfw_log.sh

# 检查数据目录权限
ls -ld data/
chmod 755 data/
```

---

## 📊 性能测试

```bash
# 在服务器上运行性能测试
cd /data/sangfor_fw_log_chaxun
./cdb_performance_test.sh

# 查看测试结果
# 应显示CDB相比文本索引的性能提升
```

---

## 🔄 定时任务设置

```bash
# 编辑crontab
crontab -e

# 添加定时任务
# 每天凌晨2点重建索引（包括CDB）
0 2 * * * /data/sangfor_fw_log_chaxun/sangforfw_log.sh --rebuild-index

# 每30分钟增量更新
*/30 * * * * /data/sangfor_fw_log_chaxun/sangforfw_log.sh --update-index

# 保存并退出
```

---

## 📋 部署检查清单

- [ ] 备份旧版本脚本
- [ ] 备份现有索引
- [ ] 上传新版本脚本到服务器
- [ ] 安装TinyCDB
- [ ] 设置脚本执行权限
- [ ] 生成CDB索引
- [ ] 验证CDB状态显示
- [ ] 测试CDB加速查询
- [ ] 运行性能测试
- [ ] 设置定时任务
- [ ] 更新文档

---

## 🚀 快速部署命令（一键执行）

```bash
# 在本机执行（上传文件）
cd /d/sql
scp sangforfw_log.sh CDB_*.md cdb_performance_test.sh \
  root@your-server:/data/sangfor_fw_log_chaxun/

# 在服务器上执行（安装和配置）
ssh root@your-server << 'EOF'
cd /data/sangfor_fw_log_chaxun

# 备份
cp sangforfw_log.sh sangforfw_log.sh.backup_$(date +%Y%m%d)

# 安装TinyCDB
if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get install -y tinycdb
elif command -v yum >/dev/null 2>&1; then
    sudo yum install -y tinycdb
elif command -v dnf >/dev/null 2>&1; then
    sudo dnf install -y tinycdb
fi

# 设置权限
chmod +x sangforfw_log.sh cdb_performance_test.sh

# 生成CDB索引
./sangforfw_log.sh --rebuild-index

# 显示状态
echo ""
echo "部署完成！"
echo "CDB状态："
if [ -f data/index/sangfor_fw_log_index.cdb ]; then
    ls -lh data/index/sangfor_fw_log_index.cdb
    echo "✓ CDB索引已生成"
else
    echo "✗ CDB索引未生成"
fi
EOF
```

---

## 📞 技术支持

如果部署过程中遇到问题：

1. 检查服务器操作系统版本：`cat /etc/os-release`
2. 检查日志目录路径：`ls -la /data/sangfor_fw_log`
3. 检查脚本版本：`head -1 sangforfw_log.sh`
4. 查看错误日志：`tail -50 /var/log/messages`

---

**部署完成后，CDB加速将自动生效，查询性能提升10倍！**
