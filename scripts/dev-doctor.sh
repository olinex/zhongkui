#!/usr/bin/env bash
# dev-doctor.sh - 检查本地开发环境是否就绪（工具链 / 容器运行时 / 依赖容器 / 依赖端口）
#
# 用法：
#   scripts/dev-doctor.sh                # 全量检查
#   scripts/dev-doctor.sh --tools-only   # 仅检查工具链（刚安装完依赖时使用）
#
# 退出码：0 通过（允许告警）；1 存在失败项。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

TOOLS_ONLY=0
case "${1:-}" in
-h | --help)
	sed -n '2,7p' "$0" | sed 's/^# \{0,1\}//'
	exit 0
	;;
--tools-only)
	TOOLS_ONLY=1
	;;
"") ;;
*)
	echo "未知参数: $1" >&2
	exit 1
	;;
esac

PASS=0
WARN=0
FAIL=0

pass() { printf '[ OK ] %s\n' "$1"; PASS=$((PASS + 1)); }
warn() { printf '[WARN] %s\n' "$1"; WARN=$((WARN + 1)); }
fail() { printf '[FAIL] %s\n' "$1"; FAIL=$((FAIL + 1)); }
section() { printf '\n== %s ==\n' "$1"; }

# version_ge A B：A >= B 时返回 0（仅比较前 3 段数字）
version_ge() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		split(a, x, "."); split(b, y, ".")
		for (i = 1; i <= 3; i++) {
			if ((x[i] + 0) > (y[i] + 0)) exit 0
			if ((x[i] + 0) < (y[i] + 0)) exit 1
		}
		exit 0
	}'
}

port_open() {
	(exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

section "工具链"

if command -v git >/dev/null 2>&1; then
	pass "git $(git --version | awk '{print $3}')"
else
	fail "未安装 git；执行 scripts/install-tools.sh base"
fi

if command -v make >/dev/null 2>&1; then
	pass "make 已安装"
else
	fail "未安装 make；执行 scripts/install-tools.sh base"
fi

required_go=$(awk '$1 == "go" { print $2; exit }' go.mod)
if command -v go >/dev/null 2>&1; then
	go_version=$(go version | awk '{print $3}' | sed 's/^go//')
	if version_ge "$go_version" "$required_go"; then
		pass "go ${go_version}（go.mod 要求 ${required_go}）"
	else
		fail "go ${go_version} 低于 go.mod 要求的 ${required_go}；执行 scripts/install-tools.sh go"
	fi
else
	fail "未安装 go；执行 scripts/install-tools.sh go"
fi

if command -v node >/dev/null 2>&1; then
	node_version=$(node -v | sed 's/^v//')
	node_major=${node_version%%.*}
	if [ "${node_major:-0}" -ge 26 ] 2>/dev/null; then
		pass "node $node_version"
	else
		fail "node $node_version 低于要求的 26；执行 scripts/install-tools.sh ts"
	fi
else
	fail "未安装 node；执行 scripts/install-tools.sh ts"
fi

if command -v pnpm >/dev/null 2>&1; then
	pnpm_version=$(pnpm -v)
	pnpm_major=${pnpm_version%%.*}
	if [ "${pnpm_major:-0}" -ge 10 ] 2>/dev/null; then
		pass "pnpm $pnpm_version"
	else
		fail "pnpm $pnpm_version 低于要求的 10；执行 scripts/install-tools.sh ts"
	fi
else
	fail "未安装 pnpm；执行 scripts/install-tools.sh ts"
fi

if command -v protoc >/dev/null 2>&1; then
	pass "protoc $(protoc --version | awk '{print $2}')"
else
	warn "未安装 protoc（修改 api/*.proto 时需要）；执行 scripts/install-tools.sh protobuf"
fi

case "$(uname -s)" in
Darwin)
	if xcode-select -p >/dev/null 2>&1; then
		pass "libpcap（Xcode Command Line Tools 提供）"
	else
		fail "缺少 Xcode Command Line Tools；执行 scripts/install-tools.sh pcap"
	fi
	;;
*)
	if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libpcap; then
		pass "libpcap 开发库"
	else
		fail "缺少 libpcap 开发库；执行 scripts/install-tools.sh pcap"
	fi
	;;
esac

if [ -d web/node_modules ]; then
	pass "web/node_modules 已安装"
else
	warn "web/node_modules 不存在；首次前端构建前执行 scripts/web.sh install"
fi

section "容器运行时"

runtime=""
if command -v docker >/dev/null 2>&1; then
	runtime=docker
elif command -v nerdctl >/dev/null 2>&1; then
	runtime=nerdctl
fi

if [ -z "$runtime" ]; then
	fail "未找到 docker 或 nerdctl；执行 scripts/install-tools.sh docker"
elif ! "$runtime" info >/dev/null 2>&1; then
	fail "$runtime 无法连接守护进程；确认服务已启动，且当前用户已在 docker 组中并重新登录"
else
	pass "$runtime 可用"
	if [ "$runtime" = "docker" ]; then
		if docker compose version >/dev/null 2>&1; then
			pass "docker compose 插件可用"
		else
			fail "缺少 docker compose 插件；执行 scripts/install-tools.sh docker"
		fi
	fi
fi

if [ "$TOOLS_ONLY" -eq 0 ]; then
	section "依赖容器"
	missing=""
	if [ -n "$runtime" ] && "$runtime" info >/dev/null 2>&1; then
		running=$("$runtime" ps --format '{{.Names}}' 2>/dev/null)
		for c in redis etcd clickhouse-server postgres rustfs; do
			if printf '%s\n' "$running" | grep -qx "$c"; then
				pass "容器 $c 运行中"
			else
				missing="$missing $c"
			fi
		done
		if [ -n "$missing" ]; then
			fail "未运行的依赖容器:${missing}；执行 scripts/dev-containers.sh start"
		fi
	else
		warn "容器运行时不可用，跳过依赖容器检查"
	fi

	section "依赖端口"
	declare -a ports=(
		"5432 PostgreSQL"
		"9000 ClickHouse"
		"6379 Redis"
		"2379 etcd"
		"9002 S3"
	)
	for item in "${ports[@]}"; do
		read -r port name <<<"$item"
		if port_open "$port"; then
			pass "$name 127.0.0.1:$port 可连接"
		else
			fail "$name 127.0.0.1:$port 不可连接；确认依赖容器已启动"
		fi
	done

	section "项目配置"
	if [ -f config.toml ]; then
		pass "config.toml 存在"
	else
		warn "缺少 config.toml；按文档准备配置后才能启动服务"
	fi
fi

printf '\n== 结果 ==\n'
printf '通过 %d 项，告警 %d 项，失败 %d 项\n' "$PASS" "$WARN" "$FAIL"
if [ "$FAIL" -gt 0 ]; then
	echo "存在失败项，请按提示修复后重新执行 scripts/dev-doctor.sh" >&2
	exit 1
fi
if [ "$TOOLS_ONLY" -eq 1 ]; then
	echo "工具链检查通过。启动依赖容器后执行 scripts/dev-doctor.sh 进行全量检查。"
else
	echo "开发环境检查通过。"
fi
