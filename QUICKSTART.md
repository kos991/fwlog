# 🚀 快速开始指南

## Windows环境（当前环境）

### 方式1: 一键启动（推荐）
```bash
./quickstart.sh
```

### 方式2: 手动启动

#### 1. 安装Go（如果未安装）
下载并安装: https://go.dev/dl/go1.22.2.windows-amd64.msi

#### 2. 准备日志文件
```bash
# 将日志文件复制到
./data/sangfor_fw_log/
```

#### 3. 编译程序
```bash
# 初始化模块
go mod init fw_log_query

# 下载依赖（SQLite版本，无需CGO）
go get modernc.org/sqlite

# 编译
go build -o fw_log_query.exe fw_log_optimized.go
```

#### 4. 运行服务
```bash
# 首次运行（建立索引）
./fw_log_query.exe -logdir=./data/sangfor_fw_log -db=./data/index/fw_logs.db -rebuild -port=8080

# 后续运行
./fw_log_query.exe -logdir=./data/sangfor_fw_log -db=./data/index/fw_logs.db -port=8080
```

#### 5. 访问Web界面
打开浏览器访问: http://localhost:8080

---

## Linux/Debian环境

### 方式1: 一键安装（推荐）
```bash
chmod +x setup.sh
sudo ./setup.sh
```

### 方式2: 使用Makefile
```bash
# 编译
make build

# 运行
make run

# 重建索引
make rebuild

# 安装为系统服务
sudo make install-service
```

### 方式3: 手动安装

#### 1. 安装依赖
```bash
# 安装Go
wget https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 安装DuckDB库
wget https://github.com/duckdb/duckdb/releases/download/v0.10.2/libduckdb-linux-amd64.zip
unzip libduckdb-linux-amd64.zip
sudo cp libduckdb.so /usr/local/lib/
sudo cp duckdb.h /usr/local/include/
sudo ldconfig
```

#### 2. 编译程序
```bash
go mod download
go build -ldflags="-s -w" -o fw_log_query fw_log_optimized.go
```

#### 3. 运行服务
```bash
./fw_log_query -logdir=/data/sangfor_fw_log -db=/data/index/fw_logs.duckdb -port=8080 -rebuild
```

---

## 🎯 使用说明

### Web界面功能

1. **实时搜索**
   - 输入IP地址、日期或选择协议
   - 自动触发搜索（500ms防抖）

2. **分页浏览**
   - 每页显示100条记录
   - 点击"上一页"/"下一页"翻页

3. **性能监控**
   - 查看索引记录数
   - 查看数据库大小
   - 查看压缩率
   - 查看查询耗时（毫秒级）

4. **索引管理**
   - 点击"重建索引"按钮
   - 等待重建完成
   - 刷新页面查看新数据

### API接口

#### 查询接口
```bash
# 基础查询
curl "http://localhost:8080/api/query?page=1&page_size=100"

# IP查询
curl "http://localhost:8080/api/query?ip=192.168.1.1"

# 日期过滤
curl "http://localhost:8080/api/query?date=Apr%2027"

# 协议过滤
curl "http://localhost:8080/api/query?protocol=TCP"

# 组合查询
curl "http://localhost:8080/api/query?ip=192.168.1.1&protocol=TCP&page=1"
```

#### 统计接口
```bash
curl "http://localhost:8080/api/stats"
```

返回示例:
```json
{
  "total_records": 531664,
  "total_files": 4,
  "db_size_mb": 1.5,
  "raw_size_mb": 451.2,
  "compression_pct": 72.3,
  "last_update": "2026-04-27 22:30:15"
}
```

#### 重建索引接口
```bash
curl -X POST "http://localhost:8080/api/rebuild"
```

---

## 🔧 配置参数

```bash
-logdir   string   日志目录路径 (默认: ./data/sangfor_fw_log)
-db       string   数据库文件路径 (默认: ./data/index/fw_logs.duckdb)
-port     int      Web服务端口 (默认: 8080)
-workers  int      并行工作数 (默认: 4)
-rebuild  bool     启动时重建索引
```

---

## 📊 性能测试

运行性能测试脚本:
```bash
chmod +x test_performance.sh
./test_performance.sh
```

测试内容:
- 索引构建速度
- 查询响应时间
- 压缩率
- 内存占用

---

## 🐛 常见问题

### Q1: Windows下编译失败
**A:** DuckDB需要CGO支持，建议使用SQLite版本:
```bash
# 修改导入
sed -i 's/go-duckdb/sqlite/g' fw_log_optimized.go
go get modernc.org/sqlite
go build -o fw_log_query.exe fw_log_optimized.go
```

### Q2: 端口被占用
**A:** 修改端口号:
```bash
./fw_log_query -port=9090
```

### Q3: 日志文件找不到
**A:** 检查路径:
```bash
ls -la ./data/sangfor_fw_log/
# 确保有 .log 文件
```

### Q4: 索引构建失败
**A:** 检查磁盘空间和权限:
```bash
df -h
ls -la ./data/index/
```

### Q5: 查询速度慢
**A:** 重建索引优化:
```bash
rm ./data/index/fw_logs.db
./fw_log_query -rebuild
```

---

## 📝 下一步

1. ✅ 启动服务
2. ✅ 访问Web界面
3. ✅ 测试查询功能
4. ✅ 查看性能指标
5. 📖 阅读完整文档: [README_OPTIMIZED.md](README_OPTIMIZED.md)
6. 📊 查看对比分析: [COMPARISON.md](COMPARISON.md)

---

## 🆘 获取帮助

- 查看日志: `journalctl -u fw-log-query -f` (Linux)
- 查看进程: `ps aux | grep fw_log_query`
- 测试连接: `curl http://localhost:8080/api/stats`

---

**QJKJ Team** | 2026
