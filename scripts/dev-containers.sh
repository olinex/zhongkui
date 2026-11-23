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
# 镜像源与镜像覆盖：
#   DEV_IMAGE_REGISTRY=<镜像源>          统一替换五个镜像的来源（保留原仓库路径）
#   REDIS_IMAGE / ETCD_IMAGE / CLICKHOUSE_IMAGE / POSTGRES_IMAGE / RUSTFS_IMAGE
#                                        精确覆盖单个镜像，优先级高于 DEV_IMAGE_REGISTRY
#   PULL_TIMEOUT=<秒>                    单个镜像拉取超时，默认 0 表示不限制
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

# 统一镜像源：设置后所有镜像改为从该地址拉取（保留原仓库路径，
# Docker Hub 官方镜像自动补 library/）；五个 *_IMAGE 变量优先级更高
DEV_IMAGE_REGISTRY="${DEV_IMAGE_REGISTRY:-}"

mirror_image() {
	if [ -z "$DEV_IMAGE_REGISTRY" ]; then
		echo "$1"
		return
	fi
	local path=$1
	case "$path" in
	quay.io/*) path=${path#quay.io/} ;;
	docker.io/*) path=${path#docker.io/} ;;
	*/*) ;;
	*) path="library/$path" ;;
	esac
	echo "${DEV_IMAGE_REGISTRY%/}/$path"
}

# 依赖镜像：网络受限时可统一改 DEV_IMAGE_REGISTRY，或单独覆盖某个镜像
REDIS_IMAGE="${REDIS_IMAGE:-$(mirror_image redis:8.2.1)}"
ETCD_IMAGE="${ETCD_IMAGE:-$(mirror_image quay.io/coreos/etcd:v3.6.5)}"
CLICKHOUSE_IMAGE="${CLICKHOUSE_IMAGE:-$(mirror_image clickhouse:25.3.3)}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-$(mirror_image pgvector/pgvector:pg17)}"
RUSTFS_IMAGE="${RUSTFS_IMAGE:-$(mirror_image rustfs/rustfs:latest)}"

# 返回镜像对应的环境变量名，用于拉取失败时提示覆盖方式
image_var() {
	case "$1" in
	redis) echo REDIS_IMAGE ;;
	etcd) echo ETCD_IMAGE ;;
	clickhouse) echo CLICKHOUSE_IMAGE ;;
	postgres) echo POSTGRES_IMAGE ;;
	rustfs) echo RUSTFS_IMAGE ;;
	esac
}

# 单个镜像的拉取超时（秒），0 表示不限制
PULL_TIMEOUT="${PULL_TIMEOUT:-0}"

# run_with_timeout：为命令加总超时；Linux 用 timeout，macOS 用 gtimeout 或后台看门狗
run_with_timeout() {
	local secs=$1
	shift
	case "$secs" in
	'' | *[!0-9]*) "$@" && return 0 || return $? ;;
	0) "$@" && return 0 || return $? ;;
	esac
	if command -v timeout >/dev/null 2>&1; then
		timeout "$secs" "$@"
	elif command -v gtimeout >/dev/null 2>&1; then
		gtimeout "$secs" "$@"
	else
		"$@" &
		local pid=$!
		(sleep "$secs"; kill -TERM "$pid" >/dev/null 2>&1) &
		local watcher=$!
		wait "$pid"
		local rc=$?
		kill "$watcher" >/dev/null 2>&1
		wait "$watcher" 2>/dev/null
		return $rc
	fi
}

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
	echo "检测到宿主架构: ${arch}，拉取平台: $platform"

	local images=(
		"redis $REDIS_IMAGE"
		"etcd $ETCD_IMAGE"
		"clickhouse $CLICKHOUSE_IMAGE"
		"postgres $POSTGRES_IMAGE"
		"rustfs $RUSTFS_IMAGE"
	)
	local item name img failed=0 rc
	for item in "${images[@]}"; do
		read -r name img <<<"$item"
		echo "=== 拉取 $name ($platform): $img ==="
		run_with_timeout "$PULL_TIMEOUT" $cmd pull --platform "$platform" "$img"
		rc=$?
		if [ "$rc" -ne 0 ]; then
			if [ "$rc" -eq 124 ] || [ "$rc" -eq 143 ]; then
				echo "  !! 拉取 $name 超时（PULL_TIMEOUT=${PULL_TIMEOUT}s）；可调大 PULL_TIMEOUT 或改用可达镜像源" >&2
			else
				echo "  !! 拉取 $name 失败；可用环境变量 $(image_var "$name") 指定加速地址或私有仓库中的同名镜像后重试" >&2
			fi
			failed=1
		fi
	done
	if [ "$failed" -ne 0 ]; then
		echo "存在拉取失败的镜像：请用 DEV_IMAGE_REGISTRY 或 *_IMAGE 指定可达地址；网络较慢时可调大 PULL_TIMEOUT（0 表示不限时）。" >&2
		return 1
	fi
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
			"$REDIS_IMAGE"
		docker run -itd --restart unless-stopped --name etcd --network host \
			"$ETCD_IMAGE"
		docker run -itd --restart unless-stopped --name clickhouse-server --network host \
			--ulimit nofile=262144:262144 \
			-v "$ROOT_DIR/config/clickhouse/config.d:/etc/clickhouse-server/config.d:ro" \
			-e CLICKHOUSE_PASSWORD=root_password \
			"$CLICKHOUSE_IMAGE"
		docker run -itd --restart unless-stopped --name postgres --network host \
			-e POSTGRES_PASSWORD=root_password \
			-e POSTGRES_USER=postgres \
			-e POSTGRES_DB=default \
			"$POSTGRES_IMAGE"
		docker run -itd --restart unless-stopped --name rustfs --network host \
			-e RUSTFS_ADDRESS=localhost:9002 \
			-e RUSTFS_CONSOLE_ENABLE=true \
			-e RUSTFS_ACCESS_KEY=rustfsadmin \
			-e RUSTFS_SECRET_KEY=rustfsadmin \
			"$RUSTFS_IMAGE"
		;;
	Linux)
		local cmd
		cmd=$(detect_runtime)
		echo "使用 $cmd 启动依赖容器..."
		$cmd run -td --restart=always --name redis --net=host \
			"$REDIS_IMAGE"
		$cmd run -td --restart=always --name etcd --net=host \
			"$ETCD_IMAGE"
		$cmd run -td --restart=always --name clickhouse-server --net=host \
			--ulimit nofile=262144:262144 \
			-v "$ROOT_DIR/config/clickhouse/config.d:/etc/clickhouse-server/config.d:ro" \
			-e CLICKHOUSE_PASSWORD=root_password \
			"$CLICKHOUSE_IMAGE"
		$cmd run -td --restart=always --name postgres --net=host \
			-e POSTGRES_PASSWORD=root_password \
			-e POSTGRES_USER=postgres \
			-e POSTGRES_DB=default \
			"$POSTGRES_IMAGE"
		$cmd run -td --restart=always --name rustfs --net=host \
			-e RUSTFS_ADDRESS=localhost:9002 \
			-e RUSTFS_CONSOLE_ENABLE=true \
			-e RUSTFS_ACCESS_KEY=rustfsadmin \
			-e RUSTFS_SECRET_KEY=rustfsadmin \
			"$RUSTFS_IMAGE"
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
	sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
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
