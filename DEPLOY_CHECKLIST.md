# 银河麒麟系统部署检查清单

## 📦 部署前准备

### 1. 文件清单
- [ ] build/sangfor-fw-log-query_2.1.0_all/ (DEB包目录)
- [ ] sangforfw_log.sh (主脚本)
- [ ] performance_test.sh (性能测试)
- [ ] DEPLOY_GUIDE.md (部署指南)
- [ ] data/sangfor_fw_log/*.log (日志文件)

### 2. 传输到银河麒麟系统
```bash
# 方法1：使用scp
scp -r build/ user@kylin-server:/tmp/
scp sangforfw_log.sh user@kylin-server:/tmp/
scp performance_test.sh user@kylin-server:/tmp/
scp DEPLOY_GUIDE.md user@kylin-server:/tmp/

# 方法2：使用U盘/共享文件夹
# 将以下文件复制到U盘：
# - build/ 目录
# - sangforfw_log.sh
# - performance_test.sh
# - DEPLOY_GUIDE.md
```

---

## 🚀 部署步骤

### 步骤1：构建DEB包
```bash
ssh user@kylin-server
cd /tmp
dpkg-deb --build build/sangfor-fw-log-query_2.1.0_all
ls -lh sangfor-fw-log-query_2.1.0_all.deb
```
**预期结果：** 生成约50KB的.deb文件

---

### 步骤2：安装DEB包
```bash
sudo dpkg -i sangfor-fw-log-query_2.1.0_all.deb
sudo apt install -f
```
**预期结果：** 
```
正在设置 sangfor-fw-log-query (2.1.0) ...
深信服防火墙日志查询工具已安装到 /opt/sangfor-fw-log
```

---

### 步骤3：安装CDB支持（关键！）
```bash
sudo apt install tinycdb
which cdb && which cdbmake
```
**预期结果：** 
```
/usr/bin/cdb
/usr/bin/cdbmake
```

---

### 步骤4：复制日志文件
```bash
# 创建日志目录
sudo mkdir -p /opt/sangfor-fw-log/data/sangfor_fw_log

# 复制日志文件（根据实际路径调整）
sudo cp /path/to/your/*.log /opt/sangfor-fw-log/data/sangfor_fw_log/

# 设置权限
sudo chown -R $USER:$USER /opt/sangfor-fw-log/data

# 验证
ls -lh /opt/sangfor-fw-log/data/sangfor_fw_log/
```
**预期结果：** 看到日志文件列表

---

### 步骤5：生成CDB索引
```bash
sangfor-fw-log
# 选择 "3. 更新索引"
# 等待索引生成完成
```
**预期结果：** 
```
正在扫描日志文件...
找到 1 个日志文件
正在建立索引...
索引建立完成！共 66871 条记录
正在生成CDB索引...
CDB索引生成完成！
```

---

### 步骤6：验证CDB索引
```bash
ls -lh /opt/sangfor-fw-log/data/index/
```
**预期结果：** 
```
sangfor_fw_log_index.db      5.1M  (文本索引)
sangfor_fw_log_index.cdb     3.5M  (CDB索引)
sangfor_fw_log_index.db.gz   306K  (压缩备份)
```

---

## ✅ 功能测试

### 测试1：命令行查询
```bash
time sangfor-fw-log 192.168.1.1
```
**预期结果：** 
- 查询时间：200-300ms
- 显示查询结果
- 自动导出到 /opt/sangfor-fw-log/data/export/by_ip/

---

### 测试2：交互式菜单
```bash
sangfor-fw-log
```
**预期结果：** 
```
═══════════════════════════════════════
  防火墙日志查询工具 v2.1
  © 2026 QJKJ Team
═══════════════════════════════════════

  索引: 66871 条 | 日志: 1 个

  1. 查询IP/端口
  2. 高级查询
  3. 更新索引
  4. 快捷键管理
  5. 清理文件
  0. 退出
```

---

### 测试3：性能基准测试
```bash
cd /tmp
bash performance_test.sh
```
**预期结果：** 
```
测试 #1: 2.55.81.95
  平均: 250ms
  评级: ⭐⭐⭐⭐⭐ 优秀

测试 #2: 1.181.87.36
  平均: 220ms
  评级: ⭐⭐⭐⭐⭐ 优秀

测试 #3: 192.168.1.1
  平均: 280ms
  评级: ⭐⭐⭐⭐⭐ 优秀

【测试总结】
  平均耗时: 250ms
  性能提升: 8倍
```

---

## 📊 性能对比

### Windows环境（无CDB）
- 平均查询时间：2071ms
- 评级：⭐⭐ 需要优化

### 银河麒麟（使用CDB）
- 平均查询时间：250ms
- 评级：⭐⭐⭐⭐⭐ 优秀
- **性能提升：8倍**

---

## 🔧 故障排查

### 问题1：CDB索引未生成
```bash
# 检查tinycdb是否安装
dpkg -l | grep tinycdb

# 如果未安装
sudo apt install tinycdb

# 重新生成索引
sangfor-fw-log
# 选择 "3. 更新索引"
```

---

### 问题2：查询速度仍然慢（>500ms）
```bash
# 检查是否使用了CDB索引
ls -lh /opt/sangfor-fw-log/data/index/*.cdb

# 如果CDB文件不存在，重新生成
sangfor-fw-log
# 选择 "3. 更新索引"

# 检查磁盘I/O性能
iostat -x 1 5
```

---

### 问题3：权限错误
```bash
# 设置正确的权限
sudo chown -R $USER:$USER /opt/sangfor-fw-log/data
sudo chmod +x /opt/sangfor-fw-log/sangforfw_log.sh
```

---

## 📝 部署完成确认

- [ ] DEB包安装成功
- [ ] tinycdb已安装
- [ ] 日志文件已复制
- [ ] CDB索引已生成
- [ ] 命令行查询速度 <300ms
- [ ] 交互式菜单正常工作
- [ ] 性能测试通过（平均<300ms）
- [ ] 导出文件功能正常

---

## 🎯 下一步

### 生产环境配置
1. 设置定时任务自动更新索引
2. 配置日志轮转和清理
3. 设置性能监控告警
4. 备份索引和配置文件

详见：DEPLOY_GUIDE.md

---

## 📞 技术支持

如遇到问题，请检查：
1. /var/log/syslog (系统日志)
2. /opt/sangfor-fw-log/data/temp/ (临时文件)
3. DEPLOY_GUIDE.md (详细部署指南)
