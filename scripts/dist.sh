#!/usr/bin/env bash
# dist.sh - 打包 Release 制品（供 Gitee Release 上传）
#
# 制品结构：
#   core/zki_linux_amd64           linux/amd64 二进制
#   core/zki_linux_arm64           linux/arm64 二进制
#   web/html/                      管理端静态产物（web/ 构建结果）
#   web/html/docs/                 文档站静态产物（docs/ VitePress 构建结果）
#   deploys/binary/                二进制部署示例（配置、systemd unit、nginx 模板、OTel）
#   LICENSE / NOTICE / THIRD_PARTY_LICENSES.md / README.md / CHANGELOG.md
#
# 二进制获取方式：
#   1. 预置：BIN_AMD64 / BIN_ARM64 同时指定已有二进制；或放在
#      dist/bin/zki_linux_amd64 与 dist/bin/zki_linux_arm64。
#   2. 自动构建：未预置时用 docker buildx 调用
#      deploys/containers/normal.Dockerfile 的 binary-export 阶段分别构建两个架构。
#
# 基础镜像：
#   默认使用 Dockerfile 内的 GOLANG_IMAGE / RUNTIME_IMAGE。
#   如需走镜像加速源，由调用方通过环境变量注入完整镜像地址，脚本不内置
#   任何仓库地址：
#     GOLANG_IMAGE=<registry>/library/golang:<tag> \
#     RUNTIME_IMAGE=<registry>/library/debian:<tag> scripts/dist.sh v1.2.3
#
# 版本信息由 make version-env 统一生成到 dist/build.env（VERSION / COMMIT /
# BUILD_TIME / BUILDER），再复用到二进制构建与前端构建标识。
#
# 用法：
#   scripts/dist.sh v1.2.3
#   scripts/dist.sh v1.2.3-beta.1
#   BIN_AMD64=/path/zki_amd64 BIN_ARM64=/path/zki_arm64 scripts/dist.sh v1.2.3
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

die() {
	echo "ERROR: $*" >&2
	exit 1
}

VERSION="${1:-${VERSION:-}}"
[ -n "$VERSION" ] || die "请设置版本号，例如: scripts/dist.sh v1.2.3"

# 版本元数据：复用 make version-env 的校验与统一取值，避免二进制与前端标识不一致
make version-env VERSION="$VERSION" || exit 1
# shellcheck source=/dev/null
. ./dist/build.env

# ----------------------------------------------------------
# 准备暂存目录
# ----------------------------------------------------------
STAGE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/zki-dist-XXXXXX") || die "创建暂存目录失败"
trap 'rm -rf "$STAGE_DIR"' EXIT
mkdir -p "$STAGE_DIR/core" "$STAGE_DIR/web/html" "$STAGE_DIR/deploys" || exit 1

for f in LICENSE NOTICE THIRD_PARTY_LICENSES.md README.md CHANGELOG.md; do
	[ -f "$f" ] || die "缺少文件: $f"
	cp "$f" "$STAGE_DIR/" || exit 1
done

[ -d deploys/binary ] || die "缺少 deploys/binary"
cp -a deploys/binary "$STAGE_DIR/deploys/" || exit 1

# ----------------------------------------------------------
# 获取 linux/amd64 与 linux/arm64 二进制
# ----------------------------------------------------------
BIN_AMD64_WAS_SET="${BIN_AMD64:+1}"
BIN_ARM64_WAS_SET="${BIN_ARM64:+1}"
BIN_AMD64="${BIN_AMD64:-dist/bin/zki_linux_amd64}"
BIN_ARM64="${BIN_ARM64:-dist/bin/zki_linux_arm64}"

# 基础镜像地址由调用方按需注入；未注入时沿用 Dockerfile 的默认值。
DOCKERFILE=deploys/containers/normal.Dockerfile
[ -f "$DOCKERFILE" ] || die "缺少 Dockerfile: $DOCKERFILE"

BUILD_ARGS=(
	--build-arg "VERSION=$VERSION"
	--build-arg "COMMIT=$COMMIT"
	--build-arg "BUILDER=$BUILDER"
)
if [ -n "${GOLANG_IMAGE:-}" ]; then
	BUILD_ARGS+=(--build-arg "GOLANG_IMAGE=$GOLANG_IMAGE")
fi
if [ -n "${RUNTIME_IMAGE:-}" ]; then
	BUILD_ARGS+=(--build-arg "RUNTIME_IMAGE=$RUNTIME_IMAGE")
fi

build_binary_with_docker() {
	local arch="$1" dest="$2" tmp
	command -v docker >/dev/null 2>&1 || die "未找到 docker，无法构建 $arch 二进制；请安装 Docker 或通过 BIN_AMD64/BIN_ARM64 预置二进制"
	docker buildx version >/dev/null 2>&1 || die "docker buildx 不可用，无法构建 $arch 二进制"
	tmp=$(mktemp -d "${TMPDIR:-/tmp}/zki-bin-$arch-XXXXXX") || return 1
	docker buildx build \
		--platform "linux/$arch" \
		--target binary-export \
		-f "$DOCKERFILE" \
		"${BUILD_ARGS[@]}" \
		--output "type=local,dest=$tmp" \
		. || {
		rm -rf "$tmp"
		return 1
	}
	[ -f "$tmp/zki" ] || {
		rm -rf "$tmp"
		die "binary-export 未产出 zki（$arch）"
	}
	mkdir -p "$(dirname "$dest")" || return 1
	install -m 0755 "$tmp/zki" "$dest" || {
		rm -rf "$tmp"
		return 1
	}
	rm -rf "$tmp"
	echo "已构建 $dest"
}

if [ ! -f "$BIN_AMD64" ] || [ ! -f "$BIN_ARM64" ]; then
	# 显式指定时要求两个文件都存在，避免只给一个时静默混用预置与自动构建
	if [ -n "$BIN_AMD64_WAS_SET" ] || [ -n "$BIN_ARM64_WAS_SET" ]; then
		die "通过 BIN_AMD64/BIN_ARM64 指定的二进制必须同时存在: $BIN_AMD64 / $BIN_ARM64"
	fi
	# 自动构建依赖仓库中的 RAG 快照，先按当前源码重新烘焙
	make rag-snapshot || exit 1
	[ -f "$BIN_AMD64" ] || build_binary_with_docker amd64 "$BIN_AMD64" || exit 1
	[ -f "$BIN_ARM64" ] || build_binary_with_docker arm64 "$BIN_ARM64" || exit 1
fi

[ -f "$BIN_AMD64" ] || die "缺少 linux/amd64 二进制: $BIN_AMD64"
[ -f "$BIN_ARM64" ] || die "缺少 linux/arm64 二进制: $BIN_ARM64"
install -m 0755 "$BIN_AMD64" "$STAGE_DIR/core/zki_linux_amd64" || exit 1
install -m 0755 "$BIN_ARM64" "$STAGE_DIR/core/zki_linux_arm64" || exit 1

# ----------------------------------------------------------
# 构建管理端与文档站静态产物
# ----------------------------------------------------------
command -v pnpm >/dev/null 2>&1 || die "未找到 pnpm，请先运行 scripts/install-tools.sh ts"

(cd web && pnpm install --frozen-lockfile) || die "web 依赖安装失败"
(
	cd web &&
		VITE_ZHONGKUI_BASE_URL=/api/v1 \
			VITE_DOCS_URL=/docs/ \
			VITE_BUILD_TAG="$VERSION" \
			VITE_BUILD_COMMIT="$COMMIT" \
			pnpm build-only
) || die "管理端构建失败"

(cd docs && pnpm install --frozen-lockfile) || die "docs 依赖安装失败"
(
	cd docs &&
		DOCS_BASE=/docs/ \
			DOCS_CONSOLE_URL=/console \
			DOCS_LAST_UPDATED=false \
			pnpm build
) || die "文档站构建失败"

[ -d web/dist ] || die "缺少 web/dist 构建产物"
[ -d docs/.vitepress/dist ] || die "缺少 docs/.vitepress/dist 构建产物"

cp -a web/dist/. "$STAGE_DIR/web/html/" || exit 1
mkdir -p "$STAGE_DIR/web/html/docs" || exit 1
cp -a docs/.vitepress/dist/. "$STAGE_DIR/web/html/docs/" || exit 1

# ----------------------------------------------------------
# 打包与摘要
# ----------------------------------------------------------
archive="zki-${VERSION}.tar.gz"
mkdir -p dist || exit 1
rm -f "dist/$archive"

tar -C "$STAGE_DIR" -czf "dist/$archive" \
	core \
	web \
	deploys \
	LICENSE NOTICE THIRD_PARTY_LICENSES.md README.md CHANGELOG.md || exit 1

if command -v sha256sum >/dev/null 2>&1; then
	hash=$(sha256sum "dist/$archive" | awk '{print $1}')
else
	hash=$(shasum -a 256 "dist/$archive" | awk '{print $1}')
fi

echo "已生成 dist/$archive"
echo "sha256: $hash"
