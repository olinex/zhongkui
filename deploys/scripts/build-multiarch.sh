#!/usr/bin/env bash
#
# build-multiarch.sh - zki 二进制交叉编译（amd64 + arm64）
#
# 统一的交叉编译入口，供以下场景复用：
#   1. Gitee CI 前置编译步骤（--skip-apt 不设）：apt 装依赖 + 编译两架构 → dist/bin/
#   2. deploys/local/core.Dockerfile build 阶段（--skip-apt --arch --out）：
#      apt 由 builder 阶段完成，这里只编排单架构编译
#
# 交叉编译逻辑与原 deploys/local/core.Dockerfile 中 build_one 函数完全一致：
#   宿主 amd64 → 原生编译 amd64，用 aarch64-linux-gnu-gcc 交叉编译 arm64
#   宿主 arm64 → 原生编译 arm64，用 x86_64-linux-gnu-gcc 交叉编译 amd64
#
# 最大限度复用仓库内已有的 Makefile 目标：
#   make version-env   生成版本元数据 dist/build.env
#   make build-release 实际 go build（RELEASE_LDFLAGS 版本注入；不烘焙 RAG 快照，
#                      快照由外层脚本自行保证存在）
#
# 用法：
#   # CI 场景：同时编译两架构，产物落到 dist/bin/zki-linux-{arch}
#   deploys/scripts/build-multiarch.sh v1.2.3
#   VERSION=v1.2.3 deploys/scripts/build-multiarch.sh
#
#   # Dockerfile 场景：跳过 apt，只编译单一 TARGETARCH 到指定路径
#   deploys/scripts/build-multiarch.sh --skip-apt --arch amd64 --out /out/zki v1.2.3
#
#   # 版本元数据支持外部覆盖（Dockerfile 通过 --build-arg 注入）
#   COMMIT=abc123 BUILDER=ci@runner deploys/scripts/build-multiarch.sh v1.2.3
#
# 退出码：0 成功；非 0 失败。
set -euo pipefail

# ----------------------------------------------------------
# 参数解析
# ----------------------------------------------------------
SKIP_APT=0
ARCH=""
OUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-apt) SKIP_APT=1; shift ;;
    --arch)     ARCH="$2"; shift 2 ;;
    --out)      OUT="$2"; shift 2 ;;
    --*)        echo "ERROR: 未知选项 $1" >&2; exit 1 ;;
    *)          break ;;
  esac
done

VERSION="${1:-${VERSION:-}}"
if [ -z "$VERSION" ]; then
  echo "ERROR: 请设置版本号，例如: deploys/scripts/core-build-multiarch.sh v1.2.3" >&2
  exit 1
fi

if [ -n "$ARCH" ] && [ -z "$OUT" ]; then
  echo "ERROR: --arch 指定时必须同时用 --out 指定输出路径" >&2
  exit 1
fi

# ----------------------------------------------------------
# 路径 & Go 缓存
# ----------------------------------------------------------
ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR"

# Go 缓存固定到 /go 下，方便 Gitee CI 的 caches 字段持久化：
#   caches:
#     - /go/pkg/mod      # GOMODCACHE
#     - /go/go-build     # GOCACHE（amd64/arm64 共享同一目录，按 GOOS/GOARCH 分子目录）
export GOMODCACHE="${GOMODCACHE:-/go/pkg/mod}"
export GOCACHE="${GOCACHE:-/go/go-build}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

# ----------------------------------------------------------
# 1. 生成版本元数据（统一入口，CI / Dockerfile 都走这里）
#    COMMIT / BUILDER / BUILD_TIME 允许通过环境变量覆盖，
#    Dockerfile 用 --build-arg 传入，CI 走 Makefile 内部默认值。
# ----------------------------------------------------------
make version-env \
  VERSION="$VERSION" \
  COMMIT="${COMMIT:-unknown}" \
  BUILDER="${BUILDER:-docker}"
# shellcheck source=/dev/null
. ./dist/build.env

# ----------------------------------------------------------
# 2. 安装系统依赖（仅 CI 场景执行；Dockerfile builder 阶段已装）
#    委托给 setup-build-env.sh，参数 --host-arch 自动探测。
# ----------------------------------------------------------
HOST_ARCH=$(go env GOARCH)
echo ">>> 宿主架构: ${HOST_ARCH}"

if [ "$SKIP_APT" -eq 0 ]; then
  bash deploys/scripts/setup-build-env.sh --host-arch "$HOST_ARCH"
else
  echo ">>> 跳过 apt 安装（由 Dockerfile builder 阶段完成）"
fi

# ----------------------------------------------------------
# 3. 快照存在性检查（Makefile build-release 不烘焙，外层脚本负责保证）
# ----------------------------------------------------------
RAG_SNAPSHOT="internal/docs/ragdata/snapshot.json"
if [ ! -f "$RAG_SNAPSHOT" ]; then
  echo "ERROR: 缺少 RAG 知识快照 $RAG_SNAPSHOT" >&2
  echo "       请先执行 make rag-snapshot 确认快照已烘焙后提交进仓库" >&2
  echo "       （CI docs-check 在合入前把关快照与代码一致性）" >&2
  exit 1
fi

# ----------------------------------------------------------
# 4. 编译
# ----------------------------------------------------------

# build_one <goarch> <out>
# 按目标架构配置 CC/CGO_CFLAGS/CGO_LDFLAGS，再调用 make build-release。
# 原生 HOST_ARCH 编译无需交叉工具链；编译另一架构时由上面 apt（或 Dockerfile builder）预装的交叉工具链提供。
# 快照（internal/docs/ragdata/snapshot.json）需在调用前已存在。
build_one() {
  local goarch="$1" out="$2"
  local cc="" cgo_cflags="" cgo_ldflags=""

  case "${HOST_ARCH}-${goarch}" in
    amd64-arm64)
      cc=aarch64-linux-gnu-gcc
      cgo_cflags="-I/usr/include"
      cgo_ldflags="-L/usr/lib/aarch64-linux-gnu"
      ;;
    arm64-amd64)
      cc=x86_64-linux-gnu-gcc
      cgo_cflags="-I/usr/include"
      cgo_ldflags="-L/usr/lib/x86_64-linux-gnu"
      ;;
  esac

  echo ">>> 构建 linux/${goarch}${cc:+（交叉，CC=${cc}）}"
  CC="$cc" CGO_CFLAGS="$cgo_cflags" CGO_LDFLAGS="$cgo_ldflags" \
    GOOS=linux GOARCH="$goarch" CGO_ENABLED=1 \
    make build-release \
      OUT="$out" \
      VERSION="$VERSION" \
      COMMIT="$COMMIT" \
      BUILD_TIME="$BUILD_TIME" \
      BUILDER="$BUILDER"
}

if [ -n "$ARCH" ]; then
  # Dockerfile 场景：单架构编译到指定路径
  mkdir -p "$(dirname "$OUT")"
  build_one "$ARCH" "$OUT"
else
  # CI 场景：同时编译两架构
  mkdir -p dist/bin
  build_one amd64 dist/bin/zki-linux-amd64
  build_one arm64 dist/bin/zki-linux-arm64

  echo ">>> 构建完成:"
  ls -lh dist/bin/zki-linux-amd64 dist/bin/zki-linux-arm64
fi
