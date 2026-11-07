#!/usr/bin/env bash
# validate-semver.sh - 校验语义化版本号（SemVer 2.0.0）
#
# 供 build-release / release / dist 共用，避免在 Makefile 里重复写
# 正则校验。版本号格式：vX.Y.Z，可带预发布与构建元数据，
# 例如 v1.2.3、v0.1.0-alpha.1、v1.2.3-rc.2+build.5。
#
# 用法：
#   scripts/validate-semver.sh v1.2.3            # 校验通过，退出码 0
#   scripts/validate-semver.sh bad-version       # 校验失败，退出码 1
#
# 退出码：0 合法；1 不合法。
set -uo pipefail

SEMVER_PATTERN='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$'

if [ $# -lt 1 ]; then
	echo "用法: $0 <版本号>" >&2
	exit 1
fi

if echo "$1" | grep -Eq "$SEMVER_PATTERN"; then
	exit 0
fi

echo "ERROR: 版本号 '$1' 不是合法的语义化版本号，示例: v1.2.3 或 v0.1.0-alpha.1" >&2
exit 1
