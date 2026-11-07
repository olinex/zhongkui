#!/usr/bin/env bash
# p0f-update.sh - 刷新内嵌的上游 p0f 指纹签名库并运行指纹测试
#
# 下载社区维护的 p0f3 签名库覆盖 internal/network/huginn/p0f.fp，
# 校验格式后运行 TestFingerprintFromPcap 验证。本地定制签名保存在
# p0f.local.fp，不会被覆盖。
#
# 用法：
#   scripts/p0f-update.sh                        # 使用默认上游源
#   scripts/p0f-update.sh <upstream-url>         # 指定上游源
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

P0F_FP="internal/network/huginn/p0f.fp"
P0F_UPSTREAM_URL="${1:-https://raw.githubusercontent.com/peace-maker/p0f3-database/main/p0f.fp}"

echo "=== 下载上游 p0f 签名库: $P0F_UPSTREAM_URL ==="
curl -fsSL "$P0F_UPSTREAM_URL" -o "$P0F_FP.tmp" || {
	echo "下载失败，请检查网络或 P0F_UPSTREAM_URL" >&2
	exit 1
}

echo "=== 下载完成，校验格式（行数/首个 section）==="
wc -l "$P0F_FP.tmp"
grep -m1 '^\[tcp:request\]' "$P0F_FP.tmp" >/dev/null || {
	echo "校验失败：下载内容不含 [tcp:request] section，中止" >&2
	rm -f "$P0F_FP.tmp"
	exit 1
}

mv "$P0F_FP.tmp" "$P0F_FP"
echo "=== 运行指纹测试验证 ==="
go test ./internal/network/detectors -run '^TestFingerprintFromPcap$' -v
