#!/usr/bin/env bash
# check.sh - 代码检查统一入口
#
# 封装 gofmt、构建、vet、测试、注释、许可证、文档与部署配置漂移检查；
# 文档生成等构建输入维护仍由 make docs / make rag-snapshot 负责。
#
# 用法：
#   scripts/check.sh fmt          # gofmt 格式检查
#   scripts/check.sh build        # go build ./...
#   scripts/check.sh vet          # go vet ./...
#   scripts/check.sh test [args]  # go test ./...，额外参数原样传入
#   scripts/check.sh lint         # 注释完备性检查
#   scripts/check.sh license      # 第三方依赖许可证门禁
#   scripts/check.sh docs         # 文档生成物与 RAG 快照漂移检查
#   scripts/check.sh config       # 部署配置样例漂移检查（需要 helm）
#
# 退出码：0 通过；非零表示存在失败项。
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 2

# macOS 上 gopacket/pcap (cgo) 会重复链接 -lpcap，抑制 ld 警告（与 Makefile 保持一致）
if [ "$(uname -s)" = "Darwin" ]; then
	export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-Wl,-no_warn_duplicate_libraries"
fi

usage() {
	sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-}" in
-h | --help | "")
	usage
	exit 0
	;;
esac

sub="$1"
shift

case "$sub" in
fmt)
	# git ls-files 会列出「已跟踪但在工作区被删除」的文件，直接交给 gofmt 会因
	# lstat 失败而中断；先过滤出磁盘上仍存在的文件再检查。
	file_list=$(mktemp)
	trap 'rm -f "$file_list"' EXIT
	{
		git ls-files -z '*.go'
		git ls-files -z --others --exclude-standard '*.go'
	} | while IFS= read -r -d '' file; do
		[ -f "$file" ] && printf '%s\0' "$file"
	done >"$file_list"

	unformatted=""
	if [ -s "$file_list" ]; then
		unformatted=$(xargs -0 gofmt -l <"$file_list")
	fi
	if [ -n "$unformatted" ]; then
		echo "以下文件未通过 gofmt:"
		echo "$unformatted"
		exit 1
	fi
	;;
build)
	go build ./...
	;;
vet)
	go vet ./...
	;;
test)
	go test ./... "$@"
	;;
lint)
	go run cmd/commentlint/main.go ./...
	;;
license)
	go run cmd/licensecheck/main.go --root .
	;;
docs)
	go run cmd/zki/main.go create docs --check
	go run cmd/docsnap/main.go -root . -check
	;;
config)
	# 以 `zki create default-config` 为唯一权威源，校验部署配置样例漂移：
	# 二进制 / Compose 样例、Helm 渲染出的 config.toml，以及 ZKI_* 覆盖键。
	HELM=${HELM_BIN:-}
	if [ -z "$HELM" ]; then
		HELM=$(command -v helm || true)
	fi
	if [ -z "$HELM" ]; then
		echo "未找到 helm：请安装 helm，或通过 HELM_BIN 指定可执行文件" >&2
		exit 2
	fi

	tmp=$(mktemp -d "${TMPDIR:-/tmp%/}/zki-config-check.XXXXXX")
	trap 'rm -rf "$tmp"' EXIT

	# 1) 生成权威配置
	authority="$tmp/authority.toml"
	if ! go run cmd/zki/main.go create default-config -o "$authority" --force >/dev/null; then
		echo "生成权威配置失败" >&2
		exit 2
	fi
	if [ ! -s "$authority" ]; then
		echo "权威配置为空：$authority" >&2
		exit 2
	fi

	# 2) 渲染 Helm Chart 中的 config.toml
	configmap="$tmp/configmap.yaml"
	if ! "$HELM" template zhongkui deploys/helm/zhongkui --show-only templates/configmap.yaml >"$configmap" 2>"$tmp/helm.err"; then
		echo "helm template 失败：" >&2
		cat "$tmp/helm.err" >&2
		exit 2
	fi

	helm_config="$tmp/helm-config.toml"
	awk '
		/^[[:space:]]*config\.toml:[[:space:]]*\|[[:space:]]*$/ {
			match($0, /^[[:space:]]*/)
			need = RLENGTH + 2
			found = 1
			next
		}
		found {
			match($0, /^[[:space:]]*/)
			if (RLENGTH >= need) {
				print substr($0, need + 1)
				next
			}
			exit
		}
	' "$configmap" >"$helm_config"

	if [ ! -s "$helm_config" ]; then
		echo "未能从 Helm 渲染结果中提取 config.toml" >&2
		exit 2
	fi

	# 3) 比对键集合、标量类型与 ZKI_* 覆盖键
	go run cmd/configcheck/main.go \
		--authority "$authority" \
		--toml "binary=deploys/binary/config.example.toml" \
		--toml "compose=deploys/compose/config.example.toml" \
		--toml "helm=$helm_config" \
		--env "compose-env=deploys/compose/docker-compose.yml" \
		--env "helm-env=deploys/helm/zhongkui/templates/_helpers.tpl"
	;;
*)
	echo "未知子命令: $sub" >&2
	usage >&2
	exit 2
	;;
esac
