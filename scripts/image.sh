#!/usr/bin/env bash
# image.sh - 构建 zhongkui 镜像（normal core / slim core / web）
#
# 用法：
#   scripts/image.sh normal <version> [docker build 参数...]
#   scripts/image.sh slim   <version> [docker build 参数...]
#   scripts/image.sh web    [version] [docker build 参数...]
#
# 三种目标：
#   normal  后端完整镜像：镜像内交叉编译 zki（debian 基底，保留 shell 便于排障）
#   slim    后端最小镜像：从已构建的 normal 镜像提取 zki，免编译（scratch 基底）
#   web     前端镜像：nginx 基底，构建 web/ 控制台与 docs/ 文档站静态产物
#
# 环境变量：
#   VERSION         版本号；也可作为位置参数传入（web 可省略，默认 latest）
#   PLATFORMS       buildx 平台列表（逗号分隔）；不设置则用 docker build 构建本机架构
#   IMAGE           完整覆盖镜像 tag
#   NORMAL_IMAGE    slim 目标的 normal 镜像地址；默认取同版本号的 zhongkui:<version>
#
# 默认 tag：
#   normal  zhongkui:<version>
#   slim    zhongkui:<version>-slim
#   web     zhongkui-web:<version>
#   单平台（PLATFORMS 只含一个）时再追加 -<arch> 后缀，避免覆盖本机架构 tag
#
# 产物去向：未显式指定 --push / --load / --output 时，单平台自动 --load；
#   多平台产出 manifest list，必须自行追加 --push 或 --output。
#
# 示例：
#   scripts/image.sh normal v1.2.3
#   scripts/image.sh slim v1.2.3
#   scripts/image.sh web v1.2.3
#   PLATFORMS=linux/amd64,linux/arm64 scripts/image.sh normal v1.2.3 --push
#   NORMAL_IMAGE=registry.example.com/zk:v1.2.3 scripts/image.sh slim v1.2.3 --push
#   scripts/image.sh normal v1.2.3 --no-cache --progress=plain
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

usage() {
	awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
}

# ----------------------------------------------------------
# 目标与版本
# ----------------------------------------------------------
TARGET="${1:-}"
case "$TARGET" in
-h | --help | "")
	usage
	exit 0
	;;
normal | slim | web) shift ;;
*)
	echo "ERROR: 未知目标 '$TARGET'，可选 normal / slim / web" >&2
	usage >&2
	exit 1
	;;
esac

# 第二个位置参数是版本号；以 - 开头则视为 docker 参数（web 可完全省略版本）
case "${1:-}" in
-h | --help)
	usage
	exit 0
	;;
esac

# 位置参数优先于环境变量
VERSION="${VERSION:-}"
if [ $# -gt 0 ] && [ "${1#-}" = "$1" ]; then
	VERSION="$1"
	shift
fi

case "$TARGET" in
normal | slim)
	if [ -z "$VERSION" ]; then
		echo "ERROR: $TARGET 目标需要版本号，例如: scripts/image.sh $TARGET v1.2.3" >&2
		exit 1
	fi
	scripts/validate-semver.sh "$VERSION" || exit 1
	;;
web)
	VERSION="${VERSION:-latest}"
	;;
esac

# ----------------------------------------------------------
# 平台校验（Dockerfile 只支持 amd64 / arm64）
# ----------------------------------------------------------
PLATFORMS="${PLATFORMS:-}"
if [ -n "$PLATFORMS" ]; then
	IFS=',' read -ra platform_list <<<"$PLATFORMS"
	for p in "${platform_list[@]}"; do
		arch="${p#linux/}"
		arch="${arch%%/*}"
		case "$arch" in
		amd64 | arm64) ;;
		*)
			echo "ERROR: 仅支持 linux/amd64 与 linux/arm64，当前: $p" >&2
			exit 1
			;;
		esac
	done
fi

# ----------------------------------------------------------
# 默认 tag：单平台追加架构后缀，避免与本机架构 tag 相互覆盖
# ----------------------------------------------------------
case "$TARGET" in
normal) default_tag="zhongkui:${VERSION}" ;;
slim) default_tag="zhongkui:${VERSION}-slim" ;;
web) default_tag="zhongkui-web:${VERSION}" ;;
esac
if [ -n "$PLATFORMS" ] && [[ "$PLATFORMS" != *,* ]]; then
	platform_arch="${PLATFORMS#*/}"
	default_tag="${default_tag}-${platform_arch//\//-}"
fi
IMAGE="${IMAGE:-$default_tag}"

# ----------------------------------------------------------
# 构建命令：buildx（指定平台）/ docker build（本机架构）
# ----------------------------------------------------------
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

# ----------------------------------------------------------
# 各目标的 Dockerfile 与 build-arg
# ----------------------------------------------------------
case "$TARGET" in
normal)
	DOCKERFILE="deploys/containers/normal.Dockerfile"
	build_args=(
		--build-arg VERSION="$VERSION"
		--build-arg COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
		--build-arg BUILDER="$(whoami)"
	)
	;;
slim)
	DOCKERFILE="deploys/containers/slim.Dockerfile"
	# 默认取同版本号的 normal tag；单平台场景下追加架构后缀（与默认 tag 规则对齐）
	default_normal_tag="zhongkui:${VERSION}"
	if [ -n "$PLATFORMS" ] && [[ "$PLATFORMS" != *,* ]]; then
		platform_arch="${PLATFORMS#*/}"
		default_normal_tag="${default_normal_tag}-${platform_arch//\//-}"
	fi
	NORMAL_IMAGE="${NORMAL_IMAGE:-$default_normal_tag}"
	echo ">>> slim 目标：从 normal 镜像提取 zki -> ${NORMAL_IMAGE}"
	build_args=(
		--build-arg NORMAL_IMAGE="$NORMAL_IMAGE"
	)
	;;
web)
	DOCKERFILE="deploys/containers/web.Dockerfile"
	build_args=()
	;;
esac

"${build_cmd[@]}" "$@" \
	-f "$DOCKERFILE" \
	${build_args[@]+"${build_args[@]}"} \
	-t "$IMAGE" . || exit 1

echo "镜像已构建: ${IMAGE}（目标: ${TARGET}，平台: ${PLATFORMS:-构建机原生架构}）"
