#!/usr/bin/env bash
#
# web-setup-env.sh - 前端编译环境准备（pnpm）
#
# 被 deploys/containers/web.Dockerfile 的 builder 阶段调用；本地也可直接执行。
# 策略：优先检测运行环境是否已有可用的 pnpm 10.x（预装环境下可直接复用），
# 版本够就跳过安装，避免 npm install -g 浪费时间；否则依次尝试 corepack 与 npm。
#
# 用法：
#   bash web-setup-env.sh [pnpm 版本]
#   bash web-setup-env.sh                # 目标版本 10.34.9
#   PNPM_VERSION=10.34.9 bash web-setup-env.sh
#
# 退出码：0 成功；非 0 失败。
set -euo pipefail

PNPM_VERSION="${1:-${PNPM_VERSION:-10.34.9}}"

# pnpm 主版本号：10、11 …
pnpm_major() {
	command -v pnpm >/dev/null 2>&1 || return 1
	pnpm --version 2>/dev/null | awk -F. '{print $1}'
}

echo ">>> setup-web-env: target pnpm@${PNPM_VERSION}, registry=${npm_config_registry:-<默认>}"

TARGET_MAJOR="${PNPM_VERSION%%.*}"
CUR_MAJOR=""

if CUR_MAJOR=$(pnpm_major); then
	if [ "$CUR_MAJOR" -ge "$TARGET_MAJOR" ]; then
		echo ">>> setup-web-env: 检测到 pnpm ${CUR_MAJOR}.x，版本满足要求，跳过安装"
		pnpm --version
		exit 0
	fi
	echo ">>> setup-web-env: 当前 pnpm ${CUR_MAJOR}.x，需要升级到 ${TARGET_MAJOR}.x"
fi

if command -v corepack >/dev/null 2>&1; then
	# Dockerfile / 本地有 corepack：启用并钉死版本，层缓存稳定
	corepack enable
	corepack prepare "pnpm@${PNPM_VERSION}" --activate
else
	# 精简镜像没有 corepack：回退到 npm 全局安装
	echo ">>> setup-web-env: 未找到 corepack，改用 npm 安装 pnpm@${PNPM_VERSION}"
	npm install -g "pnpm@${PNPM_VERSION}"
fi

pnpm --version
