#
# zhongkui 后端镜像（原生架构）。
# 构建上下文为仓库根目录；版本信息（VERSION / COMMIT / BUILD_TIME / BUILDER）在构建阶段
# 由容器内部直接生成 dist/build.env，不依赖宿主机预先执行 make version-env。
#
#   scripts/docker-image.sh v1.2.3              # normal 变体（默认）
#   VARIANT=slim scripts/docker-image.sh v1.2.3 # slim 变体
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
ARG GOLANG_IMAGE=golang:1.27.3-bookworm
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com
ARG GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
# 最终镜像变体：normal 或 slim（见文件头注释）
ARG IMAGE_VARIANT=normal

FROM ${GOLANG_IMAGE} AS build

ARG APT_MIRROR
ARG GOPROXY
# 版本信息由构建参数注入，镜像构建阶段在容器内生成 dist/build.env：
#   VERSION   语义化版本号（必填）
#   COMMIT    git commit 短哈希（构建上下文不含 .git，需由调用方传入）
#   BUILDER   构建者标识
# BUILD_TIME 在容器内按 UTC 实时生成，保证镜像内时间戳准确。
ARG VERSION=
ARG COMMIT=unknown
ARG BUILDER=docker

RUN set -eu; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update \
    && apt-get install -y --no-install-recommends libpcap-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=1

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# 镜像编译通过 scripts/build-image-binary.sh 中转，scripts 再调用 Makefile 的 build-release，
# 保证与宿主机发布产物使用完全相同的构建参数（-trimpath、-s -w、同一套 ldflags）。
# 版本信息统一由 Makefile 的 version-env 生成，Dockerfile 不再重复实现；
# VERSION 必须是合法的语义化版本号，否则 make version-env 直接失败。
RUN set -eu; \
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

