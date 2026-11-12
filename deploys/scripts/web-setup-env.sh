#!/usr/bin/env bash
#
# web-setup-env.sh - 前端编译环境准备（corepack + pnpm）
#
# 被 deploys/local/web.Dockerfile 的 builder 阶段调用；本地也可直接执行用于固定 pnpm 版本。
# npm 镜像与重试参数由调用方通过环境变量提供（Dockerfile 用 ENV 持久化到后续阶段），
# 脚本只负责启用 corepack 并把 pnpm 钉死到指定版本，避免每次构建重新解析 "pnpm@10"。
#
# 用法：
#   bash web-setup-env.sh [pnpm 版本]
#   bash web-setup-env.sh                # 默认 10.34.9
#   PNPM_VERSION=10.34.9 bash web-setup-env.sh
#
# 退出码：0 成功；非 0 失败。
set -euo pipefail

PNPM_VERSION="${1:-${PNPM_VERSION:-10.34.9}}"

echo ">>> setup-web-env: pnpm@${PNPM_VERSION}, registry=${npm_config_registry:-<默认>}"

corepack enable

# 钉死版本：该层可稳定命中 Docker 缓存，且不依赖 corepack 的在线版本解析
corepack prepare "pnpm@${PNPM_VERSION}" --activate

pnpm --version
