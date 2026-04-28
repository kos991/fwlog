# 快捷键问题解决方案

## 问题现象
```bash
[root@localhost sangfor_fw_log_chaxun]# fwlog
-bash: fwlog: command not found
```

## 原因分析
快捷键已经安装到配置文件（~/.bashrc 或 ~/.zshrc），但当前Shell会话还没有重新加载配置。

## 解决方法

### 方法1：重新加载配置（推荐）⭐
```bash
source ~/.bashrc
# 或者
source ~/.zshrc
```

然后再次运行：
```bash
fwlog
```

### 方法2：使用完整路径（临时方案）
```bash
# 假设脚本在当前目录
./fwlog

# 或使用绝对路径
/path/to/sangfor_fw_log_chaxun/fwlog
```

### 方法3：重新打开终端
```bash
exit
# 重新登录后，快捷键自动生效
```

### 方法4：运行诊断脚本
```bash
chmod +x fix_shortcut.sh
./fix_shortcut.sh
```

## 验证快捷键是否安装

### 检查配置文件
```bash
# Bash用户
grep fwlog ~/.bashrc

# Zsh用户
grep fwlog ~/.zshrc
```

应该看到类似输出：
```
alias fwlog='/path/to/sangfor_fw_log_chaxun/fwlog'
```

### 检查包装脚本
```bash
ls -l fwlog
```

应该看到：
```
-rwxr-xr-x 1 root root 123 Apr 27 10:00 fwlog
```

## 快捷键使用示例

### 打开交互式菜单
```bash
fwlog
```

### 快速查询
```bash
fwlog 192.168.1.100                    # 查询IP
fwlog 20260427 192.168.1.100           # 查询IP+日期
fwlog 20260427 192.168.1.100:443       # 查询IP+端口+日期
```

## 字符编码警告问题

### 问题现象
```
awk: warning: Invalid multibyte data detected.
```

### 解决方案
已在脚本中修复，使用 `LC_ALL=C` 避免多字节字符警告：
```bash
LC_ALL=C awk '...' file 2>/dev/null
```

这个警告不影响功能，只是提示日志文件中包含非UTF-8字符。修复后警告会被抑制。

## 完整的首次使用流程

```bash
# 1. 进入脚本目录
cd /path/to/sangfor_fw_log_chaxun

# 2. 赋予执行权限
chmod +x sangforfw_log.sh

# 3. 安装快捷键
./sangforfw_log.sh --install-shortcut

# 4. 重新加载配置
source ~/.bashrc

# 5. 使用快捷键
fwlog

# 6. 首次使用需要建立索引
# 在菜单中选择：3. 重建索引
```

## 常见问题

### Q: 为什么需要 source ~/.bashrc？
A: 修改配置文件后，当前Shell会话不会自动重新读取。`source` 命令会重新加载配置，使快捷键立即生效。

### Q: 每次登录都需要 source 吗？
A: 不需要。只有第一次安装快捷键后需要 source。以后每次登录时，Shell会自动加载配置文件。

### Q: 可以不用快捷键吗？
A: 可以。直接运行 `./sangforfw_log.sh` 也能使用所有功能。快捷键只是为了方便。

### Q: 快捷键可以自定义名称吗？
A: 可以。编辑 ~/.bashrc，修改 alias 行：
```bash
alias myfwlog='/path/to/fwlog'
```
然后 `source ~/.bashrc`

### Q: 如何卸载快捷键？
A: 运行卸载命令：
```bash
./sangforfw_log.sh --uninstall-shortcut
source ~/.bashrc
```
