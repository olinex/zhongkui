#!/usr/bin/env bash
# secret-scan.sh - 本地密钥扫描（Betterleaks），用与 CI 门禁相同的工具在提交前自检。
#
# 为什么不在仓库目录直接扫：
#   Betterleaks 默认不读 .gitignore，直接扫工作区会把 node_modules、dist、
#   docs/.vitepress/dist 等构建产物一并扫进来，噪音远大于收益。这里先把「将要
#   提交的文件」复制到临时目录，再对副本扫描——既只覆盖会真正进入仓库的内容，
#   也与 CI 在干净 clone 上看到的文件集一致。临时目录在退出时自动清理。
#
# 为什么基线要提交：
#   仓库里已有大量测试夹具（pcapng 样本、测试用密码、JWT 等）会被扫描器命中。
#   基线文件 .betterleaks-baseline.json 纳入版本控制，本地和 Gitee CI 共用同一份，
#   只对「基线之外的新增发现」失败。CI 侧通过 sc@betterleaks 的 baselinePath 引用。
#
# 用法：
#   scripts/secret-scan.sh             # 默认：未提交的改动（暂存 + 未暂存 + 新文件）
#   scripts/secret-scan.sh --staged    # 仅暂存区（适合接到 pre-commit）
#   scripts/secret-scan.sh --all       # 全部受版本控制的文件
#   scripts/secret-scan.sh --baseline  # 重建基线文件（提交它才会在 CI 生效）
#
# 退出码：0 无新增发现；1 有新增发现；2 参数或环境错误。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
BASELINE="$ROOT_DIR/.betterleaks-baseline.json"
REPORT="$ROOT_DIR/.cicd/secret-scan/betterleaks.json"

usage() {
	sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
}

MODE=changed
WRITE_BASELINE=0
for arg in "$@"; do
	case "$arg" in
	--changed) MODE=changed ;;
	--staged) MODE=staged ;;
	--all) MODE=all ;;
	--baseline) WRITE_BASELINE=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "未知参数: $arg" >&2
		usage >&2
		exit 2
		;;
	esac
done

cd "$ROOT_DIR" || exit 2

if ! command -v betterleaks >/dev/null 2>&1; then
	echo "未安装 betterleaks，请执行：make secret-scan-install" >&2
	exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
	echo "缺少 jq，请先安装：brew install jq" >&2
	exit 2
fi

# mktemp 模板里的 TMPDIR 可能自带结尾斜杠，去掉以免路径前缀匹配不上
tmp=$(mktemp -d "${TMPDIR:-/tmp%/}/zki-secret-scan.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

# 1) 计算待扫描文件集（\0 分隔，兼容含空格的文件名）
#    重建基线必须覆盖全部文件，否则基线会漏掉既有发现
[ "$WRITE_BASELINE" -eq 1 ] && MODE=all
files="$tmp/files.z"
case "$MODE" in
all)
	# 重建基线时只取受版本控制的文件：基线要提交进仓库，不能掺入本地临时文件
	if [ "$WRITE_BASELINE" -eq 1 ]; then
		git ls-files -z --cached >"$files"
	else
		git ls-files -z --cached --others --exclude-standard >"$files"
	fi
	;;
staged)
	git diff --cached --name-only -z --diff-filter=ACMR >"$files"
	;;
changed)
	{
		git diff --name-only -z --diff-filter=ACMR HEAD
		git ls-files -z --others --exclude-standard
	} >"$files"
	;;
esac

if [ ! -s "$files" ]; then
	echo "没有需要扫描的改动（模式：${MODE}）。"
	exit 0
fi

# 2) 复制到临时目录 tree/，保留相对路径。
#    排除基线文件本身：它记录了历史发现的原文，重建基线时不应把自己算进去。
mkdir -p "$tmp/tree"
if ! rsync -a --quiet --from0 --files-from="$files" --exclude='/.betterleaks-baseline.json' ./ "$tmp/tree/"; then
	echo "复制待扫描文件失败" >&2
	exit 2
fi

# 3) 扫描。以 json 报告为准（Betterleaks 有发现时退出码为 1），并统一 cwd 到副本根目录，
#    使报告里的路径是仓库相对路径，与基线文件和 CI 结果可直接比对。
mkdir -p "$(dirname "$REPORT")"
rm -f "$REPORT"

if [ "$WRITE_BASELINE" -eq 1 ]; then
	(cd "$tmp/tree" && betterleaks dir . --no-banner --no-color -f json -r "$BASELINE" >/dev/null 2>&1)
	if [ ! -s "$BASELINE" ]; then
		echo "重建基线失败" >&2
		exit 2
	fi
	echo "基线已更新：.betterleaks-baseline.json（$(jq 'length' "$BASELINE") 条既有发现）"
	echo "记得把该文件一并提交，Gitee CI 才会使用新基线。"
	exit 0
fi

scan_args=(dir . --no-banner --no-color -f json -r "$REPORT")
# 基线存在时只报基线之外的新增发现；不存在则全量报告
[ -f "$BASELINE" ] && scan_args+=(-b "$BASELINE")
(cd "$tmp/tree" && betterleaks "${scan_args[@]}" >/dev/null 2>&1)

n=0
[ -s "$REPORT" ] && n=$(jq 'length' "$REPORT" 2>/dev/null || echo 0)

echo "扫描模式：${MODE}；新增发现 $n 处（报告：.cicd/secret-scan/betterleaks.json）"
if [ "$n" -eq 0 ]; then
	exit 0
fi

echo ""
jq -r '.[:30][] | "  \(.File):\(.StartLine)  \(.RuleID)"' "$REPORT" 2>/dev/null
[ "$n" -gt 30 ] && echo "  ... 其余 $((n - 30)) 处见报告文件"

if [ ! -f "$BASELINE" ]; then
	echo ""
	echo "提示：若确认这些是测试夹具等既有内容，可执行 make secret-scan-baseline 建立基线。"
fi

exit 1
