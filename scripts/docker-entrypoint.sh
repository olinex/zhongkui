#!/bin/sh
# zhongkui 镜像 entrypoint
#
# 优先 zki，探测失败才走 shell：
#   $1 以 - 开头 → 是 flag（如 -c /app/config.toml），直接当作 zki 调用
#   $1 为空      → 走默认 CMD，当 zki 调用
#   其他         → gosu app zki "$1" --help 探测，退出码 0 说明是已知子命令
#                 否则当作外部 shell 命令（execute@docker 等）
#
# 构建期（Dockerfile runtime-base 层）已完成 setcap 兼容性探测。
set -eu

is_zki() {
  case "$1" in
    ""|-*) return 0 ;;   # 空 or flag → 直接认为是 zki
    *)     gosu app /usr/local/bin/zki "$1" --help >/dev/null 2>&1 ;;
  esac
}

if [ -x /usr/sbin/gosu ]; then
  if is_zki "${1:-}"; then
    exec gosu app /usr/local/bin/zki "$@"
  fi
  exec gosu app /bin/sh -c "$(printf '%s\n' "$@")"
fi

# 没 gosu（slim scratch）→ 直接 exec zki
exec /usr/local/bin/zki "$@"
