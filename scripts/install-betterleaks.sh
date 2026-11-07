#!/usr/bin/env bash
# install-betterleaks.sh - 跨平台安装 Betterleaks（本地密钥扫描器）
#
# 支持 macOS、Linux、Windows(Git Bash/MSYS2)，x86_64 与 arm64。
# 默认从 GitHub Release 下载对应平台的可执行文件，安装到 ~/.local/bin，
# 不依赖系统包管理器；macOS / Linuxbrew 用户可加 --brew 走 Homebrew。
#
# 用法：
#   scripts/install-betterleaks.sh                 # 下载预编译二进制
#   scripts/install-betterleaks.sh --brew          # 用 Homebrew 安装
#   scripts/install-betterleaks.sh --prefix DIR    # 指定安装目录
#   scripts/install-betterleaks.sh --force         # 重新安装已存在的版本
#
# 退出码：0 成功；1 安装失败；2 参数或环境错误。
set -uo pipefail

REPO=betterleaks/betterleaks
TOOL=betterleaks
PREFIX=${PREFIX:-$HOME/.local/bin}
USE_BREW=0
FORCE=0

usage() {
	sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
	case "$1" in
	--brew) USE_BREW=1 ;;
	--force) FORCE=1 ;;
	--prefix)
		[ $# -ge 2 ] || {
			echo "--prefix 需要一个目录参数" >&2
			exit 2
		}
		PREFIX=$2
		shift
		;;
	--prefix=*) PREFIX=${1#*=} ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "未知参数: $1" >&2
		usage >&2
		exit 2
		;;
	esac
	shift
done

# 1) 识别平台
case "$(uname -s)" in
Darwin) OS=darwin ;;
Linux) OS=linux ;;
MINGW* | MSYS* | CYGWIN*) OS=windows ;;
*)
	echo "暂不支持的平台: $(uname -s)" >&2
	exit 2
	;;
esac

case "$(uname -m)" in
x86_64 | amd64) ARCH=x64 ;;
arm64 | aarch64) ARCH=arm64 ;;
*)
	echo "暂不支持的架构: $(uname -m)" >&2
	exit 2
	;;
esac

echo "平台：$OS / $(uname -m)；安装目录：$PREFIX"

# 2) Homebrew 分支
if [ "$USE_BREW" -eq 1 ]; then
	if ! command -v brew >/dev/null 2>&1; then
		echo "--brew 需要 Homebrew，请先安装：https://brew.sh" >&2
		exit 2
	fi
	brew install betterleaks || exit 1
	path=$(command -v betterleaks 2>/dev/null || true)
	[ -n "$path" ] && echo "已安装：$path"
	exit 0
fi

command -v curl >/dev/null 2>&1 || {
	echo "缺少 curl" >&2
	exit 2
}
command -v tar >/dev/null 2>&1 || {
	echo "缺少 tar" >&2
	exit 2
}

if [ "$FORCE" -eq 0 ] && command -v "$TOOL" >/dev/null 2>&1; then
	echo "已存在：$(command -v "$TOOL")（--force 可重装）"
	exit 0
fi

# 3) 解析最新版本。GitHub 网页版重定向在国内网络下经常超时，改用 API。
tag=$(curl -fsSL --max-time 20 "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
	sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
case "$tag" in
"" | *releases/latest*)
	echo "获取最新版本失败，可尝试 --brew 或手动下载" >&2
	exit 1
	;;
esac

ext=tar.gz
[ "$OS" = windows ] && ext=zip
asset="${TOOL}_${tag#v}_${OS}_${ARCH}.${ext}"
url="https://github.com/$REPO/releases/download/$tag/$asset"

tmp=$(mktemp -d "${TMPDIR:-/tmp%/}/zki-install.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
pkg="$tmp/$asset"

echo "下载 $tag：$asset"
if ! curl -fsSL --connect-timeout 15 --retry 2 -o "$pkg" "$url"; then
	echo "下载失败：$url" >&2
	exit 1
fi

case "$asset" in
*.zip)
	command -v unzip >/dev/null 2>&1 || {
		echo "解压 .zip 需要 unzip" >&2
		exit 2
	}
	unzip -q "$pkg" -d "$tmp" || exit 1
	;;
*)
	tar xzf "$pkg" -C "$tmp" || exit 1
	;;
esac

bin=$(find "$tmp" -type f \( -name "$TOOL" -o -name "$TOOL.exe" \) | head -n 1)
if [ -z "$bin" ]; then
	echo "归档中未找到 $TOOL 可执行文件" >&2
	exit 1
fi

mkdir -p "$PREFIX" || exit 2
dest="$PREFIX/$TOOL"
[ "$OS" = windows ] && dest="$dest.exe"
if ! install -m 0755 "$bin" "$dest" 2>/dev/null; then
	cp "$bin" "$dest" && chmod 0755 "$dest" || {
		echo "安装到 $dest 失败" >&2
		exit 1
	}
fi

echo "已安装 $tag → $dest"
case ":$PATH:" in
*":$PREFIX:"*) : ;;
*) echo "提示：$PREFIX 不在 PATH 中，请加入后再运行 make secret-scan。" ;;
esac
