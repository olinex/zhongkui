#
# zhongkui 后端镜像（多架构：amd64 / arm64；其他架构需在对应架构的构建节点上构建）。
# 构建上下文为仓库根目录；版本信息（VERSION / COMMIT / BUILD_TIME / BUILDER）在构建阶段
# 由容器内部直接生成 dist/build.env，不依赖宿主机预先执行 make version-env。
#
# 目标平台由 BuildKit 的 TARGETOS / TARGETARCH / TARGETVARIANT 自动注入，无需手工传参：
#   docker buildx build --platform linux/amd64 -t zhongkui:local --load --build-arg VERSION=v1.2.3 .
#   docker buildx build --platform linux/arm64 -t zhongkui:local-arm64 --load --build-arg VERSION=v1.2.3 .
#   docker buildx build --platform linux/amd64,linux/arm64 -t registry/zhongkui:v1.2.3 --push .
# 采集侧依赖 cgo（libpcap），构建阶段固定在构建机原生架构（$BUILDPLATFORM）执行：
# amd64 构建机上的 arm64 目标通过 aarch64 交叉工具链编译，不经 QEMU 模拟，
# 因此多平台构建的耗时与单平台基本一致；运行基底仍按目标架构拉取并安装 libpcap0.8。
#
#   scripts/docker-image.sh v1.2.3                      # 本机原生架构，normal 变体（默认）
#   VARIANT=slim scripts/docker-image.sh v1.2.3         # slim 变体
#   PLATFORMS=linux/arm64 scripts/docker-image.sh v1.2.3
#   PLATFORMS=linux/amd64,linux/arm64 scripts/docker-image.sh v1.2.3 --push
#
# 镜像变体由 --build-arg IMAGE_VARIANT=normal|slim 控制（默认 normal）：
#   normal  基于 debian:bookworm-slim，保留 shell 与系统工具，便于线上排障（约 97MB）
#   slim    基于 scratch，仅含二进制、动态库闭包、CA 证书、时区数据与最小 etc 配置（约 13MB）
#
# 直接执行 docker build 时通过 --build-arg 传入版本信息，例如：
#   docker build --build-arg VERSION=v1.2.3 --build-arg COMMIT=$(git rev-parse --short HEAD) .
# 未指定合法 VERSION 时构建直接失败，不允许产出 unknown 版本的镜像。
#
# 运行（配置文件与证书通过挂载提供）：
#   docker run --rm -p 8080:8080 -p 8081:8081 \
#     -v "$PWD/config.toml:/app/config.toml:ro" \
#     -v "$PWD/certs:/app/certs:ro" zhongkui:local

# 基础镜像、apt 源、Go 模块代理均可通过 --build-arg 覆盖（CI 中指向国内/内网地址）。
# 基础镜像均为多架构镜像，--platform 指定的目标架构会自动选取对应 manifest。
ARG GOLANG_IMAGE=golang:1.27.3-bookworm
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com
ARG GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
# 最终镜像变体：normal 或 slim（见文件头注释）
ARG IMAGE_VARIANT=normal

# 构建阶段固定跑在构建机原生架构上（BUILDPLATFORM），配合下方 aarch64 交叉工具链，
# 避免 arm64 目标经 QEMU 模拟执行整个 cgo 编译（实测慢 8 倍以上）。
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS build

ARG APT_MIRROR
ARG GOPROXY
# 目标平台参数由 BuildKit 自动注入（来自 --platform，未指定时为构建机原生平台），
# 用于选择交叉工具链并强制 Go 编译产物与镜像平台一致。
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
# BUILDARCH 是构建机架构，用于判断是否需要交叉编译。
ARG BUILDARCH
# 版本信息由构建参数注入，镜像构建阶段在容器内生成 dist/build.env：
#   VERSION   语义化版本号（必填）
#   COMMIT    git commit 短哈希（构建上下文不含 .git，需由调用方传入）
#   BUILDER   构建者标识
# BUILD_TIME 在容器内按 UTC 实时生成，保证镜像内时间戳准确。
ARG VERSION=
ARG COMMIT=unknown
ARG BUILDER=docker

# 交叉编译 arm64 需要：arm64 的 libpcap 开发包（multiarch）+ aarch64 交叉工具链。
# 构建机本身是 arm64 时无需交叉工具链，只安装原生 libpcap-dev。
RUN set -eu; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update \
    && apt-get install -y --no-install-recommends libpcap-dev ca-certificates \
    && if [ "$BUILDARCH" != "arm64" ]; then \
         dpkg --add-architecture arm64; \
         apt-get update; \
         apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu libpcap-dev:arm64; \
       fi; \
    rm -rf /var/lib/apt/lists/*

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=1 \
    GOMODCACHE=/go/pkg/mod \
    GOCACHE=/root/.cache/go-build

WORKDIR /src
COPY go.mod go.sum ./
# 模块缓存与 Go 编译缓存都放在 BuildKit cache mount 上：镜像层内的缓存会随 RUN 层失效
# 一起丢弃，导致每次 tag 构建都要从零编译全部依赖。
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    go mod download
COPY . .

# 镜像编译由 scripts/build-image-binary.sh 直接完成，参数与宿主机发布构建（make build-release）
# 保持一致（-trimpath、-s -w、同一套 ldflags）。
# RAG 快照直接复用仓库内已提交的 internal/docs/ragdata/snapshot.json，不在镜像内重新烘焙：
# 快照内容与 CPU 架构无关，重新烘焙只是额外一次全量编译（arm64 经 QEMU 时占单次构建一半耗时），
# 一致性由 CI 的 docs-check 在合入前把关。
# 目标平台统一转换为 GOOS/GOARCH（arm 32 位再映射 GOARM），并据此选择交叉工具链：
# 目标架构与构建架构不一致时只允许 amd64 → arm64（aarch64-linux-gnu-gcc），其余组合直接拒绝，
# 避免产出架构错误或链接错库的二进制。
# 版本信息统一由 Makefile 的 version-env 生成，Dockerfile 不再重复实现；
# VERSION 必须是合法的语义化版本号，否则 make version-env 直接失败。
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build \
    set -eu; \
    goos="${TARGETOS:-$(go env GOHOSTOS)}"; \
    goarch="${TARGETARCH:-$(go env GOHOSTARCH)}"; \
    buildarch="${BUILDARCH:-$(go env GOHOSTARCH)}"; \
    if [ "$goarch" != "$buildarch" ]; then \
      if [ "$buildarch" = "amd64" ] && [ "$goarch" = "arm64" ]; then \
        export CC="aarch64-linux-gnu-gcc" CXX="aarch64-linux-gnu-g++"; \
        export CGO_CFLAGS="-I/usr/include ${CGO_CFLAGS:-}"; \
        export CGO_LDFLAGS="-L/usr/lib/aarch64-linux-gnu ${CGO_LDFLAGS:-}"; \
      else \
        echo "ERROR: 不支持从 ${buildarch} 交叉编译到 ${goarch}。" >&2; \
        echo "       当前仅内置 amd64 → arm64 的交叉工具链；其余架构请在该架构的构建节点上构建。" >&2; \
        exit 1; \
      fi; \
    fi; \
    if [ "$goarch" = "arm" ] && [ -n "${TARGETVARIANT:-}" ]; then \
      export GOARM="${TARGETVARIANT#v}"; \
    fi; \
    export GOOS="$goos" GOARCH="$goarch"; \
    echo ">>> 目标平台 ${GOOS}/${GOARCH}（构建架构 ${buildarch}${CC:+，CC=${CC}}）${GOARM:+ (GOARM=${GOARM})}"; \
    make version-env VERSION="${VERSION}" COMMIT="${COMMIT}" BUILDER="${BUILDER}"; \
    set -a; . ./dist/build.env; set +a; \
    scripts/build-image-binary.sh

# 运行期公共基底：仅安装 libpcap（CGO 抓包必需）、CA 证书与时区数据，
# 供 normal 变体直接使用，也是 slim 变体收集最小文件集的来源。
FROM ${RUNTIME_IMAGE} AS runtime-base

ARG APT_MIRROR

RUN set -eu; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update \
    && apt-get install -y --no-install-recommends libpcap0.8 ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/zki /usr/local/bin/zki

# slim 变体的最小文件集装配：二进制、动态库闭包、CA 证书、时区数据与最小 etc 配置。
FROM runtime-base AS slim-prep

RUN set -eu; \
    root=/rootfs; \
    mkdir -p "$root/tmp" "$root/app" "$root/etc/ssl/certs"; \
    # 1) 二进制动态库闭包（libpcap 及其间接依赖、glibc、ld.so）
    mkdir -p "$root/usr/local/bin"; \
    cp -a /usr/local/bin/zki "$root/usr/local/bin/zki"; \
    ldd /usr/local/bin/zki | awk '{ if ($2 == "=>") print $3; else if ($1 ~ /^\//) print $1 }' | \
      grep '^/' | sort -u | \
      while read -r lib; do \
        mkdir -p "$root$(dirname "$lib")"; \
        cp -L "$lib" "$root$lib"; \
      done; \
    # 2) cgo 解析器按需加载的 NSS 模块
    for m in libnss_files.so.2 libnss_dns.so.2 libnss_compat.so.2; do \
      p=$(ls /usr/lib/*-linux-gnu/"$m" 2>/dev/null | head -n 1 || true); \
      if [ -n "$p" ]; then \
        mkdir -p "$root$(dirname "$p")"; \
        cp -L "$p" "$root$p"; \
      fi; \
    done; \
    # 3) CA 证书、时区数据与最小 etc 配置
    cp -a /etc/ssl/certs/ca-certificates.crt "$root/etc/ssl/certs/"; \
    cp -a /usr/share/zoneinfo "$root/usr/share/"; \
    for f in nsswitch.conf passwd group services protocols; do \
      if [ -f "/etc/$f" ]; then cp -a "/etc/$f" "$root/etc/"; fi; \
    done; \
    chmod 1777 "$root/tmp"

# normal 变体：保留 debian 基底的 shell 与系统工具，便于线上排障。
FROM runtime-base AS final-normal

WORKDIR /app
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/config.toml"]

# slim 变体：基于 scratch，不含 shell/包管理器，体积最小、攻击面最小。
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

WORKDIR /app
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/config.toml"]

# 按 IMAGE_VARIANT 选择最终镜像；变体名非法时此处找不到目标阶段，构建直接失败。
FROM final-${IMAGE_VARIANT} AS final

