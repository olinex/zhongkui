#!/usr/bin/env bash
# test-report.sh - 以 Gitee Go 可解析的格式运行单元测试并生成覆盖率报告
#
# 产物位于 .cicd/golang-report/：report.jsonl（过滤后的 JSON 行）、
# index.html（覆盖率页面）、coverage.txt（覆盖率汇总）、stderr.log。
#
# 为什么不直接用 go test -json：少数情况下会输出非 JSON 行（如 cgo 库
# 直接向 stdout 打印），而 Gitee Go 的 UT 插件要求每行都是 JSON 对象。
# 因此先写到 report.raw.jsonl，再用 jq 过滤只保留合法 JSON 对象行。
#
# 用法：
#   scripts/test-report.sh
#
# 退出码：与 go test 一致（0 全部通过，非 0 有失败）。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

GOLANG_REPORT_DIR="${GOLANG_REPORT_DIR:-.cicd/golang-report}"
mkdir -p "$GOLANG_REPORT_DIR"

set +e
raw="$GOLANG_REPORT_DIR/report.raw.jsonl"
go test -v -json -cover -coverprofile="$GOLANG_REPORT_DIR/cover.out" ./... \
	>"$raw" 2>"$GOLANG_REPORT_DIR/stderr.log"
status=$?

if command -v jq >/dev/null 2>&1; then
	jq -R -c 'fromjson? | select(type=="object")' "$raw" >"$GOLANG_REPORT_DIR/report.jsonl"
else
	grep -E '^\{.*\}$' "$raw" >"$GOLANG_REPORT_DIR/report.jsonl" || true
fi
rm -f "$raw"

go tool cover -html="$GOLANG_REPORT_DIR/cover.out" -o "$GOLANG_REPORT_DIR/index.html" 2>/dev/null || true
go tool cover -func="$GOLANG_REPORT_DIR/cover.out" >"$GOLANG_REPORT_DIR/coverage.txt" 2>/dev/null || true
tail -n 1 "$GOLANG_REPORT_DIR/coverage.txt" 2>/dev/null || true

if [ "$status" -ne 0 ]; then
	echo "单元测试失败，完整报告: $GOLANG_REPORT_DIR/report.jsonl"
	grep '"Action":"fail"' "$GOLANG_REPORT_DIR/report.jsonl" | tail -n 20 || true
	tail -n 40 "$GOLANG_REPORT_DIR/stderr.log" || true
fi

exit "$status"
