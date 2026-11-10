#!/usr/bin/env bash
#
# build-web.sh - 在前端构建环境镜像内编译控制台与文档站静态产物
#
# 在仓库根目录执行（CI 由 execute@docker 把仓库挂载到 /root/workspace；本地经 docker run 挂载，
# 见 deploys/build/web.builder.Dockerfile 头注释）。
#
# 产物：web/dist（控制台）、docs/.vitepress/dist（文档站），由 web.Dockerfile 直接装配。
# 控制台与文档站串行构建：内存受限环境下两个 Node 构建并发会触发 OOM。
# 设置 HOST_UID/HOST_GID 时产物 chown 回宿主机用户。
# 退出码：0 成功；1 失败。
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR" || exit 1

# 控制台构建参数（Vite 在编译期固化）：API 基址固定为同源 /api/v1，由 Nginx 反代到后端；
# VITE_DOCS_URL 是控制台顶栏的文档入口地址。
export VITE_ZHONGKUI_BASE_URL="${VITE_ZHONGKUI_BASE_URL:-/api/v1}"
export VITE_DOCS_URL="${VITE_DOCS_URL:-/docs/}"
# 文档站构建参数：DOCS_BASE 部署子路径；DOCS_CONSOLE_URL 回跳控制台地址（由 nginx 302 到 /）；
# 构建上下文不含 .git，关闭 lastUpdated，避免 VitePress 调用 git 失败。
export DOCS_BASE="${DOCS_BASE:-/docs/}"
export DOCS_CONSOLE_URL="${DOCS_CONSOLE_URL:-/console}"
export DOCS_LAST_UPDATED=false

echo ">>> 构建控制台（web/）"
cd web && pnpm install --frozen-lockfile && pnpm build && cd "$ROOT_DIR"

echo ">>> 构建文档站（docs/）"
cd docs && pnpm install --frozen-lockfile && pnpm build && cd "$ROOT_DIR"

if [ -n "${HOST_UID:-}" ]; then
	chown -R "${HOST_UID}:${HOST_GID:-$HOST_UID}" web/dist docs/.vitepress/dist
fi
