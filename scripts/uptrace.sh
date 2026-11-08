#!/usr/bin/env bash
# uptrace.sh - OpenTelemetry 后端 (uptrace/uptrace) 工具集
#
# uptrace 是一个开源 APM 平台，支持 traces / metrics / logs 全链路监控。
# 依赖 ClickHouse + PostgreSQL + Redis（通过 scripts/dev-deps.sh start 启动）。
#   - Web UI:      http://localhost:14318
#   - OTLP gRPC:   localhost:4317
#   - OTLP HTTP:   http://localhost:4318
# 安装流程参考: https://uptrace.dev/get/hosted/install
# 项目地址: https://github.com/uptrace/uptrace
#
# 用法：
#   scripts/uptrace.sh install     # 下载并安装 uptrace 二进制
#   scripts/uptrace.sh config      # 生成并修改配置文件为本机连接信息
#   scripts/uptrace.sh init        # 初始化数据库（迁移 + 种子数据）
#   scripts/uptrace.sh start       # 后台启动 uptrace serve
#   scripts/uptrace.sh stop        # 停止后台运行的 uptrace serve
#   scripts/uptrace.sh clear       # 清除 uptrace-init 创建的数据库及数据
#   scripts/uptrace.sh uninstall   # 卸载 uptrace 二进制和配置文件
#   scripts/uptrace.sh cli-help    # 打印 uptrace CLI 常用命令参考
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

UPTRACE_VERSION="${UPTRACE_VERSION:-2.0.3}"
GOBIN="${GOBIN:-$(go env GOBIN)}"
UPTRACE_BIN="$GOBIN/uptrace"
UPTRACE_CONFIG="config/uptrace.yml"
UPTRACE_PID="/tmp/uptrace.pid"

# Linux 用 nerdctl，其他平台用 docker
if [ "$(uname -s)" = "Linux" ]; then
	UPTRACE_DOCKER_CMD="nerdctl"
else
	UPTRACE_DOCKER_CMD="docker"
fi

# sed 原地编辑参数：macOS 的 BSD sed 需要 -i ''，Linux GNU sed 为 -i
if [ "$(uname -s)" = "Darwin" ]; then
	SED_I=(sed -i '')
else
	SED_I=(sed -i)
fi

cmd_install() {
	if [ -f "$UPTRACE_BIN" ] && [ -x "$UPTRACE_BIN" ]; then
		echo "uptrace 已安装: $UPTRACE_BIN"
		"$UPTRACE_BIN" version || true
		return 0
	fi

	echo "=== 安装 uptrace v$UPTRACE_VERSION ==="
	mkdir -p "$GOBIN"

	local arch url
	case "$(uname -s)" in
	Darwin)
		arch=$(uname -m)
		if [ "$arch" = "arm64" ]; then
			url="https://github.com/uptrace/uptrace/releases/download/v$UPTRACE_VERSION/uptrace_darwin_arm64"
		else
			url="https://github.com/uptrace/uptrace/releases/download/v$UPTRACE_VERSION/uptrace_darwin_amd64"
		fi
		;;
	Linux)
		arch=$(uname -m)
		if [ "$arch" = "aarch64" ]; then
			url="https://github.com/uptrace/uptrace/releases/download/v$UPTRACE_VERSION/uptrace_linux_arm64"
		else
			url="https://github.com/uptrace/uptrace/releases/download/v$UPTRACE_VERSION/uptrace_linux_amd64"
		fi
		;;
	*)
		echo "不支持的平台，请手动下载: https://github.com/uptrace/uptrace/releases" >&2
		exit 1
		;;
	esac

	echo "  下载: $url"
	curl -sL -o "$UPTRACE_BIN" "$url" && chmod +x "$UPTRACE_BIN"
	echo "  已安装到 $UPTRACE_BIN"
	"$UPTRACE_BIN" version || true
}

cmd_config() {
	if [ ! -f "$UPTRACE_BIN" ]; then
		echo "uptrace 未安装，请先运行 scripts/uptrace.sh install" >&2
		exit 1
	fi

	if [ -f "$UPTRACE_CONFIG" ]; then
		echo "配置文件已存在: $UPTRACE_CONFIG，跳过生成"
		return 0
	fi

	echo "=== 生成配置文件 $UPTRACE_CONFIG ==="
	mkdir -p "$(dirname "$UPTRACE_CONFIG")"
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" config create

	"${SED_I[@]}" '/^pg:/,/^[a-z]/{s/^  user: .*/  user: postgres/;s/^  password: .*/  password: root_password/;}' "$UPTRACE_CONFIG"
	"${SED_I[@]}" '/^ch_cluster:/,/^[a-z]/{s/^          user: .*/          user: default/;s/^          password: .*/          password: ""/;}' "$UPTRACE_CONFIG"
	"${SED_I[@]}" 's|^    addr: :80|    addr: :14318|' "$UPTRACE_CONFIG"
	"${SED_I[@]}" 's|^  url: http://uptrace.local|  url: http://localhost:14318|' "$UPTRACE_CONFIG"
	"${SED_I[@]}" 's|^  dsn: http://project1_secret@uptrace.local?grpc=4317|  dsn: http://project1_secret@localhost:14318?grpc=4317|' "$UPTRACE_CONFIG"
	"${SED_I[@]}" '/^redis_cache:/,/^[a-z]/{s/^  password: .*/  password: ""/;}' "$UPTRACE_CONFIG"

	echo "  配置文件已生成 ($UPTRACE_CONFIG)"
}

cmd_init() {
	if [ ! -f "$UPTRACE_BIN" ]; then
		echo "uptrace 未安装，请先运行 scripts/uptrace.sh install" >&2
		exit 1
	fi
	if [ ! -f "$UPTRACE_CONFIG" ]; then
		echo "配置文件 $UPTRACE_CONFIG 不存在，请先运行 scripts/uptrace.sh config" >&2
		exit 1
	fi

	echo "=== 创建 ClickHouse 数据库 ==="
	if ! $UPTRACE_DOCKER_CMD exec clickhouse-server clickhouse-client -q "CREATE DATABASE IF NOT EXISTS uptrace" 2>/dev/null; then
		echo "  ClickHouse 不可用，请先运行 scripts/dev-deps.sh start" >&2
		exit 1
	fi
	echo "  数据库已就绪"
	echo ""
	echo "=== 创建 PostgreSQL 数据库 ==="
	$UPTRACE_DOCKER_CMD exec postgres psql -U postgres -c "CREATE DATABASE uptrace" 2>/dev/null || true
	echo "  数据库已就绪"
	echo ""
	echo "=== 初始化 PostgreSQL ==="
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" pg init
	echo ""
	echo "=== 运行 PostgreSQL 迁移 ==="
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" pg migrate
	echo ""
	echo "=== 初始化 ClickHouse ==="
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" ch init
	echo ""
	echo "=== 运行 ClickHouse 迁移 ==="
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" ch migrate
	echo ""
	echo "=== 创建 ClickHouse 分布式表 ==="
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" ch dist
	echo ""
	echo "=== 写入种子数据 ==="
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" db seed 2>/dev/null || echo "  (种子数据将在 serve 首次启动时加载)"
	echo ""
	echo "数据库初始化完成"
}

cmd_start() {
	if [ ! -f "$UPTRACE_BIN" ]; then
		echo "uptrace 未安装，请先运行 scripts/uptrace.sh install" >&2
		exit 1
	fi

	if [ -f "$UPTRACE_PID" ] && kill -0 "$(cat "$UPTRACE_PID")" 2>/dev/null; then
		echo "uptrace 已在运行 (PID $(cat "$UPTRACE_PID"))"
		return 0
	fi

	echo "=== 后台启动 uptrace serve ==="
	nohup "$UPTRACE_BIN" --config="$UPTRACE_CONFIG" serve >/tmp/uptrace.log 2>&1 &
	echo $! >"$UPTRACE_PID"
	sleep 3
	if kill -0 "$(cat "$UPTRACE_PID")" 2>/dev/null; then
		echo "uptrace 已启动 (PID $(cat "$UPTRACE_PID"))"
	else
		echo "uptrace 启动失败，查看日志:" >&2
		tail -20 /tmp/uptrace.log >&2
		rm -f "$UPTRACE_PID"
		exit 1
	fi

	echo ""
	echo "┌──────────────────────────────────────────────────────────┐"
	echo "│  uptrace Web UI (traces / metrics / logs 可视化)         │"
	echo "│    http://localhost:14318                                  │"
	echo "├──────────────────────────────────────────────────────────┤"
	echo "│  OTLP gRPC 端点 (应用上报地址)                              │"
	echo "│    localhost:4317                                          │"
	echo "├──────────────────────────────────────────────────────────┤"
	echo "│  OTLP HTTP 端点                                            │"
	echo "│    http://localhost:4318                                   │"
	echo "├──────────────────────────────────────────────────────────┤"
	echo "│  首次使用请在 Web UI 中创建项目，复制项目的 DSN            │"
	echo "│  (Data Source Name)，格式如下:                             │"
	echo "│    https://<token>@localhost:14318?grpc=4317             │"
	echo "└──────────────────────────────────────────────────────────┘"
	echo ""
	echo "在运行应用之前，设置以下环境变量以启用 OpenTelemetry 上报:"
	echo ""
	echo "  export OTEL_SERVICE_NAME=zki"
	echo "  export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317"
	echo '  export OTEL_EXPORTER_OTLP_HEADERS="uptrace-dsn=http://project1_secret@localhost:14318?grpc=4317"'
	echo "  export OTEL_EXPORTER_OTLP_COMPRESSION=gzip"
	echo "  export OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=DELTA"
	echo "  export OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION=BASE2_EXPONENTIAL_BUCKET_HISTOGRAM"
	echo ""
	echo "  # 然后正常运行应用:"
	echo "  go run cmd/zki/main.go run"
	echo ""
	echo "日志文件: tail -f /tmp/uptrace.log"
	echo "CLI 帮助: scripts/uptrace.sh cli-help"
	echo "停止服务: scripts/uptrace.sh stop"
}

cmd_stop() {
	if [ -f "$UPTRACE_PID" ] && kill -0 "$(cat "$UPTRACE_PID")" 2>/dev/null; then
		local pid
		pid=$(cat "$UPTRACE_PID")
		echo "=== 停止 uptrace (PID $pid) ==="
		kill "$pid" 2>/dev/null || true
		sleep 1
		if kill -0 "$pid" 2>/dev/null; then
			echo "强制终止..."
			kill -9 "$pid" 2>/dev/null || true
		fi
		rm -f "$UPTRACE_PID"
		echo "已停止"
	else
		echo "uptrace 未在运行"
		rm -f "$UPTRACE_PID"
	fi
}

cmd_clear() {
	if [ -f "$UPTRACE_PID" ] && kill -0 "$(cat "$UPTRACE_PID")" 2>/dev/null; then
		echo "uptrace 正在运行，请先执行 scripts/uptrace.sh stop" >&2
		exit 1
	fi
	if [ ! -f "$UPTRACE_BIN" ]; then
		echo "uptrace 未安装，请先运行 scripts/uptrace.sh install" >&2
		exit 1
	fi

	echo "=== 清除 uptrace 数据 ==="
	echo ""
	echo "--- 重置 ClickHouse schema ---"
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" ch reset
	echo ""
	echo "--- 重置 PostgreSQL schema ---"
	"$UPTRACE_BIN" --config="$UPTRACE_CONFIG" pg reset
	echo ""
	echo "清除完成，可通过 scripts/uptrace.sh init 重新初始化"
}

cmd_uninstall() {
	if [ -f "$UPTRACE_PID" ] && kill -0 "$(cat "$UPTRACE_PID")" 2>/dev/null; then
		echo "uptrace 正在运行，请先执行 scripts/uptrace.sh stop" >&2
		exit 1
	fi

	echo "=== 卸载 uptrace ==="
	rm -f "$UPTRACE_BIN" && echo "  已删除 $UPTRACE_BIN" || true
	rm -f "$UPTRACE_CONFIG" && echo "  已删除 $UPTRACE_CONFIG" || true
	rm -f "$UPTRACE_PID"
	echo "卸载完成"
}

cmd_cli_help() {
	cat <<'EOF'
uptrace CLI 常用命令:

  uptrace version         - 查看版本
  uptrace serve           - 前台启动服务 (调试用)
  uptrace config create   - 生成配置文件模板
  uptrace config validate - 验证配置文件

  数据库管理 (按官方文档顺序):
  uptrace pg init         - 初始化 PostgreSQL (仅首次)
  uptrace pg migrate      - 运行 PostgreSQL 迁移
  uptrace ch init         - 初始化 ClickHouse (仅首次)
  uptrace ch migrate      - 运行 ClickHouse 迁移
  uptrace ch dist         - 创建 ClickHouse 分布式表
  uptrace db seed         - 写入项目/用户种子数据

  在浏览器打开 http://localhost:14318 查看 Web UI
EOF
}

usage() {
	sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
install) cmd_install ;;
config) cmd_config ;;
init) cmd_init ;;
start) cmd_start ;;
stop) cmd_stop ;;
clear) cmd_clear ;;
uninstall) cmd_uninstall ;;
cli-help) cmd_cli_help ;;
-h | --help | "") usage ;;
*)
	echo "未知子命令: $1" >&2
	usage >&2
	exit 1
	;;
esac
