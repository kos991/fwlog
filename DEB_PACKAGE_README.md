# DEB安装包构建指南

## 📦 包信息

- **包名**: sangfor-fw-log-query
- **版本**: 2.1.0
- **架构**: all (纯Shell脚本，支持所有架构)
- **目标系统**: 银河麒麟SP1 V10 / Ubuntu / Debian

## 🚀 快速开始

### 1. 构建DEB包

```bash
# 在Windows/Linux环境下构建
chmod +x build_deb.sh
./build_deb.sh
```

构建完成后会生成：`sangfor-fw-log-query_2.1.0_all.deb`

### 2. 安装到银河麒麟系统

```bash
# 上传DEB包到银河麒麟系统
scp sangfor-fw-log-query_2.1.0_all.deb user@kylin-server:/tmp/

# SSH登录到银河麒麟系统
ssh user@kylin-server

# 安装DEB包
sudo dpkg -i /tmp/sangfor-fw-log-query_2.1.0_all.deb

# 安装依赖（包括tinycdb）
sudo apt install -f
```

### 3. 使用工具

```bash
# 交互模式
sangfor-fw-log

# 命令行模式
sangfor-fw-log 192.168.1.100
sangfor-fw-log 20260427 192.168.1.100
sangfor-fw-log 20260427 192.168.1.100:443
```

## 📁 安装后的目录结构

```
/opt/sangfor-fw-log/
├── sangforfw_log.sh              # 主程序
├── config/
│   └── default.conf              # 默认配置
└── data/
    ├── sangfor_fw_log/           # 日志目录（放置防火墙日志）
    ├── index/                    # 索引目录（自动生成）
    └── export/                   # 导出目录（查询结果）
        ├── by_ip/
        ├── by_port/
        ├── by_date/
        └── by_query/

/etc/sangfor-fw-log/
└── config.conf -> /opt/sangfor-fw-log/config/default.conf

/usr/bin/
└── sangfor-fw-log -> /opt/sangfor-fw-log/sangforfw_log.sh
```

## ⚙️ 配置文件

编辑 `/etc/sangfor-fw-log/config.conf`：

```bash
# 日志目录
LOG_DIR="/opt/sangfor-fw-log/data/sangfor_fw_log"

# 索引目录
INDEX_DIR="/opt/sangfor-fw-log/data/index"

# 导出目录
EXPORT_DIR="/opt/sangfor-fw-log/data/export"

# 索引类型 (auto/cdb/text)
# auto: 自动检测，优先使用CDB
INDEX_TYPE="auto"

# 并行查询进程数
PARALLEL_JOBS=8

# 自动压缩索引
AUTO_COMPRESS_INDEX=1

# 查询结果最大行数
MAX_RESULT_LINES=10000
```

## 🔧 依赖说明

### 必需依赖（自动安装）
- bash >= 4.0
- gawk
- grep
- gzip
- coreutils

### 推荐依赖（显著提升性能）
- **tinycdb**: 提供CDB索引支持，查询速度提升5-8倍

```bash
# 安装tinycdb
sudo apt install tinycdb

# 验证安装
cdbmake --version
```

### 可选依赖
- **ripgrep**: 比grep快3-5倍（未来版本支持）

## 📊 性能对比

| 索引类型 | 查询时间 | 索引大小 | 说明 |
|---------|---------|---------|------|
| 文本索引 | 1.77秒 | 5.1MB (压缩后306KB) | 默认模式 |
| CDB索引 | 0.2-0.3秒 | ~8MB | 需要安装tinycdb |

**测试环境**: 531,664行日志，66,871条索引记录

## 📝 使用示例

### 放置日志文件

```bash
# 将防火墙日志复制到日志目录
sudo cp /path/to/firewall/*.log /opt/sangfor-fw-log/data/sangfor_fw_log/

# 或创建符号链接
sudo ln -s /data/sangfor_fw_log/*.log /opt/sangfor-fw-log/data/sangfor_fw_log/
```

### 建立索引

```bash
# 方式1: 交互模式
sangfor-fw-log
# 选择菜单: 2) 更新索引

# 方式2: 命令行模式（未来版本）
# sangfor-fw-log --update-index
```

### 查询日志

```bash
# 查询单个IP的所有日志
sangfor-fw-log 192.168.1.100

# 查询指定日期的IP日志
sangfor-fw-log 20260427 192.168.1.100

# 查询指定IP和端口
sangfor-fw-log 20260427 192.168.1.100:443

# 查询日期范围
sangfor-fw-log 20260401-20260430 192.168.1.100
```

### 导出结果

查询结果自动导出到：
- `/opt/sangfor-fw-log/data/export/by_ip/` - 按IP查询的结果
- `/opt/sangfor-fw-log/data/export/by_port/` - 按端口查询的结果
- `/opt/sangfor-fw-log/data/export/by_date/` - 按日期查询的结果

## 🔄 升级和卸载

### 升级到新版本

```bash
# 安装新版本DEB包（自动升级）
sudo dpkg -i sangfor-fw-log-query_2.2.0_all.deb
```

### 卸载（保留数据）

```bash
sudo apt remove sangfor-fw-log-query
# 数据保留在 /opt/sangfor-fw-log/data
```

### 完全卸载（删除所有数据）

```bash
sudo apt purge sangfor-fw-log-query
# 删除所有数据和配置文件
```

## 🐛 故障排查

### 1. 提示"未检测到tinycdb"

```bash
# 安装tinycdb
sudo apt install tinycdb

# 如果apt源没有，手动编译安装
wget http://www.corpit.ru/mjt/tinycdb/tinycdb-0.78.tar.gz
tar xzf tinycdb-0.78.tar.gz
cd tinycdb-0.78
make
sudo make install
```

### 2. 查询速度慢

```bash
# 检查是否使用CDB索引
sangfor-fw-log
# 查看主菜单是否显示"索引类型: CDB"

# 如果显示"文本索引"，检查tinycdb安装
which cdbmake

# 重新生成索引
# 在交互模式选择: 2) 更新索引
```

### 3. 权限问题

```bash
# 检查目录权限
ls -la /opt/sangfor-fw-log/

# 修复权限
sudo chmod 755 /opt/sangfor-fw-log/sangforfw_log.sh
sudo chmod -R 755 /opt/sangfor-fw-log/data/
```

### 4. 日志文件找不到

```bash
# 检查日志目录
ls -lh /opt/sangfor-fw-log/data/sangfor_fw_log/

# 确认日志文件格式
# 支持格式: *.log, *_2026-04-27.log
```

## 📚 查看帮助

```bash
# 查看man手册
man sangfor-fw-log

# 查看README
cat /usr/share/doc/sangfor-fw-log-query/README.md

# 交互模式帮助
sangfor-fw-log
# 选择菜单: 5) 帮助
```

## 🔐 安全建议

1. **日志文件权限**: 确保日志文件只有授权用户可读
2. **导出目录**: 定期清理导出目录，避免敏感信息泄露
3. **网络隔离**: 建议在内网环境使用
4. **审计日志**: 记录查询操作（未来版本支持）

## 📈 性能优化建议

1. **使用SSD存储**: 日志文件放在SSD上可提升I/O性能
2. **安装tinycdb**: 必装，查询速度提升5-8倍
3. **调整并行度**: 根据CPU核心数调整 `PARALLEL_JOBS`
4. **定期压缩**: 启用 `AUTO_COMPRESS_INDEX=1`
5. **日志归档**: 定期归档旧日志，减少索引大小

## 🛠️ 开发者信息

### 构建环境要求

- bash
- dpkg-deb (Debian包构建工具)
- gzip

### 修改源码后重新构建

```bash
# 1. 修改 sangforfw_log.sh
vim sangforfw_log.sh

# 2. 更新版本号
vim build_deb.sh  # 修改 VERSION="2.1.1"

# 3. 重新构建
./build_deb.sh

# 4. 测试安装
sudo dpkg -i sangfor-fw-log-query_2.1.1_all.deb
```

### 目录结构说明

```
build/
└── sangfor-fw-log-query_2.1.0_all/
    ├── DEBIAN/
    │   ├── control      # 包元数据
    │   ├── postinst     # 安装后脚本
    │   ├── prerm        # 卸载前脚本
    │   └── postrm       # 卸载后脚本
    ├── usr/
    │   ├── bin/
    │   │   └── sangfor-fw-log  # 符号链接
    │   └── share/
    │       ├── doc/
    │       │   └── sangfor-fw-log-query/
    │       │       └── README.md
    │       └── man/
    │           └── man1/
    │               └── sangfor-fw-log.1.gz
    ├── opt/
    │   └── sangfor-fw-log/
    │       ├── sangforfw_log.sh
    │       ├── config/
    │       │   └── default.conf
    │       └── data/
    └── etc/
        └── sangfor-fw-log/
            └── config.conf  # 符号链接
```

## 📞 技术支持

- 问题反馈: 提交Issue到项目仓库
- 邮件支持: support@qjkj.com

## 📄 版权信息

© 2026 QJKJ Team  
深信服防火墙日志查询工具 v2.1.0

---

**最后更新**: 2026-04-27
