#!/usr/bin/env bash
# migrate.sh - 本地数据库迁移
#
# 从源码运行 zki migrate，参数原样透传；本地开发无需先编译二进制。
#
# 用法：
#   scripts/migrate.sh up                 # 执行全部未应用的迁移
#   scripts/migrate.sh down               # 回滚一个版本
#   scripts/migrate.sh status             # 查看迁移状态
#   scripts/migrate.sh dry-run            # 试运行迁移
#   scripts/migrate.sh up-to <version>    # 迁移到指定版本
#   scripts/migrate.sh down-to <version>  # 回滚到指定版本
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

usage() {
	sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
-h | --help)
	usage
	exit 0
	;;
esac

if [ $# -eq 0 ]; then
	usage >&2
	exit 1
fi

go run cmd/zki/main.go migrate "$@"
