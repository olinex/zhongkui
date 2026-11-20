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

# git ls-files 会列出「已跟踪但在工作区被删除」的文件，直接交给 gofmt 会因
# lstat 失败而中断；先过滤出磁盘上仍存在的文件再检查。
file_list=$(mktemp)
trap 'rm -f "$file_list"' EXIT
{
	git ls-files -z '*.go'
	git ls-files -z --others --exclude-standard '*.go'
} | while IFS= read -r -d '' file; do
	[ -f "$file" ] && printf '%s\0' "$file"
done >"$file_list"

unformatted=""
if [ -s "$file_list" ]; then
	unformatted=$(xargs -0 gofmt -l <"$file_list")
fi
if [ -n "$unformatted" ]; then
	echo "以下文件未通过 gofmt:"
	echo "$unformatted"
	exit 1
fi
