#!/usr/bin/env bash
#
# 打包 FWLOG 查询 Chrome 扩展（MV3）为可分发的 zip。
#
# 用法：
#   scripts/build-query-extension.sh [--output <path>]
#
# 默认输出：web/public/downloads/fwlog-query-extension.zip
#
# 包内成员（无顶层目录名，路径统一用 "/" 分隔，与历史分发 zip 一致）：
#   manifest.json, popup.html / popup.css / popup.js,
#   background.js, status-metrics.js, date-availability.js,
#   icons/*, README.md
#
# 取舍说明（README.md）：
#   历史分发包一直带有 README.md（安装/证书信任说明），故此处保留，
#   以维持既有分发习惯与用户可见的安装指引。
#
# 依赖：zip（Git Bash / Linux 均可用）。缺失时直接报错退出，不静默降级。
# 可重复执行：先删除旧 zip 再重新打包，保证成员集确定。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SRC_DIR="${REPO_ROOT}/chrome-extension"
OUT="${REPO_ROOT}/web/public/downloads/fwlog-query-extension.zip"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      [[ $# -ge 2 ]] || { echo "错误：--output 缺少参数" >&2; exit 2; }
      OUT="$2"
      shift 2
      ;;
    -h|--help)
      echo "用法：scripts/build-query-extension.sh [--output <path>]"
      exit 0
      ;;
    *)
      echo "错误：未知参数 $1" >&2
      exit 2
      ;;
  esac
done

if ! command -v zip >/dev/null 2>&1; then
  echo "错误：未找到 'zip' 命令，无法打包扩展。" >&2
  echo "  Git Bash 可安装：pacman -S zip（MSYS2）；Debian/Ubuntu：apt-get install zip。" >&2
  exit 1
fi

[[ -d "${SRC_DIR}" ]] || { echo "错误：扩展源码目录不存在：${SRC_DIR}" >&2; exit 1; }

# 解析输出为绝对路径，并确保其目录存在。
OUT_DIR="$(dirname -- "${OUT}")"
mkdir -p -- "${OUT_DIR}"
OUT="$(cd -- "${OUT_DIR}" && pwd)/$(basename -- "${OUT}")"

# 先删除旧 zip，保证重跑结果确定。
rm -f -- "${OUT}"

# 收集成员文件：相对源码根、以 "/" 分隔、去掉顶层目录名、排除 Mac 元数据。
mapfile -t FILES < <(
  cd -- "${SRC_DIR}" &&
    find . -type f ! -name '.DS_Store' \
      | sed 's|^\./||' \
      | LC_ALL=C sort
)

[[ ${#FILES[@]} -gt 0 ]] || { echo "错误：${SRC_DIR} 下没有可打包文件" >&2; exit 1; }

( cd -- "${SRC_DIR}" && zip -X -q -r "${OUT}" "${FILES[@]}" )

echo "已生成：${OUT}"
echo "成员列表："
if command -v unzip >/dev/null 2>&1; then
  unzip -l "${OUT}"
else
  zipinfo -1 "${OUT}" 2>/dev/null || echo "（未找到 unzip/zipinfo，跳过成员列表）"
fi
