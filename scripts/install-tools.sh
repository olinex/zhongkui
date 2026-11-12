#!/usr/bin/env bash
# install-tools.sh - 安装开发辅助工具链（protobuf / TypeScript / libpcap / Go 开发工具）
#
# 把 protobuf-tools / ts-tools / pcap-tools 等目标的跨平台安装逻辑
# 从 Makefile 抽出来。每个子命令对应一个工具集，按需执行。
#
# 用法：
#   scripts/install-tools.sh protobuf   # 安装 protoc + Go 插件
#   scripts/install-tools.sh ts         # 安装 node/npm/pnpm
#   scripts/install-tools.sh pcap       # 安装 libpcap 开发库
#   scripts/install-tools.sh dev        # 安装 asynq / pkgsite / goconvey
#
# 退出码：0 成功；1 失败。
set -uo pipefail

cmd_protobuf() {
	case "$(uname -s)" in
	Darwin) HOMEBREW_NO_AUTO_UPDATE=1 brew install protobuf ;;
	Linux) apt install -y protobuf-compiler ;;
	MINGW* | MSYS* | CYGWIN*)
		echo "请从 https://github.com/protocolbuffers/protobuf/releases 手动安装 protobuf"
		;;
	*) echo "警告: 不支持的操作系统 '$(uname -s)'，请手动安装 protobuf。" ;;
	esac
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

	case "$(uname -s)" in
	Darwin)
		local shell_profile=~/.zshrc
		[ -f ~/.bash_profile ] && shell_profile=~/.bash_profile
		echo 'export PATH=$PATH:~/go/bin' >>"$shell_profile"
		;;
	*) echo 'export PATH=$PATH:~/go/bin' >>~/.bashrc ;;
	esac
}

cmd_ts() {
	case "$(uname -s)" in
	Darwin)
		if ! command -v node >/dev/null 2>&1; then
			HOMEBREW_NO_AUTO_UPDATE=1 brew install node
		else
			echo "node 已安装，跳过"
		fi
		;;
	Linux)
		if ! command -v node >/dev/null 2>&1; then
			curl -fsSL https://deb.nodesource.com/setup_26.x | bash - && apt install -y nodejs
		else
			echo "node 已安装，跳过"
		fi
		;;
	MINGW* | MSYS* | CYGWIN*)
		if ! command -v node >/dev/null 2>&1; then
			echo "请从 https://nodejs.org 安装 Node.js 后重新执行该目标" >&2
			exit 1
		fi
		;;
	*)
		if ! command -v node >/dev/null 2>&1; then
			echo "警告: 不支持的操作系统 '$(uname -s)'，请从 https://nodejs.org 手动安装 Node.js" >&2
			exit 1
		fi
		;;
	esac
	npm config set registry=https://registry.npmjs.org/
	npm install -g npm@10
	npm install -g pnpm@latest-10
	pnpm self-update
}

cmd_pcap() {
	case "$(uname -s)" in
	Darwin)
		xcode-select -p >/dev/null 2>&1 || xcode-select --install
		echo "libpcap 已包含在 macOS Xcode Command Line Tools 中"
		;;
	Linux) apt install -y libpcap-dev ;;
	MINGW* | MSYS* | CYGWIN*) echo "请从 https://npcap.com/dist/ 安装 Npcap SDK" ;;
	*) echo "警告: 不支持的操作系统 '$(uname -s)'，请手动安装 libpcap。" ;;
	esac
}

cmd_dev() {
	echo "安装 Go 开发辅助工具 (asynq / pkgsite / goconvey)..."
	go install github.com/hibiken/asynq/tools/asynq@latest || exit 1
	go install golang.org/x/pkgsite/cmd/pkgsite@latest || exit 1
	go install github.com/smartystreets/goconvey@v1.8.1 || exit 1
	echo "安装完成，可执行文件位于 $(go env GOPATH)/bin"
}

usage() {
	sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
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
