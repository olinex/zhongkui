#!/usr/bin/env bash
# dependences-update.sh - 下载/更新随仓库提交的外部资源（构建依赖 + 数据资产）
#
# 统一管理所有"下载 → 校验 → 提交 git"的外部资源更新入口，避免脚本散落、
# 逻辑重复。产物存放在各自的目标位置（见下方各 target），均跟随 git。
#
# 用法：
#   scripts/dependences-update.sh                       # 更新所有资源
#   scripts/dependences-update.sh upx                   # 只更新 upx
#   scripts/dependences-update.sh p0f                   # 只更新 p0f 指纹库
#   scripts/dependences-update.sh upx 4.2.4             # 指定版本
#   scripts/dependences-update.sh p0f <upstream-url>    # 指定 p0f 上游源
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

DEPS_DIR="deploys/dependences"
mkdir -p "$DEPS_DIR"

# 默认版本 / 上游源（可被位置参数或环境变量覆盖）
UPX_VERSION="${UPX_VERSION:-4.2.4}"
P0F_UPSTREAM_URL="${P0F_UPSTREAM_URL:-https://raw.githubusercontent.com/peace-maker/p0f3-database/main/p0f.fp}"

TARGET="${1:-all}"
shift || true   # 吃掉 target，剩余的位置参数传给各 update_* 函数

# ============================================================
# upx：从 GitHub release 下载预编译静态二进制
# 产物：deploys/dependences/upx.linux-{amd64,arm64}.v<version>
# ============================================================
update_upx() {
	local version="${1:-$UPX_VERSION}"
	echo "=== 更新 upx 到 v${version} ==="

	for arch in amd64 arm64; do
		local url="https://github.com/upx/upx/releases/download/v${version}/upx-${version}-${arch}_linux.tar.xz"
		local tmpdir
		tmpdir=$(mktemp -d)

		echo "下载 ${url}"
		if ! curl -fsSL "$url" -o "$tmpdir/upx.tar.xz"; then
			echo "下载失败，请检查网络或版本号 v${version}" >&2
			rm -rf "$tmpdir"
			exit 1
		fi

		tar -xf "$tmpdir/upx.tar.xz" -C "$tmpdir" || {
			echo "解压失败" >&2
			rm -rf "$tmpdir"
			exit 1
		}

		# UPX release 的目录结构：upx-<version>-<arch>_linux/upx
		local upx_bin
		upx_bin=$(find "$tmpdir" -name upx -type f -perm -u+x | head -n1)
		if [ -z "$upx_bin" ]; then
			echo "未找到 upx 二进制" >&2
			rm -rf "$tmpdir"
			exit 1
		fi

		local out="$DEPS_DIR/upx.linux-${arch}.v${version}"
		cp -f "$upx_bin" "$out"
		chmod +x "$out"

		# 基本校验：应为 ELF 静态二进制
		if ! file "$out" | grep -q ELF; then
			echo "校验失败：$out 不是 ELF 二进制" >&2
			rm -rf "$tmpdir"
			exit 1
		fi

		echo "  -> $out ($(du -h "$out" | cut -f1))"
		rm -rf "$tmpdir"
	done

	echo "upx 更新完成"
}

# ============================================================
# p0f：从社区维护的 p0f3 数据库下载上游签名库，并运行指纹测试
# 产物：internal/network/huginn/p0f.fp
# ============================================================
update_p0f() {
	local url="${1:-$P0F_UPSTREAM_URL}"
	local p0f_fp="internal/network/huginn/p0f.fp"

	echo "=== 更新 p0f 指纹库 ==="
	echo "下载 ${url}"
	if ! curl -fsSL "$url" -o "$p0f_fp.tmp"; then
		echo "下载失败，请检查网络或 P0F_UPSTREAM_URL" >&2
		exit 1
	fi

	echo "校验格式（行数 / 首个 section）"
	wc -l "$p0f_fp.tmp"
	if ! grep -m1 '^\[tcp:request\]' "$p0f_fp.tmp" >/dev/null; then
		echo "校验失败：下载内容不含 [tcp:request] section，中止" >&2
		rm -f "$p0f_fp.tmp"
		exit 1
	fi

	mv "$p0f_fp.tmp" "$p0f_fp"
	echo "运行指纹测试验证"
	go test ./internal/network/detectors -run '^TestFingerprintFromPcap$' -v
}

# ============================================================
# 分发
# ============================================================
case "$TARGET" in
	all)
		update_upx "$@"
		update_p0f "$@"
		;;
	upx)
		update_upx "$@"
		;;
	p0f)
		update_p0f "$@"
		;;
	*)
		echo "用法: $0 [all|upx|p0f] [version|url]" >&2
		exit 1
		;;
esac

echo
echo "资源已更新，记得提交到 git"
