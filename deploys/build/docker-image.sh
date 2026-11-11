#!/usr/bin/env bash
#
# docker-image.sh - 构建后端镜像（支持按平台架构参数化）
#
# 编译器与运行环境在同一条 Dockerfile（deploys/build/core.Dockerfile）内完成：
# 步骤 1 交叉编译环境 → 步骤 2 运行环境 → 步骤 3 Go 包环境 → 步骤 4 编译制品 →
# 步骤 5 注入运行环境。源码在镜像构建阶段内编译，无需宿主机预编译二进制。
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
# 无论哪种平台组合，交叉编译都在构建环境阶段完成，不经 QEMU 模拟执行。
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
if [ "$VARIANT" != "slim" ] && [ "$VARIANT" != "normal" ] && [ "$VARIANT" != "all" ]; then
	echo "ERROR: VARIANT 只支持 normal / slim / all，当前值: $VARIANT" >&2
	exit 1
fi

PLATFORMS="${PLATFORMS:-}"

if [ -n "$PLATFORMS" ]; then
	IFS=',' read -ra platform_list <<<"$PLATFORMS"
	for p in "${platform_list[@]}"; do
		arch="${p#linux/}"
		arch="${arch%%/*}"
		case "$arch" in
			amd64 | arm64) ;;
			*) echo "ERROR: 仅支持 linux/amd64 与 linux/arm64，当前: $p" >&2; exit 1 ;;
		esac
	done
fi

# 默认 tag：normal 无后缀，slim 追加 -slim，all 追加 -all；
# 显式指定单平台（PLATFORMS）时再追加架构后缀，避免与构建机原生架构的镜像 tag 相互覆盖。
default_tag="zhongkui:${VERSION}"
case "$VARIANT" in
  slim) default_tag="${default_tag}-slim" ;;
  all)  default_tag="${default_tag}-all" ;;
esac
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
	--build-arg VERSION="$VERSION" \
	--build-arg COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
	--build-arg BUILDER="$(whoami)" \
	-t "$IMAGE" . || exit 1

echo "镜像已构建: ${IMAGE}（平台: ${PLATFORMS:-构建机原生架构}）"
