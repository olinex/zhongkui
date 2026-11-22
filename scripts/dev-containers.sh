#!/usr/bin/env bash
# dev-containers.sh - 开发依赖容器管理（redis/etcd/clickhouse-server/postgres/rustfs）
#
# 统一管理五个依赖容器的拉取、启动、停止与删除，并初始化测试库。
# 容器运行时：macOS/Windows 使用 docker；Linux 优先 docker，未安装时回退 nerdctl。
#
# 用法：
#   scripts/dev-containers.sh create-dbs   # 创建 core_test (postgres) 与 raw_test (clickhouse)
#   scripts/dev-containers.sh pull         # 按宿主架构拉取开发依赖镜像
#   scripts/dev-containers.sh start        # 启动开发依赖容器
#   scripts/dev-containers.sh stop         # 停止开发依赖容器（保留数据）
#   scripts/dev-containers.sh remove       # 停止并删除开发依赖容器（数据丢失）
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

# 根据操作系统选择容器运行时：macOS/Windows 走 docker；Linux 优先 docker，未安装时回退 nerdctl
detect_runtime() {
	case "$(uname -s)" in
	Darwin | MINGW* | MSYS* | CYGWIN*) echo docker ;;
	Linux)
		if command -v docker >/dev/null 2>&1; then
			echo docker
		else
			echo nerdctl
		fi
		;;
	*)
		echo "警告: 不支持的操作系统 '$(uname -s)'。" >&2
		exit 1
		;;
	esac
}

CONTAINERS=(redis etcd clickhouse-server postgres rustfs)

cmd_create_dbs() {
	local cmd psql_cli clickhouse_cli
	cmd=$(detect_runtime)
	clickhouse_cli="$cmd exec clickhouse-server clickhouse-client"
	psql_cli="PGPASSWORD=root_password $cmd exec postgres psql -U postgres"

	echo "创建测试数据库 core_test (postgres) 与 raw_test (clickhouse)..."
	if $psql_cli -tAc "SELECT 1 FROM pg_database WHERE datname='core_test'" | grep -q 1; then
		echo "  core_test 已存在"
	else
		$psql_cli -c "CREATE DATABASE core_test"
	fi
	$clickhouse_cli -q "CREATE DATABASE IF NOT EXISTS raw_test"
	echo "测试库就绪 (core_test@postgres, raw_test@clickhouse)"
}

cmd_pull() {
	local cmd arch platform
	cmd=$(detect_runtime)
	arch=$(uname -m)
	case "$arch" in
	arm64 | aarch64) platform=linux/arm64 ;;
	x86_64 | amd64) platform=linux/amd64 ;;
	*)
		echo "警告: 无法识别的架构 '$arch'，缺省使用 linux/amd64"
		platform=linux/amd64
		;;
	esac
	echo "检测到宿主架构: $arch，拉取平台: $platform"

	local images=(
		"redis redis:8.2.1"
		"etcd quay.io/coreos/etcd:v3.6.5"
		"clickhouse clickhouse:25.3.3"
		"postgres pgvector/pgvector:pg17"
		"rustfs rustfs/rustfs:latest"
	)
	local item name img
	for item in "${images[@]}"; do
		read -r name img <<<"$item"
		echo "=== 拉取 $name ($platform) ==="
		$cmd pull --platform "$platform" "$img" ||
			echo "  !! 拉取 $name 失败，镜像可能不提供 $platform 架构"
	done
}

# 所有依赖容器都在运行时返回 0，供重复执行 start 时跳过
all_running() {
	local cmd c
	cmd=$(detect_runtime)
	for c in "${CONTAINERS[@]}"; do
		$cmd ps --format '{{.Names}}' 2>/dev/null | grep -qx "$c" || return 1
	done
}

cmd_start() {
	if all_running; then
		echo "开发依赖容器已在运行，跳过启动"
		return 0
	fi

	case "$(uname -s)" in
	Darwin | MINGW* | MSYS* | CYGWIN*)
		echo "使用 docker 启动依赖容器..."
		docker run -itd --restart unless-stopped --name redis --network host \
			redis:8.2.1
		docker run -itd --restart unless-stopped --name etcd --network host \
			quay.io/coreos/etcd:v3.6.5
		docker run -itd --restart unless-stopped --name clickhouse-server --network host \
			--ulimit nofile=262144:262144 \
			-v "$ROOT_DIR/config/clickhouse/config.d:/etc/clickhouse-server/config.d:ro" \
			-e CLICKHOUSE_PASSWORD=root_password \
			clickhouse:25.3.3
		docker run -itd --restart unless-stopped --name postgres --network host \
			-e POSTGRES_PASSWORD=root_password \
			-e POSTGRES_USER=postgres \
			-e POSTGRES_DB=default \
			pgvector/pgvector:pg17
		docker run -itd --restart unless-stopped --name rustfs --network host \
			-e RUSTFS_ADDRESS=localhost:9002 \
			-e RUSTFS_CONSOLE_ENABLE=true \
			-e RUSTFS_ACCESS_KEY=rustfsadmin \
			-e RUSTFS_SECRET_KEY=rustfsadmin \
			rustfs/rustfs:latest
		;;
	Linux)
		local cmd
		cmd=$(detect_runtime)
		echo "使用 $cmd 启动依赖容器..."
		$cmd run -td --restart=always --name redis --net=host \
			redis:8.2.1
		$cmd run -td --restart=always --name etcd --net=host \
			quay.io/coreos/etcd:v3.6.5
		$cmd run -td --restart=always --name clickhouse-server --net=host \
			--ulimit nofile=262144:262144 \
			-v "$ROOT_DIR/config/clickhouse/config.d:/etc/clickhouse-server/config.d:ro" \
			-e CLICKHOUSE_PASSWORD=root_password \
			clickhouse:25.3.3
		$cmd run -td --restart=always --name postgres --net=host \
			-e POSTGRES_PASSWORD=root_password \
			-e POSTGRES_USER=postgres \
			-e POSTGRES_DB=default \
			pgvector/pgvector:pg17
		$cmd run -td --restart=always --name rustfs --net=host \
			-e RUSTFS_ADDRESS=localhost:9002 \
			-e RUSTFS_CONSOLE_ENABLE=true \
			-e RUSTFS_ACCESS_KEY=rustfsadmin \
			-e RUSTFS_SECRET_KEY=rustfsadmin \
			rustfs/rustfs:latest
		;;
	*)
		echo "警告: 不支持的操作系统 '$(uname -s)'，请手动启动依赖服务。" >&2
		exit 1
		;;
	esac
}

cmd_stop() {
	local cmd c
	cmd=$(detect_runtime)
	echo "停止开发依赖容器..."
	for c in "${CONTAINERS[@]}"; do
		if $cmd ps --format '{{.Names}}' | grep -q "^${c}$" 2>/dev/null; then
			echo "  正在停止 $c..."
			$cmd stop "$c"
		else
			echo "  $c 未在运行，跳过"
		fi
	done
}

cmd_remove() {
	local cmd c
	# 先停止再删除，避免残留运行中的容器
	cmd_stop
	cmd=$(detect_runtime)
	echo "删除开发依赖容器..."
	for c in "${CONTAINERS[@]}"; do
		if $cmd ps -a --format '{{.Names}}' | grep -q "^${c}$" 2>/dev/null; then
			echo "  正在删除 $c..."
			$cmd rm -f "$c"
		else
			echo "  $c 不存在，跳过"
		fi
	done
}

usage() {
	sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
create-dbs) cmd_create_dbs ;;
pull) cmd_pull ;;
start) cmd_start ;;
stop) cmd_stop ;;
remove) cmd_remove ;;
-h | --help | "") usage ;;
*)
	echo "未知子命令: $1" >&2
	usage >&2
	exit 1
	;;
esac
