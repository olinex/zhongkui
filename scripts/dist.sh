#!/usr/bin/env bash
# dist.sh - 打包 release 制品与 sha256 校验和（供 Gitee Release 上传）
#
# 校验版本号后执行 build-release，再把 bin/zki 与许可证文件打包成
# zki_<version>_<os>_<arch>.tar.gz，并生成同名 .sha256 校验和。
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

scripts/validate-semver.sh "$VERSION" || exit 1

make build-release VERSION="$VERSION"

version_number=${VERSION#v}
os=$(go env GOOS)
arch=$(go env GOARCH)
archive="zki_${version_number}_${os}_${arch}.tar.gz"

mkdir -p dist
cp bin/zki LICENSE NOTICE THIRD_PARTY_LICENSES.md dist/
tar -C dist -czf "dist/$archive" zki LICENSE NOTICE THIRD_PARTY_LICENSES.md

if command -v sha256sum >/dev/null 2>&1; then
	sha256sum "dist/$archive" >"dist/$archive.sha256"
else
	shasum -a 256 "dist/$archive" >"dist/$archive.sha256"
fi

echo "已生成 dist/$archive 与 dist/$archive.sha256"
