# 部署说明 - v2.1

## 📦 部署到Linux服务器

### 方法1：直接上传（推荐）

```bash
# 1. 上传脚本到服务器
scp sangforfw_log.sh root@your-server:/data/sangfor_fw_log_chaxun/

# 2. 登录服务器
ssh root@your-server

# 3. 进入目录
cd /data/sangfor_fw_log_chaxun

# 4. 运行脚本（自动修复权限）
bash sangforfw_log.sh
```

### 方法2：使用Git

```bash
# 1. 登录服务器
ssh root@your-server

# 2. 克隆或更新代码
cd /data/sangfor_fw_log_chaxun
git pull

# 3. 运行脚本
bash sangforfw_log.sh
```

---

## 🔧 首次配置

### 1. 修改日志目录路径

编辑脚本，修改LOG_DIR变量：

```bash
vi sangforfw_log.sh

# 找到这一行并修改为实际路径
LOG_DIR="/data/sangfor_fw_log"
```

### 2. 建立初始索引

```bash
# 运行脚本
bash sangforfw_log.sh

# 选择菜单选项
3. 重建索引（全量）

# 等待索引建立完成
```

### 3. 安装快捷键

```bash
# 在菜单中选择
5. 快捷键管理
1. 安装快捷键

# 重新加载配置
source ~/.bashrc

# 测试快捷键
fwlog
```

---

## 🚀 快速开始

### 基本使用

```bash
# 打开主菜单
fwlog

# 或直接运行脚本
bash /data/sangfor_fw_log_chaxun/sangforfw_log.sh
```

### 常用操作

```bash
# 1. 查询IP
fwlog
# 选择：1. 快速查询 → 1. IP地址查询

# 2. 查询端口
fwlog
# 选择：1. 快速查询 → 2. 端口查询

# 3. 更新索引（每天一次）
fwlog
# 选择：4. 增量更新索引

# 4. 清理旧文件（每周一次）
fwlog
# 选择：6. 清理导出文件
```

---

## 📁 目录结构

部署后的目录结构：

```
/data/sangfor_fw_log_chaxun/
├── sangforfw_log.sh              # 主脚本
├── fwlog                         # 快捷键脚本（自动生成）
├── data/                         # 数据目录（自动创建）
│   ├── index/                    # 索引文件
│   │   ├── sangfor_fw_log_index.db
│   │   ├── sangfor_fw_log_index.db.gz
│   │   └── sangfor_fw_log_index.meta
│   ├── export/                   # 导出结果
│   │   ├── by_ip/                # IP查询结果
│   │   ├── by_port/              # 端口查询结果
│   │   ├── by_date/              # 日期查询结果
│   │   └── by_query/             # 组合查询结果
│   └── temp/                     # 临时文件
├── README.md                     # 功能说明
├── MENU_GUIDE.md                 # 菜单指南
├── DIRECTORY_STRUCTURE.md        # 目录说明
└── UPDATE_v2.1.md                # 更新说明
```

---

## ⚙️ 配置检查

### 检查日志目录

```bash
# 确认日志目录存在
ls -la /data/sangfor_fw_log

# 确认有日志文件
ls -la /data/sangfor_fw_log/*.log | head -5
```

### 检查脚本权限

```bash
# 脚本会自动修复权限，但也可以手动检查
ls -la /data/sangfor_fw_log_chaxun/sangforfw_log.sh
ls -la /data/sangfor_fw_log_chaxun/fwlog

# 如果没有执行权限，运行脚本会自动修复
bash /data/sangfor_fw_log_chaxun/sangforfw_log.sh
```

### 检查磁盘空间

```bash
# 检查可用空间（建议至少1GB）
df -h /data

# 检查索引大小
du -sh /data/sangfor_fw_log_chaxun/data/
```

---

## 🔄 日常维护

### 每天

```bash
# 更新索引（增量）
fwlog
# 选择：4. 增量更新索引
```

### 每周

```bash
# 清理导出文件
fwlog
# 选择：6. 清理导出文件 → 1. 清理所有导出文件
```

### 每月

```bash
# 重建索引（全量）
fwlog
# 选择：3. 重建索引（全量）
```

---

## 🐛 故障排除

### 问题1：Permission denied

**现象**：
```
/data/sangfor_fw_log_chaxun/fwlog: line 9: Permission denied
```

**解决**：
```bash
# 方法1：脚本会自动修复，直接运行
bash /data/sangfor_fw_log_chaxun/sangforfw_log.sh

# 方法2：手动修复
chmod +x /data/sangfor_fw_log_chaxun/sangforfw_log.sh
chmod +x /data/sangfor_fw_log_chaxun/fwlog
```

### 问题2：fwlog命令不存在

**现象**：
```
bash: fwlog: command not found
```

**解决**：
```bash
# 重新加载配置
source ~/.bashrc

# 或重新安装快捷键
bash /data/sangfor_fw_log_chaxun/sangforfw_log.sh
# 选择：5. 快捷键管理 → 1. 安装快捷键
```

### 问题3：日志目录不存在

**现象**：
```
日志目录 /data/sangfor_fw_log 不存在
```

**解决**：
```bash
# 检查实际日志目录路径
find /data -name "*.log" -type f | head -5

# 修改脚本中的LOG_DIR变量
vi /data/sangfor_fw_log_chaxun/sangforfw_log.sh
```

### 问题4：索引损坏

**现象**：
- 查询结果不完整
- 索引文件异常

**解决**：
```bash
fwlog
# 选择：3. 重建索引（全量）
```

### 问题5：磁盘空间不足

**现象**：
```
No space left on device
```

**解决**：
```bash
# 清理导出文件
fwlog
# 选择：6. 清理导出文件 → 1. 清理所有导出文件

# 清理临时文件
rm -rf /data/sangfor_fw_log_chaxun/data/temp/*

# 检查空间
df -h /data
```

---

## 📊 性能优化

### 索引优化

```bash
# 1. 使用增量更新而非全量重建
fwlog → 4. 增量更新索引

# 2. 定期压缩索引（自动）
# 索引会自动压缩，无需手动操作

# 3. 清理旧索引备份
rm -f /data/sangfor_fw_log_chaxun/data/index/*.bak
```

### 查询优化

```bash
# 1. 优先使用快速查询（基于索引）
fwlog → 1. 快速查询

# 2. 使用具体条件而非模糊查询
# 好：查询具体IP 192.168.1.100
# 差：查询IP段 192.168.*

# 3. 限制日期范围
# 好：查询最近7天
# 差：查询全部历史
```

### 存储优化

```bash
# 1. 定期清理导出文件
fwlog → 6. 清理导出文件

# 2. 归档旧日志
mv /data/sangfor_fw_log/2025*.log /data/archive/

# 3. 压缩归档文件
tar -czf logs_2025.tar.gz /data/archive/2025*.log
```

---

## 🔐 安全建议

### 权限设置

```bash
# 脚本目录权限
chmod 755 /data/sangfor_fw_log_chaxun
chmod 755 /data/sangfor_fw_log_chaxun/sangforfw_log.sh

# 数据目录权限
chmod 700 /data/sangfor_fw_log_chaxun/data
chmod 600 /data/sangfor_fw_log_chaxun/data/index/*
```

### 访问控制

```bash
# 限制只有root用户可以访问
chown -R root:root /data/sangfor_fw_log_chaxun

# 或创建专用用户
useradd -r -s /bin/bash fwlog
chown -R fwlog:fwlog /data/sangfor_fw_log_chaxun
```

### 日志审计

```bash
# 记录查询操作
# 在脚本中添加审计日志功能（可选）
echo "$(date) - $USER - Query: $QUERY" >> /var/log/fwlog_audit.log
```

---

## 📞 技术支持

### 检查版本

```bash
# 查看脚本版本
grep "INDEX_VERSION=" /data/sangfor_fw_log_chaxun/sangforfw_log.sh
```

### 查看日志

```bash
# 查看索引元数据
cat /data/sangfor_fw_log_chaxun/data/index/sangfor_fw_log_index.meta

# 查看索引统计
wc -l /data/sangfor_fw_log_chaxun/data/index/sangfor_fw_log_index.db
```

### 运行测试

```bash
# 运行测试脚本
bash /data/sangfor_fw_log_chaxun/test_v2.1.sh
```

---

## ✅ 部署检查清单

- [ ] 脚本已上传到服务器
- [ ] LOG_DIR路径已配置正确
- [ ] 脚本有执行权限（自动修复）
- [ ] 初始索引已建立
- [ ] 快捷键已安装
- [ ] 快捷键测试通过
- [ ] 查询功能测试通过
- [ ] 增量更新测试通过
- [ ] 清理功能测试通过
- [ ] 磁盘空间充足（>1GB）

---

**版本**: 2.1  
**部署日期**: 2026-04-27  
**维护团队**: QJKJ Team
