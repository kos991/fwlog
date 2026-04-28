#!/bin/bash
# 深信服防火墙日志查询工具
# 适用于深信服防火墙V10 SP3，日志目录 /data/sangfor_fw_log
# 日志文件命名格式：IP_YYYY-MM-DD.log（例如：10.10.10.1_2026-04-24.log）

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PATH="$(realpath "${BASH_SOURCE[0]}")"

# 检查并修复脚本权限
check_and_fix_permissions() {
    local need_fix=0

    # 检查主脚本权限
    if [ ! -x "$SCRIPT_PATH" ]; then
        chmod +x "$SCRIPT_PATH" 2>/dev/null && \
            echo -e "\033[32m✓ 已修复主脚本执行权限\033[0m" || \
            echo -e "\033[31m✗ 无法修复主脚本权限，请手动执行: chmod +x $SCRIPT_PATH\033[0m"
        need_fix=1
    fi

    # 检查包装脚本权限
    local wrapper_script="$SCRIPT_DIR/fwlog"
    if [ -f "$wrapper_script" ] && [ ! -x "$wrapper_script" ]; then
        chmod +x "$wrapper_script" 2>/dev/null && \
            echo -e "\033[32m✓ 已修复快捷键脚本执行权限\033[0m" || \
            echo -e "\033[31m✗ 无法修复快捷键脚本权限，请手动执行: chmod +x $wrapper_script\033[0m"
        need_fix=1
    fi

    return $need_fix
}

# 启动时检查权限
check_and_fix_permissions

# 首次启动配置函数
first_time_setup() {
    local config_file="$SCRIPT_DIR/.fw_log_config"

    # 如果配置文件已存在，跳过
    if [ -f "$config_file" ]; then
        return 0
    fi

    clear
    echo -e "\033[36m════════════════════════════════════════\033[0m"
    echo -e "\033[36m  欢迎使用防火墙日志查询工具 v2.1\033[0m"
    echo -e "\033[36m════════════════════════════════════════\033[0m"
    echo ""
    echo -e "\033[33m检测到首次运行，请配置日志目录路径\033[0m"
    echo ""
    echo "常见路径："
    echo "  1. /data/sangfor_fw_log          (深信服默认路径)"
    echo "  2. /var/log/sangfor_fw_log       (标准日志路径)"
    echo "  3. 自定义路径"
    echo ""

    read -r -p "请选择 [1-3]: " path_choice

    case $path_choice in
        1)
            CUSTOM_LOG_DIR="/data/sangfor_fw_log"
            ;;
        2)
            CUSTOM_LOG_DIR="/var/log/sangfor_fw_log"
            ;;
        3)
            echo ""
            read -r -p "请输入日志目录完整路径: " CUSTOM_LOG_DIR
            ;;
        *)
            echo -e "\033[33m无效选择，使用默认路径: /data/sangfor_fw_log\033[0m"
            CUSTOM_LOG_DIR="/data/sangfor_fw_log"
            ;;
    esac

    # 验证目录是否存在
    if [ ! -d "$CUSTOM_LOG_DIR" ]; then
        echo ""
        echo -e "\033[31m警告: 目录不存在: $CUSTOM_LOG_DIR\033[0m"
        echo ""
        read -r -p "是否创建该目录? [y/N]: " create_dir
        if [[ "$create_dir" =~ ^[Yy]$ ]]; then
            mkdir -p "$CUSTOM_LOG_DIR" 2>/dev/null
            if [ $? -eq 0 ]; then
                echo -e "\033[32m✓ 目录创建成功\033[0m"
            else
                echo -e "\033[31m✗ 目录创建失败，请检查权限\033[0m"
                echo "将使用相对路径: $SCRIPT_DIR/data/sangfor_fw_log"
                CUSTOM_LOG_DIR=""
            fi
        else
            echo "将使用相对路径: $SCRIPT_DIR/data/sangfor_fw_log"
            CUSTOM_LOG_DIR=""
        fi
    else
        # 检查目录中是否有日志文件
        log_count=$(ls -1 "$CUSTOM_LOG_DIR"/*.log 2>/dev/null | wc -l)
        echo ""
        echo -e "\033[32m✓ 目录存在\033[0m"
        echo "  路径: $CUSTOM_LOG_DIR"
        echo "  日志文件: $log_count 个"
    fi

    # 保存配置
    if [ -n "$CUSTOM_LOG_DIR" ]; then
        echo "LOG_DIR=\"$CUSTOM_LOG_DIR\"" > "$config_file"
        echo -e "\033[32m✓ 配置已保存\033[0m"
    else
        touch "$config_file"
    fi

    echo ""
    read -r -p "按回车键继续..."
}

# 加载配置
load_config() {
    local config_file="$SCRIPT_DIR/.fw_log_config"
    if [ -f "$config_file" ]; then
        source "$config_file"
    fi
}

# 首次启动配置
first_time_setup

# 加载已保存的配置
load_config

# 智能检测日志目录（优先使用配置文件中的路径）
if [ -n "$LOG_DIR" ] && [ -d "$LOG_DIR" ]; then
    # 使用配置文件中的路径
    DATA_DIR="$SCRIPT_DIR/data"
elif [ -d "/data/sangfor_fw_log" ]; then
    # 服务器环境：使用绝对路径
    LOG_DIR="/data/sangfor_fw_log"
    DATA_DIR="$SCRIPT_DIR/data"
else
    # 本地环境：使用相对路径
    LOG_DIR="$SCRIPT_DIR/data/sangfor_fw_log"
    DATA_DIR="$SCRIPT_DIR/data"
fi
INDEX_DIR="$DATA_DIR/index"
EXPORT_DIR="$DATA_DIR/export"
EXPORT_BY_IP="$EXPORT_DIR/by_ip"
EXPORT_BY_PORT="$EXPORT_DIR/by_port"
EXPORT_BY_DATE="$EXPORT_DIR/by_date"
EXPORT_BY_QUERY="$EXPORT_DIR/by_query"
TEMP_DIR="$DATA_DIR/temp"
BACKUP_DIR="$DATA_DIR/backup"

# 索引文件路径
INDEX_FILE="$INDEX_DIR/sangfor_fw_log_index.db"
INDEX_FILE_GZ="$INDEX_DIR/sangfor_fw_log_index.db.gz"
INDEX_FILE_CDB="$INDEX_DIR/sangfor_fw_log_index.cdb"
INDEX_META="$INDEX_DIR/sangfor_fw_log_index.meta"

INDEX_READY_IN_SESSION=0
INDEX_VERSION="2.1"
PARALLEL_JOBS=8

# CDB支持检测
HAS_CDB=0
HAS_CDBMAKE=0
CDB_CMD=""
CDBMAKE_CMD=""

# 自动编译安装TinyCDB函数
auto_install_tinycdb() {
    local TINYCDB_TAR="$SCRIPT_DIR/tinycdb-0.78.tar.gz"
    local TINYCDB_DIR="$SCRIPT_DIR/tinycdb-0.78"
    local BIN_DIR="$SCRIPT_DIR/bin"

    # 检查是否已有源码包
    if [ ! -f "$TINYCDB_TAR" ]; then
        return 1
    fi

    # 检查编译工具
    if ! command -v gcc >/dev/null 2>&1 || ! command -v make >/dev/null 2>&1; then
        return 1
    fi

    echo -e "\033[33m检测到TinyCDB源码包，正在自动编译安装...\033[0m"

    # 解压
    if [ -d "$TINYCDB_DIR" ]; then
        rm -rf "$TINYCDB_DIR"
    fi

    tar -xzf "$TINYCDB_TAR" -C "$SCRIPT_DIR" 2>/dev/null || return 1

    # 编译
    cd "$TINYCDB_DIR"
    if ! make >/dev/null 2>&1; then
        cd "$SCRIPT_DIR"
        rm -rf "$TINYCDB_DIR"
        return 1
    fi

    # 安装到bin目录
    mkdir -p "$BIN_DIR"
    cp -f cdb "$BIN_DIR/" 2>/dev/null || return 1
    cp -f cdbmake "$BIN_DIR/" 2>/dev/null || return 1
    chmod +x "$BIN_DIR/cdb" "$BIN_DIR/cdbmake"

    # 清理
    cd "$SCRIPT_DIR"
    rm -rf "$TINYCDB_DIR"

    # 测试
    if "$BIN_DIR/cdb" -h >/dev/null 2>&1 && "$BIN_DIR/cdbmake" -h >/dev/null 2>&1; then
        echo -e "\033[32m✓ TinyCDB编译安装成功\033[0m"
        return 0
    else
        return 1
    fi
}

# 手动安装CDB组件（菜单选项）
install_cdb_component() {
    clear
    echo -e "\033[36m════════════════════════════════════════\033[0m"
    echo -e "\033[36m  安装CDB加速组件\033[0m"
    echo -e "\033[36m════════════════════════════════════════\033[0m"
    echo ""
    echo "CDB (Constant Database) 是一个快速的键值存储库"
    echo "可将查询速度提升 10 倍（0.45秒 → 0.04秒）"
    echo ""

    local TINYCDB_TAR="$SCRIPT_DIR/tinycdb-0.78.tar.gz"

    # 检查源码包
    if [ ! -f "$TINYCDB_TAR" ]; then
        echo -e "\033[31m✗ 错误: 未找到 tinycdb-0.78.tar.gz\033[0m"
        echo ""
        echo "请将 tinycdb-0.78.tar.gz 放到以下目录："
        echo "  $SCRIPT_DIR"
        echo ""
        echo "下载地址："
        echo "  http://www.corpit.ru/mjt/tinycdb/tinycdb-0.78.tar.gz"
        echo ""
        read -r -p "按回车键返回主菜单..."
        return 1
    fi

    echo -e "\033[32m✓ 找到源码包: tinycdb-0.78.tar.gz\033[0m"
    echo ""

    # 检查编译工具
    echo "检查编译环境..."
    if ! command -v gcc >/dev/null 2>&1; then
        echo -e "\033[31m✗ 未找到 gcc 编译器\033[0m"
        echo ""
        echo "请先安装编译工具："
        echo "  CentOS/RHEL: yum groupinstall 'Development Tools'"
        echo "  Ubuntu/Debian: apt-get install build-essential"
        echo ""
        read -r -p "按回车键返回主菜单..."
        return 1
    fi

    if ! command -v make >/dev/null 2>&1; then
        echo -e "\033[31m✗ 未找到 make 工具\033[0m"
        echo ""
        echo "请先安装编译工具："
        echo "  CentOS/RHEL: yum install make"
        echo "  Ubuntu/Debian: apt-get install make"
        echo ""
        read -r -p "按回车键返回主菜单..."
        return 1
    fi

    echo -e "\033[32m✓ gcc: $(gcc --version | head -n1)\033[0m"
    echo -e "\033[32m✓ make: $(make --version | head -n1)\033[0m"
    echo ""

    # 确认安装
    read -r -p "是否开始编译安装? [Y/n]: " confirm
    if [[ ! "$confirm" =~ ^[Yy]?$ ]]; then
        echo "已取消"
        sleep 1
        return 0
    fi

    echo ""
    echo "正在编译安装，请稍候..."
    echo ""

    # 调用自动安装函数
    if auto_install_tinycdb; then
        # 更新全局变量
        HAS_CDB=1
        HAS_CDBMAKE=1
        CDB_CMD="$SCRIPT_DIR/bin/cdb"
        CDBMAKE_CMD="$SCRIPT_DIR/bin/cdbmake"

        echo ""
        echo -e "\033[32m════════════════════════════════════════\033[0m"
        echo -e "\033[32m  ✓ 安装成功！\033[0m"
        echo -e "\033[32m════════════════════════════════════════\033[0m"
        echo ""
        echo "安装位置："
        echo "  $SCRIPT_DIR/bin/cdb"
        echo "  $SCRIPT_DIR/bin/cdbmake"
        echo ""
        echo "下一步："
        echo "  1. 选择菜单 '3. 更新索引' 生成CDB索引"
        echo "  2. 使用 '2. 高级查询' 体验10倍速度提升"
        echo ""
    else
        echo ""
        echo -e "\033[31m════════════════════════════════════════\033[0m"
        echo -e "\033[31m  ✗ 安装失败\033[0m"
        echo -e "\033[31m════════════════════════════════════════\033[0m"
        echo ""
        echo "可能的原因："
        echo "  1. 编译工具版本不兼容"
        echo "  2. 磁盘空间不足"
        echo "  3. 权限不足"
        echo ""
        echo "解决方法："
        echo "  1. 检查编译日志"
        echo "  2. 手动编译："
        echo "     cd $SCRIPT_DIR"
        echo "     tar xzf tinycdb-0.78.tar.gz"
        echo "     cd tinycdb-0.78"
        echo "     make"
        echo "     mkdir -p ../bin"
        echo "     cp cdb cdbmake ../bin/"
        echo ""
    fi

    read -r -p "按回车键返回主菜单..."
}

# 检查项目bin目录下的cdb工具
if [ -x "$SCRIPT_DIR/bin/cdb" ] && [ -x "$SCRIPT_DIR/bin/cdbmake" ]; then
    HAS_CDB=1
    HAS_CDBMAKE=1
    CDB_CMD="$SCRIPT_DIR/bin/cdb"
    CDBMAKE_CMD="$SCRIPT_DIR/bin/cdbmake"
# 检查系统PATH中的cdb工具
elif command -v cdb >/dev/null 2>&1; then
    HAS_CDB=1
    CDB_CMD="cdb"
    if command -v cdbmake >/dev/null 2>&1; then
        HAS_CDBMAKE=1
        CDBMAKE_CMD="cdbmake"
    elif command -v python3 >/dev/null 2>&1 || command -v python >/dev/null 2>&1; then
        # Python可用，可以用Python生成CDB
        HAS_CDBMAKE=2
    fi
# 如果都没有，尝试自动编译安装
else
    if auto_install_tinycdb; then
        HAS_CDB=1
        HAS_CDBMAKE=1
        CDB_CMD="$SCRIPT_DIR/bin/cdb"
        CDBMAKE_CMD="$SCRIPT_DIR/bin/cdbmake"
    fi
fi

# 快捷命令别名
SHORTCUT_NAME="fwlog"

# 初始化数据目录结构
init_data_dirs() {
    mkdir -p "$INDEX_DIR" "$EXPORT_BY_IP" "$EXPORT_BY_PORT" "$EXPORT_BY_DATE" "$EXPORT_BY_QUERY" "$TEMP_DIR" "$BACKUP_DIR" 2>/dev/null
    return 0
}

install_shortcut() {
    local shell_rc=""
    local current_shell=$(basename "$SHELL")
    local wrapper_script="$SCRIPT_DIR/fwlog"

    case "$current_shell" in
        bash)
            shell_rc="$HOME/.bashrc"
            ;;
        zsh)
            shell_rc="$HOME/.zshrc"
            ;;
        *)
            echo -e "\033[33m警告：未识别的Shell ($current_shell)，跳过快捷键安装\033[0m"
            return 1
            ;;
    esac

    # 创建包装脚本
    cat > "$wrapper_script" << 'WRAPPER_EOF'
#!/bin/bash
# 快捷键包装脚本 - 直接调出菜单

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAIN_SCRIPT="$SCRIPT_DIR/sangforfw_log.sh"

# 如果没有参数，直接启动交互式菜单
if [ $# -eq 0 ]; then
    exec "$MAIN_SCRIPT"
else
    # 有参数时传递给主脚本
    exec "$MAIN_SCRIPT" "$@"
fi
WRAPPER_EOF

    chmod +x "$wrapper_script"

    # 检查是否已安装
    if grep -q "alias $SHORTCUT_NAME=" "$shell_rc" 2>/dev/null; then
        echo -e "\033[33m快捷键 '$SHORTCUT_NAME' 已存在，正在更新...\033[0m"
        # 删除旧的
        sed -i.bak "/# 深信服防火墙日志查询工具快捷键/d" "$shell_rc"
        sed -i.bak "/alias $SHORTCUT_NAME=/d" "$shell_rc"
    fi

    # 添加别名
    echo "" >> "$shell_rc"
    echo "# 深信服防火墙日志查询工具快捷键 (自动添加于 $(date '+%Y-%m-%d %H:%M:%S'))" >> "$shell_rc"
    echo "alias $SHORTCUT_NAME='$wrapper_script'" >> "$shell_rc"

    echo -e "\033[32m✓ 快捷键已安装到 $shell_rc\033[0m"
    echo -e "\033[32m✓ 包装脚本已创建: $wrapper_script\033[0m"
    echo ""
    echo -e "\033[33m请运行以下命令使其生效：\033[0m"
    echo -e "  source $shell_rc"
    echo ""
    echo -e "\033[32m使用方法：\033[0m"
    echo -e "  $SHORTCUT_NAME                    # 直接打开交互式菜单"
    echo -e "  $SHORTCUT_NAME 192.168.1.100      # 快速查询IP"
    echo -e "  $SHORTCUT_NAME 20260427 192.168.1.100:443  # 查询IP+端口+日期"
    echo ""
    return 0
}

uninstall_shortcut() {
    local shell_rc=""
    local current_shell=$(basename "$SHELL")
    local wrapper_script="$SCRIPT_DIR/fwlog"

    case "$current_shell" in
        bash) shell_rc="$HOME/.bashrc" ;;
        zsh) shell_rc="$HOME/.zshrc" ;;
        *) return 1 ;;
    esac

    if [ ! -f "$shell_rc" ]; then
        echo -e "\033[31m配置文件不存在: $shell_rc\033[0m"
        return 1
    fi

    # 删除快捷键相关行
    sed -i.bak "/# 深信服防火墙日志查询工具快捷键/d" "$shell_rc"
    sed -i.bak "/alias $SHORTCUT_NAME=/d" "$shell_rc"

    # 删除包装脚本
    if [ -f "$wrapper_script" ]; then
        rm -f "$wrapper_script"
        echo -e "\033[32m✓ 包装脚本已删除\033[0m"
    fi

    echo -e "\033[32m✓ 快捷键已卸载\033[0m"
    return 0
}

check_shortcut_status() {
    local shell_rc=""
    local current_shell=$(basename "$SHELL")

    case "$current_shell" in
        bash) shell_rc="$HOME/.bashrc" ;;
        zsh) shell_rc="$HOME/.zshrc" ;;
        *) return 1 ;;
    esac

    if grep -q "alias $SHORTCUT_NAME=" "$shell_rc" 2>/dev/null; then
        local installed_path=$(grep "alias $SHORTCUT_NAME=" "$shell_rc" | sed "s/alias $SHORTCUT_NAME='\(.*\)'/\1/")
        echo -e "\033[32m快捷键状态：已安装\033[0m"
        echo -e "  命令: $SHORTCUT_NAME"
        echo -e "  指向: $installed_path"
        if [ "$installed_path" != "$SCRIPT_DIR/fwlog" ]; then
            echo -e "\033[33m  警告：指向路径与当前脚本不一致\033[0m"
        fi
        return 0
    else
        echo -e "\033[33m快捷键状态：未安装\033[0m"
        return 1
    fi
}

auto_refresh_shortcut() {
    local shell_rc=""
    local current_shell=$(basename "$SHELL")
    local wrapper_script="$SCRIPT_DIR/fwlog"

    case "$current_shell" in
        bash) shell_rc="$HOME/.bashrc" ;;
        zsh) shell_rc="$HOME/.zshrc" ;;
        *) return 1 ;;
    esac

    # 检查快捷键是否已安装
    if ! grep -q "alias $SHORTCUT_NAME=" "$shell_rc" 2>/dev/null; then
        return 0  # 未安装，不需要刷新
    fi

    # 检查包装脚本是否存在
    if [ ! -f "$wrapper_script" ]; then
        echo -e "\033[33m检测到快捷键配置但包装脚本缺失，正在修复...\033[0m"
        install_shortcut >/dev/null 2>&1
        echo -e "\033[32m✓ 快捷键已修复\033[0m"
        echo -e "\033[33m提示：请运行 'source $shell_rc' 使快捷键生效\033[0m"
        echo ""
        return 0
    fi

    # 检查包装脚本是否可执行
    if [ ! -x "$wrapper_script" ]; then
        chmod +x "$wrapper_script" 2>/dev/null
    fi

    # 检查快捷键路径是否正确
    local installed_path=$(grep "alias $SHORTCUT_NAME=" "$shell_rc" | sed "s/alias $SHORTCUT_NAME='\(.*\)'/\1/")
    if [ "$installed_path" != "$wrapper_script" ]; then
        echo -e "\033[33m检测到快捷键路径不一致，正在更新...\033[0m"
        # 删除旧的
        sed -i.bak "/# 深信服防火墙日志查询工具快捷键/d" "$shell_rc"
        sed -i.bak "/alias $SHORTCUT_NAME=/d" "$shell_rc"
        # 添加新的
        echo "" >> "$shell_rc"
        echo "# 深信服防火墙日志查询工具快捷键 (自动更新于 $(date '+%Y-%m-%d %H:%M:%S'))" >> "$shell_rc"
        echo "alias $SHORTCUT_NAME='$wrapper_script'" >> "$shell_rc"
        echo -e "\033[32m✓ 快捷键路径已更新\033[0m"
        echo -e "\033[33m提示：请运行 'source $shell_rc' 使更新生效\033[0m"
        echo ""
    fi

    return 0
}

list_log_files() {
    find "$LOG_DIR" -maxdepth 1 -type f -name "*.log" 2>/dev/null | sort
}

escape_ere() {
    printf '%s' "$1" | sed 's/[][(){}.^$+*?|\\]/\\&/g'
}

build_ip_pattern() {
    local escaped_ip
    escaped_ip=$(escape_ere "$1")
    printf '(^|[^0-9.])%s([^0-9.]|$)' "$escaped_ip"
}

build_port_pattern() {
    printf '(源端口|目的端口|目标端口):%s([,[:space:]]|$)' "$1"
}

get_log_date_from_path() {
    basename "$1" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | head -1 | tr -d '-'
}

extract_log_fields() {
    local log_file="$1"
    local date="$2"

    # 设置LC_ALL=C避免多字节字符警告
    LC_ALL=C awk -v date="$date" -v file="$log_file" '
    {
        # 提取IP地址
        ips = ""
        while (match($0, /[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}/)) {
            ip = substr($0, RSTART, RLENGTH)
            if (ips == "") ips = ip
            else if (index(ips, ip) == 0) ips = ips "," ip
            $0 = substr($0, RSTART + RLENGTH)
        }

        # 提取端口
        ports = ""
        if (match($0, /(源端口|目的端口|目标端口):([0-9]{1,5})/)) {
            port = substr($0, RSTART, RLENGTH)
            gsub(/.*:/, "", port)
            ports = port
        }

        # 提取协议
        proto = ""
        if (match($0, /协议:([0-9]+)/)) {
            proto = substr($0, RSTART, RLENGTH)
            gsub(/.*:/, "", proto)
        }

        # 提取动作
        action = ""
        if (match($0, /(允许|拒绝|丢弃|阻断)/)) {
            action = substr($0, RSTART, RLENGTH)
        }

        # 输出索引记录：IP|日期|文件|端口|协议|动作
        if (ips != "") {
            split(ips, ip_array, ",")
            for (i in ip_array) {
                printf "%s|%s|%s|%s|%s|%s\n", ip_array[i], date, file, ports, proto, action
            }
        }
    }
    ' "$log_file" 2>/dev/null | sort -u
}

check_parallel_support() {
    if command -v parallel >/dev/null 2>&1; then
        echo "gnu-parallel"
    elif command -v xargs >/dev/null 2>&1; then
        # 检查xargs是否支持-P参数
        if echo | xargs -P 2 echo 2>/dev/null; then
            echo "xargs"
        else
            echo "none"
        fi
    else
        echo "none"
    fi
}

show_progress_bar() {
    local current=$1
    local total=$2
    local width=50
    local percentage=$((current * 100 / total))
    local completed=$((width * current / total))
    local remaining=$((width - completed))

    # 构建进度条
    printf "\r["
    printf "%${completed}s" | tr ' ' '='
    printf "%${remaining}s" | tr ' ' '-'
    printf "] %3d%% (%d/%d)" "$percentage" "$current" "$total"
}

clear_progress_bar() {
    printf "\r%80s\r" " "
}

write_file_state_snapshot() {
    local output_file="$1"
    local file

    : > "$output_file"
    while IFS= read -r file; do
        [ -n "$file" ] || continue
        stat -c '%n|%Y|%s|%s' "$file" >> "$output_file"
    done < <(list_log_files)
}

count_items_in_file() {
    awk 'NF {count++} END {print count + 0}' "$1"
}

sanitize_export_name() {
    printf '%s' "$1" | sed 's/[^0-9A-Za-z._-]/_/g'
}

export_result_file() {
    local source_file="$1"
    local query_key="$2"
    local query_desc="$3"
    local export_time export_time_display safe_query_key target_file line_count byte_count target_dir

    query_key=${query_key:-unknown}
    query_desc=${query_desc:-查询}

    if [ ! -s "$source_file" ]; then
        echo -e "\033[31m结果文件为空，无法导出\033[0m"
        return 1
    fi

    # 根据查询类型确定目标目录
    if echo "$query_desc" | grep -q "类型=单IP查询"; then
        target_dir="$EXPORT_BY_IP"
    elif echo "$query_desc" | grep -q "类型=双IP查询"; then
        target_dir="$EXPORT_BY_IP"
    elif echo "$query_desc" | grep -q "类型=端口查询"; then
        target_dir="$EXPORT_BY_PORT"
    elif echo "$query_desc" | grep -q "类型=IP端口组合"; then
        target_dir="$EXPORT_BY_QUERY"
    elif echo "$query_desc" | grep -q "类型=日期查询"; then
        target_dir="$EXPORT_BY_DATE"
    elif echo "$query_desc" | grep -q "类型=时间范围"; then
        target_dir="$EXPORT_BY_DATE"
    elif echo "$query_desc" | grep -q "类型=组合查询"; then
        target_dir="$EXPORT_BY_QUERY"
    else
        target_dir="$EXPORT_DIR"
    fi

    if ! mkdir -p "$target_dir" 2>/dev/null; then
        echo -e "\033[31m无法创建导出目录 $target_dir\033[0m"
        return 1
    fi

    export_time=$(date '+%Y%m%d_%H%M%S')
    export_time_display=$(date '+%Y-%m-%d %H:%M:%S')
    safe_query_key=$(sanitize_export_name "$query_key")
    target_file="$target_dir/${safe_query_key}_${export_time}.log"

    {
        printf '导出时间: %s\n' "$export_time_display"
        printf '查询IP/端口: %s\n' "$query_key"
        printf '查询条件: %s\n' "$query_desc"
        printf '\n'
        cat "$source_file"
    } > "$target_file" || {
        echo -e "\033[31m写入导出文件失败 $target_file\033[0m"
        return 1
    }

    line_count=$(wc -l < "$target_file")
    byte_count=$(wc -c < "$target_file")
    if [ "$byte_count" -le 0 ]; then
        echo -e "\033[31m导出文件（$target_file）大小为零\033[0m"
        return 1
    fi

    echo -e "\033[32m结果已导出到：$target_file（$line_count 行，$byte_count 字节）\033[0m"
    return 0
}

sort_result_file() {
    local input_file="$1"
    local output_file="$2"

    if [ ! -s "$input_file" ]; then
        : > "$output_file"
        return 0
    fi

    awk -F: '
        {
            file = $1
            line = $2 + 0
            date = "99999999"
            if (match(file, /[0-9]{4}-[0-9]{2}-[0-9]{2}/)) {
                date = substr(file, RSTART, RLENGTH)
                gsub("-", "", date)
            }
            printf "%s\t%s\t%09d\t%s\n", date, file, line, $0
        }
    ' "$input_file" | sort -k1,1 -k2,2 -k3,3n | cut -f4- > "$output_file"
}

normalize_date() {
    local raw="$1"
    local cleaned
    cleaned=$(printf '%s' "$raw" | tr -d '-')

    case ${#cleaned} in
        4)
            printf '%s0101|%s1231\n' "$cleaned" "$cleaned"
            ;;
        6)
            printf '%s01|%s31\n' "$cleaned" "$cleaned"
            ;;
        8)
            printf '%s|%s\n' "$cleaned" "$cleaned"
            ;;
        *)
            return 1
            ;;
    esac
}

find_files_by_date_range() {
    local start_date="$1"
    local end_date="$2"
    local file file_date

    while IFS= read -r file; do
        [ -n "$file" ] || continue
        file_date=$(get_log_date_from_path "$file")
        if [ -n "$file_date" ] && [ "$file_date" -ge "$start_date" ] && [ "$file_date" -le "$end_date" ]; then
            printf '%s\n' "$file"
        fi
    done < <(list_log_files)
}

run_file_query() {
    local query_ip="$1"
    local query_port="$2"
    local output_file="$3"
    local file_list="$4"
    local log_file ip_pattern port_pattern

    ip_pattern=$(build_ip_pattern "$query_ip")
    : > "$output_file"

    if [ -n "$query_port" ] && ! printf '%s' "$query_port" | grep -Eq '^[0-9]{1,5}$'; then
        echo "端口格式错误"
        return 1
    fi

    if [ -n "$query_port" ]; then
        port_pattern=$(build_port_pattern "$query_port")
    fi

    # 优化：使用LC_ALL=C加速grep
    export LC_ALL=C
    if [ -n "$query_port" ]; then
        # 使用8个并行进程
        cat "$file_list" | xargs -P 8 -I {} sh -c "grep -EHn '$ip_pattern' {} 2>/dev/null | grep -E '$port_pattern'" >> "$output_file"
    else
        # 直接传递所有文件给grep，避免子shell开销
        if [ -s "$file_list" ]; then
            xargs -a "$file_list" grep -EHn "$ip_pattern" 2>/dev/null >> "$output_file"
        fi
    fi
    unset LC_ALL
}

append_index_entries_for_files() {
    local file_list="$1"
    local output_file="$2"
    local LOG_FILE FILENAME DATE IP

    while IFS= read -r LOG_FILE; do
        [ -n "$LOG_FILE" ] || continue
        [ -f "$LOG_FILE" ] || continue
        FILENAME=$(basename "$LOG_FILE")
        DATE=$(printf '%s' "$FILENAME" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | head -1 | tr -d '-')

        while IFS= read -r IP; do
            [ -n "$IP" ] || continue
            printf '%s|%s|%s\n' "$IP" "$DATE" "$LOG_FILE" >> "$output_file"
        done < <(grep -oE '[0-9]{1,3}(\.[0-9]{1,3}){3}' "$LOG_FILE" 2>/dev/null | sort -u)
    done < "$file_list"
}

# ============================================
# CDB索引功能
# ============================================

generate_cdb_index() {
    local silent_mode="${1:-0}"

    if [ ! -f "$INDEX_FILE" ]; then
        [ "$silent_mode" -ne 1 ] && echo "错误: 索引文件不存在，请先建立索引"
        return 1
    fi

    if [ "$HAS_CDBMAKE" -eq 0 ]; then
        [ "$silent_mode" -ne 1 ] && echo "警告: 无cdbmake或python，跳过CDB索引生成"
        return 1
    fi

    local CDB_TMP="${INDEX_FILE_CDB}.tmp"
    local CDB_INPUT="${INDEX_FILE_CDB}.input"

    [ "$silent_mode" -ne 1 ] && echo "正在生成CDB索引..."

    # 方式1: 使用cdbmake（如果可用）
    if [ "$HAS_CDBMAKE" -eq 1 ]; then
        # 从文本索引生成CDB输入格式
        # 格式: +klen,dlen:key->data
        awk -F'|' '{
            key = $1 "|" $2  # IP|日期
            value = $3       # 文件路径
            printf "+%d,%d:%s->%s\n", length(key), length(value), key, value
        }' "$INDEX_FILE" > "$CDB_INPUT"

        # 添加结束标记
        echo "" >> "$CDB_INPUT"

        # 使用cdbmake生成CDB文件
        if $CDBMAKE_CMD "$CDB_TMP" "${CDB_TMP}.tmp" < "$CDB_INPUT" 2>/dev/null; then
            mv "$CDB_TMP" "$INDEX_FILE_CDB"
            rm -f "${CDB_TMP}.tmp" "$CDB_INPUT"

            if [ "$silent_mode" -ne 1 ]; then
                local cdb_size=$(du -h "$INDEX_FILE_CDB" 2>/dev/null | cut -f1)
                local txt_size=$(du -h "$INDEX_FILE" 2>/dev/null | cut -f1)
                echo -e "\033[32m✓ CDB索引生成成功 (cdbmake)\033[0m"
                echo "  文本索引: $txt_size"
                echo "  CDB索引: $cdb_size"
            fi
            return 0
        else
            rm -f "$CDB_TMP" "${CDB_TMP}.tmp" "$CDB_INPUT"
            [ "$silent_mode" -ne 1 ] && echo -e "\033[33m警告: CDB索引生成失败\033[0m"
            return 1
        fi

    # 方式2: 使用Python生成CDB（如果cdbmake不可用）
    elif [ "$HAS_CDBMAKE" -eq 2 ]; then
        local PYTHON_CMD=""
        if command -v python3 >/dev/null 2>&1; then
            PYTHON_CMD="python3"
        elif command -v python >/dev/null 2>&1; then
            PYTHON_CMD="python"
        fi

        # 使用Python生成CDB
        $PYTHON_CMD - "$INDEX_FILE" "$CDB_TMP" << 'PYTHON_EOF'
import sys
import struct
import hashlib

def cdb_hash(key):
    """CDB hash function"""
    h = 5381
    for c in key:
        h = ((h << 5) + h) ^ ord(c)
    return h & 0xffffffff

def make_cdb(input_file, output_file):
    """Generate CDB file from text index"""
    # Read all records
    records = []
    with open(input_file, 'r') as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            parts = line.split('|')
            if len(parts) >= 3:
                key = parts[0] + '|' + parts[1]  # IP|日期
                value = parts[2]  # 文件路径
                records.append((key.encode(), value.encode()))

    # Build hash table
    tables = [[] for _ in range(256)]
    for key, value in records:
        h = cdb_hash(key.decode())
        tables[h % 256].append((h, key, value))

    # Write CDB file
    with open(output_file, 'wb') as f:
        # Reserve space for header (256 * 8 bytes)
        header_pos = f.tell()
        f.write(b'\x00' * 2048)

        # Write hash tables and build header
        header = []
        for table in tables:
            table_pos = f.tell()
            table_len = len(table) * 2

            if table_len == 0:
                header.append((0, 0))
                continue

            # Build hash table
            slots = [None] * table_len
            for h, key, value in table:
                slot = (h >> 8) % table_len
                while slots[slot] is not None:
                    slot = (slot + 1) % table_len
                slots[slot] = (h, key, value)

            # Write hash table
            for slot in slots:
                if slot is None:
                    f.write(struct.pack('<II', 0, 0))
                else:
                    h, key, value = slot
                    # Write hash and position
                    data_pos = f.tell() + table_len * 8 - (slots.index(slot) * 8)
                    f.write(struct.pack('<II', h, data_pos))

            # Write data
            for slot in slots:
                if slot is not None:
                    h, key, value = slot
                    f.write(struct.pack('<II', len(key), len(value)))
                    f.write(key)
                    f.write(value)

            header.append((table_pos, table_len))

        # Write header
        f.seek(header_pos)
        for pos, length in header:
            f.write(struct.pack('<II', pos, length))

if __name__ == '__main__':
    if len(sys.argv) != 3:
        sys.exit(1)
    try:
        make_cdb(sys.argv[1], sys.argv[2])
        sys.exit(0)
    except Exception as e:
        print("Error:", str(e), file=sys.stderr)
        sys.exit(1)
PYTHON_EOF

        if [ $? -eq 0 ] && [ -f "$CDB_TMP" ]; then
            mv "$CDB_TMP" "$INDEX_FILE_CDB"

            if [ "$silent_mode" -ne 1 ]; then
                local cdb_size=$(du -h "$INDEX_FILE_CDB" 2>/dev/null | cut -f1)
                local txt_size=$(du -h "$INDEX_FILE" 2>/dev/null | cut -f1)
                echo -e "\033[32m✓ CDB索引生成成功 (python)\033[0m"
                echo "  文本索引: $txt_size"
                echo "  CDB索引: $cdb_size"
            fi
            return 0
        else
            rm -f "$CDB_TMP"
            [ "$silent_mode" -ne 1 ] && echo -e "\033[33m警告: CDB索引生成失败\033[0m"
            return 1
        fi
    fi

    return 1
}

query_cdb_index() {
    local query_ip="$1"
    local query_date="$2"

    if [ ! -f "$INDEX_FILE_CDB" ]; then
        return 1
    fi

    # 构造CDB查询键
    local cdb_key="${query_ip}|${query_date}"

    # 使用cdb查询
    $CDB_CMD -q "$INDEX_FILE_CDB" "$cdb_key" 2>/dev/null
    return $?
}

fast_query_with_cdb() {
    local query_ip="$1"
    local query_date="$2"
    local output_file="$3"

    if [ "$HAS_CDB" -eq 0 ] || [ ! -f "$INDEX_FILE_CDB" ]; then
        return 1
    fi

    # 转换日期格式 YYYY-MM-DD -> YYYYMMDD
    local date_key=$(echo "$query_date" | tr -d '-')

    # CDB查询获取文件路径
    local log_files=$(query_cdb_index "$query_ip" "$date_key")

    if [ -z "$log_files" ]; then
        return 1
    fi

    # 从日志文件中提取匹配记录
    local ip_pattern=$(build_ip_pattern "$query_ip")
    export LC_ALL=C

    echo "$log_files" | while IFS= read -r log_file; do
        [ -f "$log_file" ] && grep -EHn "$ip_pattern" "$log_file" 2>/dev/null
    done >> "$output_file"

    unset LC_ALL
    return 0
}


auto_update_index() {
    local silent_mode="${1:-0}"
    local CURRENT_FILES CURRENT_COUNT PROCESSED LOG_FILE FILENAME DATE INDEX_COUNT
    local CURRENT_META_TMP CHANGED_FILES_TMP REMOVED_FILES_TMP REBUILD_FILES_TMP FILTERED_INDEX_TMP
    local CHANGED_COUNT REMOVED_COUNT TOTAL_CHANGED TEMP_INDEX
    local current_size old_record old_offset

    if [ ! -d "$LOG_DIR" ]; then
        echo -e "\033[31m日志目录 $LOG_DIR 不存在\033[0m"
        return 1
    fi

    CURRENT_FILES=$(list_log_files)
    CURRENT_COUNT=$(printf '%s\n' "$CURRENT_FILES" | grep -c .)

    if [ "$CURRENT_COUNT" -eq 0 ]; then
        echo -e "\033[31m日志目录中无日志文件\033[0m"
        return 1
    fi

    # 首次建立索引
    if [ ! -f "$INDEX_FILE" ]; then
        echo -e "\033[33m首次运行，正在建立索引...\033[0m"
        echo "日志文件数：$CURRENT_COUNT 个"
        echo "预计耗时：$(($CURRENT_COUNT / 10)) - $(($CURRENT_COUNT / 2)) 分钟"

        PARALLEL_TYPE=$(check_parallel_support)
        if [ "$PARALLEL_TYPE" != "none" ]; then
            echo "并行模式：$PARALLEL_TYPE (使用 $PARALLEL_JOBS 个进程)"
        fi
        echo ""

        : > "$INDEX_FILE"
        PROCESSED=0

        # 定义单文件索引函数供并行调用
        export -f extract_log_fields get_log_date_from_path
        export LOG_DIR

        if [ "$PARALLEL_TYPE" = "gnu-parallel" ]; then
            # 使用GNU parallel
            printf '%s\n' "$CURRENT_FILES" | parallel -j "$PARALLEL_JOBS" --line-buffer '
                FILENAME=$(basename {})
                DATE=$(printf "%s" "$FILENAME" | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}" | head -1 | tr -d "-")
                extract_log_fields {} "$DATE"
            ' > "$INDEX_FILE"
            PROCESSED=$CURRENT_COUNT
        elif [ "$PARALLEL_TYPE" = "xargs" ]; then
            # 使用xargs -P
            printf '%s\n' "$CURRENT_FILES" | xargs -P "$PARALLEL_JOBS" -I {} bash -c '
                FILENAME=$(basename "{}")
                DATE=$(printf "%s" "$FILENAME" | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}" | head -1 | tr -d "-")
                extract_log_fields "{}" "$DATE"
            ' > "$INDEX_FILE"
            PROCESSED=$CURRENT_COUNT
        else
            # 串行处理（带进度条）
            echo "正在建立索引..."
            while IFS= read -r LOG_FILE; do
                [ -n "$LOG_FILE" ] || continue
                PROCESSED=$((PROCESSED + 1))

                FILENAME=$(basename "$LOG_FILE")
                DATE=$(printf '%s' "$FILENAME" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | head -1 | tr -d '-')
                extract_log_fields "$LOG_FILE" "$DATE" >> "$INDEX_FILE"

                # 显示进度条
                show_progress_bar "$PROCESSED" "$CURRENT_COUNT"
            done < <(printf '%s\n' "$CURRENT_FILES")
            clear_progress_bar
            echo "索引建立完成"
        fi

        sort -u "$INDEX_FILE" -o "$INDEX_FILE"
        write_file_state_snapshot "$INDEX_META"

        INDEX_COUNT=$(wc -l < "$INDEX_FILE")
        echo ""
        echo -e "\033[32m索引建立完成，共索引 $INDEX_COUNT 条\033[0m"
        echo ""
        return 0
    fi

    if [ ! -f "$INDEX_META" ]; then
        rm -f "$INDEX_FILE"
        auto_update_index "$silent_mode"
        return $?
    fi

    # ??????3????????4???
    if ! awk -F'|' 'NF != 3 && NF != 4 {exit 1}' "$INDEX_META"; then
        rm -f "$INDEX_FILE" "$INDEX_META"
        auto_update_index "$silent_mode"
        return $?
    fi

    CURRENT_META_TMP="/tmp/fw_current_meta_$$.tmp"
    CHANGED_FILES_TMP="/tmp/fw_changed_files_$$.tmp"
    REMOVED_FILES_TMP="/tmp/fw_removed_files_$$.tmp"
    REBUILD_FILES_TMP="/tmp/fw_rebuild_files_$$.tmp"
    FILTERED_INDEX_TMP="/tmp/fw_filtered_index_$$.tmp"
    TEMP_INDEX="/tmp/fw_index_new_$$.db"

    write_file_state_snapshot "$CURRENT_META_TMP"

    awk -F'|' '
        NR==FNR { old[$1] = $2 "|" $3; next }
        {
            current = $2 "|" $3
            if (!( $1 in old ) || old[$1] != current) {
                print $1
            }
        }
    ' "$INDEX_META" "$CURRENT_META_TMP" | sort -u > "$CHANGED_FILES_TMP"

    awk -F'|' '
        NR==FNR { current[$1] = 1; next }
        !($1 in current) { print $1 }
    ' "$CURRENT_META_TMP" "$INDEX_META" | sort -u > "$REMOVED_FILES_TMP"

    cat "$CHANGED_FILES_TMP" "$REMOVED_FILES_TMP" | sed '/^$/d' | sort -u > "$REBUILD_FILES_TMP"
    CHANGED_COUNT=$(count_items_in_file "$CHANGED_FILES_TMP")
    REMOVED_COUNT=$(count_items_in_file "$REMOVED_FILES_TMP")
    TOTAL_CHANGED=$((CHANGED_COUNT + REMOVED_COUNT))

    if [ "$CHANGED_COUNT" -eq 0 ] && [ "$REMOVED_COUNT" -eq 0 ]; then
        mv "$CURRENT_META_TMP" "$INDEX_META"
        rm -f "$CHANGED_FILES_TMP" "$REMOVED_FILES_TMP" "$REBUILD_FILES_TMP" "$FILTERED_INDEX_TMP" "$TEMP_INDEX"
        return 0
    fi

    if [ "$silent_mode" -ne 1 ] && [ "$TOTAL_CHANGED" -gt 1 ]; then
        echo -e "\033[33m检测到文件变更/删除 $CHANGED_COUNT 个，已删除 $REMOVED_COUNT 个，正在更新索引...\033[0m"
    fi

    # 只删除已删除文件和被截断文件的索引
    TRUNCATED_FILES_TMP="/tmp/fw_truncated_files_$$.tmp"
    : > "$TRUNCATED_FILES_TMP"

    if [ -s "$CHANGED_FILES_TMP" ]; then
        while IFS= read -r LOG_FILE; do
            [ -n "$LOG_FILE" ] || continue
            [ -f "$LOG_FILE" ] || continue
            current_size=$(stat -c %s "$LOG_FILE" 2>/dev/null || echo 0)
            old_record=$(grep -F "$LOG_FILE|" "$INDEX_META" 2>/dev/null | head -1)
            if [ -n "$old_record" ]; then
                old_offset=$(echo "$old_record" | cut -d'|' -f4)
                old_offset=${old_offset:-0}
                if [ "$current_size" -lt "$old_offset" ]; then
                    echo "$LOG_FILE" >> "$TRUNCATED_FILES_TMP"
                fi
            fi
        done < "$CHANGED_FILES_TMP"
    fi

    # 合并需要删除索引的文件列表（已删除的文件 + 被截断的文件）
    cat "$REMOVED_FILES_TMP" "$TRUNCATED_FILES_TMP" | sed '/^$/d' | sort -u > "$REBUILD_FILES_TMP"

    if [ -s "$REBUILD_FILES_TMP" ] && [ -f "$INDEX_FILE" ]; then
        awk -F'|' '
            NR==FNR { skip[$0] = 1; next }
            !($3 in skip) { print $0 }
        ' "$REBUILD_FILES_TMP" "$INDEX_FILE" > "$FILTERED_INDEX_TMP"
        mv "$FILTERED_INDEX_TMP" "$INDEX_FILE"
    fi

    : > "$TEMP_INDEX"

    # 增量更新变更的文件
    PROCESSED=0
    if [ -s "$CHANGED_FILES_TMP" ]; then
        if [ "$silent_mode" -ne 1 ]; then
            echo "正在更新索引..."
        fi
        while IFS= read -r LOG_FILE; do
            PROCESSED=$((PROCESSED + 1))
            if [ "$silent_mode" -ne 1 ]; then
                show_progress_bar "$PROCESSED" "$CHANGED_COUNT"
            fi
            [ -n "$LOG_FILE" ] || continue
            [ -f "$LOG_FILE" ] || continue

            current_size=$(stat -c %s "$LOG_FILE" 2>/dev/null || echo 0)
            old_record=$(grep -F "$LOG_FILE|" "$INDEX_META" 2>/dev/null | head -1)

            FILENAME=$(basename "$LOG_FILE")
            DATE=$(printf '%s' "$FILENAME" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | head -1 | tr -d '-')

            if [ -n "$old_record" ]; then
                old_offset=$(echo "$old_record" | cut -d'|' -f4)
                old_offset=${old_offset:-0}

                if [ "$current_size" -lt "$old_offset" ]; then
                    # 文件被截断，重新全量索引
                    extract_log_fields "$LOG_FILE" "$DATE" >> "$TEMP_INDEX"
                elif [ "$current_size" -gt "$old_offset" ]; then
                    # 文件增长，只索引新增部分（旧索引已保留）
                    tail -c +$((old_offset + 1)) "$LOG_FILE" 2>/dev/null | \
                        LC_ALL=C awk -v date="$DATE" -v file="$LOG_FILE" '
                        {
                            ips = ""
                            while (match($0, /[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}/)) {
                                ip = substr($0, RSTART, RLENGTH)
                                if (ips == "") ips = ip
                                else if (index(ips, ip) == 0) ips = ips "," ip
                                $0 = substr($0, RSTART + RLENGTH)
                            }
                            ports = ""
                            if (match($0, /(源端口|目的端口|目标端口):([0-9]{1,5})/)) {
                                port = substr($0, RSTART, RLENGTH)
                                gsub(/.*:/, "", port)
                                ports = port
                            }
                            proto = ""
                            if (match($0, /协议:([0-9]+)/)) {
                                proto = substr($0, RSTART, RLENGTH)
                                gsub(/.*:/, "", proto)
                            }
                            action = ""
                            if (match($0, /(允许|拒绝|丢弃|阻断)/)) {
                                action = substr($0, RSTART, RLENGTH)
                            }
                            if (ips != "") {
                                split(ips, ip_array, ",")
                                for (i in ip_array) {
                                    printf "%s|%s|%s|%s|%s|%s\n", ip_array[i], date, file, ports, proto, action
                                }
                            }
                        }
                        ' 2>/dev/null | sort -u >> "$TEMP_INDEX"
                fi
            else
                # 新增文件，全量索引
                extract_log_fields "$LOG_FILE" "$DATE" >> "$TEMP_INDEX"
            fi
        done < "$CHANGED_FILES_TMP"
        if [ "$silent_mode" -ne 1 ]; then
            clear_progress_bar
            echo "索引更新完成"
        fi
    fi

    cat "$TEMP_INDEX" >> "$INDEX_FILE"
    sort -u "$INDEX_FILE" -o "$INDEX_FILE"
    mv "$CURRENT_META_TMP" "$INDEX_META"

    # 压缩索引文件
    if command -v gzip >/dev/null 2>&1; then
        INDEX_SIZE_BEFORE=$(wc -c < "$INDEX_FILE")
        gzip -c "$INDEX_FILE" > "$INDEX_FILE_GZ" 2>/dev/null
        if [ -f "$INDEX_FILE_GZ" ]; then
            INDEX_SIZE_AFTER=$(wc -c < "$INDEX_FILE_GZ")
            COMPRESS_RATIO=$((100 - INDEX_SIZE_AFTER * 100 / INDEX_SIZE_BEFORE))
            if [ "$silent_mode" -ne 1 ] && [ "$TOTAL_CHANGED" -gt 1 ]; then
                echo "索引压缩：$INDEX_SIZE_BEFORE 字节 -> $INDEX_SIZE_AFTER 字节 (压缩率 ${COMPRESS_RATIO}%)"
            fi
        fi
    fi

    rm -f "$TEMP_INDEX" "$CHANGED_FILES_TMP" "$REMOVED_FILES_TMP" "$REBUILD_FILES_TMP" "$FILTERED_INDEX_TMP" "$TRUNCATED_FILES_TMP"

    # 生成CDB索引（如果tinycdb可用）
    if [ $HAS_CDB -eq 1 ] && [ $HAS_CDBMAKE -ge 1 ]; then
        if [ "$silent_mode" -ne 1 ] && [ "$TOTAL_CHANGED" -gt 1 ]; then
            echo "正在生成CDB索引..."
        fi
        generate_cdb_index "$silent_mode"
    fi

    if [ "$silent_mode" -ne 1 ] && [ "$TOTAL_CHANGED" -gt 1 ]; then
        echo -e "\033[32m索引更新完成\033[0m"
        echo ""
    fi
    return 0
}

ensure_index_ready() {
    if [ "$INDEX_READY_IN_SESSION" -eq 1 ]; then
        return 0
    fi

    auto_update_index 1
    if [ $? -eq 0 ]; then
        INDEX_READY_IN_SESSION=1
        return 0
    else
        return 1
    fi
}

show_main_menu() {
    clear
    echo -e "\033[32m═══════════════════════════════════════\033[0m"
    echo -e "\033[32m  防火墙日志查询工具 v2.1\033[0m"
    echo -e "\033[32m  © 2026 QJKJ Team\033[0m"
    echo -e "\033[32m═══════════════════════════════════════\033[0m"
    echo ""

    # 系统状态信息
    if [ -f "$INDEX_FILE" ]; then
        INDEX_COUNT=$(wc -l < "$INDEX_FILE")
        LOG_COUNT=$(list_log_files | grep -c .)
        echo -e "  索引: \033[32m$INDEX_COUNT 条\033[0m | 日志: $LOG_COUNT 个"

        # CDB状态显示
        if [ "$HAS_CDB" -eq 1 ]; then
            if [ -f "$INDEX_FILE_CDB" ]; then
                echo -e "  CDB: \033[32m✓ 已启用\033[0m (快速查询)"
            else
                echo -e "  CDB: \033[33m○ 可用但未生成\033[0m"
            fi
        else
            echo -e "  CDB: \033[90m✗ 未安装\033[0m (tinycdb)"
        fi
    else
        echo -e "  索引: \033[33m未建立\033[0m"
    fi
    echo ""

    # 功能菜单
    echo -e "  1. 查询IP/端口"
    echo -e "  2. 高级查询"
    echo -e "  3. 更新索引"
    echo -e "  4. 快捷键管理"
    echo -e "  5. 清理文件"

    # 如果CDB未安装，显示安装选项
    if [ "$HAS_CDB" -eq 0 ]; then
        echo -e "  6. 安装CDB加速组件"
    fi

    echo -e "  0. 退出"
    echo ""

    if [ "$HAS_CDB" -eq 0 ]; then
        read -r -p "选择 [0-6]: " MAIN_CHOICE
    else
        read -r -p "选择 [0-5]: " MAIN_CHOICE
    fi
}

run_cli_query() {
    local date_arg=""
    local target_arg=""
    local query_ip=""
    local query_port=""
    local start_date=""
    local end_date=""

    # 命令行模式：跳过索引检查，直接使用现有索引
    INDEX_READY_IN_SESSION=1
    local normalized=""
    local temp_files temp_raw temp_result file_count result_count

    if [ $# -eq 1 ]; then
        target_arg="$1"
    elif [ $# -eq 2 ]; then
        date_arg="$1"
        target_arg="$2"
    else
        echo "用法："
        echo "  $0 IP"
        echo "  $0 日期 IP"
        echo "  $0 日期 IP:端口"
        echo "示例："
        echo "  $0 2.55.81.117"
        echo "  $0 20260424 2.55.81.117"
        echo "  $0 20260424 2.55.81.117:51337"
        echo "  $0 2026-04-24 2.55.81.117:51337"
        return 1
    fi

    if [[ "$target_arg" == *:* ]]; then
        query_ip="${target_arg%%:*}"
        query_port="${target_arg##*:}"
    else
        query_ip="$target_arg"
    fi

    if [ -z "$query_ip" ]; then
        echo "错误：IP地址为空"
        return 1
    fi

    if [ ! -d "$LOG_DIR" ]; then
        echo "日志目录 $LOG_DIR 不存在"
        return 1
    fi

    if [ -n "$date_arg" ]; then
        normalized=$(normalize_date "$date_arg") || {
            echo "日期格式错误，支持格式： YYYY / YYYYMM / YYYYMMDD / YYYY-MM / YYYY-MM-DD"
            return 1
        }
        start_date="${normalized%%|*}"
        end_date="${normalized##*|}"
    fi

    temp_files="/tmp/fw_cli_files_$$.txt"
    temp_raw="/tmp/fw_cli_raw_$$.log"
    temp_result="/tmp/fw_cli_result_$$.log"

    # 使用索引查询（如果索引存在）
    if [ -f "$INDEX_FILE" ]; then
        # 使用索引快速定位文件
        awk -F'|' -v ip="$query_ip" -v port="$query_port" \
            -v start="$start_date" -v end="$end_date" '
            {
                # 字段：IP|日期|文件|端口|协议|动作
                if ($1 != ip) next
                if (start != "" && ($2 < start || $2 > end)) next
                if (port != "" && index($4, port) == 0) next
                print $3
            }
        ' "$INDEX_FILE" | sort -u > "$temp_files"
    else
        # 索引不存在，使用传统方式
        if [ -n "$date_arg" ]; then
            find_files_by_date_range "$start_date" "$end_date" > "$temp_files"
        else
            list_log_files > "$temp_files"
        fi
    fi

    file_count=$(count_items_in_file "$temp_files")
    if [ "$file_count" -eq 0 ]; then
        echo "未找到符合条件的日志文件"
        rm -f "$temp_files" "$temp_raw" "$temp_result"
        return 1
    fi

    run_file_query "$query_ip" "$query_port" "$temp_raw" "$temp_files" || {
        rm -f "$temp_files" "$temp_raw" "$temp_result"
        return 1
    }
    sort_result_file "$temp_raw" "$temp_result"
    result_count=$(count_items_in_file "$temp_result")

    echo "=========================================="
    echo "查询条件："
    [ -n "$date_arg" ] && echo "  日期：$date_arg"
    echo "  IP：$query_ip"
    [ -n "$query_port" ] && echo "  端口：$query_port"
    echo "=========================================="

    if [ "$result_count" -eq 0 ]; then
        echo "未找到匹配记录"
        rm -f "$temp_files" "$temp_raw" "$temp_result"
        return 1
    fi

    echo "找到 $result_count 条匹配记录："
    echo ""
    cat "$temp_result"
    echo ""
    export_result_file "$temp_result" "$query_ip" "类型=单IP查询; 日期=${date_arg:-全部}; IP=$query_ip; 端口=${query_port:-全部}"

    rm -f "$temp_files" "$temp_raw" "$temp_result"
    return 0
}

fast_query() {
    local INDEX_COUNT MODE QUERY_IP QUERY_PORT QUERY_PROTO START_DATE END_DATE START_DATE_FULL END_DATE_FULL
    local normalized_start normalized_end TEMP_FILES TEMP_RAW TEMP_RESULT FILE_COUNT RESULT_COUNT

    clear
    echo -e "\033[32m=============================================\033[0m"
    echo -e "\033[32m   快速查询（增强版）\033[0m"
    echo -e "\033[32m=============================================\033[0m"
    echo ""

    ensure_index_ready
    if [ $? -ne 0 ]; then
        read -r -p "按回车键返回主菜单..."
        return
    fi

    INDEX_COUNT=$(wc -l < "$INDEX_FILE")
    echo "当前索引记录数：$INDEX_COUNT 条"
    echo ""

    echo "请选择查询模式："
    echo "1. IP查询（全部时间）"
    echo "2. IP+时间范围查询"
    echo "3. IP+端口查询"
    echo "4. IP+协议查询"
    echo "5. 组合查询（IP+端口+协议+时间）"
    read -r -p "请输入选项（1-5）：" MODE

    if [ "$MODE" != "1" ] && [ "$MODE" != "2" ] && [ "$MODE" != "3" ] && [ "$MODE" != "4" ] && [ "$MODE" != "5" ]; then
        echo -e "\033[31m无效的选项\033[0m"
        read -r -p "按回车键返回主菜单..."
        return
    fi

    read -r -p "请输入要查询的IP地址：" QUERY_IP
    if [ -z "$QUERY_IP" ]; then
        echo -e "\033[31m错误：IP地址不能为空\033[0m"
        read -r -p "按回车键返回主菜单..."
        return
    fi

    QUERY_PORT=""
    QUERY_PROTO=""
    START_DATE_FULL=""
    END_DATE_FULL=""

    if [ "$MODE" = "3" ] || [ "$MODE" = "5" ]; then
        read -r -p "请输入端口号：" QUERY_PORT
        if [ -n "$QUERY_PORT" ] && ! printf '%s' "$QUERY_PORT" | grep -Eq '^[0-9]{1,5}$'; then
            echo -e "\033[31m错误：端口号格式错误\033[0m"
            read -r -p "按回车键返回主菜单..."
            return
        fi
    fi

    if [ "$MODE" = "4" ] || [ "$MODE" = "5" ]; then
        echo "协议类型："
        echo "  6 = TCP"
        echo "  17 = UDP"
        echo "  1 = ICMP"
        read -r -p "请输入协议号：" QUERY_PROTO
    fi

    if [ "$MODE" = "2" ] || [ "$MODE" = "5" ]; then
        echo ""
        echo "日期格式示例：20260424 或 2026-04-24 或 202604 或 2026"
        read -r -p "请输入开始日期：" START_DATE
        read -r -p "请输入结束日期：" END_DATE

        if [ -z "$START_DATE" ] || [ -z "$END_DATE" ]; then
            echo -e "\033[31m错误：日期不能为空\033[0m"
            read -r -p "按回车键返回主菜单..."
            return
        fi

        normalized_start=$(normalize_date "$START_DATE") || {
            echo -e "\033[31m开始日期格式错误\033[0m"
            read -r -p "按回车键返回主菜单..."
            return
        }
        normalized_end=$(normalize_date "$END_DATE") || {
            echo -e "\033[31m结束日期格式错误\033[0m"
            read -r -p "按回车键返回主菜单..."
            return
        }
        START_DATE_FULL="${normalized_start%%|*}"
        END_DATE_FULL="${normalized_end##*|}"
    fi

    echo ""
    echo -e "\033[33m正在查询索引...\033[0m"

    TEMP_FILES="/tmp/fw_files_$$.txt"
    TEMP_RAW="/tmp/fw_result_raw_$$.log"
    TEMP_RESULT="/tmp/fw_result_$$.log"

    # 使用增强索引查询（支持多字段过滤）
    awk -F'|' -v ip="$QUERY_IP" -v port="$QUERY_PORT" -v proto="$QUERY_PROTO" \
        -v start="$START_DATE_FULL" -v end="$END_DATE_FULL" '
        {
            # 字段：IP|日期|文件|端口|协议|动作
            if ($1 != ip) next
            if (start != "" && ($2 < start || $2 > end)) next
            if (port != "" && index($4, port) == 0) next
            if (proto != "" && $5 != proto) next
            print $3
        }
    ' "$INDEX_FILE" | sort -u > "$TEMP_FILES"

    FILE_COUNT=$(count_items_in_file "$TEMP_FILES")
    if [ "$FILE_COUNT" -eq 0 ]; then
        echo -e "\033[31m索引中未找到该IP的记录\033[0m"
        rm -f "$TEMP_FILES" "$TEMP_RAW" "$TEMP_RESULT"
        read -r -p "按回车键返回主菜单..."
        return
    fi

    echo "索引命中：$FILE_COUNT 个日志文件"
    echo -e "\033[33m正在提取日志内容...\033[0m"
    echo "------------------------------------------------------------"

    run_file_query "$QUERY_IP" "$QUERY_PORT" "$TEMP_RAW" "$TEMP_FILES" || {
        rm -f "$TEMP_FILES" "$TEMP_RAW" "$TEMP_RESULT"
        read -r -p "按回车键返回主菜单..."
        return
    }
    sort_result_file "$TEMP_RAW" "$TEMP_RESULT"
    RESULT_COUNT=$(count_items_in_file "$TEMP_RESULT")

    echo "------------------------------------------------------------"
    echo ""

    if [ "$RESULT_COUNT" -eq 0 ]; then
        echo -e "\033[31m未找到匹配的日志记录\033[0m"
    else
        echo -e "\033[32m查询完成，共找到 $RESULT_COUNT 条记录\033[0m"
        echo ""
        cat "$TEMP_RESULT"
        echo ""

        read -r -p "是否导出结果到 $EXPORT_DIR 目录？(y/n)：" EXPORT
        if [ "$EXPORT" = "y" ] || [ "$EXPORT" = "Y" ]; then
            export_result_file "$TEMP_RESULT" "$QUERY_IP" "模式=快速查询增强; 日期=${START_DATE:-全部}~${END_DATE:-全部}; IP=$QUERY_IP; 端口=${QUERY_PORT:-全部}; 协议=${QUERY_PROTO:-全部}"
        fi
    fi

    rm -f "$TEMP_FILES" "$TEMP_RAW" "$TEMP_RESULT"
    echo ""
    read -r -p "按回车键返回主菜单..."
}

advanced_query() {
    local LOG_COUNT QUERY_TYPE TEMP_RAW TEMP_RESULT QUERY_IP SRC_IP DST_IP PORT PORT_TYPE
    local PROTO_TYPE PROTO START_DATE END_DATE normalized_start normalized_end start_full end_full
    local KEYWORD EXPORT escaped_src escaped_dst ip_pattern FILE_LIST RESULT_COUNT EXPORT_KEY EXPORT_DESC

    clear
    echo -e "\033[32m=============================================\033[0m"
    echo -e "\033[32m   高级查询\033[0m"
    echo -e "\033[32m=============================================\033[0m"
    echo ""

    if [ ! -d "$LOG_DIR" ]; then
        echo -e "\033[31m日志目录 $LOG_DIR 不存在\033[0m"
        read -r -p "按回车键返回主菜单..."
        return
    fi

    LOG_COUNT=$(list_log_files | grep -c .)
    if [ "$LOG_COUNT" -eq 0 ]; then
        echo -e "\033[31m日志目录中无日志文件\033[0m"
        read -r -p "按回车键返回主菜单..."
        return
    fi

    echo "日志文件数：$LOG_COUNT 个"
    echo ""
    echo "请选择查询类型："
    echo "1. 单IP查询（源或目的IP均可匹配）"
    echo "2. 双IP查询（同时匹配源IP和目的IP）"
    echo "3. 端口查询（可选源端口、目的端口或任意）"
    echo "4. 协议类型查询（TCP/UDP/ICMP）"
    echo "5. 时间范围查询（导出指定时间段的所有日志）"
    echo "6. 关键字查询（支持正则表达式）"
    echo "7. 组合查询（IP+端口+时间范围）"
    read -r -p "请输入选项（1-7）：" QUERY_TYPE

    TEMP_RAW="/tmp/fw_advanced_raw_$$.log"
    TEMP_RESULT="/tmp/fw_advanced_$$.log"
    EXPORT_KEY="advanced"
    EXPORT_DESC="高级查询"
    : > "$TEMP_RAW"

    case $QUERY_TYPE in
        1)
            read -r -p "请输入IP地址：" QUERY_IP
            if [ -z "$QUERY_IP" ]; then
                echo -e "\033[31m错误：IP地址不能为空\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi

            # 可选：指定日期以使用CDB加速
            read -r -p "请输入日期（YYYY-MM-DD，留空查询所有日期）：" QUERY_DATE

            echo ""
            echo -e "\033[33m正在搜索...\033[0m"

            # 尝试使用CDB快速查询
            local use_cdb=0
            if [ "$HAS_CDB" -eq 1 ] && [ -f "$INDEX_FILE_CDB" ] && [ -n "$QUERY_DATE" ]; then
                echo -e "\033[36m[CDB加速模式]\033[0m"
                if fast_query_with_cdb "$QUERY_IP" "$QUERY_DATE" "$TEMP_RAW"; then
                    use_cdb=1
                fi
            fi

            # 回退到传统grep查询
            if [ "$use_cdb" -eq 0 ]; then
                [ -n "$QUERY_DATE" ] && echo -e "\033[33m[传统查询模式]\033[0m"
                echo "------------------------------------------------------------"
                ip_pattern=$(build_ip_pattern "$QUERY_IP")
                if [ -n "$QUERY_DATE" ]; then
                    # 指定日期查询
                    grep -EHn "$ip_pattern" "$LOG_DIR"/*_${QUERY_DATE}.log 2>/dev/null > "$TEMP_RAW"
                else
                    # 全部日期查询
                    grep -EHn "$ip_pattern" "$LOG_DIR"/*.log 2>/dev/null > "$TEMP_RAW"
                fi
            fi

            EXPORT_KEY="$QUERY_IP"
            EXPORT_DESC="类型=单IP查询; IP=$QUERY_IP"
            [ -n "$QUERY_DATE" ] && EXPORT_DESC="$EXPORT_DESC; 日期=$QUERY_DATE"
            ;;
        2)
            read -r -p "请输入源IP：" SRC_IP
            read -r -p "请输入目的IP：" DST_IP
            if [ -z "$SRC_IP" ] || [ -z "$DST_IP" ]; then
                echo -e "\033[31m错误：IP地址不能为空\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi
            echo ""
            echo -e "\033[33m正在搜索...\033[0m"
            echo "------------------------------------------------------------"
            escaped_src=$(escape_ere "$SRC_IP")
            escaped_dst=$(escape_ere "$DST_IP")
            grep -EHn "源IP:${escaped_src}([,[:space:]]|$)" "$LOG_DIR"/*.log 2>/dev/null | \
                grep -E "目的IP:${escaped_dst}([,[:space:]]|$)" > "$TEMP_RAW"
            EXPORT_KEY="$SRC_IP"
            EXPORT_DESC="类型=双IP查询; 源IP=$SRC_IP; 目的IP=$DST_IP"
            ;;
        3)
            read -r -p "请输入端口号：" PORT
            if ! printf '%s' "$PORT" | grep -Eq '^[0-9]{1,5}$'; then
                echo -e "\033[31m错误：端口号格式错误\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi
            echo "请选择端口类型："
            echo "1. 源端口"
            echo "2. 目的端口"
            echo "3. 任意端口"
            read -r -p "请输入选项（1-3）：" PORT_TYPE
            echo ""
            echo -e "\033[33m正在搜索...\033[0m"
            echo "------------------------------------------------------------"
            case $PORT_TYPE in
                1) grep -EHn "源端口:${PORT}([,[:space:]]|$)" "$LOG_DIR"/*.log 2>/dev/null > "$TEMP_RAW" ;;
                2) grep -EHn "目的端口:${PORT}([,[:space:]]|$)" "$LOG_DIR"/*.log 2>/dev/null > "$TEMP_RAW" ;;
                3) grep -EHn "$(build_port_pattern "$PORT")" "$LOG_DIR"/*.log 2>/dev/null > "$TEMP_RAW" ;;
                *)
                    echo -e "\033[31m无效的选项\033[0m"
                    read -r -p "按回车键返回主菜单..."
                    return
                    ;;
            esac
            EXPORT_KEY="port_$PORT"
            EXPORT_DESC="类型=端口查询; 类型=$PORT_TYPE; 端口=$PORT"
            ;;
        4)
            echo "请选择协议类型："
            echo "1. TCP（协议号:6）"
            echo "2. UDP（协议号:17）"
            echo "3. ICMP（协议号:1）"
            read -r -p "请输入选项（1-3）：" PROTO_TYPE
            case $PROTO_TYPE in
                1) PROTO="6" ;;
                2) PROTO="17" ;;
                3) PROTO="1" ;;
                *)
                    echo -e "\033[31m无效的选项\033[0m"
                    read -r -p "按回车键返回主菜单..."
                    return
                    ;;
            esac
            echo ""
            echo -e "\033[33m正在搜索...\033[0m"
            echo "------------------------------------------------------------"
            grep -EHn "协议:${PROTO}([,[:space:]]|$)" "$LOG_DIR"/*.log 2>/dev/null > "$TEMP_RAW"
            EXPORT_KEY="proto_$PROTO"
            EXPORT_DESC="类型=端口查询; 协议=$PROTO"
            ;;
        5)
            echo "日期格式示例：20260424 或 2026-04-24 或 202604 或 2026"
            read -r -p "请输入开始日期：" START_DATE
            read -r -p "请输入结束日期：" END_DATE
            if [ -z "$START_DATE" ] || [ -z "$END_DATE" ]; then
                echo -e "\033[31m错误：日期不能为空\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi
            normalized_start=$(normalize_date "$START_DATE") || {
                echo -e "\033[31m开始日期格式错误\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            }
            normalized_end=$(normalize_date "$END_DATE") || {
                echo -e "\033[31m结束日期格式错误\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            }
            start_full="${normalized_start%%|*}"
            end_full="${normalized_end##*|}"
            echo ""
            echo -e "\033[33m正在搜索...\033[0m"
            echo "------------------------------------------------------------"
            FILE_LIST="/tmp/fw_time_files_$$.txt"
            find_files_by_date_range "$start_full" "$end_full" > "$FILE_LIST"
            while IFS= read -r file; do
                [ -n "$file" ] || continue
                grep -Hn "" "$file" 2>/dev/null
            done < "$FILE_LIST" > "$TEMP_RAW"
            rm -f "$FILE_LIST"
            EXPORT_KEY="time_${START_DATE}_${END_DATE}"
            EXPORT_DESC="类型=日期查询; 开始=$START_DATE; 结束=$END_DATE"
            ;;
        6)
            read -r -p "请输入关键字：" KEYWORD
            if [ -z "$KEYWORD" ]; then
                echo -e "\033[31m错误：关键字不能为空\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi
            echo ""
            echo -e "\033[33m正在搜索...\033[0m"
            echo "------------------------------------------------------------"
            grep -EHn -- "$KEYWORD" "$LOG_DIR"/*.log 2>/dev/null > "$TEMP_RAW"
            EXPORT_KEY="keyword_$KEYWORD"
            EXPORT_DESC="类型=关键字查询; 关键字=$KEYWORD"
            ;;
        7)
            read -r -p "请输入IP地址：" QUERY_IP
            read -r -p "请输入端口号（可选，直接回车跳过）：" PORT
            echo "日期格式示例：20260424 或 2026-04-24 或 202604 或 2026"
            read -r -p "请输入开始日期：" START_DATE
            read -r -p "请输入结束日期：" END_DATE
            if [ -z "$QUERY_IP" ] || [ -z "$START_DATE" ] || [ -z "$END_DATE" ]; then
                echo -e "\033[31m错误：IP地址和日期不能为空\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi
            if [ -n "$PORT" ] && ! printf '%s' "$PORT" | grep -Eq '^[0-9]{1,5}$'; then
                echo -e "\033[31m错误：端口号格式错误\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            fi
            normalized_start=$(normalize_date "$START_DATE") || {
                echo -e "\033[31m开始日期格式错误\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            }
            normalized_end=$(normalize_date "$END_DATE") || {
                echo -e "\033[31m结束日期格式错误\033[0m"
                read -r -p "按回车键返回主菜单..."
                return
            }
            start_full="${normalized_start%%|*}"
            end_full="${normalized_end##*|}"
            echo ""
            echo -e "\033[33m正在搜索...\033[0m"
            echo "------------------------------------------------------------"
            FILE_LIST="/tmp/fw_combo_files_$$.txt"
            find_files_by_date_range "$start_full" "$end_full" > "$FILE_LIST"
            run_file_query "$QUERY_IP" "$PORT" "$TEMP_RAW" "$FILE_LIST" || {
                rm -f "$FILE_LIST" "$TEMP_RAW" "$TEMP_RESULT"
                read -r -p "按回车键返回主菜单..."
                return
            }
            rm -f "$FILE_LIST"
            EXPORT_KEY="$QUERY_IP"
            EXPORT_DESC="类型=组合查询; IP=$QUERY_IP; 端口=${PORT:-全部}; 开始=$START_DATE; 结束=$END_DATE"
            ;;
        *)
            echo -e "\033[31m无效的选项\033[0m"
            read -r -p "按回车键返回主菜单..."
            return
            ;;
    esac

    sort_result_file "$TEMP_RAW" "$TEMP_RESULT"
    RESULT_COUNT=$(count_items_in_file "$TEMP_RESULT")

    echo "------------------------------------------------------------"
    echo ""

    if [ "$RESULT_COUNT" -eq 0 ]; then
        echo -e "\033[31m未找到匹配的日志记录\033[0m"
    else
        echo -e "\033[32m查询完成，共找到 $RESULT_COUNT 条记录\033[0m"
        echo ""
        cat "$TEMP_RESULT"
        echo ""

        read -r -p "是否导出结果到 $EXPORT_DIR 目录？(y/n)：" EXPORT
        if [ "$EXPORT" = "y" ] || [ "$EXPORT" = "Y" ]; then
            export_result_file "$TEMP_RESULT" "$EXPORT_KEY" "$EXPORT_DESC"
        fi
    fi

    rm -f "$TEMP_RAW" "$TEMP_RESULT"
    echo ""
    read -r -p "按回车键返回主菜单..."
}

rebuild_index() {
    clear
    echo -e "\033[32m=============================================\033[0m"
    echo -e "\033[32m   重建索引（全量）\033[0m"
    echo -e "\033[32m=============================================\033[0m"
    echo ""

    echo -e "\033[33m警告：此操作将删除现有索引并重新建立\033[0m"
    read -r -p "确认继续？(y/n)：" CONFIRM

    if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
        return
    fi

    rm -f "$INDEX_FILE" "$INDEX_META"
    INDEX_READY_IN_SESSION=0

    echo ""
    echo "正在重建索引..."
    auto_update_index 0
    if [ $? -eq 0 ]; then
        INDEX_READY_IN_SESSION=1
    fi
    echo ""
    read -r -p "按回车键返回主菜单..."
}

incremental_update_index() {
    clear
    echo -e "\033[32m=============================================\033[0m"
    echo -e "\033[32m   增量更新索引\033[0m"
    echo -e "\033[32m=============================================\033[0m"
    echo ""

    if [ ! -f "$INDEX_FILE" ]; then
        echo -e "\033[31m错误：索引尚未建立，请先选择"重建索引"\033[0m"
        echo ""
        read -r -p "按回车键返回主菜单..."
        return
    fi

    echo "正在检测变更的日志文件..."
    auto_update_index 0

    echo ""
    read -r -p "按回车键返回主菜单..."
}

clean_export_files() {
    clear
    echo -e "\033[32m=============================================\033[0m"
    echo -e "\033[32m   清理导出文件\033[0m"
    echo -e "\033[32m=============================================\033[0m"
    echo ""

    echo "请选择清理范围："
    echo "1. 清理所有导出文件"
    echo "2. 清理IP查询结果"
    echo "3. 清理端口查询结果"
    echo "4. 清理日期查询结果"
    echo "5. 清理组合查询结果"
    echo "6. 清理临时文件"
    echo "0. 返回主菜单"
    echo ""
    read -r -p "请输入选项（0-6）：" CLEAN_CHOICE

    case $CLEAN_CHOICE in
        1)
            echo ""
            echo -e "\033[33m警告：将删除所有导出文件\033[0m"
            read -r -p "确认继续？(y/n)：" CONFIRM
            if [ "$CONFIRM" = "y" ] || [ "$CONFIRM" = "Y" ]; then
                rm -rf "$EXPORT_BY_IP"/* "$EXPORT_BY_PORT"/* "$EXPORT_BY_DATE"/* "$EXPORT_BY_QUERY"/* 2>/dev/null
                echo -e "\033[32m✓ 已清理所有导出文件\033[0m"
            fi
            ;;
        2)
            rm -rf "$EXPORT_BY_IP"/* 2>/dev/null
            echo -e "\033[32m✓ 已清理IP查询结果\033[0m"
            ;;
        3)
            rm -rf "$EXPORT_BY_PORT"/* 2>/dev/null
            echo -e "\033[32m✓ 已清理端口查询结果\033[0m"
            ;;
        4)
            rm -rf "$EXPORT_BY_DATE"/* 2>/dev/null
            echo -e "\033[32m✓ 已清理日期查询结果\033[0m"
            ;;
        5)
            rm -rf "$EXPORT_BY_QUERY"/* 2>/dev/null
            echo -e "\033[32m✓ 已清理组合查询结果\033[0m"
            ;;
        6)
            rm -rf "$TEMP_DIR"/* 2>/dev/null
            echo -e "\033[32m✓ 已清理临时文件\033[0m"
            ;;
        0)
            return
            ;;
        *)
            echo -e "\033[31m无效的选项\033[0m"
            ;;
    esac

    echo ""
    read -r -p "按回车键返回主菜单..."
}

manage_shortcut() {
    clear
    echo -e "\033[32m=============================================\033[0m"
    echo -e "\033[32m   快捷键管理\033[0m"
    echo -e "\033[32m=============================================\033[0m"
    echo ""

    echo "当前脚本路径：$SCRIPT_PATH"
    echo "快捷键命令：$SHORTCUT_NAME"
    echo ""

    check_shortcut_status
    echo ""

    echo "请选择操作："
    echo "1. 安装快捷键"
    echo "2. 卸载快捷键"
    echo "3. 查看快捷键状态"
    echo "0. 返回主菜单"
    echo ""
    read -r -p "请输入选项（0-3）：" SHORTCUT_CHOICE

    case $SHORTCUT_CHOICE in
        1)
            echo ""
            install_shortcut
            ;;
        2)
            echo ""
            uninstall_shortcut
            ;;
        3)
            echo ""
            check_shortcut_status
            ;;
        0)
            return
            ;;
        *)
            echo -e "\033[31m无效的选项\033[0m"
            ;;
    esac

    echo ""
    read -r -p "按回车键返回主菜单..."
}

# 初始化数据目录结构
init_data_dirs

# 自动刷新快捷键（在脚本启动时调用）
auto_refresh_shortcut

if [ $# -gt 0 ]; then
    # 处理特殊参数
    case "$1" in
        --install-shortcut)
            install_shortcut
            exit $?
            ;;
        --uninstall-shortcut)
            uninstall_shortcut
            exit $?
            ;;
        --check-shortcut)
            check_shortcut_status
            exit $?
            ;;
        *)
            # 正常查询
            ensure_index_ready
            run_cli_query "$@"
            exit $?
            ;;
    esac
fi

ensure_index_ready

while true; do
    show_main_menu

    case $MAIN_CHOICE in
        1) fast_query ;;
        2) advanced_query ;;
        3) incremental_update_index ;;
        4) manage_shortcut ;;
        5) clean_export_files ;;
        6)
            if [ "$HAS_CDB" -eq 0 ]; then
                install_cdb_component
            else
                echo -e "\033[31m无效选项\033[0m"
                sleep 1
            fi
            ;;
        0)
            echo ""
            echo "再见！"
            exit 0
            ;;
        *)
            echo -e "\033[31m无效选项\033[0m"
            sleep 1
            ;;
    esac
done
