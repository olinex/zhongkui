#!/usr/bin/env bash
#
# core-slim-assemble.sh - 组装 slim 变体的最小 rootfs
#
# 把 app 阶段注入的 zki 二进制及其运行期依赖收敛成 scratch 镜像所需的文件树：
#   1) 二进制（cp -a 保留 setcap 写入的 security.capability 扩展属性）
#   2) 动态库闭包（libpcap 及其间接依赖、glibc、ld.so）
#   3) cgo 解析器按需加载的 NSS 模块
#   4) CA 证书、时区数据与最小 etc 配置
#   5) 镜像内置默认配置
#   6) busybox 与 wget/sh 符号链接（scratch 无 shell，HEALTHCHECK 依赖它）
#
# 被 deploys/containers/slim.Dockerfile 的 slim-prep 阶段使用。
#
# 用法：
#   bash core-slim-assemble.sh <rootfs 目录> <busybox 二进制路径>
#   bash core-slim-assemble.sh /rootfs /tmp/busybox
#
# 调用前提：所在阶段已安装 libpcap0.8 / ca-certificates / tzdata，
# 且 app 阶段已生成 /app/default-config.toml。
# 退出码：0 成功；非 0 失败。
set -euo pipefail

ROOT="${1:?用法: core-slim-assemble.sh <rootfs> <busybox>}"
BUSYBOX="${2:?用法: core-slim-assemble.sh <rootfs> <busybox>}"
BIN="${ZKI_BIN:-/usr/local/bin/zki}"

mkdir -p "$ROOT/tmp" "$ROOT/app" "$ROOT/etc/ssl/certs" "$ROOT/usr/local/bin" \
  "$ROOT/usr/share" "$ROOT/bin"

# 1) 二进制（保留 security.capability 扩展属性）
cp -a "$BIN" "$ROOT/usr/local/bin/zki"

# 2) 动态库闭包
ldd "$BIN" | awk '{ if ($2 == "=>") print $3; else if ($1 ~ /^\//) print $1 }' |
  grep '^/' | sort -u |
  while read -r lib; do
    mkdir -p "$ROOT$(dirname "$lib")"
    cp -L "$lib" "$ROOT$lib"
  done

# 3) cgo 解析器按需加载的 NSS 模块
for m in libnss_files.so.2 libnss_dns.so.2 libnss_compat.so.2; do
  p=$(ls /usr/lib/*-linux-gnu/"$m" 2>/dev/null | head -n 1 || true)
  if [ -n "$p" ]; then
    mkdir -p "$ROOT$(dirname "$p")"
    cp -L "$p" "$ROOT$p"
  fi
done

# 4) CA 证书、时区数据与最小 etc 配置
cp -a /etc/ssl/certs/ca-certificates.crt "$ROOT/etc/ssl/certs/"
cp -a /usr/share/zoneinfo "$ROOT/usr/share/"
for f in nsswitch.conf passwd group services protocols; do
  if [ -f "/etc/$f" ]; then cp -a "/etc/$f" "$ROOT/etc/"; fi
done

# 5) 镜像内置默认配置（供默认 CMD 使用）
cp -a /app/default-config.toml "$ROOT/app/default-config.toml"

# 6) busybox（静态，约 800KB），提供 wget / sh 供 HEALTHCHECK 与紧急排障使用
cp -a "$BUSYBOX" "$ROOT/usr/local/bin/busybox"
ln -sf /usr/local/bin/busybox "$ROOT/bin/wget"
ln -sf /usr/local/bin/busybox "$ROOT/bin/sh"

chmod 1777 "$ROOT/tmp"
