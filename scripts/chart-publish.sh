#!/usr/bin/env bash
# chart-publish.sh - 打包 Helm Chart 并推送到 OCI 仓库（支持无 helm 的 CI 节点）
#
# 用户无需 clone 源码即可安装：
#   helm install zhongkui oci://<registry>/<namespace>/charts/zhongkui \
#     --version <X.Y.Z> -n zhongkui --create-namespace -f values-prod.yaml
#
# 用法：
#   scripts/chart-publish.sh v1.2.3                     # 仅打包到 dist/
#   scripts/chart-publish.sh v1.2.3 --push              # 打包并推送（需已 helm registry login）
#   HELM_REGISTRY_USERNAME=xx HELM_REGISTRY_PASSWORD=yy \
#     scripts/chart-publish.sh v1.2.3 --push            # CI：自动登录并推送
#
# CI 节点没有 helm 时，脚本会按平台自动下载到缓存目录，无需预装：
#   可选环境变量：
#     HELM_VERSION        下载的 Helm 版本，默认 v4.2.4（v 前缀可省略）
#     HELM_BIN            直接指定已有 helm 可执行文件（优先于自动下载）
#     HELM_CACHE_DIR      helm 二进制缓存目录，默认 $TMPDIR/zhongkui-helm
#     HELM_DOWNLOAD_URL   自定义下载地址（内网镜像 / 对象存储），默认 get.helm.sh
#     HELM_SHA256         下载包 sha256（可选，用于校验）
#     HELM_OCI_REPO       推送目标目录
#                         默认 oci://zhongkui-cn-beijing.cr.volces.com/open-source/charts
#     HELM_REGISTRY_USERNAME / HELM_REGISTRY_PASSWORD
#                         同时设置时，推送前自动 helm registry login（密码经 stdin 传入）
#
# 版本映射（与镜像、二进制制品保持一致）：
#   Chart version  1.2.3   去掉 v 前缀，Helm 要求 SemVer
#   appVersion     v1.2.3  与镜像 tag 一致；values.yaml 中 image.tag 为空时取该值
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

die() {
	echo "ERROR: $*" >&2
	exit 1
}

info() {
	echo "$*"
}

usage() {
	awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
}

# ensure_helm 选择或下载 helm，结果写入全局 HELM
ensure_helm() {
	if [ -n "${HELM_BIN:-}" ]; then
		[ -x "$HELM_BIN" ] || die "HELM_BIN 不可执行: $HELM_BIN"
		HELM="$HELM_BIN"
		return 0
	fi
	if command -v helm >/dev/null 2>&1; then
		HELM=$(command -v helm)
		return 0
	fi

	local os arch version cache url archive tmpdir
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "不支持的系统 $(uname -s)，请通过 HELM_BIN 指定 helm 可执行文件" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "不支持的架构 $(uname -m)，请通过 HELM_BIN 指定 helm 可执行文件" ;;
	esac

	version="${HELM_VERSION:-v4.2.4}"
	case "$version" in
	v*) ;;
	*) version="v$version" ;;
	esac
	cache="${HELM_CACHE_DIR:-${TMPDIR:-/tmp}/zhongkui-helm}"
	mkdir -p "$cache" || die "创建 Helm 缓存目录失败: $cache"
	HELM="$cache/helm"

	if [ ! -x "$HELM" ]; then
		url="${HELM_DOWNLOAD_URL:-https://get.helm.sh/helm-${version}-${os}-${arch}.tar.gz}"
		archive="$cache/helm-${version}-${os}-${arch}.tar.gz"
		info "未找到 helm，下载 $url"
		if command -v curl >/dev/null 2>&1; then
			curl -fsSL --retry 3 -o "$archive" "$url" ||
				die "下载 helm 失败；可用 HELM_DOWNLOAD_URL 指向内网镜像"
		elif command -v wget >/dev/null 2>&1; then
			wget -q -O "$archive" "$url" ||
				die "下载 helm 失败；可用 HELM_DOWNLOAD_URL 指向内网镜像"
		else
			die "未找到 curl/wget，无法下载 helm；请预置 HELM_BIN"
		fi
		if [ -n "${HELM_SHA256:-}" ]; then
			local actual
			if command -v sha256sum >/dev/null 2>&1; then
				actual=$(sha256sum "$archive" | awk '{print $1}')
			else
				actual=$(shasum -a 256 "$archive" | awk '{print $1}')
			fi
			[ "$actual" = "$HELM_SHA256" ] ||
				die "helm 校验和不匹配：期望 $HELM_SHA256，实际 $actual"
		fi
		tmpdir=$(mktemp -d "$cache/extract.XXXXXX") || die "创建临时目录失败"
		tar -xzf "$archive" -C "$tmpdir" || die "解压 helm 失败"
		cp "$tmpdir/${os}-${arch}/helm" "$HELM" &&
			chmod 0755 "$HELM" || die "安装 helm 失败"
		rm -rf "$tmpdir" "$archive"
	fi
	"$HELM" version --short >/dev/null 2>&1 || die "helm 不可用: $HELM"
}

# login_if_needed 在提供凭据时执行非交互登录，密码不落盘、不出现在进程参数中
login_if_needed() {
	if [ -z "${HELM_REGISTRY_USERNAME:-}" ] && [ -z "${HELM_REGISTRY_PASSWORD:-}" ]; then
		return 0
	fi
	[ -n "${HELM_REGISTRY_USERNAME:-}" ] && [ -n "${HELM_REGISTRY_PASSWORD:-}" ] ||
		die "HELM_REGISTRY_USERNAME 与 HELM_REGISTRY_PASSWORD 必须同时设置"

	local registry="${OCI_REPO#oci://}"
	registry="${registry%%/*}"
	# 登录信息写入独立文件，避免 CI 节点 HOME 只读或污染全局 helm 配置
	if [ -z "${HELM_REGISTRY_CONFIG:-}" ]; then
		local cache="${HELM_CACHE_DIR:-${TMPDIR:-/tmp}/zhongkui-helm}"
		mkdir -p "$cache" || die "创建 Helm 缓存目录失败: $cache"
		export HELM_REGISTRY_CONFIG="$cache/registry.json"
	fi
	info "登录 OCI 仓库: $registry"
	printf '%s' "$HELM_REGISTRY_PASSWORD" |
		"$HELM" registry login "$registry" \
			--username "$HELM_REGISTRY_USERNAME" --password-stdin ||
		die "helm registry login 失败，请检查凭据与仓库地址"
}

VERSION=""
PUSH=0
for arg in "$@"; do
	case "$arg" in
	-h | --help)
		usage
		exit 0
		;;
	--push) PUSH=1 ;;
	-*) die "未知参数: $arg" ;;
	*)
		[ -z "$VERSION" ] || die "只能指定一个版本号"
		VERSION="$arg"
		;;
	esac
done

if [ -z "$VERSION" ]; then
	usage >&2
	exit 1
fi

scripts/validate-semver.sh "$VERSION" || exit 1

CHART_DIR=deploys/helm/zhongkui
CHART_VERSION="${VERSION#v}"
OCI_REPO="${HELM_OCI_REPO:-oci://zhongkui-cn-beijing.cr.volces.com/open-source/charts}"
OCI_REPO="${OCI_REPO%/}"

[ -d "$CHART_DIR" ] || die "缺少 Chart 目录: $CHART_DIR"
ensure_helm

mkdir -p dist || exit 1
PKG="dist/zhongkui-${CHART_VERSION}.tgz"
rm -f "$PKG"

"$HELM" package "$CHART_DIR" \
	--version "$CHART_VERSION" \
	--app-version "$VERSION" \
	--destination dist || die "helm package 失败"

[ -f "$PKG" ] || die "未生成 $PKG"
info "已生成 $PKG"

if [ "$PUSH" -eq 0 ]; then
	info "未推送；如需发布到 OCI 仓库，追加 --push"
	exit 0
fi

login_if_needed
"$HELM" push "$PKG" "$OCI_REPO" ||
	die "helm push 失败；请检查仓库权限或先执行 helm registry login ${OCI_REPO#oci://}"

info "已推送 ${OCI_REPO}/zhongkui:${CHART_VERSION}"
info "用户安装：helm install zhongkui ${OCI_REPO}/zhongkui --version ${CHART_VERSION} -n zhongkui --create-namespace"
