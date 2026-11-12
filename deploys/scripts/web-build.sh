#!/usr/bin/env bash
#
# web-build.sh - 前端静态产物构建（web/ 控制台 + docs/ 文档站）
#
# 统一的依赖安装与生产构建入口，供 deploys/local/web.Dockerfile 的 deps / build 阶段复用：
#   --deps    只安装依赖（web/ 与 docs/ 各自 pnpm install --frozen-lockfile）
#   --build   只执行生产构建（web/ 的 pnpm build-only + docs/ 的 pnpm build）
#   无参数    先装依赖再构建，便于本地一键复现镜像内的编译过程
#
# 设计参考 deploys/scripts/core-build-multiarch.sh：脚本只做编排，
# 具体的 pnpm 脚本仍以各子项目 package.json 为准。
#
# 用法：
#   bash web-build.sh --deps
#   bash web-build.sh --build
#   bash web-build.sh
#
# 环境变量：
#   WEB_ROOT           仓库根目录；不设置则按脚本位置推导（脚本被 COPY 到 /tmp 时需显式指定）
#   VITE_ZHONGKUI_BASE_URL / VITE_DOCS_URL / DOCS_BASE / DOCS_CONSOLE_URL
#   DOCS_LAST_UPDATED  默认 false（构建上下文不含 .git，避免 VitePress 调用 git 失败）
#
# 退出码：0 成功；非 0 失败。
set -euo pipefail

MODE="all"
while [ $# -gt 0 ]; do
  case "$1" in
    --deps)  MODE="deps"; shift ;;
    --build) MODE="build"; shift ;;
    --*)     echo "ERROR: 未知选项 $1" >&2; exit 1 ;;
    *)       echo "ERROR: 多余参数 $1" >&2; exit 1 ;;
  esac
done

ROOT_DIR="${WEB_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT_DIR"

# 构建上下文不含 .git，关闭 VitePress 的 lastUpdated
export DOCS_LAST_UPDATED="${DOCS_LAST_UPDATED:-false}"

install_deps() {
  echo ">>> 安装控制台依赖（web/）"
  ( cd web && pnpm install --frozen-lockfile )
  echo ">>> 安装文档站依赖（docs/）"
  ( cd docs && pnpm install --frozen-lockfile )
}

build_dist() {
  # 串行构建：内存受限环境下两个 Node 构建并发会触发 OOM
  echo ">>> 构建控制台（web/）"
  ( cd web && pnpm build-only )
  echo ">>> 构建文档站（docs/）"
  ( cd docs && pnpm build )
}

case "$MODE" in
  deps)  install_deps ;;
  build) build_dist ;;
  all)   install_deps; build_dist ;;
esac

echo ">>> web-build: 完成（模式: ${MODE}）"
