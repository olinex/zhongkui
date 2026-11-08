#!/usr/bin/env bash
# dist.sh - 打包 release 制品与 sha256 校验和（供 Gitee Release 上传）
#
# 校验版本号后生成 dist/build.env（VERSION / COMMIT / BUILD_TIME / BUILDER），
# source 后复用同一份值执行 build-release，把 bin/zki 与许可证文件打包成
# zki_<version>_<os>_<arch>.tar.gz，并生成同名 .sha256 校验和。
#
# dist/build.env 只是本次打包的版本元数据载体，位于 dist/ 下，不进版本库；
# 镜像构建不复用该文件——Dockerfile 会在容器内通过 make version-env 重新生成。
#
# 用法：
#   scripts/dist.sh v1.2.3
#   scripts/dist.sh v1.2.3-beta.1
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

VERSION="${1:-${VERSION:-}}"

if [ -z "$VERSION" ]; then
	echo "ERROR: 请设置版本号，例如: scripts/dist.sh v1.2.3" >&2
	exit 1
fi

# 版本信息由 make version-env 统一生成到 dist/build.env（其中会校验语义化版本号），
# 再复用同一份值构建宿主机产物，避免 build.env 与二进制版本信息出现差异
make version-env VERSION="$VERSION" || exit 1
# shellcheck source=/dev/null
. ./dist/build.env

make build-release VERSION="$VERSION" COMMIT="$COMMIT" BUILD_TIME="$BUILD_TIME" BUILDER="$BUILDER"

version_number=${VERSION#v}
os=$(go env GOOS)
arch=$(go env GOARCH)
archive="zki_${version_number}_${os}_${arch}.tar.gz"

cp bin/zki LICENSE NOTICE THIRD_PARTY_LICENSES.md dist/
tar -C dist -czf "dist/$archive" zki LICENSE NOTICE THIRD_PARTY_LICENSES.md

if command -v sha256sum >/dev/null 2>&1; then
	sha256sum "dist/$archive" >"dist/$archive.sha256"
else
	shasum -a 256 "dist/$archive" >"dist/$archive.sha256"
fi

echo "已生成 dist/$archive 与 dist/$archive.sha256（版本环境文件 dist/build.env）"
