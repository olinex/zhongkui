#!/usr/bin/env bash
# fmt-check.sh - 检查已跟踪的 Go 文件是否符合 gofmt
#
# 同时检查已跟踪文件和未跟踪但未被忽略的 Go 文件。发现未格式化文件时
# 打印文件列表并以非零退出码失败，适合作为 CI / 提交前检查。
#
# 用法：
#   scripts/fmt-check.sh
#
# 退出码：0 全部通过；1 存在未格式化文件。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

unformatted=$(gofmt -l $(git ls-files '*.go') $(git ls-files --others --exclude-standard '*.go'))
if [ -n "$unformatted" ]; then
	echo "以下文件未通过 gofmt:"
	echo "$unformatted"
	exit 1
fi
