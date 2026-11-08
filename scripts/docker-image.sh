#!/usr/bin/env bash
#
# docker-image.sh - 构建后端镜像
#
# 版本信息通过 --build-arg 传入，由 Dockerfile 在构建阶段内部生成 dist/build.env，
# 不再依赖宿主机预先执行 make version-env；镜像与宿主机发布产物参数保持一致。
#
# 镜像变体由环境变量 VARIANT 控制（normal 或 slim，默认 normal）：
#   normal  基于 debian:bookworm-slim 的完整镜像，保留 shell 便于排障
#   slim    基于 scratch 的最小镜像，tag 自动追加 -slim 后缀
# 两种变体分两次执行本脚本分别构建；IMAGE 环境变量可完全覆盖默认 tag。
#
# 用法：
#   scripts/docker-image.sh v1.2.3                # 构建 zhongkui:v1.2.3
#   VARIANT=slim scripts/docker-image.sh v1.2.3   # 构建 zhongkui:v1.2.3-slim
#   IMAGE=zhongkui:latest scripts/docker-image.sh v1.2.3
#   scripts/docker-image.sh v1.2.3 --no-cache --progress=plain
#
# 参数：第一个参数为版本号（也可用环境变量 VERSION），其余参数原样传给 docker build。
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

VERSION="${1:-${VERSION:-}}"
if [ $# -gt 0 ]; then
	shift
fi

if [ -z "$VERSION" ]; then
	echo "ERROR: 请设置版本号，例如: scripts/docker-image.sh v1.2.3" >&2
	exit 1
fi

VARIANT="${VARIANT:-normal}"
if [ "$VARIANT" != "slim" ] && [ "$VARIANT" != "normal" ]; then
	echo "ERROR: VARIANT 只支持 slim 或 normal，当前值: $VARIANT" >&2
	exit 1
fi

if [ "$VARIANT" = "slim" ]; then
	IMAGE="${IMAGE:-zhongkui:${VERSION}-slim}"
else
	IMAGE="${IMAGE:-zhongkui:${VERSION}}"
fi

scripts/validate-semver.sh "$VERSION" || exit 1
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILDER=$(whoami)

docker build "$@" \
	--build-arg VERSION="$VERSION" \
	--build-arg COMMIT="$COMMIT" \
	--build-arg BUILDER="$BUILDER" \
	--build-arg IMAGE_VARIANT="$VARIANT" \
	-t "$IMAGE" . || exit 1

echo "镜像已构建: $IMAGE"
