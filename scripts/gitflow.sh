#!/usr/bin/env bash
#
# zhongkui gitflow 辅助脚本
#
# 在分支保护（master / develop / release/* 仅允许 PR）前提下，把常见的
# 分支创建、远程同步、推送与 Gitee 对比页跳转组合成一条命令。
# 脚本不替代 make release / make ci，只负责需要人工决策以外的重复动作。
#
# 用法: scripts/gitflow.sh help

set -euo pipefail

REMOTE="${GITFLOW_REMOTE:-origin}"
DEVELOP="${GITFLOW_DEVELOP:-develop}"
MASTER="${GITFLOW_MASTER:-master}"

# 全局开关，由 main 解析前置参数后写入
NO_OPEN=0
RUN_CI=0
FORCE=0

if [ -t 1 ]; then
	C_INFO=$'\033[36m'
	C_WARN=$'\033[33m'
	C_ERR=$'\033[31m'
	C_RESET=$'\033[0m'
else
	C_INFO=''
	C_WARN=''
	C_ERR=''
	C_RESET=''
fi

info() { printf '%s==>%s %s\n' "$C_INFO" "$C_RESET" "$*"; }
warn() { printf '%s警告:%s %s\n' "$C_WARN" "$C_RESET" "$*" >&2; }
die() { printf '%s错误:%s %s\n' "$C_ERR" "$C_RESET" "$*" >&2; exit 1; }

usage() {
	cat <<'EOF'
zhongkui gitflow 辅助脚本

用法:
  scripts/gitflow.sh <命令> [参数]

分支生命周期:
  feature start <name>              从 develop 切出 feature/<name>
  feature finish                    推送当前 feature 分支并打开 feature → develop 的 PR
  hotfix start <name>               从 master 切出 hotfix/<name>
  hotfix finish [--version vX.Y.Z]  推送并打开 hotfix → master 的 PR；
                                    PR 合并后带 --version 重跑，创建标签并打开回合并 PR
  release start <version>           从 develop 切出 release/<version> 并尝试推送
  release finish [<version>]        推送并打开 release/* → master 的 PR；
                                    PR 合并后重跑，创建标签并打开 master → develop 的 PR

日常操作:
  commit [选项] [-- <文件>...]      按 Angular 规范生成并提交 message；
                                    缺少类型或主题时进入交互问答
      -t, --type <type>             提交类型: build ci docs feat fix perf refactor
                                    revert style test chore
      -s, --scope <scope>           作用域，如 release、docs
      -m, --subject <text>          主题（必填，祈使句，结尾不加句号）
      -b, --body <text>             正文
          --breaking <text>         破坏性变更说明，自动加 ! 与 BREAKING CHANGE
      -i, --issues <12,#34>         关联 Issue，生成 Closes 页脚
      -a, --all                     暂存所有已跟踪文件的修改
      -y, --yes                     跳过确认
          --dry-run                 只预览 message，不创建提交
  pr [--base <branch>]              推送当前分支并打开 PR（base 默认按分支类型推断）
  sync                              fetch --prune，并快进本地 develop / master
  status                            展示当前分支、保护状态、领先/落后情况
  clean [-r|--remote]               删除已合入目标分支的本地分支
                                    （feature → develop，hotfix/release → master）；
                                    -r 同时删除对应远程分支

通用参数:
  --ci                              推送前执行 make ci
  --force                           推送时使用 --force-with-lease
  --no-open                         只打印 PR 链接，不打开浏览器
  -h, --help                        查看帮助

环境变量:
  GITFLOW_REMOTE     远程名，默认 origin
  GITFLOW_DEVELOP    开发分支名，默认 develop
  GITFLOW_MASTER     发布分支名，默认 master
  GITFLOW_NO_OPEN    置 1 等价于 --no-open
EOF
}

# ---------- 仓库与分支基础工具 ----------

require_repo() {
	git rev-parse --git-dir >/dev/null 2>&1 || die "当前目录不是 Git 仓库"
}

# current_branch 只读取当前分支名，分离头指针时输出空串，由调用方决定如何处理
current_branch() {
	git symbolic-ref --short -q HEAD 2>/dev/null || true
}

require_branch_attached() {
	local branch
	branch=$(current_branch)
	[ -n "$branch" ] || die "当前处于分离头指针状态，请先切换到具体分支"
	printf '%s' "$branch"
}

is_protected() {
	case "$1" in
		"$MASTER" | "$DEVELOP" | release/*) return 0 ;;
		*) return 1 ;;
	esac
}

branch_exists_local() {
	git show-ref --verify --quiet "refs/heads/$1"
}

branch_exists_remote() {
	git show-ref --verify --quiet "refs/remotes/$REMOTE/$1"
}

tag_exists() {
	git rev-parse -q --verify "refs/tags/$1" >/dev/null 2>&1
}

require_clean() {
	[ -z "$(git status --porcelain)" ] || die "工作区不干净，请先提交或 stash 改动"
}

validate_name() {
	case "$1" in
		'' | *[!A-Za-z0-9._-]*) die "分支名 '$1' 不合法，只允许字母、数字、点、下划线与连字符" ;;
	esac
	git check-ref-format "refs/heads/topic/$1" >/dev/null 2>&1 ||
		die "分支名 '$1' 不符合 Git 引用格式"
}

# validate_semver 只做入参的粗校验，完整版本规则与发布前检查由 make release 负责
validate_semver() {
	printf '%s' "$1" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' ||
		die "版本号 '$1' 不是合法 SemVer，示例: v1.2.3 或 v1.2.3-rc.1"
}

require_topic_branch() {
	local prefix="$1" branch
	branch=$(require_branch_attached)
	case "$branch" in
		"$prefix"/*) ;;
		*) die "当前分支为 ${branch}，不是 $prefix/* 分支" ;;
	esac
	printf '%s' "$branch"
}

fetch_remote() {
	info "同步远程引用（${REMOTE}）"
	git fetch "$REMOTE" --prune --tags
}

# ahead_behind 输出两个数字：$1 领先 $2 的提交数、$1 落后 $2 的提交数
ahead_behind() {
	git rev-list --left-right --count "$2...$1" | awk '{print $2, $1}'
}

default_base() {
	case "$1" in
		feature/*) printf '%s' "$DEVELOP" ;;
		hotfix/* | release/*) printf '%s' "$MASTER" ;;
		*) printf '%s' "$DEVELOP" ;;
	esac
}

maybe_ci() {
	[ "$RUN_CI" -eq 1 ] || return 0
	info "执行 make ci"
	make ci
}

push_branch() {
	local branch="$1" args=(-u)
	is_protected "$branch" && die "受保护分支 $branch 不允许直接推送，请通过 PR 修改"
	[ "$FORCE" -eq 1 ] && args+=(--force-with-lease)
	info "推送 $branch 到 $REMOTE"
	git push "${args[@]}" "$REMOTE" "$branch"
}

# remote_has_branch 直接查询远程是否存在分支，避免依赖本地过期的远程跟踪引用
remote_has_branch() {
	git ls-remote --exit-code --heads "$REMOTE" "$1" >/dev/null 2>&1
}

# attempt_push 用于 release 分支：分支保护可能拒绝推送，失败时给出平台侧操作指引
attempt_push() {
	local branch="$1" args=(-u)
	[ "$FORCE" -eq 1 ] && args+=(--force-with-lease)
	if git push "${args[@]}" "$REMOTE" "$branch"; then
		return 0
	fi
	warn "推送 ${branch} 被拒绝：release/* 属于受保护分支，仅允许通过 PR 合入"
	printf '  需要具备推送权限的维护者执行以下命令（或临时放开保护）:\n'
	printf '    git push -u %s %s\n' "$REMOTE" "$branch"
	printf '  也可以在 Gitee 仓库页面「代码 → 分支 → 新建分支」按 %s 创建，再由维护者推送本地提交\n' "$DEVELOP"
	return 1
}

# ---------- PR 链接 ----------

url_encode() {
	printf '%s' "$1" | sed 's|/|%2F|g'
}

# pr_url 根据 origin 地址推导 Gitee 对比页，支持 SSH 与 HTTPS 两种远程写法
pr_url() {
	local base="$1" head="$2" remote_url url rest host path
	remote_url=$(git remote get-url "$REMOTE" 2>/dev/null || true)
	[ -n "$remote_url" ] || {
		warn "未找到远程 $REMOTE"
		return 1
	}
	url=${remote_url%.git}
	case "$url" in
		*://*)
			rest=${url#*://}
			rest=${rest#*@}
			host=${rest%%/*}
			path=${rest#*/}
			;;
		*:*)
			host=${url%%:*}
			host=${host#*@}
			path=${url#*:}
			;;
		*)
			warn "无法从远程地址推导 Web 地址: $remote_url"
			return 1
			;;
	esac
	host=${host%%:*}
	printf 'https://%s/%s/compare/%s...%s\n' "$host" "$path" "$(url_encode "$base")" "$(url_encode "$head")"
}

open_url() {
	local url="$1"
	[ "$NO_OPEN" -eq 0 ] || return 0
	[ "${GITFLOW_NO_OPEN:-0}" != "1" ] || return 0
	[ -t 1 ] || return 0
	case "$(uname -s)" in
		Darwin) open "$url" >/dev/null 2>&1 || true ;;
		Linux) xdg-open "$url" >/dev/null 2>&1 || true ;;
	esac
}

print_pr() {
	local base="$1" head="$2" url
	if url=$(pr_url "$base" "$head"); then
		printf '  创建 PR: %s\n' "$url"
		open_url "$url"
	else
		printf '  请在 %s 页面手动创建 PR: %s -> %s\n' "$REMOTE" "$head" "$base"
	fi
}

# ---------- 提交 ----------

COMMIT_TYPES='build ci docs feat fix perf refactor revert style test chore'

choose_commit_type() {
	local answer
	printf '选择提交类型:\n'
	printf '   1) feat     新功能\n'
	printf '   2) fix      缺陷修复\n'
	printf '   3) docs     文档\n'
	printf '   4) style    格式（不影响逻辑）\n'
	printf '   5) refactor 重构\n'
	printf '   6) perf     性能优化\n'
	printf '   7) test     测试\n'
	printf '   8) build    构建或依赖\n'
	printf '   9) ci       CI 配置\n'
	printf '  10) chore    杂项\n'
	printf '  11) revert   回滚\n'
	printf '类型编号或名称: '
	read -r answer
	case "$answer" in
		1) CHOSEN_TYPE=feat ;;
		2) CHOSEN_TYPE=fix ;;
		3) CHOSEN_TYPE=docs ;;
		4) CHOSEN_TYPE=style ;;
		5) CHOSEN_TYPE=refactor ;;
		6) CHOSEN_TYPE=perf ;;
		7) CHOSEN_TYPE=test ;;
		8) CHOSEN_TYPE=build ;;
		9) CHOSEN_TYPE=ci ;;
		10) CHOSEN_TYPE=chore ;;
		11) CHOSEN_TYPE=revert ;;
		*) CHOSEN_TYPE="$answer" ;;
	esac
}

validate_commit_type() {
	case "$1" in
		build | ci | docs | feat | fix | perf | refactor | revert | style | test | chore) ;;
		*) die "提交类型 '$1' 不符合 Angular 规范，可选: $COMMIT_TYPES" ;;
	esac
}

validate_commit_scope() {
	case "$1" in
		'') ;;
		*[!a-z0-9._/-]*) die "scope 只允许小写字母、数字、点、下划线、连字符与斜杠" ;;
	esac
}

validate_commit_subject() {
	[ -n "$1" ] || die "提交主题不能为空"
	case "$1" in
		' '* | *' ') die "提交主题首尾不能有空格" ;;
		*. | *。) die "提交主题结尾不要加句号" ;;
	esac
	[ "${#1}" -le 100 ] || die "提交主题超过 100 字符，请精简"
	[ "${#1}" -le 72 ] || warn "提交主题超过 72 字符，建议精简"
}

# format_issues 把 "12,#34" 归一化为 "Closes #12, #34"
format_issues() {
	local rest="$1" out='' item
	rest=$(printf '%s' "$rest" | tr ',' ' ')
	for item in $rest; do
		item=${item#\#}
		case "$item" in
			'' | *[!0-9]*) die "Issue 编号 '$1' 不合法，示例: 12 或 #12" ;;
		esac
		if [ -z "$out" ]; then
			out="#$item"
		else
			out="$out, #$item"
		fi
	done
	printf 'Closes %s' "$out"
}

commit_cmd() {
	local type='' scope='' subject='' body='' breaking='' issues=''
	local stage_all=0 assume_yes=0 dry_run=0 interactive=0 branch message header footer answer CHOSEN_TYPE=''
	local -a files=()

	while [ $# -gt 0 ]; do
		case "$1" in
			-t | --type)
				shift
				type="${1:-}"
				[ -n "$type" ] || die "--type 需要提交类型"
				;;
			-s | --scope)
				shift
				scope="${1:-}"
				[ -n "$scope" ] || die "--scope 需要作用域"
				;;
			-m | --subject)
				shift
				subject="${1:-}"
				[ -n "$subject" ] || die "--subject 需要提交主题"
				;;
			-b | --body)
				shift
				body="${1:-}"
				[ -n "$body" ] || die "--body 需要正文"
				;;
			--breaking)
				shift
				breaking="${1:-}"
				[ -n "$breaking" ] || die "--breaking 需要破坏性变更说明"
				;;
			-i | --issues)
				shift
				issues="${1:-}"
				[ -n "$issues" ] || die "--issues 需要 Issue 编号"
				;;
			-a | --all) stage_all=1 ;;
			-y | --yes) assume_yes=1 ;;
			--dry-run) dry_run=1 ;;
			--)
				shift
				while [ $# -gt 0 ]; do
					files+=("$1")
					shift
				done
				break
				;;
			-*) die "未知参数: $1" ;;
			*) files+=("$1") ;;
		esac
		shift
	done

	branch=$(require_branch_attached)
	if [ "$branch" = "$MASTER" ] || [ "$branch" = "$DEVELOP" ]; then
		die "受保护分支 ${branch} 禁止本地提交，请先执行 scripts/gitflow.sh feature start <name>"
	fi

	# 预览模式不触碰暂存区，便于在未 add 时校验 message
	if [ "$dry_run" -eq 0 ]; then
		if [ ${#files[@]} -gt 0 ]; then
			git add -- "${files[@]}"
		fi
		if [ "$stage_all" -eq 1 ]; then
			git add -u
		fi
		if git diff --cached --quiet; then
			die "没有已暂存的改动，请先 git add，或使用 -a 暂存已跟踪文件的修改"
		fi
	fi

	# 仅在缺少类型或主题且处于交互终端时进入问答，避免已传齐参数时被二次打扰
	if [ -t 0 ] && { [ -z "$type" ] || [ -z "$subject" ]; }; then
		interactive=1
	fi
	if [ "$interactive" -eq 1 ]; then
		if [ -z "$type" ]; then
			choose_commit_type
			type="$CHOSEN_TYPE"
		fi
		if [ -z "$scope" ]; then
			printf '作用域（可留空）: '
			read -r scope
		fi
		if [ -z "$subject" ]; then
			printf '提交主题（祈使句，结尾不加句号）: '
			read -r subject
		fi
		if [ -z "$body" ]; then
			printf '正文（可留空）: '
			read -r body
		fi
		if [ -z "$breaking" ]; then
			printf '破坏性变更说明（可留空）: '
			read -r breaking
		fi
		if [ -z "$issues" ]; then
			printf '关联 Issue（可留空，如 12,#34）: '
			read -r issues
		fi
	fi

	[ -n "$type" ] || die "缺少提交类型，使用 -t/--type 指定（可选: ${COMMIT_TYPES}）"
	validate_commit_type "$type"
	validate_commit_scope "$scope"
	validate_commit_subject "$subject"

	header="$type"
	[ -n "$scope" ] && header="$header($scope)"
	[ -n "$breaking" ] && header="$header!"
	header="$header: $subject"

	message="$header"
	if [ -n "$body" ]; then
		message="$message

$body"
	fi
	if [ -n "$breaking" ]; then
		message="$message

BREAKING CHANGE: $breaking"
	fi
	if [ -n "$issues" ]; then
		footer=$(format_issues "$issues")
		message="$message

$footer"
	fi

	info "提交信息预览:"
	printf '%s\n' "$message" | sed 's/^/  /'
	if [ "$dry_run" -eq 1 ]; then
		info "仅预览，未创建提交"
		return 0
	fi
	if [ "$assume_yes" -eq 0 ]; then
		if [ ! -t 0 ]; then
			die "非交互环境请加 -y 确认提交"
		fi
		printf '确认提交？[y/N] '
		read -r answer
		case "$answer" in
			y | Y | yes | YES) ;;
			*) die "已取消提交" ;;
		esac
	fi
	printf '%s\n' "$message" | git commit -F -
	git log -1 --oneline
}

# ---------- feature ----------

feature_start() {
	local name="${1:-}" branch
	[ -n "$name" ] || die "用法: scripts/gitflow.sh feature start <name>"
	validate_name "$name"
	branch="feature/$name"
	require_clean
	fetch_remote
	branch_exists_local "$branch" && die "本地已存在分支 $branch"
	branch_exists_remote "$branch" && die "远程已存在分支 $REMOTE/$branch"
	branch_exists_remote "$DEVELOP" || die "远程不存在 $REMOTE/$DEVELOP"
	git checkout -b "$branch" --no-track "$REMOTE/$DEVELOP"
	info "已基于 $REMOTE/$DEVELOP 创建 $branch"
	printf '  完成开发后执行: scripts/gitflow.sh feature finish\n'
}

feature_finish() {
	local branch base="$DEVELOP" ahead
	branch=$(require_topic_branch feature)
	require_clean
	maybe_ci
	ahead=$(git rev-list --count "$REMOTE/$base..HEAD")
	[ "$ahead" -gt 0 ] || warn "当前分支相对 $REMOTE/$base 没有新提交，PR 可能为空"
	push_branch "$branch"
	print_pr "$base" "$branch"
	printf '  合并前确认 make ci 通过、PR 模板检查项已勾选\n'
}

feature_cmd() {
	[ $# -gt 0 ] || die "用法: scripts/gitflow.sh feature <start|finish> [name]"
	local sub="$1"
	shift
	case "$sub" in
		start) feature_start "$@" ;;
		finish) feature_finish "$@" ;;
		*) die "未知子命令: feature $sub" ;;
	esac
}

# ---------- hotfix ----------

hotfix_start() {
	local name="${1:-}" branch
	[ -n "$name" ] || die "用法: scripts/gitflow.sh hotfix start <name>"
	validate_name "$name"
	branch="hotfix/$name"
	require_clean
	fetch_remote
	branch_exists_local "$branch" && die "本地已存在分支 $branch"
	branch_exists_remote "$branch" && die "远程已存在分支 $REMOTE/$branch"
	branch_exists_remote "$MASTER" || die "远程不存在 $REMOTE/$MASTER"
	git checkout -b "$branch" --no-track "$REMOTE/$MASTER"
	info "已基于 $REMOTE/$MASTER 创建 $branch"
	printf '  修复完成后执行: scripts/gitflow.sh hotfix finish\n'
}

next_patch_version() {
	local latest number major minor patch
	latest=$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || true)
	[ -n "$latest" ] || return 1
	number=${latest#v}
	number=${number%%[-+]*}
	major=$(printf '%s' "$number" | cut -d. -f1)
	minor=$(printf '%s' "$number" | cut -d. -f2)
	patch=$(printf '%s' "$number" | cut -d. -f3)
	printf 'v%s.%s.%s\n' "$major" "$minor" $((patch + 1))
}

hotfix_finish() {
	local version='' branch merged=0 suggestion
	while [ $# -gt 0 ]; do
		case "$1" in
			--version)
				shift
				version="${1:-}"
				[ -n "$version" ] || die "--version 需要版本号"
				validate_semver "$version"
				;;
			*) die "未知参数: $1" ;;
		esac
		shift
	done
	branch=$(require_topic_branch hotfix)
	require_clean
	fetch_remote
	if branch_exists_local "$branch" &&
		git merge-base --is-ancestor "refs/heads/$branch" "$REMOTE/$MASTER"; then
		merged=1
	fi
	if [ "$merged" -eq 0 ]; then
		maybe_ci
		push_branch "$branch"
		print_pr "$MASTER" "$branch"
		printf '  PR 合并后重跑本命令并指定 --version，脚本会创建标签并打开回合并 PR\n'
		return 0
	fi
	if [ -z "$version" ]; then
		suggestion=$(next_patch_version || true)
		warn "分支已合入 ${MASTER}，但未指定发布版本"
		[ -n "$suggestion" ] && printf '  建议版本: %s\n' "$suggestion"
		printf '  执行: scripts/gitflow.sh hotfix finish --version %s\n' "${suggestion:-vX.Y.Z}"
		return 0
	fi
	finalize_release "$version"
}

hotfix_cmd() {
	[ $# -gt 0 ] || die "用法: scripts/gitflow.sh hotfix <start|finish> [参数]"
	local sub="$1"
	shift
	case "$sub" in
		start) hotfix_start "$@" ;;
		finish) hotfix_finish "$@" ;;
		*) die "未知子命令: hotfix $sub" ;;
	esac
}

# ---------- release ----------

release_start() {
	local version="${1:-}" branch
	[ -n "$version" ] || die "用法: scripts/gitflow.sh release start <version>"
	validate_semver "$version"
	branch="release/$version"
	require_clean
	fetch_remote
	branch_exists_local "$branch" && die "本地已存在分支 $branch"
	branch_exists_remote "$branch" && die "远程已存在分支 $REMOTE/$branch"
	branch_exists_remote "$DEVELOP" || die "远程不存在 $REMOTE/$DEVELOP"
	git checkout -b "$branch" --no-track "$REMOTE/$DEVELOP"
	info "已基于 $REMOTE/$DEVELOP 创建 $branch"
	attempt_push "$branch" || true
	printf '  发布准备清单:\n'
	printf '    1. 更新 CHANGELOG.md 的 ## [%s] - YYYY-MM-DD 段\n' "${version#v}"
	printf '    2. 对齐 web/package.json 的 version 为 %s\n' "${version#v}"
	printf '    3. make ci 通过后执行: scripts/gitflow.sh release finish %s\n' "$version"
}

# print_backmerge_pr 在 master 尚未合入 develop 时打印回合并 PR，避免重复提示
print_backmerge_pr() {
	if branch_exists_remote "$MASTER" && branch_exists_remote "$DEVELOP" &&
		git merge-base --is-ancestor "$REMOTE/$MASTER" "$REMOTE/$DEVELOP"; then
		info "develop 已包含 master，无需回合并"
		return 0
	fi
	print_pr "$DEVELOP" "$MASTER"
}

# finalize_release 在 release/hotfix 已合入 master 后创建标签，并打开 master → develop 回合并 PR
finalize_release() {
	local version="$1" tag_ref="refs/tags/$1"
	require_clean
	info "切换到 $MASTER 并快进到 $REMOTE/$MASTER"
	git checkout "$MASTER"
	git merge --ff-only "$REMOTE/$MASTER" ||
		die "本地 ${MASTER} 无法快进到 $REMOTE/${MASTER}，请先处理分叉"
	if tag_exists "$version"; then
		info "标签 $version 已存在，跳过创建"
	else
		info "执行 make release VERSION=${version}（含发布前检查）"
		make release VERSION="$version"
	fi
	if git ls-remote --exit-code --tags "$REMOTE" "$tag_ref" >/dev/null 2>&1; then
		info "标签 $version 已存在于远程"
	else
		info "只推送标签（$MASTER 受保护，不直接推送分支）"
		git push "$REMOTE" "$tag_ref"
	fi
	print_backmerge_pr
	printf '  回合并 PR 通过后，可删除对应的 release / hotfix 分支并执行 clean\n'
}

release_finish() {
	local version="${1:-}" branch merged=0
	if [ -n "$version" ]; then
		validate_semver "$version"
		branch="release/$version"
	else
		branch=$(current_branch)
		case "$branch" in
			release/v*) version=${branch#release/} ;;
			*) die "请指定版本号: scripts/gitflow.sh release finish v1.2.3" ;;
		esac
		validate_semver "$version"
	fi
	require_clean
	fetch_remote
	if tag_exists "$version"; then
		info "标签 $version 已存在，视为发布完成"
		if ! git ls-remote --exit-code --tags "$REMOTE" "refs/tags/$version" >/dev/null 2>&1; then
			info "推送本地标签 $version"
			git push "$REMOTE" "refs/tags/$version"
		fi
		print_backmerge_pr
		return 0
	fi
	branch_exists_local "$branch" || branch_exists_remote "$branch" ||
		die "找不到分支 ${branch}，且标签 $version 不存在"
	if branch_exists_local "$branch" &&
		git merge-base --is-ancestor "refs/heads/$branch" "$REMOTE/$MASTER"; then
		merged=1
	fi
	if [ "$merged" -eq 0 ]; then
		maybe_ci
		if branch_exists_local "$branch"; then
			attempt_push "$branch" || true
		fi
		if remote_has_branch "$branch"; then
			print_pr "$MASTER" "$branch"
			printf '  PR 合并后重跑本命令，脚本会创建标签并打开 master → develop 回合并 PR\n'
		else
			warn "远程不存在 ${branch}，无法创建 review PR"
			printf '  请先按上面的指引让维护者推送该分支，再执行: scripts/gitflow.sh release finish %s\n' "$version"
			return 1
		fi
		return 0
	fi
	finalize_release "$version"
}

release_cmd() {
	[ $# -gt 0 ] || die "用法: scripts/gitflow.sh release <start|finish> [version]"
	local sub="$1"
	shift
	case "$sub" in
		start) release_start "$@" ;;
		finish) release_finish "$@" ;;
		*) die "未知子命令: release $sub" ;;
	esac
}

# ---------- 日常操作 ----------

pr_cmd() {
	local base='' branch
	while [ $# -gt 0 ]; do
		case "$1" in
			--base)
				shift
				base="${1:-}"
				[ -n "$base" ] || die "--base 需要分支名"
				;;
			*) die "未知参数: $1" ;;
		esac
		shift
	done
	branch=$(require_branch_attached)
	is_protected "$branch" && die "当前处于受保护分支 ${branch}，请从特性分支发起 PR"
	[ -n "$base" ] || base=$(default_base "$branch")
	[ -z "$(git status --porcelain)" ] || warn "工作区有未提交改动，本次只推送已提交内容"
	maybe_ci
	push_branch "$branch"
	print_pr "$base" "$branch"
}

# update_tracking_branch 仅在本地分支是远程分支祖先时快进，避免覆盖本地提交
update_tracking_branch() {
	local branch="$1" cur
	branch_exists_remote "$branch" || {
		warn "远程不存在 $REMOTE/${branch}，跳过"
		return 0
	}
	if ! branch_exists_local "$branch"; then
		git branch "$branch" "$REMOTE/$branch"
		info "已创建本地分支 $branch"
		return 0
	fi
	cur=$(current_branch)
	if [ "$cur" = "$branch" ]; then
		if git merge --ff-only "$REMOTE/$branch" >/dev/null; then
			info "$branch 已快进到 $REMOTE/$branch"
		else
			warn "$branch 与 $REMOTE/$branch 已分叉，未自动处理"
		fi
	elif git merge-base --is-ancestor "refs/heads/$branch" "$REMOTE/$branch"; then
		git branch -f "$branch" "$REMOTE/$branch"
		info "$branch 已快进到 $REMOTE/$branch"
	else
		warn "$branch 与 $REMOTE/$branch 已分叉，未自动处理"
	fi
}

sync_cmd() {
	fetch_remote
	update_tracking_branch "$DEVELOP"
	update_tracking_branch "$MASTER"
}

status_cmd() {
	local cur upstream counts base leading lagging branch
	cur=$(current_branch)
	printf '当前分支: %s\n' "${cur:-<分离头指针>}"
	if [ -n "$cur" ] && is_protected "$cur"; then
		printf '保护状态: 受保护，仅支持通过 PR 合入\n'
	elif [ -n "$cur" ]; then
		printf '保护状态: 可直接推送\n'
	fi
	if [ -n "$(git status --porcelain)" ]; then
		printf '工作区: 有未提交改动\n'
	else
		printf '工作区: 干净\n'
	fi
	upstream=$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)
	if [ -n "$upstream" ]; then
		counts=$(git rev-list --left-right --count "HEAD...$upstream")
		printf '上游: %s（领先 %s / 落后 %s）\n' "$upstream" \
			"$(printf '%s' "$counts" | awk '{print $1}')" \
			"$(printf '%s' "$counts" | awk '{print $2}')"
	else
		printf '上游: 未关联\n'
	fi
	printf '\n本地分支状态（数据来自上次 fetch，先运行 sync 获取最新）:\n'
	for branch in $(git for-each-ref --format='%(refname:short)' refs/heads); do
		case "$branch" in
			feature/* | hotfix/* | release/*) ;;
			*) continue ;;
		esac
		case "$branch" in
			release/*) base="$DEVELOP" ;;
			*) base=$(default_base "$branch") ;;
		esac
		if ! branch_exists_remote "$base"; then
			printf '  %-42s 基线 %s 不存在\n' "$branch" "$REMOTE/$base"
			continue
		fi
		counts=$(ahead_behind "refs/heads/$branch" "$REMOTE/$base")
		leading=${counts%% *}
		lagging=${counts##* }
		printf '  %-42s 相对 %-10s 领先 %s / 落后 %s\n' "$branch" "$base" "$leading" "$lagging"
	done
}

# merged_into_target 判断分支是否已合入它对应的目标分支：
# feature → develop；hotfix / release → master（release 也可凭版本标签判定完成）
merged_into_target() {
	local branch="$1" target
	case "$branch" in
		feature/*) target="$DEVELOP" ;;
		hotfix/*) target="$MASTER" ;;
		release/*)
			target="$MASTER"
			tag_exists "v${branch#release/}" && return 0
			;;
		*) return 1 ;;
	esac
	branch_exists_remote "$target" &&
		git merge-base --is-ancestor "refs/heads/$branch" "$REMOTE/$target"
}

clean_cmd() {
	local with_remote=0 cur branch upstream base ahead released
	while [ $# -gt 0 ]; do
		case "$1" in
			-r | --remote) with_remote=1 ;;
			*) die "未知参数: $1" ;;
		esac
		shift
	done
	fetch_remote
	cur=$(current_branch)
	for branch in $(git for-each-ref --format='%(refname:short)' refs/heads); do
		case "$branch" in
			feature/* | hotfix/* | release/*) ;;
			*) continue ;;
		esac
		if [ "$branch" = "$cur" ]; then
			info "跳过当前分支 $branch"
			continue
		fi
		if merged_into_target "$branch"; then
			# 已存在版本标签的 release 分支视为发布完成，不受空分支保护限制
			released=0
			case "$branch" in
				release/*) tag_exists "v${branch#release/}" && released=1 ;;
			esac
			# 刚创建、从未推送且与基线完全一致的分支视为空分支，避免误删正在准备的分支
			upstream=$(git for-each-ref --format='%(upstream)' "refs/heads/$branch")
			case "$branch" in
				release/*) base="$DEVELOP" ;;
				*) base=$(default_base "$branch") ;;
			esac
			ahead=0
			if branch_exists_remote "$base"; then
				ahead=$(git rev-list --count "$REMOTE/$base..refs/heads/$branch")
			fi
			if [ "$released" -eq 0 ] && [ -z "$upstream" ] && [ "$ahead" -eq 0 ]; then
				warn "保留 ${branch}（空分支，尚未推送）"
				continue
			fi
			git branch -D "$branch" >/dev/null
			info "已删除本地分支 $branch"
			if [ "$with_remote" -eq 1 ] && branch_exists_remote "$branch" && ! is_protected "$branch"; then
				git push "$REMOTE" --delete "$branch" >/dev/null
				info "已删除远程分支 $REMOTE/$branch"
			fi
		else
			warn "保留 ${branch}（尚未合入目标分支）"
		fi
	done
}

# ---------- 入口 ----------

main() {
	local args=() arg cmd
	for arg in "$@"; do
		case "$arg" in
			--no-open) NO_OPEN=1 ;;
			--ci) RUN_CI=1 ;;
			--force) FORCE=1 ;;
			*) args+=("$arg") ;;
		esac
	done
	set -- ${args[@]+"${args[@]}"}
	[ $# -gt 0 ] || {
		usage
		exit 2
	}
	cmd="$1"
	shift
	case "$cmd" in
		help | -h | --help) usage ;;
		status) require_repo && status_cmd ;;
		sync) require_repo && sync_cmd ;;
		commit) require_repo && commit_cmd "$@" ;;
		pr) require_repo && pr_cmd "$@" ;;
		clean) require_repo && clean_cmd "$@" ;;
		feature) require_repo && feature_cmd "$@" ;;
		hotfix) require_repo && hotfix_cmd "$@" ;;
		release) require_repo && release_cmd "$@" ;;
		*) die "未知命令: ${cmd}（运行 scripts/gitflow.sh help 查看用法）" ;;
	esac
}

main "$@"
