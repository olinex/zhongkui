#!/usr/bin/env bash
# release.sh - 创建发布标签（在 master 上，要求 CHANGELOG 与 web/package.json 已对齐）
#
# 前置检查：当前分支为 master、工作区干净、CHANGELOG 存在对应版本段、
# web/package.json 的 version 已对齐、版本标签未创建。通过后运行 make ci，
# 最后创建 annotated tag。
#
# 用法：
#   scripts/release.sh v1.2.3
#   scripts/release.sh v1.2.3-rc.1
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

VERSION="${1:-${VERSION:-}}"

if [ -z "$VERSION" ]; then
	echo "ERROR: 请设置版本号，例如: scripts/release.sh v1.2.3" >&2
	exit 1
fi

scripts/validate-semver.sh "$VERSION" || exit 1

if [ "$(git rev-parse --abbrev-ref HEAD)" != "master" ]; then
	echo "ERROR: 发布必须在 master 分支上进行，当前为 $(git rev-parse --abbrev-ref HEAD)" >&2
	exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
	echo "ERROR: 工作区不干净，请先提交或清理改动" >&2
	exit 1
fi

if git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null 2>&1; then
	echo "ERROR: 标签 $VERSION 已存在" >&2
	exit 1
fi

version_number=${VERSION#v}
grep -Fq "## [$version_number] - " CHANGELOG.md || {
	echo "ERROR: CHANGELOG.md 缺少 '## [$version_number] - YYYY-MM-DD' 版本段" >&2
	exit 1
}
grep -Fq "\"version\": \"$version_number\"" web/package.json || {
	echo "ERROR: web/package.json 的 version 未对齐 $version_number" >&2
	exit 1
}

make ci
git tag -a "$VERSION" -m "zhongkui $VERSION"
echo "已创建标签 $VERSION；推送: git push origin master --follow-tags"
