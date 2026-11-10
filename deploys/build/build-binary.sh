#!/usr/bin/env bash
#
# build-binary.sh - 在构建环境镜像内交叉编译多平台 linux 二进制
#
# 在仓库根目录执行（CI 由 execute@docker 把仓库挂载到 /root/workspace；本地经 docker run 挂载，
# 见 deploys/build/docker-image.sh 与 deploys/build/core.builder.Dockerfile 头注释）。
# 版本信息由环境变量注入（VERSION 必填；COMMIT / BUILDER 可选），统一经
# make version-env 生成 dist/build.env 后 source，保证多架构二进制版本元数据一致。
#
# 交叉编译：目标架构与构建机架构不一致时切换到对应 GNU 交叉工具链
# （amd64 ↔ arm64 双向，工具链由构建环境镜像预装），cgo 全程原生编译、不经 QEMU。
#
# 用法：
#   VERSION=v1.2.3 deploys/build/build-binary.sh              # amd64 + arm64
#   VERSION=v1.2.3 deploys/build/build-binary.sh arm64        # 只构建指定架构
#
# 产物：dist/bin/zki-linux-<arch>；设置 HOST_UID/HOST_GID 时产物 chown 回宿主机用户。
# 退出码：0 成功；1 失败。
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR" || exit 1

VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
	echo "ERROR: 请设置版本号，例如: VERSION=v1.2.3 deploys/build/build-binary.sh" >&2
	exit 1
fi

make version-env VERSION="$VERSION" COMMIT="${COMMIT:-unknown}" BUILDER="${BUILDER:-docker}"
# shellcheck source=/dev/null
set -a; . ./dist/build.env; set +a

buildarch=$(go env GOHOSTARCH)
arches=("$@")
if [ "${#arches[@]}" -eq 0 ]; then
	arches=(amd64 arm64)
fi

for goarch in "${arches[@]}"; do
	unset CC CXX CGO_CFLAGS 2>/dev/null || true
	if [ "$goarch" != "$buildarch" ]; then
		case "${buildarch}-${goarch}" in
			amd64-arm64)
				export CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++
				export CGO_CFLAGS="-I/usr/include ${CGO_CFLAGS:-}"
				export CGO_LDFLAGS="-L/usr/lib/aarch64-linux-gnu"
				;;
			arm64-amd64)
				export CC=x86_64-linux-gnu-gcc CXX=x86_64-linux-gnu-g++
				export CGO_CFLAGS="-I/usr/include ${CGO_CFLAGS:-}"
				export CGO_LDFLAGS="-L/usr/lib/x86_64-linux-gnu"
				;;
			*)
				echo "ERROR: 不支持从 ${buildarch} 交叉编译到 ${goarch}（仅支持 amd64 ↔ arm64）" >&2
				exit 1
				;;
		esac
	else
		unset CGO_LDFLAGS 2>/dev/null || true
	fi
	echo ">>> 构建 linux/${goarch}（构建架构 ${buildarch}${CC:+，CC=${CC}}）"
	GOOS=linux GOARCH="$goarch" make build-image OUT="dist/bin/zki-linux-${goarch}"
done

if [ -n "${HOST_UID:-}" ]; then
	chown -R "${HOST_UID}:${HOST_GID:-$HOST_UID}" dist/bin dist/build.env
fi

ls -l dist/bin/
