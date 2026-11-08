#!/usr/bin/env bash
# docs.sh - 文档站点操作（docs/，VitePress）
#
# 封装 docs/ 下的依赖安装、本地预览与静态站点构建，
# 部署到子路径时通过 DOCS_BASE 指定 base。
#
# 用法：
#   scripts/docs.sh install   # 安装站点依赖（pnpm install）
#   scripts/docs.sh dev       # 本地预览，额外参数原样传给 vitepress
#   scripts/docs.sh build     # 构建静态站点到 docs/.vitepress/dist
#   scripts/docs.sh preview   # 预览已构建的站点
#
# 示例：
#   DOCS_BASE=/zhongkui/ scripts/docs.sh build
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
DOCS_DIR="$ROOT_DIR/docs"

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
install | dev | build | preview)
	cd "$DOCS_DIR" || exit 1
	pnpm "$@"
	;;
*)
	echo "未知子命令: $1" >&2
	usage >&2
	exit 1
	;;
esac
