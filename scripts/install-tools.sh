#!/usr/bin/env bash
# install-tools.sh - 安装开发环境依赖（基础工具 / Docker / Go / protobuf / TypeScript / libpcap / Go 开发工具）
#
# 用法：
#   scripts/install-tools.sh all        # 一次性安装完整开发环境（推荐）
#   scripts/install-tools.sh base       # git / make / curl / ca-certificates / gnupg
#   scripts/install-tools.sh docker     # Docker Engine + Compose 插件
#   scripts/install-tools.sh go         # Go 1.27.3
#   scripts/install-tools.sh protobuf   # protoc + Go 插件
#   scripts/install-tools.sh ts         # node/npm/pnpm
#   scripts/install-tools.sh pcap       # libpcap 开发库
#   scripts/install-tools.sh dev        # asynq / pkgsite / goconvey
#
# 需要提权的步骤自动调用 sudo；以 root 执行时直接生效。
# 退出码：0 成功；1 失败。
set -uo pipefail

GO_VERSION="${GO_VERSION:-1.27.3}"

# Go 模块代理与校验库：与 deploys/containers/normal.Dockerfile 保持一致。
# 优先级：显式环境变量 > go env 中已配置的非默认值 > 脚本内置镜像。
GO_DEFAULT_PROXY="https://proxy.golang.org,direct"
GO_MIRROR_PROXY="https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct"
GO_DEFAULT_SUMDB="sum.golang.org"
GO_MIRROR_SUMDB="sum.golang.google.cn"

resolve_go_env() {
	local key=$1 default=$2 mirror=$3 current
	current=$(go env "$key" 2>/dev/null || true)
	if [ -z "$current" ] || [ "$current" = "$default" ]; then
		echo "$mirror"
	else
		echo "$current"
	fi
}

export GOPROXY="${GOPROXY:-$(resolve_go_env GOPROXY "$GO_DEFAULT_PROXY" "$GO_MIRROR_PROXY")}"
export GOSUMDB="${GOSUMDB:-$(resolve_go_env GOSUMDB "$GO_DEFAULT_SUMDB" "$GO_MIRROR_SUMDB")}"

# 提权命令前缀；TARGET_USER 为实际使用开发环境的用户（sudo 场景下取 SUDO_USER）
if [ "$(id -u)" -eq 0 ]; then
	SUDO=""
	TARGET_USER="${SUDO_USER:-root}"
else
	SUDO="sudo"
	TARGET_USER="$(id -un)"
fi

# 以目标用户身份执行用户级安装（go install / npm 全局包），避免写入 /root
as_user() {
	if [ "$(id -u)" -eq 0 ] && [ "$TARGET_USER" != "root" ]; then
		$SUDO -u "$TARGET_USER" -H "$@"
	else
		"$@"
	fi
}

target_home() {
	local home
	home=$(getent passwd "$TARGET_USER" 2>/dev/null | cut -d: -f6)
	[ -n "$home" ] || home=$(eval echo "~$TARGET_USER")
	echo "$home"
}

go_bin() {
	command -v go 2>/dev/null || echo /usr/local/go/bin/go
}

require_go() {
	if ! command -v go >/dev/null 2>&1 && [ ! -x /usr/local/go/bin/go ]; then
		echo "ERROR: 未找到 go，请先执行 scripts/install-tools.sh go" >&2
		return 1
	fi
}

run_go() {
	local bin path
	bin=$(go_bin)
	path="$(dirname "$bin"):/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	as_user env PATH="$path" GOPROXY="$GOPROXY" GOSUMDB="$GOSUMDB" "$bin" "$@"
}

# go_install 执行 go install；失败时提示可覆盖的模块代理
go_install() {
	run_go install "$@" && return 0
	echo "ERROR: go install $* 失败；网络受限时可用环境变量覆盖代理后重试，例如：" >&2
	echo "  GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn scripts/install-tools.sh <子命令>" >&2
	return 1
}

# 把 Go 的 bin 目录写入目标用户的 shell 配置（幂等）
ensure_gobin_path() {
	local profile entry
	profile="$(target_home)/.bashrc"
	[ "$(uname -s)" = "Darwin" ] && profile="$(target_home)/.zshrc"
	entry='export PATH=$PATH:$HOME/go/bin'
	[ -f "$profile" ] || as_user touch "$profile"
	grep -qF '$HOME/go/bin' "$profile" 2>/dev/null ||
		printf '%s\n' "$entry" | $SUDO tee -a "$profile" >/dev/null
}

cmd_base() {
	case "$(uname -s)" in
	Darwin)
		HOMEBREW_NO_AUTO_UPDATE=1 brew install git make curl gnupg
		;;
	Linux)
		$SUDO apt-get update
		$SUDO apt-get install -y git make curl ca-certificates gnupg
		;;
	*)
		echo "警告: 不支持的操作系统 '$(uname -s)'，请手动安装 git / make / curl。" >&2
		return 1
		;;
	esac
}

cmd_docker() {
	case "$(uname -s)" in
	Darwin)
		if ! command -v docker >/dev/null 2>&1; then
			echo "请先安装 Docker Desktop: https://docs.docker.com/desktop/setup/install/mac-install/" >&2
			return 1
		fi
		docker compose version
		;;
	Linux)
		if command -v docker >/dev/null 2>&1 && $SUDO docker compose version >/dev/null 2>&1; then
			echo "Docker 与 Compose 插件已安装，跳过"
		else
			local repo
			. /etc/os-release
			case "${ID:-}" in
			debian) repo=debian ;;
			ubuntu) repo=ubuntu ;;
			*)
				echo "警告: 当前发行版 '${ID:-unknown}' 不在自动安装范围内，请按 https://docs.docker.com/engine/install/ 手动安装 Docker。" >&2
				return 1
				;;
			esac
			$SUDO install -m 0755 -d /etc/apt/keyrings
			$SUDO rm -f /etc/apt/keyrings/docker.gpg
			curl -fsSL "https://download.docker.com/linux/${repo}/gpg" |
				$SUDO gpg --dearmor -o /etc/apt/keyrings/docker.gpg
			$SUDO chmod a+r /etc/apt/keyrings/docker.gpg
			echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${repo} ${VERSION_CODENAME} stable" |
				$SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null
			$SUDO apt-get update
			$SUDO apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
		fi
		$SUDO systemctl enable --now docker
		if [ "$TARGET_USER" != "root" ]; then
			$SUDO usermod -aG docker "$TARGET_USER"
			echo "已将 $TARGET_USER 加入 docker 组，重新登录后生效"
		fi
		;;
	*)
		echo "警告: 不支持的操作系统 '$(uname -s)'。" >&2
		return 1
		;;
	esac
}

cmd_go() {
	case "$(uname -s)" in
	Darwin)
		HOMEBREW_NO_AUTO_UPDATE=1 brew install go
		;;
	Linux)
		local arch tmp archive
		arch=$(dpkg --print-architecture)
		export PATH="$PATH:/usr/local/go/bin"
		if command -v go >/dev/null 2>&1 && go version | grep -q "go${GO_VERSION}"; then
			echo "Go ${GO_VERSION} 已安装，跳过"
		else
			tmp=$(mktemp -d)
			archive="$tmp/go.tar.gz"
			echo "下载 Go ${GO_VERSION} (linux/${arch})..."
			if ! curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o "$archive"; then
				rm -rf "$tmp"
				echo "ERROR: 下载 Go 失败；可设置 GO_VERSION 指定其他版本。" >&2
				return 1
			fi
			$SUDO rm -rf /usr/local/go
			$SUDO tar -C /usr/local -xzf "$archive"
			rm -rf "$tmp"
			echo 'export PATH=$PATH:/usr/local/go/bin' | $SUDO tee /etc/profile.d/go.sh >/dev/null
		fi
		# 软链到 /usr/local/bin：/etc/profile.d 只对登录 shell 生效，软链保证当前 shell 与非登录 shell 立即可用
		$SUDO ln -sf /usr/local/go/bin/go /usr/local/bin/go
		$SUDO ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
		export PATH="$PATH:/usr/local/go/bin"
		go version
		;;
	*)
		echo "警告: 不支持的操作系统 '$(uname -s)'。" >&2
		return 1
		;;
	esac
}

cmd_protobuf() {
	require_go || return 1
	case "$(uname -s)" in
	Darwin) HOMEBREW_NO_AUTO_UPDATE=1 brew install protobuf ;;
	Linux) $SUDO apt-get install -y protobuf-compiler ;;
	*)
		echo "警告: 不支持的操作系统 '$(uname -s)'，请从 https://github.com/protocolbuffers/protobuf/releases 手动安装 protoc。" >&2
		return 1
		;;
	esac
	go_install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest || return 1
	go_install google.golang.org/protobuf/cmd/protoc-gen-go@latest || return 1
	ensure_gobin_path
}

cmd_ts() {
	if ! command -v node >/dev/null 2>&1; then
		case "$(uname -s)" in
		Darwin) HOMEBREW_NO_AUTO_UPDATE=1 brew install node ;;
		Linux)
			curl -fsSL https://deb.nodesource.com/setup_26.x | $SUDO bash - &&
				$SUDO apt-get install -y nodejs
			;;
		*)
			echo "请从 https://nodejs.org 安装 Node.js 后重新执行该目标" >&2
			return 1
			;;
		esac
	else
		echo "node 已安装，跳过"
	fi
	$SUDO npm config set registry=https://registry.npmjs.org/ --location=global
	$SUDO npm install -g npm@10
	$SUDO npm install -g pnpm@latest-10
	node -v
	pnpm -v
}

cmd_pcap() {
	case "$(uname -s)" in
	Darwin)
		xcode-select -p >/dev/null 2>&1 || xcode-select --install
		echo "libpcap 已包含在 macOS Xcode Command Line Tools 中"
		;;
	Linux) $SUDO apt-get install -y libpcap-dev ;;
	MINGW* | MSYS* | CYGWIN*) echo "请从 https://npcap.com/dist/ 安装 Npcap SDK" ;;
	*) echo "警告: 不支持的操作系统 '$(uname -s)'，请手动安装 libpcap。" ;;
	esac
}

cmd_dev() {
	require_go || return 1
	echo "安装 Go 开发辅助工具 (asynq / pkgsite / goconvey)..."
	go_install github.com/hibiken/asynq/tools/asynq@latest || return 1
	go_install golang.org/x/pkgsite/cmd/pkgsite@latest || return 1
	go_install github.com/smartystreets/goconvey@v1.8.1 || return 1
	ensure_gobin_path
	echo "安装完成，可执行文件位于 $(run_go env GOPATH)/bin"
}

# all - 按依赖顺序安装全部开发环境组件
cmd_all() {
	local step
	local -a steps=(base docker go protobuf ts pcap dev)
	for step in "${steps[@]}"; do
		echo
		echo "=== [$step] ==="
		if ! "cmd_${step}"; then
			echo "ERROR: 步骤 $step 失败，修复后重新执行 scripts/install-tools.sh all" >&2
			return 1
		fi
	done
	echo
	echo "开发环境依赖安装完成。"
	echo "若本次新加入 docker 组，请重新登录后执行：scripts/dev-containers.sh pull && scripts/dev-containers.sh start"
}

usage() {
	sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
all) cmd_all ;;
base) cmd_base ;;
docker) cmd_docker ;;
go) cmd_go ;;
protobuf) cmd_protobuf ;;
ts) cmd_ts ;;
pcap) cmd_pcap ;;
dev) cmd_dev ;;
-h | --help | "") usage ;;
*)
	echo "未知子命令: $1" >&2
	usage >&2
	exit 1
	;;
esac
