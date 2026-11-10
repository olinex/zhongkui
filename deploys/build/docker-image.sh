#!/usr/bin/env bash
#
# docker-image.sh - 构建后端镜像（支持按平台架构参数化）
#
# 镜像不再内嵌编译：zki 二进制由构建环境镜像（deploys/build/core.builder.Dockerfile）
# 预编译到 dist/bin/zki-linux-<arch>。本脚本在镜像构建前检查目标架构二进制是否齐备，
# 缺失时自动经构建环境镜像补齐（多架构二进制同批编译、版本元数据一致）。
# 注意：二进制按存在性复用，源码变更后需删除 dist/bin/ 或设 REBUILD=1 强制重编译。
#
# 镜像变体由环境变量 VARIANT 控制（normal 或 slim，默认 normal）：
#   normal  基于 debian:bookworm-slim 的完整镜像，保留 shell 便于排障
#   slim    基于 scratch 的最小镜像，tag 自动追加 -slim 后缀
# 两种变体分两次执行本脚本分别构建；IMAGE 环境变量可完全覆盖默认 tag。
#
# 目标平台由环境变量 PLATFORMS 控制（docker buildx 的 --platform 语法，逗号分隔）：
#   未设置  跟随构建机原生架构，走 docker build
#   单平台  走 docker buildx，默认追加 --load 载入本地，tag 自动追加架构后缀
#   多平台  走 docker buildx 生成 manifest list，需自行追加 --push 或 --output
#
# 构建环境镜像由环境变量 BUILDER_IMAGE 指定（默认 zhongkui-builder:go1.27.3），
# 本地不存在时由 deploys/build/core.builder.Dockerfile 自动构建；CI 流水线使用 registry 中的镜像。
#
# 用法：
#   deploys/build/docker-image.sh v1.2.3                                # 本机架构，zhongkui:v1.2.3
#   VARIANT=slim deploys/build/docker-image.sh v1.2.3                     # zhongkui:v1.2.3-slim
#   PLATFORMS=linux/arm64 deploys/build/docker-image.sh v1.2.3            # zhongkui:v1.2.3-arm64
#   PLATFORMS=linux/amd64 deploys/build/docker-image.sh v1.2.3            # zhongkui:v1.2.3-amd64
#   PLATFORMS=linux/amd64,linux/arm64 deploys/build/docker-image.sh v1.2.3 --push
#   IMAGE=zhongkui:latest deploys/build/docker-image.sh v1.2.3
#   deploys/build/docker-image.sh v1.2.3 --no-cache --progress=plain
#
# 参数：第一个参数为版本号（也可用环境变量 VERSION），其余参数原样传给 docker build(x)。
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR" || exit 1

VERSION="${1:-${VERSION:-}}"
if [ $# -gt 0 ]; then
	shift
fi

if [ -z "$VERSION" ]; then
	echo "ERROR: 请设置版本号，例如: deploys/build/docker-image.sh v1.2.3" >&2
	exit 1
fi

VARIANT="${VARIANT:-normal}"
if [ "$VARIANT" != "slim" ] && [ "$VARIANT" != "normal" ]; then
	echo "ERROR: VARIANT 只支持 slim 或 normal，当前值: $VARIANT" >&2
	exit 1
fi

PLATFORMS="${PLATFORMS:-}"
BUILDER_IMAGE="${BUILDER_IMAGE:-zhongkui-builder:go1.27.3}"

# 目标平台 → 需要的 GOARCH 列表（未指定平台时取构建机原生架构）
host_goarch=
case "$(uname -m)" in
	x86_64) host_goarch=amd64 ;;
	aarch64 | arm64) host_goarch=arm64 ;;
	*) echo "ERROR: 不支持的构建机架构: $(uname -m)" >&2; exit 1 ;;
esac

arches=()
if [ -n "$PLATFORMS" ]; then
	IFS=',' read -ra platform_list <<<"$PLATFORMS"
	for p in "${platform_list[@]}"; do
		arch="${p#linux/}"
		arch="${arch%%/*}"
		case "$arch" in
			amd64 | arm64) ;;
			*) echo "ERROR: 仅支持 linux/amd64 与 linux/arm64，当前: $p" >&2; exit 1 ;;
		esac
		arches+=("$arch")
	done
else
	arches+=("$host_goarch")
fi

# 补齐缺失架构的二进制：经构建环境镜像执行 build-binary.sh（容器内 root 产出，
# HOST_UID/HOST_GID 让产物 chown 回当前用户；Go 模块/编译缓存走命名卷复用）。
# REBUILD=1 无视已有产物强制重编译。
missing=()
for arch in "${arches[@]}"; do
	if [ "${REBUILD:-}" = "1" ] || [ ! -f "dist/bin/zki-linux-${arch}" ]; then
		missing+=("$arch")
	fi
done
if [ "${#missing[@]}" -gt 0 ]; then
	scripts/validate-semver.sh "$VERSION" || exit 1
	if ! docker image inspect "$BUILDER_IMAGE" >/dev/null 2>&1; then
		echo ">>> 构建环境镜像 $BUILDER_IMAGE 不存在，本地构建"
		docker build -f deploys/build/core.builder.Dockerfile -t "$BUILDER_IMAGE" . || exit 1
	fi
	echo ">>> 经构建环境镜像编译缺失架构: ${missing[*]}"
	docker run --rm \
		-v "$ROOT_DIR:/root/workspace" \
		-v zhongkui-go-mod:/go/pkg/mod \
		-v zhongkui-go-build:/root/.cache/go-build \
		-e VERSION="$VERSION" \
		-e COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
		-e BUILDER="$(whoami)" \
		-e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
		"$BUILDER_IMAGE" deploys/build/build-binary.sh "${missing[@]}" || exit 1
fi

# 默认 tag：normal 无后缀，slim 追加 -slim；显式指定单平台时再追加架构后缀，
# 避免与构建机原生架构的镜像 tag 相互覆盖（linux/arm/v7 -> arm-v7）。
default_tag="zhongkui:${VERSION}"
if [ "$VARIANT" = "slim" ]; then
	default_tag="${default_tag}-slim"
fi
if [ -n "$PLATFORMS" ] && [[ "$PLATFORMS" != *,* ]]; then
	platform_arch="${PLATFORMS#*/}"
	default_tag="${default_tag}-${platform_arch//\//-}"
fi
IMAGE="${IMAGE:-$default_tag}"

# 是否已显式指定产物去向；缺省时单平台自动 --load，多平台必须由调用方指定
has_output=0
for arg in "$@"; do
	case "$arg" in
		--push | --load | --output | --output=* | -o) has_output=1 ;;
	esac
done

if [ -n "$PLATFORMS" ]; then
	build_cmd=(docker buildx build --platform "$PLATFORMS")
	if [ "$has_output" -eq 0 ]; then
		if [[ "$PLATFORMS" == *,* ]]; then
			echo "ERROR: 多平台构建产出 manifest list，无法 --load 到本地，请追加 --push 或 --output" >&2
			exit 1
		fi
		build_cmd+=(--load)
	fi
else
	build_cmd=(docker build)
fi

scripts/validate-semver.sh "$VERSION" || exit 1

"${build_cmd[@]}" "$@" \
	-f deploys/build/core.Dockerfile \
	--build-arg IMAGE_VARIANT="$VARIANT" \
	-t "$IMAGE" . || exit 1

echo "镜像已构建: ${IMAGE}（平台: ${PLATFORMS:-构建机原生架构}）"
