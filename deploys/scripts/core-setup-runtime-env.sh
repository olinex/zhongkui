#!/usr/bin/env bash
#
# core-setup-runtime-env.sh - zki 运行时环境安装（libpcap0.8 + gosu + setcap + 非 root 用户）
#
# 覆盖所有"构建运行时基底"的场景：
#   1. deploys/local/core.Dockerfile runtime 阶段（normal/slim 变体共用）
#   2. deploys/gitee/core.Dockerfile runtime 阶段（gitee 构建场景，与 local 完全一致）
#
# 两个 Dockerfile 里这段 apt 安装 + 用户创建原本一字不差地重复出现，
# 任何一处改动都要手动同步另一处，容易遗漏。脚本统一后各自只是一行调用。
#
# 职责：
#   - sed 替换 apt mirror（可选）
#   - 安装运行时依赖：libpcap0.8（抓包共享库）+ ca-certificates + tzdata（时区）
#     + gosu（normal 变体降权）+ libcap2-bin（setcap 给 zki 授予抓包 capability）
#     + wget（HEALTHCHECK 用）
#   - 创建非 root 运行用户 app:1000
#     setcap 在 Dockerfile 后续步骤对二进制调用，脚本只负责装工具和建用户
#
# 用法：
#   # Dockerfile runtime 阶段
#   bash /tmp/core-setup-runtime-env.sh --mirror "${APT_MIRROR}"
#
# 参数：
#   --mirror <mirror>    sed 替换 /etc/apt/sources.list 里的 deb.debian.org；不传则跳过
#   --skip-user          跳过 app 用户创建（某些场景用户已存在）
#   --keep-lists         不 rm -rf /var/lib/apt/lists/*（默认清理，让 Dockerfile 层体积更小）
#
# 退出码：0 成功；非 0 失败。
set -euo pipefail

# ----------------------------------------------------------
# 参数解析
# ----------------------------------------------------------
MIRROR=""
SKIP_USER=0
KEEP_LISTS=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mirror)     MIRROR="$2"; shift 2 ;;
    --skip-user)  SKIP_USER=1; shift ;;
    --keep-lists) KEEP_LISTS=1; shift ;;
    --*)          echo "ERROR: 未知选项 $1" >&2; exit 1 ;;
    *)            echo "ERROR: 多余参数 $1" >&2; exit 1 ;;
  esac
done

echo ">>> setup-runtime-env: mirror=${MIRROR:-<跳过>}, 创建用户=$((SKIP_USER == 0 ? 1 : 0))"

# ----------------------------------------------------------
# 1. 替换 apt mirror
# ----------------------------------------------------------
if [ -n "$MIRROR" ]; then
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do
    if [ -f "$f" ]; then
      sed -i \
        "s|deb.debian.org|${MIRROR}|g; s|security.debian.org|${MIRROR}|g" \
        "$f"
    fi
  done
fi

# ----------------------------------------------------------
# 2. 安装运行时依赖
#    libpcap0.8        - zki 运行时链接的抓包共享库
#    ca-certificates   - HTTPS 请求需要的 CA 证书
#    tzdata            - 时区数据（日志、调度器）
#    gosu              - normal 变体入口脚本降权（比 sudo 轻量）
#    libcap2-bin       - setcap 命令，给 zki 二进制授予 CAP_NET_RAW/CAP_NET_ADMIN
#    wget              - HEALTHCHECK 探针调用
# ----------------------------------------------------------
apt-get update
apt-get install -y --no-install-recommends \
  libpcap0.8 ca-certificates tzdata gosu libcap2-bin wget

# ----------------------------------------------------------
# 3. 创建非 root 运行用户 app（uid=1000, gid=1000）
#    slim 变体最终用 USER 1000:1000 切用户；normal 变体用 gosu 降权到这个用户。
# ----------------------------------------------------------
if [ "$SKIP_USER" -eq 0 ]; then
  groupadd -g 1000 app 2>/dev/null || true
  useradd  -u 1000 -g app -d /app -s /usr/sbin/nologin -M app 2>/dev/null || true
  mkdir -p /app
  chown app:app /app
fi

# ----------------------------------------------------------
# 4. 清理 apt lists
# ----------------------------------------------------------
if [ "$KEEP_LISTS" -eq 0 ]; then
  rm -rf /var/lib/apt/lists/*
fi

echo ">>> setup-runtime-env: 完成"
