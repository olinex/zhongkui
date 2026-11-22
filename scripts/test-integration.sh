#!/usr/bin/env bash
# test-integration.sh - 运行依赖外部服务的集成测试
#
# 自动启动开发依赖容器、创建测试数据库，再以 ZKI_TEST_INTEGRATION=1
# 运行全部测试；额外参数原样传给 go test。
#
# 用法：
#   scripts/test-integration.sh            # 运行全部集成测试
#   scripts/test-integration.sh -v -run TestFoo   # 传递 go test 参数
#
# 自定义 DSN 时可直接设置 ZKI_DATABASE_CORE_TEST_DSN / ZKI_DATABASE_RAW_TEST_DSN。
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

usage() {
	sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
-h | --help) usage && exit 0 ;;
esac

scripts/dev-containers.sh start || exit 1
scripts/dev-containers.sh create-dbs || exit 1

ZKI_TEST_INTEGRATION=1 go test ./... "$@"
