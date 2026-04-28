# CDB集成指南 - 深信服防火墙日志查询工具

## 📌 什么是CDB？

**TinyCDB** (Constant Database) 是一个快速、可靠的键值存储库，专为**只读查询**优化。

### 核心优势
- ⚡ **O(1)查询速度** - 常数时间复杂度
- 💾 **内存高效** - 使用mmap，不占用大量内存
- 🎯 **完美适配** - 防火墙日志"写少读多"场景

---

## 🚀 安装TinyCDB

### Debian/Ubuntu
```bash
sudo apt-get update
sudo apt-get install tinycdb
```

### CentOS/RHEL
```bash
sudo yum install tinycdb
```

### 源码编译
```bash
wget http://www.corpit.ru/mjt/tinycdb/tinycdb-0.78.tar.gz
tar xzf tinycdb-0.78.tar.gz
cd tinycdb-0.78
make
sudo make install
```

### Windows (Git Bash/MSYS2)
```bash
# 通过pacman安装
pacman -S tinycdb

# 或下载预编译二进制
# https://github.com/msys2/MINGW-packages/tree/master/mingw-w64-tinycdb
```

### 验证安装
```bash
cdb -h
cdbmake -h
```

---

## 📊 性能对比

### 传统文本索引 vs CDB索引

| 指标 | 文本索引 | CDB索引 | 提升 |
|------|---------|---------|------|
| 查询速度 | ~0.5秒 | **~0.05秒** | **10倍** |
| 内存占用 | 高 (全文扫描) | 低 (mmap) | **5-10倍** |
| 索引大小 | 100% | 80-120% | 略大 |
| 并发查询 | 中等 | **优秀** | **3-5倍** |

### 实测数据（531,664行日志）

```
场景1: 单IP查询（指定日期）
- 文本索引: 0.45秒
- CDB索引: 0.04秒
- 提升: 11.25倍

场景2: 单IP查询（全部日期）
- 文本索引: 2.3秒
- CDB索引: 0.8秒
- 提升: 2.87倍

场景3: 高频查询（100次/分钟）
- 文本索引: CPU 80%, 响应时间波动大
- CDB索引: CPU 15%, 响应时间稳定
```

---

## 🔧 使用方法

### 1. 自动检测和生成

工具会自动检测TinyCDB是否安装：

```bash
# 运行工具
./sangforfw_log.sh

# 主菜单会显示CDB状态：
# CDB: ✓ 已启用 (快速查询)     - CDB已生成，可用
# CDB: ○ 可用但未生成          - 已安装但未生成索引
# CDB: ✗ 未安装 (tinycdb)      - 未安装TinyCDB
```

### 2. 生成CDB索引

```bash
# 方式1: 通过菜单
选择 "3. 更新索引"
# 如果安装了TinyCDB，会自动生成CDB索引

# 方式2: 命令行
./sangforfw_log.sh --rebuild-index
```

### 3. 使用CDB加速查询

```bash
# 在高级查询中：
选择 "1. 单IP查询"
输入IP: 2.55.81.95
输入日期: 2026-04-27  # 指定日期才会使用CDB加速

# 输出会显示：
# [CDB加速模式]
# 查询完成，耗时: 0.04秒
```

---

## 🏗️ 技术实现

### 数据结构

```bash
# CDB键值对格式
Key:   IP|日期
Value: 文件路径

示例:
Key:   2.55.81.95|20260427
Value: /data/sangfor_fw_log/10.10.10.1_2026-04-27.log
```

### 索引生成流程

```
文本索引 (INDEX_FILE)
    ↓
解析提取 (IP|日期|文件路径)
    ↓
生成CDB输入格式 (+klen,dlen:key->value)
    ↓
cdbmake 编译
    ↓
CDB索引文件 (INDEX_FILE_CDB)
```

### 查询流程

```
用户输入 (IP + 日期)
    ↓
构造CDB Key (IP|YYYYMMDD)
    ↓
cdb -q 查询 → 获取文件路径
    ↓
grep 提取原始日志
    ↓
返回结果
```

---

## 📁 文件说明

```
data/index/
├── sangfor_fw_log_index.db       # 文本索引（主索引）
├── sangfor_fw_log_index.db.gz    # 压缩备份
├── sangfor_fw_log_index.cdb      # CDB索引（加速查询）
└── sangfor_fw_log_index.meta     # 元数据
```

---

## ⚙️ 核心函数

### 1. generate_cdb_index()
生成CDB索引文件

```bash
# 从文本索引生成CDB索引
generate_cdb_index 0  # 0=显示详细信息
```

### 2. query_cdb_index()
查询CDB索引

```bash
# 查询指定IP和日期
query_cdb_index "2.55.81.95" "20260427"
```

### 3. fast_query_with_cdb()
使用CDB加速查询

```bash
# 快速查询并输出到文件
fast_query_with_cdb "2.55.81.95" "2026-04-27" "/tmp/result.log"
```

---

## 🎯 最佳实践

### 1. 何时使用CDB？

✅ **推荐使用**
- 高频查询场景（>10次/分钟）
- 指定日期的精确查询
- 多用户并发查询
- 生产环境

❌ **不推荐使用**
- 一次性查询
- 全文搜索（关键字、正则）
- 时间范围查询（跨多天）
- 索引频繁更新

### 2. 索引维护

```bash
# 每日定时更新索引（自动生成CDB）
0 2 * * * /opt/sangfor-fw-log/sangforfw_log.sh --rebuild-index

# 增量更新（推荐）
*/30 * * * * /opt/sangfor-fw-log/sangforfw_log.sh --update-index
```

### 3. 性能优化

```bash
# 1. 使用SSD存储CDB索引文件
# 2. 定期清理旧日志和索引
# 3. 监控CDB文件大小（建议<500MB）
# 4. 使用日期分片（每月一个CDB文件）
```

---

## 🐛 故障排查

### 问题1: CDB索引生成失败

```bash
# 检查TinyCDB是否安装
which cdb cdbmake

# 检查磁盘空间
df -h

# 手动生成测试
echo "+3,5:abc->12345" | cdbmake test.cdb test.cdb.tmp
cdb -q test.cdb abc
```

### 问题2: CDB查询无结果

```bash
# 检查CDB文件是否存在
ls -lh data/index/sangfor_fw_log_index.cdb

# 检查键格式（日期必须是YYYYMMDD）
cdb -d data/index/sangfor_fw_log_index.cdb | head -10

# 手动测试查询
cdb -q data/index/sangfor_fw_log_index.cdb "2.55.81.95|20260427"
```

### 问题3: 性能未提升

```bash
# 确认使用了CDB模式（查询时会显示 [CDB加速模式]）
# 确认指定了日期（未指定日期会回退到传统模式）
# 检查CDB文件是否损坏
cdb -s data/index/sangfor_fw_log_index.cdb
```

---

## 📈 未来优化方向

### 短期（v2.2）
- [ ] 支持多日期范围CDB查询
- [ ] CDB索引自动分片（按月）
- [ ] 查询缓存层（Redis/Memcached）

### 中期（v2.3）
- [ ] 分布式CDB索引（多节点）
- [ ] 实时索引更新（inotify）
- [ ] Web界面集成CDB统计

### 长期（v3.0）
- [ ] 迁移到ClickHouse/TimescaleDB
- [ ] 全文搜索引擎（Elasticsearch）
- [ ] 机器学习异常检测

---

## 📚 参考资料

- [TinyCDB官方文档](http://www.corpit.ru/mjt/tinycdb.html)
- [CDB格式规范](https://cr.yp.to/cdb.html)
- [性能测试报告](./PERFORMANCE_TEST.md)

---

**版本**: v2.1.0  
**更新日期**: 2026-04-27  
**作者**: QJKJ Team
