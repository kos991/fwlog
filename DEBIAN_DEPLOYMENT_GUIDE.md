# 🚀 Debian服务器部署完整指南

**部署包**: fw_log_query_debian_20260427_234230.tar.gz (20KB)  
**目标系统**: Debian 10/11/12 或 Ubuntu 20.04/22.04/24.04

---

## 📦 部署包内容

```
fw_log_query_debian_20260427_234230/
├── fw_log_optimized.go          # 主程序源码
├── deploy_debian.sh              # 自动部署脚本
├── Makefile                      # 构建脚本
├── fw-log-query.service          # systemd服务配置
├── go.mod                        # Go模块定义
├── README_OPTIMIZED.md           # 完整文档
├── QUICKSTART.md                 # 快速指南
├── COMPARISON.md                 # 性能对比
├── QUICK_REFERENCE.txt           # 快速参考
├── check_deployment.sh           # 部署检查（可选）
└── test_performance.sh           # 性能测试（可选）
```

---

## 🎯 部署步骤

### 步骤1: 传输部署包到Debian服务器

#### 方式A: 使用scp（推荐）
```bash
# 在Windows Git Bash中执行
scp fw_log_query_debian_20260427_234230.tar.gz user@192.168.1.100:/tmp/
```

#### 方式B: 使用WinSCP/FileZilla
1. 打开WinSCP或FileZilla
2. 连接到Debian服务器
3. 上传 `fw_log_query_debian_20260427_234230.tar.gz` 到 `/tmp/` 目录

#### 方式C: 使用rsync
```bash
rsync -avz fw_log_query_debian_20260427_234230.tar.gz user@192.168.1.100:/tmp/
```

---

### 步骤2: SSH登录到Debian服务器

```bash
ssh user@192.168.1.100
```

---

### 步骤3: 解压部署包

```bash
cd /tmp
tar -xzf fw_log_query_debian_20260427_234230.tar.gz
cd fw_log_query_debian_20260427_234230
```

---

### 步骤4: 运行自动部署脚本

```bash
chmod +x deploy_debian.sh
sudo ./deploy_debian.sh
```

**部署脚本会自动完成以下操作**：
- ✅ 更新系统软件包
- ✅ 安装Go 1.22.2
- ✅ 安装DuckDB开发库
- ✅ 创建工作目录（/opt/fw_log_query, /data/sangfor_fw_log, /data/index）
- ✅ 编译优化版二进制程序
- ✅ 安装systemd服务
- ✅ 配置自动重启和安全加固

**预计耗时**: 3-5分钟（取决于网络速度）

---

### 步骤5: 复制日志文件

```bash
# 如果日志文件在本地
sudo cp /path/to/*.log /data/sangfor_fw_log/

# 如果需要从其他服务器传输
scp user@log-server:/path/to/*.log /tmp/
sudo mv /tmp/*.log /data/sangfor_fw_log/

# 检查日志文件
ls -lh /data/sangfor_fw_log/
```

---

### 步骤6: 启动服务

```bash
# 启动服务
sudo systemctl start fw-log-query

# 查看状态
sudo systemctl status fw-log-query

# 设置开机自启
sudo systemctl enable fw-log-query

# 查看实时日志
sudo journalctl -u fw-log-query -f
```

---

### 步骤7: 访问Web界面

打开浏览器访问：
```
http://服务器IP:8080
```

例如：
```
http://192.168.1.100:8080
```

---

## 🔧 手动测试（可选）

如果想先手动测试再启动服务：

```bash
# 手动运行并建立索引
sudo /opt/fw_log_query/fw_log_query \
    -logdir=/data/sangfor_fw_log \
    -db=/data/index/fw_logs.duckdb \
    -port=8080 \
    -rebuild

# 按Ctrl+C停止后，再启动服务
sudo systemctl start fw-log-query
```

---

## 📊 性能测试（可选）

如果部署包中包含性能测试脚本：

```bash
chmod +x test_performance.sh
./test_performance.sh
```

---

## 🔍 验证部署

### 1. 检查服务状态
```bash
sudo systemctl status fw-log-query
```

应该显示：
```
● fw-log-query.service - Firewall Log Query System
   Loaded: loaded (/etc/systemd/system/fw-log-query.service; enabled)
   Active: active (running) since ...
```

### 2. 检查端口监听
```bash
sudo netstat -tlnp | grep 8080
# 或
sudo ss -tlnp | grep 8080
```

应该显示：
```
tcp6       0      0 :::8080                 :::*                    LISTEN      12345/fw_log_query
```

### 3. 测试API接口
```bash
curl http://localhost:8080/api/stats
```

应该返回JSON格式的统计信息：
```json
{
  "total_records": 531664,
  "total_files": 4,
  "db_size_mb": 1.5,
  "raw_size_mb": 451.2,
  "compression_pct": 72.3,
  "last_update": "2026-04-27 23:45:00"
}
```

### 4. 访问Web界面
打开浏览器访问 `http://服务器IP:8080`，应该看到：
- 统计卡片（索引记录数、数据库大小、压缩率等）
- 搜索面板
- 现代化的Tailwind CSS界面

---

## 🐛 故障排查

### 问题1: 服务无法启动

**检查日志**：
```bash
sudo journalctl -u fw-log-query -n 50
```

**常见原因**：
- 端口8080被占用 → 修改端口：编辑 `/etc/systemd/system/fw-log-query.service`
- 日志目录不存在 → 创建：`sudo mkdir -p /data/sangfor_fw_log`
- 权限问题 → 检查：`ls -la /data/`

### 问题2: 编译失败

**检查Go环境**：
```bash
go version
```

**检查DuckDB库**：
```bash
ls -la /usr/local/lib/libduckdb.so
```

**重新安装**：
```bash
sudo ./deploy_debian.sh
```

### 问题3: 无法访问Web界面

**检查防火墙**：
```bash
# Debian/Ubuntu使用ufw
sudo ufw status
sudo ufw allow 8080/tcp

# 或使用iptables
sudo iptables -I INPUT -p tcp --dport 8080 -j ACCEPT
```

**检查服务状态**：
```bash
sudo systemctl status fw-log-query
```

### 问题4: 查询速度慢

**重建索引**：
```bash
sudo systemctl stop fw-log-query
sudo rm /data/index/fw_logs.duckdb
sudo /opt/fw_log_query/fw_log_query -rebuild
sudo systemctl start fw-log-query
```

---

## 🔧 配置调整

### 修改端口

编辑服务配置：
```bash
sudo nano /etc/systemd/system/fw-log-query.service
```

修改 `-port=8080` 为其他端口，然后：
```bash
sudo systemctl daemon-reload
sudo systemctl restart fw-log-query
```

### 修改日志目录

编辑服务配置：
```bash
sudo nano /etc/systemd/system/fw-log-query.service
```

修改 `-logdir=/data/sangfor_fw_log` 为其他目录，然后：
```bash
sudo systemctl daemon-reload
sudo systemctl restart fw-log-query
```

### 修改并行数

编辑服务配置：
```bash
sudo nano /etc/systemd/system/fw-log-query.service
```

修改 `-workers=4` 为其他值（建议为CPU核心数），然后：
```bash
sudo systemctl daemon-reload
sudo systemctl restart fw-log-query
```

---

## 📋 常用命令

```bash
# 启动服务
sudo systemctl start fw-log-query

# 停止服务
sudo systemctl stop fw-log-query

# 重启服务
sudo systemctl restart fw-log-query

# 查看状态
sudo systemctl status fw-log-query

# 开机自启
sudo systemctl enable fw-log-query

# 禁用自启
sudo systemctl disable fw-log-query

# 查看日志
sudo journalctl -u fw-log-query -f

# 查看最近50行日志
sudo journalctl -u fw-log-query -n 50

# 查看今天的日志
sudo journalctl -u fw-log-query --since today

# 重建索引
sudo systemctl stop fw-log-query
sudo rm /data/index/fw_logs.duckdb
sudo /opt/fw_log_query/fw_log_query -rebuild
sudo systemctl start fw-log-query
```

---

## 🔒 安全建议

### 1. 配置防火墙
```bash
# 只允许特定IP访问
sudo ufw allow from 192.168.1.0/24 to any port 8080

# 或使用iptables
sudo iptables -A INPUT -p tcp -s 192.168.1.0/24 --dport 8080 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 -j DROP
```

### 2. 使用Nginx反向代理（可选）
```bash
# 安装Nginx
sudo apt-get install nginx

# 配置反向代理
sudo nano /etc/nginx/sites-available/fw-log-query

# 添加以下内容：
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

# 启用配置
sudo ln -s /etc/nginx/sites-available/fw-log-query /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

### 3. 配置HTTPS（可选）
```bash
# 使用Let's Encrypt
sudo apt-get install certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

---

## 📊 性能优化

### 1. 增加文件描述符限制
```bash
sudo nano /etc/systemd/system/fw-log-query.service
```

修改：
```
LimitNOFILE=65536
```

### 2. 调整数据库参数

编辑程序源码 `fw_log_optimized.go`，修改：
```go
db.Exec("SET memory_limit='4GB'")  // 增加内存限制
db.Exec("SET threads=8")           // 增加线程数
```

重新编译：
```bash
cd /tmp/fw_log_query_debian_20260427_234230
sudo CGO_ENABLED=1 go build -ldflags="-s -w" -o /opt/fw_log_query/fw_log_query fw_log_optimized.go
sudo systemctl restart fw-log-query
```

---

## 📖 文档参考

部署包中包含的文档：
```bash
cat QUICK_REFERENCE.txt      # 快速参考卡片
cat QUICKSTART.md             # 快速开始指南
cat README_OPTIMIZED.md       # 完整使用文档
cat COMPARISON.md             # 性能对比
```

---

## ✅ 部署完成检查清单

- [ ] 部署包已传输到服务器
- [ ] 部署脚本执行成功
- [ ] 日志文件已复制到 /data/sangfor_fw_log/
- [ ] 服务已启动（systemctl status显示active）
- [ ] 端口8080已监听
- [ ] Web界面可正常访问
- [ ] API接口返回正常
- [ ] 查询功能正常
- [ ] 分页功能正常
- [ ] 性能监控显示正常
- [ ] 服务已设置开机自启

---

## 🎉 部署成功！

访问Web界面：`http://服务器IP:8080`

享受20倍性能提升和现代化界面！

---

**QJKJ Team** | 2026-04-27
