#!/usr/bin/env bash
# ci.sh - 提交与 CI 的统一检查入口。
#
# 串联格式、构建、vet、测试、注释、许可证、文档与部署配置漂移检查；
# config 子命令依赖 helm（可用 HELM_BIN 指定）。
#
# 用法：
#   scripts/ci.sh
#
# 退出码：0 全部通过；非零表示存在失败项。
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 2

case "${1:-}" in
-h | --help)
	sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
	exit 0
	;;
"") ;;
*)
	echo "未知参数: $1" >&2
	exit 2
	;;
esac

scripts/check.sh fmt
scripts/check.sh build
scripts/check.sh vet
scripts/check.sh test
scripts/check.sh lint
scripts/check.sh license
scripts/check.sh docs
scripts/check.sh config

echo "CI 检查全部通过"
