# 银河麒麟系统部署指南

## 部署环境
- 操作系统：银河麒麟 SP1 V10
- 架构：x86_64 / ARM64
- 最低要求：2GB RAM, 1GB 磁盘空间

## 快速部署步骤

### 方法一：使用DEB包安装（推荐）

#### 1. 传输文件到银河麒麟系统

```bash
# 将整个build目录复制到银河麒麟系统
scp -r build/ user@kylin-server:/tmp/
```

#### 2. 构建DEB包

```bash
# SSH登录到银河麒麟系统
ssh user@kylin-server

# 进入目录
cd /tmp

# 构建DEB包
dpkg-deb --build build/sangfor-fw-log-query_2.1.0_all

# 验证包
dpkg-deb -I sangfor-fw-log-query_2.1.0_all.deb
```

#### 3. 安装DEB包

```bash
# 安装包
sudo dpkg -i sangfor-fw-log-query_2.1.0_all.deb

# 自动安装依赖
sudo apt install -f

# 安装CDB支持（重要！性能提升关键）
sudo apt install tinycdb

# 验证安装
sangfor-fw-log --version
which sangfor-fw-log
```

#### 4. 配置和使用

```bash
# 复制日志文件到指定目录
sudo mkdir -p /opt/sangfor-fw-log/data/sangfor_fw_log
sudo cp /path/to/your/*.log /opt/sangfor-fw-log/data/sangfor_fw_log/

# 设置权限
sudo chown -R $USER:$USER /opt/sangfor-fw-log/data

# 首次运行，建立索引
sangfor-fw-log

# 选择菜单 "3. 更新索引"
# 系统会自动检测tinycdb并生成CDB索引

# 测试查询性能
time sangfor-fw-log 192.168.1.1
```

---

### 方法二：手动部署（适合测试）

#### 1. 传输脚本文件

```bash
# 创建目录
mkdir -p ~/sangfor-fw-log
cd ~/sangfor-fw-log

# 从Windows传输文件
scp user@windows-pc:/d/sql/sangforfw_log.sh .
scp -r user@windows-pc:/d/sql/data .

# 或使用U盘/共享文件夹复制
```

#### 2. 安装依赖

```bash
# 安装基础依赖
sudo apt update
sudo apt install -y bash gawk grep gzip coreutils

# 安装CDB（性能优化关键）
sudo apt install -y tinycdb

# 可选：安装ripgrep（进一步提速）
sudo apt install -y ripgrep
```

#### 3. 配置脚本

```bash
# 设置执行权限
chmod +x sangforfw_log.sh

# 创建数据目录
mkdir -p data/sangfor_fw_log
mkdir -p data/index
mkdir -p data/export/{by_ip,by_port,by_date,by_query}

# 复制日志文件
cp /path/to/your/*.log data/sangfor_fw_log/
```

#### 4. 运行测试

```bash
# 交互式模式
./sangforfw_log.sh

# 命令行模式
./sangforfw_log.sh 192.168.1.1

# 性能测试
bash performance_test.sh
```

---

## CDB索引优化验证

### 检查CDB是否启用

```bash
# 查看索引文件
ls -lh data/index/

# 应该看到：
# sangfor_fw_log_index.db      # 文本索引（5.1MB）
# sangfor_fw_log_index.cdb     # CDB索引（约3-4MB）
```

### 性能对比测试

```bash
# 测试1：使用CDB索引
time ./sangforfw_log.sh 192.168.1.1
# 预期：200-300ms

# 测试2：禁用CDB（重命名CDB文件）
mv data/index/sangfor_fw_log_index.cdb data/index/sangfor_fw_log_index.cdb.bak
time ./sangforfw_log.sh 192.168.1.1
# 预期：1800-2100ms

# 恢复CDB
mv data/index/sangfor_fw_log_index.cdb.bak data/index/sangfor_fw_log_index.cdb
```

---

## 性能基准测试

### 当前性能（Windows环境，无CDB）
```
测试环境：Windows 10 + Git Bash
日志大小：109MB (531,664行)
索引记录：66,871条

查询性能：
- IP查询（1942条结果）：1950ms
- IP查询（1条结果）：2127ms
- IP查询（1174条结果）：2138ms
- 平均耗时：2071ms
- 评级：⭐⭐ 需要优化
```

### 预期性能（银河麒麟 + CDB）
```
测试环境：银河麒麟 SP1 V10 + tinycdb
日志大小：109MB (531,664行)
索引记录：66,871条

查询性能：
- IP查询（任意结果数）：200-300ms
- 平均耗时：250ms
- 性能提升：7-8倍
- 评级：⭐⭐⭐⭐⭐ 优秀
```

---

## 故障排查

### 问题1：dpkg-deb命令不存在
```bash
# 检查系统
cat /etc/os-release

# 安装dpkg工具
sudo apt install dpkg-dev
```

### 问题2：依赖包安装失败
```bash
# 检查软件源
sudo apt update

# 手动安装依赖
sudo apt install bash gawk grep gzip coreutils

# 如果tinycdb不可用，脚本会自动回退到文本索引
```

### 问题3：权限问题
```bash
# 设置正确的权限
sudo chown -R $USER:$USER /opt/sangfor-fw-log
sudo chmod +x /opt/sangfor-fw-log/sangforfw_log.sh
sudo chmod +x /usr/bin/sangfor-fw-log
```

### 问题4：CDB索引未生成
```bash
# 检查tinycdb是否安装
which cdb
which cdbmake

# 如果未安装
sudo apt install tinycdb

# 重新生成索引
sangfor-fw-log
# 选择 "3. 更新索引"
```

### 问题5：查询速度仍然慢
```bash
# 检查是否使用了CDB索引
ls -lh /opt/sangfor-fw-log/data/index/*.cdb

# 检查日志文件位置（建议使用SSD）
df -h /opt/sangfor-fw-log/data/sangfor_fw_log

# 检查系统负载
top
iostat -x 1
```

---

## 卸载

### 使用DEB包安装的卸载方法

```bash
# 保留数据和配置
sudo apt remove sangfor-fw-log-query

# 完全删除（包括数据）
sudo apt purge sangfor-fw-log-query

# 清理依赖
sudo apt autoremove
```

### 手动安装的卸载方法

```bash
# 删除程序目录
rm -rf ~/sangfor-fw-log

# 删除数据（可选）
rm -rf /opt/sangfor-fw-log
```

---

## 生产环境建议

### 1. 日志文件管理
```bash
# 定期清理旧日志（保留最近30天）
find /opt/sangfor-fw-log/data/sangfor_fw_log -name "*.log" -mtime +30 -delete

# 定期压缩旧日志
find /opt/sangfor-fw-log/data/sangfor_fw_log -name "*.log" -mtime +7 -exec gzip {} \;
```

### 2. 索引自动更新
```bash
# 添加cron任务，每天凌晨2点更新索引
crontab -e

# 添加以下行
0 2 * * * /usr/bin/sangfor-fw-log --rebuild-index >/dev/null 2>&1
```

### 3. 性能监控
```bash
# 创建性能监控脚本
cat > /opt/sangfor-fw-log/monitor.sh << 'EOF'
#!/bin/bash
LOG_FILE="/var/log/sangfor-fw-log-performance.log"
TEST_IP="192.168.1.1"

start=$(date +%s%N)
/usr/bin/sangfor-fw-log "$TEST_IP" >/dev/null 2>&1
end=$(date +%s%N)
elapsed=$(( (end - start) / 1000000 ))

echo "$(date '+%Y-%m-%d %H:%M:%S') - Query time: ${elapsed}ms" >> "$LOG_FILE"

# 如果查询时间超过500ms，发送告警
if [ $elapsed -gt 500 ]; then
    echo "WARNING: Query time exceeded 500ms: ${elapsed}ms" | logger -t sangfor-fw-log
fi
EOF

chmod +x /opt/sangfor-fw-log/monitor.sh

# 每小时执行一次监控
echo "0 * * * * /opt/sangfor-fw-log/monitor.sh" | crontab -
```

### 4. 备份策略
```bash
# 备份索引文件
cp /opt/sangfor-fw-log/data/index/*.cdb /backup/

# 备份配置文件
cp /etc/sangfor-fw-log/config.conf /backup/
```

---

## 技术支持

- 项目文档：README.md
- CDB优化方案：DEB_PACKAGE_README.md
- 性能测试：performance_test.sh
- 功能测试：test_all_functions.sh

---

## 更新日志

### v2.1.0 (2026-04-27)
- ✅ 添加CDB索引支持，查询速度提升7-8倍
- ✅ 创建DEB安装包，支持银河麒麟系统
- ✅ 优化并行查询，支持8进程并行
- ✅ 修复导出文件路径问题
- ✅ 简化主菜单界面
- ✅ 添加索引压缩功能

### v2.0.0 (2026-04-26)
- 初始版本
- 基础查询功能
- 文本索引支持
