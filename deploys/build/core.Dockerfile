#
# zhongkui 后端镜像（多架构：amd64 / arm64），单个 Dockerfile 完成交叉编译与镜像装配。
# 构建上下文为仓库根目录，源码在镜像构建阶段内编译，不再依赖宿主机的 dist/bin 预编译产物。
#
# 五个构建步骤：
#   步骤 1  builder  交叉编译环境：Go 工具链 + amd64 ↔ arm64 双向交叉工具链。
#                    固定在 BUILDPLATFORM 上运行，始终原生编译，不经 QEMU。
#   步骤 2  runtime  最终运行环境：debian 基底、运行期依赖（libpcap0.8 / CA / 时区）与非 root 运行用户。
#   步骤 3  deps     Go 包环境：只引入 go.mod / go.sum 并 go mod download，
#                    依赖层与源码层分离，源码变更不会触发重新下载。
#   步骤 4  build    制品构建：引入源码，make version-env 注入版本元数据后交叉编译出 /out/zki。
#   步骤 5  装配     把 /out/zki 注入运行环境，产出 normal / slim 两个变体。
#
# 目标平台由 BuildKit 的 TARGETARCH 自动注入，按架构选取二进制与交叉工具链，无需手工传参：
#   docker buildx build --platform linux/amd64 -f deploys/build/core.Dockerfile \
#     --build-arg VERSION=v1.2.3 -t zhongkui:local --load .
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploys/build/core.Dockerfile \
#     --build-arg VERSION=v1.2.3 -t registry/zhongkui:v1.2.3 --push .
#
#   deploys/build/docker-image.sh v1.2.3                      # 本机原生架构，normal 变体（默认）
#   VARIANT=slim deploys/build/docker-image.sh v1.2.3         # slim 变体
#   PLATFORMS=linux/amd64,linux/arm64 deploys/build/docker-image.sh v1.2.3 --push
#
# 镜像变体由 --build-arg IMAGE_VARIANT=normal|slim 控制（默认 normal）：
#   normal  基于 debian:bookworm-slim，保留 shell 与系统工具，便于线上排障（约 97MB）
#   slim    基于 scratch，仅含二进制、动态库闭包、CA 证书、时区数据与最小 etc 配置（约 13MB）
#
# 运行（镜像内置 /app/default-config.toml，由 zki create default-config 生成）：
#   docker run --rm -p 8080:8080 -p 8081:8081 zhongkui:local
# 生产环境建议挂载自有配置覆盖默认项：
#   docker run --rm -p 8080:8080 -p 8081:8081 \
#     -v "$PWD/config.toml:/app/config.toml:ro" \
#     -v "$PWD/certs:/app/certs:ro" zhongkui:local \
#     run web --config /app/config.toml

# 基础镜像与软件源可通过 --build-arg 覆盖（CI 中指向国内/内网地址）。
ARG GOLANG_IMAGE=golang:1.27.3-bookworm
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com
ARG GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
# 最终镜像变体：normal 或 slim（见文件头注释）
ARG IMAGE_VARIANT=normal

# ============================================================
# 步骤 1：交叉编译环境（Go 工具链 + 双向交叉工具链）
# ============================================================
# 固定在构建机架构上运行，用 TARGETARCH 指定编译目标；构建机与目标架构不一致时
# 才安装对应方向的 GNU 交叉工具链和反向架构的 libpcap 开发包：
#   amd64 构建机 → gcc-aarch64-linux-gnu + libpcap-dev:arm64（交叉出 arm64）
#   arm64 构建机 → gcc-x86-64-linux-gnu + libpcap-dev:amd64（交叉出 amd64）
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS builder

ARG APT_MIRROR
ARG GOPROXY
# BUILDARCH / TARGETARCH 由 BuildKit 自动注入（构建机架构 / 目标架构）。
ARG BUILDARCH
ARG TARGETARCH

RUN set -eu; \
    case "${TARGETARCH:-$BUILDARCH}" in \
      amd64 | arm64) ;; \
      *) echo "ERROR: 仅支持 amd64 与 arm64，当前 TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update \
    && apt-get install -y --no-install-recommends libpcap-dev ca-certificates \
    && if [ "${TARGETARCH:-$BUILDARCH}" != "$BUILDARCH" ]; then \
         dpkg --add-architecture "${TARGETARCH}"; \
         apt-get update; \
         if [ "$TARGETARCH" = "arm64" ]; then \
           apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu libpcap-dev:arm64; \
         else \
           apt-get install -y --no-install-recommends gcc-x86-64-linux-gnu libpcap-dev:amd64; \
         fi; \
       fi; \
    rm -rf /var/lib/apt/lists/*

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=1

# ============================================================
# 步骤 2：最终运行环境（目标架构）
# ============================================================
# 安装运行时依赖并创建非 root 运行用户；normal 变体直接继承本基底，
# slim 变体从这里挑选文件组装最小 rootfs。
FROM ${RUNTIME_IMAGE} AS runtime

ARG APT_MIRROR

# gosu 用于 normal 变体降权（比 sudo 轻量）；libcap2-bin 提供 setcap，
# 给 zki 二进制授予抓包所需的 CAP_NET_RAW/CAP_NET_ADMIN，让非 root 用户也能抓包。
RUN set -eu; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update \
    && apt-get install -y --no-install-recommends libpcap0.8 ca-certificates tzdata gosu libcap2-bin \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1000 app \
    && useradd  -u 1000 -g app -d /app -s /usr/sbin/nologin -M app \
    && mkdir -p /app \
    && chown app:app /app

# ============================================================
# 步骤 3：Go 包环境（依赖层独立缓存）
# ============================================================
# 只引入 go.mod / go.sum：依赖不变时该层直接命中缓存，源码变更不会触发重新下载。
FROM builder AS deps

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# ============================================================
# 步骤 4：引入源码并构建制品
# ============================================================
# 版本元数据由 make version-env 统一生成 dist/build.env 后 source，
# 二进制由 make build-image 编译（与 build-release 同参数，复用仓库内已提交的 RAG 快照）。
FROM deps AS build

WORKDIR /src

ARG VERSION
ARG COMMIT=unknown
ARG BUILDER=docker
ARG BUILDARCH
ARG TARGETARCH

COPY . .

# COPY 会用仓库里的 go.sum 覆盖步骤 3 生成的校验和文件，这里基于已下载的模块缓存
# 重新补齐 go.sum，保证后续 go build（默认 -mod=readonly）不会因缺少校验和而失败。
RUN go mod download

RUN set -eu; \
    make version-env VERSION="$VERSION" COMMIT="$COMMIT" BUILDER="$BUILDER"; \
    set -a; . ./dist/build.env; set +a; \
    unset CC CXX CGO_CFLAGS CGO_LDFLAGS 2>/dev/null || true; \
    case "${BUILDARCH}-${TARGETARCH}" in \
      amd64-arm64) \
        export CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++ \
               CGO_CFLAGS="-I/usr/include" CGO_LDFLAGS="-L/usr/lib/aarch64-linux-gnu" ;; \
      arm64-amd64) \
        export CC=x86_64-linux-gnu-gcc CXX=x86_64-linux-gnu-g++ \
               CGO_CFLAGS="-I/usr/include" CGO_LDFLAGS="-L/usr/lib/x86_64-linux-gnu" ;; \
      *) : ;; \
    esac; \
    echo ">>> 构建 linux/${TARGETARCH}（构建架构 ${BUILDARCH}${CC:+，CC=${CC}}）"; \
    GOOS=linux GOARCH="$TARGETARCH" make build-image OUT=/out/zki

# ============================================================
# 步骤 5：把制品注入最终运行环境
# ============================================================
# 先装配通用应用层（二进制 + 抓包 capability + 内置默认配置），
# 再由 normal / slim 两个变体分别产出最终镜像。
FROM runtime AS app

COPY --from=build /out/zki /usr/local/bin/zki
RUN chmod 0755 /usr/local/bin/zki

# 给 zki 授予抓包所需的 CAP_NET_RAW/CAP_NET_ADMIN，让非 root 用户也能抓包。
# 部分运行时（QEMU/OrbStack）setcap 写入成功但 exec 时 EPERM，
# 这里用 gosu app 做兼容性探测，失败则主动清除 capability 扩展属性。
RUN setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/zki 2>/dev/null || true; \
    if gosu app /usr/local/bin/zki --help >/dev/null 2>&1; then \
      echo "OK: file capability 在当前运行时生效"; \
    else \
      echo "WARN: file capability 不兼容，清除 setcap（非 root 抓包将不可用）"; \
      setcap -r /usr/local/bin/zki 2>/dev/null || true; \
    fi

# 生成镜像内置的默认配置：该命令依据代码中的配置结构体序列化 TOML，无需外部 config.toml，
# 产物 /app/default-config.toml 供默认 CMD 使用；生产环境可挂载自有配置覆盖。
RUN /usr/local/bin/zki create default-config -o /app/default-config.toml \
    && test -s /app/default-config.toml

# normal 变体：保留 debian 基底的 shell 与系统工具，便于线上排障。
# 注入 entrypoint wrapper（仓库 deploys/build/docker-entrypoint.sh）：
#   1. 默认模式（CMD 传入 zki 子命令）：wrapper 直接 exec zki
#   2. 外部工具（如 execute@docker）：传入 shell 命令数组，wrapper 用 /bin/sh -c 执行
#      这解决了 ENTRYPOINT 是二进制时 execute@docker 无法正确执行 shell 命令的问题。
FROM app AS final-normal

COPY deploys/build/docker-entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

WORKDIR /app
EXPOSE 8080 8081

ENTRYPOINT ["/entrypoint.sh"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

# slim 变体的最小文件集装配：二进制、动态库闭包、CA 证书、时区数据与最小 etc 配置。
FROM app AS slim-prep

RUN set -eu; \
    root=/rootfs; \
    mkdir -p "$root/tmp" "$root/app" "$root/etc/ssl/certs" "$root/usr/local/bin"; \
    # 1) 二进制（cp -a 保留 setcap 写入的 security.capability 扩展属性）
    cp -a /usr/local/bin/zki "$root/usr/local/bin/zki"; \
    # 2) 二进制动态库闭包（libpcap 及其间接依赖、glibc、ld.so）
    ldd /usr/local/bin/zki | awk '{ if ($2 == "=>") print $3; else if ($1 ~ /^\//) print $1 }' | \
      grep '^/' | sort -u | \
      while read -r lib; do \
        mkdir -p "$root$(dirname "$lib")"; \
        cp -L "$lib" "$root$lib"; \
      done; \
    # 3) cgo 解析器按需加载的 NSS 模块
    for m in libnss_files.so.2 libnss_dns.so.2 libnss_compat.so.2; do \
      p=$(ls /usr/lib/*-linux-gnu/"$m" 2>/dev/null | head -n 1 || true); \
      if [ -n "$p" ]; then \
        mkdir -p "$root$(dirname "$p")"; \
        cp -L "$p" "$root$p"; \
      fi; \
    done; \
    # 4) CA 证书、时区数据与最小 etc 配置
    cp -a /etc/ssl/certs/ca-certificates.crt "$root/etc/ssl/certs/"; \
    cp -a /usr/share/zoneinfo "$root/usr/share/"; \
    for f in nsswitch.conf passwd group services protocols; do \
      if [ -f "/etc/$f" ]; then cp -a "/etc/$f" "$root/etc/"; fi; \
    done; \
    # 5) 镜像内置默认配置（供默认 CMD 使用）
    cp -a /app/default-config.toml "$root/app/default-config.toml"; \
    chmod 1777 "$root/tmp"

# slim 变体：基于 scratch，不含 shell/包管理器，体积最小、攻击面最小。
# 以 app 用户（uid=1000）非 root 运行；zki 通过 setcap 已授予抓包 capability。
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

USER 1000:1000
WORKDIR /app
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

# 按 IMAGE_VARIANT 选择最终镜像；变体名非法时此处找不到目标阶段，构建直接失败。
FROM final-${IMAGE_VARIANT} AS final
