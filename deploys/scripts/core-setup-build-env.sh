#!/usr/bin/env bash
#
# core-setup-build-env.sh - zki 编译环境安装（libpcap-dev + 按需交叉工具链）
#
# 覆盖所有需要"在宿主机或容器内装 Go 编译依赖"的场景：
#   1. deploys/containers/core.Dockerfile builder 阶段（装 apt mirror + 原生 libpcap-dev，
#      仅目标架构与宿主不同时装对应方向的交叉工具链）
#   2. deploys/scripts/core-build-multiarch.sh --skip-apt=0 内部调用（不换 mirror，装交叉工具链）
#   3. CI 单元测试（--no-cross，只装原生 libpcap-dev）
#
# 为什么独立成脚本：
#   apt 安装 + sed mirror + 方向判断 在多处重复，任何一处改动（比如新增依赖包、换镜像源、
#   调整交叉工具链方向）都要同步改多个地方，容易遗漏。
#   脚本统一后，Dockerfile / CI 只是一行调用。
#
# 用法：
#   # Dockerfile builder 阶段：指定 APT mirror；目标与宿主相同时不装交叉工具链
#   bash deploys/scripts/core-setup-build-env.sh \
#     --host-arch "${BUILDARCH}" \
#     --target-arch "${TARGETARCH}" \
#     --mirror "${APT_MIRROR}"
#
#   # core-build-multiarch.sh 内部调用（宿主机双架构编译）：不换 mirror，装反向交叉工具链
#   bash deploys/scripts/core-setup-build-env.sh
#
#   # CI 单测：只装原生 libpcap-dev，不需要交叉工具链
#   bash deploys/scripts/core-setup-build-env.sh --no-cross
#
#   # GitHub Actions：默认 id -u 判断 root，sudo 自动处理
#   sudo deploys/scripts/core-setup-build-env.sh --no-cross
#
# 参数：
#   --host-arch <arch>   宿主架构 amd64|arm64；不传则自动探测（go env GOARCH / uname -m）
#   --target-arch <arch> 目标架构 amd64|arm64（Dockerfile 的 TARGETARCH）：
#                        与宿主相同 → 不装交叉工具链；不同 → 只装该方向的交叉工具链；
#                        不传 → 按宿主架构装反向交叉工具链（宿主机双架构编译）
#   --mirror <mirror>    sed 替换 /etc/apt/sources.list 里的 deb.debian.org；不传则跳过
#   --no-cross           不装交叉工具链（单元测试场景不需要）
#   --keep-lists         不 rm -rf /var/lib/apt/lists/*（默认清理，让 Dockerfile 层体积更小）
#
# 退出码：0 成功；非 0 失败。
set -euo pipefail

# ----------------------------------------------------------
# 参数解析
# ----------------------------------------------------------
HOST_ARCH=""
TARGET_ARCH=""
MIRROR=""
INSTALL_CROSS=1
KEEP_LISTS=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host-arch)  HOST_ARCH="$2"; shift 2 ;;
    --target-arch) TARGET_ARCH="$2"; shift 2 ;;
    --mirror)     MIRROR="$2"; shift 2 ;;
    --no-cross)   INSTALL_CROSS=0; shift ;;
    --keep-lists) KEEP_LISTS=1; shift ;;
    --*)          echo "ERROR: 未知选项 $1" >&2; exit 1 ;;
    *)            echo "ERROR: 多余参数 $1" >&2; exit 1 ;;
  esac
done

# 宿主架构：优先显式传入（Dockerfile BUILDARCH），否则自动探测
if [ -z "$HOST_ARCH" ]; then
  if command -v go >/dev/null 2>&1; then
    HOST_ARCH=$(go env GOARCH 2>/dev/null || true)
  fi
  if [ -z "$HOST_ARCH" ]; then
    case "$(uname -m)" in
      x86_64) HOST_ARCH="amd64" ;;
      aarch64) HOST_ARCH="arm64" ;;
      *) echo "ERROR: 无法探测宿主架构" >&2; exit 1 ;;
    esac
  fi
fi

case "$HOST_ARCH" in
  amd64 | arm64) ;;
  *) echo "ERROR: 仅支持 amd64 / arm64 宿主架构，当前: ${HOST_ARCH}" >&2; exit 1 ;;
esac

# 目标架构校验：与宿主相同（本机原生构建）时不需要任何交叉工具链
if [ -n "$TARGET_ARCH" ]; then
  case "$TARGET_ARCH" in
    amd64 | arm64) ;;
    *) echo "ERROR: 仅支持 amd64 / arm64 目标架构，当前: ${TARGET_ARCH}" >&2; exit 1 ;;
  esac
  if [ "$INSTALL_CROSS" -eq 1 ] && [ "$TARGET_ARCH" = "$HOST_ARCH" ]; then
    INSTALL_CROSS=0
  fi
fi

# 交叉方向：优先用显式 --target-arch；未传时按宿主架构取反向（宿主机双架构编译场景）
CROSS_ARCH=""
if [ "$INSTALL_CROSS" -eq 1 ]; then
  if [ -n "$TARGET_ARCH" ]; then
    CROSS_ARCH="$TARGET_ARCH"
  else
    case "$HOST_ARCH" in
      amd64) CROSS_ARCH="arm64" ;;
      arm64) CROSS_ARCH="amd64" ;;
    esac
  fi
fi

# root / 非 root 自动判断（兼容 Dockerfile root 容器和 CI 非 root 用户）
if [ "$(id -u)" = "0" ]; then
  SUDO=""
else
  SUDO="sudo"
fi

echo ">>> setup-build-env: 宿主架构=${HOST_ARCH}, 交叉编译=${CROSS_ARCH:-<不需要>}, mirror=${MIRROR:-<跳过>}, sudo=${SUDO:-<root>}"

# ----------------------------------------------------------
# 1. 替换 apt mirror（仅 Dockerfile 场景传入）
# ----------------------------------------------------------
if [ -n "$MIRROR" ]; then
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do
    if [ -f "$f" ]; then
      $SUDO sed -i \
        "s|deb.debian.org|${MIRROR}|g; s|security.debian.org|${MIRROR}|g" \
        "$f"
    fi
  done
fi

# ----------------------------------------------------------
# 2. 原生编译依赖 + 交叉编译器
#    交叉编译器（gcc-aarch64-linux-gnu / gcc-x86_64-linux-gnu）本身是宿主架构的包，
#    原生 apt 源即可安装，不依赖反向架构源。
#    CROSS_ARCH 为空（目标与宿主相同 / --no-cross）时只装原生 libpcap-dev。
# ----------------------------------------------------------
$SUDO apt-get update

NATIVE_PKGS="libpcap-dev ca-certificates"
case "$CROSS_ARCH" in
  amd64) NATIVE_PKGS="$NATIVE_PKGS gcc-x86-64-linux-gnu" ;;
  arm64) NATIVE_PKGS="$NATIVE_PKGS gcc-aarch64-linux-gnu" ;;
esac

$SUDO apt-get install -y --no-install-recommends $NATIVE_PKGS

# ----------------------------------------------------------
# 3. 交叉目标架构的 libpcap（交叉编译时链接用）
#    amd64 目标 → 加 amd64 架构 → 装 libpcap-dev:amd64
#    arm64 目标 → 加 arm64 架构 → 装 libpcap-dev:arm64
#
#    注意：部分 apt 镜像源不同步所有 component 的反向架构索引，
#    apt-get update 可能因 restricted/multiverse 等次要组件 404 而返回非零。
#    但 libpcap 在 main 组件里，只要 main 的反向架构索引能拉到就能安装。
#    因此这里对 update 做容错（|| true），让 install 有机会执行。
# ----------------------------------------------------------
case "$CROSS_ARCH" in
  amd64)
    $SUDO dpkg --add-architecture amd64
    $SUDO apt-get update || true
    $SUDO apt-get install -y --no-install-recommends libpcap-dev:amd64
    ;;
  arm64)
    $SUDO dpkg --add-architecture arm64
    $SUDO apt-get update || true
    $SUDO apt-get install -y --no-install-recommends libpcap-dev:arm64
    ;;
esac

# ----------------------------------------------------------
# 3.1 验证交叉工具链就绪
# ----------------------------------------------------------
CROSS_CC=""
case "$CROSS_ARCH" in
  amd64) CROSS_CC="x86_64-linux-gnu-gcc" ;;
  arm64) CROSS_CC="aarch64-linux-gnu-gcc" ;;
esac
if [ -n "$CROSS_CC" ]; then
  if ! command -v "$CROSS_CC" >/dev/null 2>&1; then
    echo "ERROR: 交叉编译器 $CROSS_CC 未安装成功" >&2
    exit 1
  fi
  echo ">>> setup-build-env: 交叉编译器 $CROSS_CC 就绪"
fi

# ----------------------------------------------------------
# 4. 清理 apt lists（Dockerfile 默认清理；CI 可选保留）
# ----------------------------------------------------------
if [ "$KEEP_LISTS" -eq 0 ]; then
  $SUDO rm -rf /var/lib/apt/lists/*
fi

echo ">>> setup-build-env: 完成"
