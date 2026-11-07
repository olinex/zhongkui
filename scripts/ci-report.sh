#!/usr/bin/env bash
# ci-report.sh - 汇总式 CI：逐项执行检查并留存日志，跑完全部检查后再统一判失败
#
# 与 `make ci` 的区别：
#   1. 不在第一个失败处中断，一次暴露所有问题；
#   2. 每项输出写入 .cicd/reports/<检查名>.log；
#   3. 测试改用 test-report，额外产出 Gitee Go 可解析的 .cicd/golang-report/。
#
# 用法：
#   scripts/ci-report.sh
#
# 退出码：0 全部通过；1 存在失败项。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

REPORT_DIR="${REPORT_DIR:-.cicd/reports}"
mkdir -p "$REPORT_DIR"

CI_CHECKS=(fmt-check build-check vet test-report lint license-check docs-check)

failed=""
for check in "${CI_CHECKS[@]}"; do
	printf '\n===== %s =====\n' "$check"
	if make --no-print-directory "$check" >"$REPORT_DIR/$check.log" 2>&1; then
		printf 'PASS  %s\n' "$check"
	else
		printf 'FAIL  %s\n' "$check"
		failed="$failed $check"
		tail -n 40 "$REPORT_DIR/$check.log"
	fi
done

if [ -n "$failed" ]; then
	printf '\nCI 检查未通过，失败项:%s\n逐项日志目录: %s\n' "$failed" "$REPORT_DIR"
	exit 1
fi

printf '\nCI 检查全部通过\n'
