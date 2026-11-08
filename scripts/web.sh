#!/usr/bin/env bash
# web.sh - 前端工程操作（web/）
#
# 封装 web/ 下的依赖安装、开发服务器、构建与检查命令，
# 文档中只需引用本脚本，不必手动 cd 到 web/。
#
# 用法：
#   scripts/web.sh install      # 安装依赖（pnpm install）
#   scripts/web.sh dev          # 启动开发服务器，额外参数原样传给 vite
#   scripts/web.sh build        # 类型检查 + 生产构建
#   scripts/web.sh preview      # 预览构建产物
#   scripts/web.sh lint         # ESLint
#   scripts/web.sh lint:style   # Stylelint
#   scripts/web.sh type-check   # vue-tsc 类型检查
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
WEB_DIR="$ROOT_DIR/web"

usage() {
	sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
-h | --help | "")
	usage
	exit 0
	;;
esac

if ! command -v pnpm >/dev/null 2>&1; then
	echo "ERROR: 未找到 pnpm，请先运行 scripts/install-tools.sh ts" >&2
	exit 1
fi

case "${1:-}" in
install | dev | build | preview | lint | lint:style | type-check)
	cd "$WEB_DIR" || exit 1
	pnpm "$@"
	;;
*)
	echo "未知子命令: $1" >&2
	usage >&2
	exit 1
	;;
esac
