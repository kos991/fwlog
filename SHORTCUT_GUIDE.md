# 快捷键功能使用指南

## 功能说明

脚本支持自动注册系统快捷键，让你可以在任何目录下快速调用防火墙日志查询工具。

---

## 快速开始

### 方法1：通过交互式菜单安装

```bash
# 1. 运行脚本
./sangforfw_log.sh

# 2. 选择菜单选项 4（快捷键管理）
# 3. 选择选项 1（安装快捷键）
# 4. 使快捷键生效
source ~/.bashrc   # 或 source ~/.zshrc
```

### 方法2：通过命令行安装

```bash
# 直接安装
./sangforfw_log.sh --install-shortcut

# 使快捷键生效
source ~/.bashrc   # 或 source ~/.zshrc
```

---

## 使用快捷键

安装后，可以在任何目录使用 `fwlog` 命令：

### 1. 启动交互式菜单
```bash
fwlog
```

### 2. 快速查询IP
```bash
fwlog 192.168.1.100
```

### 3. 查询IP+日期
```bash
fwlog 20260427 192.168.1.100
```

### 4. 查询IP+端口+日期
```bash
fwlog 20260427 192.168.1.100:443
```

---

## 快捷键管理

### 查看快捷键状态

**方法1：交互式菜单**
```bash
./sangforfw_log.sh
# 选择 4 -> 3
```

**方法2：命令行**
```bash
./sangforfw_log.sh --check-shortcut
```

**输出示例：**
```
快捷键状态：已安装
  命令: fwlog
  指向: /home/user/scripts/sangforfw_log.sh
```

### 卸载快捷键

**方法1：交互式菜单**
```bash
./sangforfw_log.sh
# 选择 4 -> 2
```

**方法2：命令行**
```bash
./sangforfw_log.sh --uninstall-shortcut
```

---

## 技术细节

### 支持的Shell

- ✓ Bash（修改 `~/.bashrc`）
- ✓ Zsh（修改 `~/.zshrc`）
- ✗ 其他Shell（需手动配置）

### 快捷键实现

快捷键通过在Shell配置文件中添加别名实现：

```bash
# 深信服防火墙日志查询工具快捷键 (自动添加于 2026-04-27 10:30:00)
alias fwlog='/path/to/sangforfw_log.sh'
```

### 自动检测

- 脚本会自动检测当前使用的Shell
- 自动选择正确的配置文件
- 避免重复安装

### 路径绑定

- 快捷键绑定到脚本的**绝对路径**
- 移动脚本后需要重新安装快捷键
- 支持多个脚本实例（使用不同的快捷键名称）

---

## 常见问题

### Q1: 安装后快捷键不生效？

**原因**：配置文件未重新加载

**解决**：
```bash
# Bash用户
source ~/.bashrc

# Zsh用户
source ~/.zshrc

# 或者重新打开终端
```

### Q2: 提示"未识别的Shell"？

**原因**：使用了不支持的Shell（如fish、tcsh等）

**解决**：手动添加别名到对应Shell的配置文件
```bash
# 例如fish用户，编辑 ~/.config/fish/config.fish
alias fwlog='/path/to/sangforfw_log.sh'
```

### Q3: 移动脚本后快捷键失效？

**原因**：快捷键绑定的是绝对路径

**解决**：
```bash
# 1. 卸载旧快捷键
./sangforfw_log.sh --uninstall-shortcut

# 2. 重新安装
./sangforfw_log.sh --install-shortcut

# 3. 重新加载配置
source ~/.bashrc
```

### Q4: 如何修改快捷键名称？

**方法1：修改脚本变量**
```bash
# 编辑 sangforfw_log.sh，修改第18行
SHORTCUT_NAME="fwlog"  # 改为你想要的名称，如 "fw"
```

**方法2：手动编辑配置文件**
```bash
# 编辑 ~/.bashrc 或 ~/.zshrc
alias fw='/path/to/sangforfw_log.sh'  # 使用自定义名称
```

### Q5: 多个脚本实例如何管理？

**场景**：在不同目录有多个脚本副本

**解决**：使用不同的快捷键名称
```bash
# 脚本1：生产环境
SHORTCUT_NAME="fwlog-prod"

# 脚本2：测试环境
SHORTCUT_NAME="fwlog-test"
```

---

## 高级用法

### 1. 创建多个快捷键

```bash
# 编辑 ~/.bashrc
alias fwlog='/path/to/sangforfw_log.sh'
alias fwq='/path/to/sangforfw_log.sh'  # 快速查询别名
alias fwi='/path/to/sangforfw_log.sh --install-shortcut'  # 安装别名
```

### 2. 带参数的快捷键

```bash
# 编辑 ~/.bashrc
alias fwlog-today='/path/to/sangforfw_log.sh $(date +%Y%m%d)'
alias fwlog-yesterday='/path/to/sangforfw_log.sh $(date -d yesterday +%Y%m%d)'
```

使用：
```bash
fwlog-today 192.168.1.100
fwlog-yesterday 192.168.1.100
```

### 3. 函数封装

```bash
# 编辑 ~/.bashrc
fwlog() {
    if [ $# -eq 0 ]; then
        /path/to/sangforfw_log.sh
    else
        /path/to/sangforfw_log.sh "$@"
    fi
}
```

---

## 卸载说明

### 完全卸载

```bash
# 1. 卸载快捷键
./sangforfw_log.sh --uninstall-shortcut

# 2. 删除脚本和数据
rm -rf /path/to/sangforfw_log.sh
rm -rf /path/to/sangfor_fw_log_index.*
rm -rf /path/to/fw_query_exports/

# 3. 重新加载配置
source ~/.bashrc
```

### 仅卸载快捷键（保留脚本）

```bash
./sangforfw_log.sh --uninstall-shortcut
source ~/.bashrc
```

---

## 安全建议

### 1. 权限控制

```bash
# 确保脚本只有所有者可写
chmod 755 sangforfw_log.sh

# 检查权限
ls -l sangforfw_log.sh
# 应显示：-rwxr-xr-x
```

### 2. 路径验证

安装前检查脚本路径：
```bash
./sangforfw_log.sh --check-shortcut
```

确认指向路径正确后再使用。

### 3. 定期检查

```bash
# 检查快捷键是否被篡改
grep "alias fwlog=" ~/.bashrc
```

---

## 命令参考

| 命令 | 说明 |
|------|------|
| `./sangforfw_log.sh --install-shortcut` | 安装快捷键 |
| `./sangforfw_log.sh --uninstall-shortcut` | 卸载快捷键 |
| `./sangforfw_log.sh --check-shortcut` | 查看快捷键状态 |
| `fwlog` | 启动交互式菜单 |
| `fwlog IP` | 快速查询IP |
| `fwlog 日期 IP` | 查询IP+日期 |
| `fwlog 日期 IP:端口` | 查询IP+端口+日期 |

---

## 更新日志

### v2.0 (2026-04-27)

**新增功能**：
- 自动快捷键注册
- 支持Bash和Zsh
- 交互式快捷键管理
- 命令行快捷键操作

**改进**：
- 自动检测Shell类型
- 避免重复安装
- 路径自动绑定

---

## 联系方式

- **团队**: QJKJ Team
- **版本**: 2.0
- **更新**: 2026-04-27

如有问题或建议，请联系技术支持。
