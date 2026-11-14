#!/usr/bin/env bash
#
# core-cap-setup.sh - 给 zki 授予抓包 capability，并做运行时兼容性探测
#
# 背景：非 root 用户抓包需要 CAP_NET_RAW/CAP_NET_ADMIN，通过 file capability 授予。
# 部分运行时（QEMU/OrbStack）setcap 写入成功但 exec 时返回 EPERM，
# 因此用目标运行用户实际执行一次 --help 探测；失败则清除扩展属性，避免容器无法启动。
#
# 被 deploys/containers/normal.Dockerfile 的 app 阶段使用（二进制来自 --from=build）。
#
# 用法：
#   bash core-cap-setup.sh [二进制路径] [运行用户]
#   bash core-cap-setup.sh                                  # 默认 /usr/local/bin/zki app
#   bash core-cap-setup.sh /usr/local/bin/zki app
#
# 退出码：0 表示 capability 生效或已安全清除；1 表示二进制不存在。
set -euo pipefail

BIN="${1:-/usr/local/bin/zki}"
RUN_USER="${2:-app}"

if [ ! -f "$BIN" ]; then
  echo "ERROR: 二进制不存在: $BIN" >&2
  exit 1
fi

# 写入 file capability；部分文件系统不支持时忽略错误，交由下面的探测判定
setcap cap_net_raw,cap_net_admin=eip "$BIN" 2>/dev/null || true

if gosu "$RUN_USER" "$BIN" --help >/dev/null 2>&1; then
  echo "OK: file capability 在当前运行时生效"
else
  echo "WARN: file capability 不兼容，清除 setcap（非 root 抓包将不可用）" >&2
  setcap -r "$BIN" 2>/dev/null || true
fi
