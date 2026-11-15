#
# zhongkui normal 变体镜像（多架构：amd64 / arm64）。
#
# 构建策略（确定性）：
#   - 所有 RUN 都在 BUILDPLATFORM 上原生执行，目标架构层只做 FROM + COPY，
#     彻底避开 QEMU（apt-get / Go 二进制在 QEMU 下会慢几十倍甚至卡死）。
#   - 目标架构的运行期依赖通过 `apt-get install -d` 下载整条依赖闭包后
#     用 `dpkg-deb -x` 解包成 rootfs，再整体 COPY 进目标架构镜像。
#   - setcap 在构建时确定性授予，不探测运行时兼容性（避免构建结果依赖
#     构建环境）；兼容性由 entrypoint 在容器启动时处理，不兼容则
#     setcap -r 并 fallback 到 root 运行。
#
# 阶段：
#   builder       编译环境：Go + libpcap-dev + 按需交叉工具链 + qemu-user-static
#   runtime-cross 原生层准备目标架构的运行期 rootfs（库、账户、CA、时区）
#   deps          只拉 go.mod/go.sum，依赖层独立缓存
#   build         交叉编译 /out/zki，并生成默认配置（如需跨架构则用 qemu 执行）
#   runtime       目标架构基底 + 预准备的 rootfs
#   app           注入 zki、默认配置、entrypoint，授予抓包 capability
#
# slim 变体见 deploys/containers/slim.Dockerfile（从本镜像提取 zki，免编译）。
#
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f deploys/containers/normal.Dockerfile --build-arg VERSION=v1.2.3 \
#     -t registry/zhongkui:v1.2.3 --push .
#   scripts/image.sh normal v1.2.3

ARG GOLANG_IMAGE=golang:1.27.3-bookworm
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com
ARG GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct

# ============================================================
# builder：编译环境（原生执行）
# ============================================================
# 交叉工具链只在 TARGETARCH != BUILDARCH 时安装该方向的一套；
# qemu-user-static 供 build 阶段原生执行目标架构的 zki（生成默认配置）。
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS builder

ARG APT_MIRROR
ARG BUILDARCH
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

RUN set -eux; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update; \
    apt-get install -y --no-install-recommends libpcap-dev ca-certificates; \
    if [ "${TARGETARCH}" != "${BUILDARCH}" ]; then \
      case "${TARGETARCH}" in \
        arm64) apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu ;; \
        amd64) apt-get install -y --no-install-recommends gcc-x86-64-linux-gnu ;; \
        *) echo "不支持的 TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
      esac; \
      apt-get install -y --no-install-recommends qemu-user-static; \
      dpkg --add-architecture "${TARGETARCH}"; \
      apt-get update || true; \
      apt-get install -y --no-install-recommends "libpcap-dev:${TARGETARCH}"; \
    fi; \
    rm -rf /var/lib/apt/lists/*

# ============================================================
# runtime-cross：原生层准备目标架构的运行期 rootfs
# ============================================================
# 输出 /runtime-root/，后续目标架构 runtime 层直接 COPY 到 /，零 RUN。
#   - CA 证书 / 时区数据与架构无关：原生安装让 postinst 生成证书包后直接搬文件
#   - 架构相关包（libpcap0.8 / gosu / libcap2-bin / wget）：按目标架构下载
#     整条依赖闭包再解包，避免在 QEMU 中执行 dpkg
FROM --platform=$BUILDPLATFORM ${RUNTIME_IMAGE} AS runtime-cross

ARG APT_MIRROR
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive

RUN set -eux; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    mkdir -p /runtime-root/etc/ssl/certs /runtime-root/usr/share /runtime-root/app; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates tzdata; \
    cp -a /etc/ssl/certs/ca-certificates.crt /runtime-root/etc/ssl/certs/; \
    cp -a /usr/share/zoneinfo /runtime-root/usr/share/; \
    rm -f /var/cache/apt/archives/*.deb; \
    dpkg --add-architecture "${TARGETARCH}"; \
    apt-get update; \
    apt-get install -d -y --no-install-recommends \
      "libpcap0.8:${TARGETARCH}" \
      "gosu:${TARGETARCH}" \
      "libcap2-bin:${TARGETARCH}" \
      "wget:${TARGETARCH}"; \
    for d in /var/cache/apt/archives/*.deb; do \
      rm -rf /tmp/x; mkdir -p /tmp/x; \
      dpkg-deb -x "$d" /tmp/x; \
      for sub in bin sbin lib lib64; do \
        if [ -d "/tmp/x/$sub" ]; then \
          mkdir -p "/runtime-root/usr/$sub"; \
          cp -a "/tmp/x/$sub/." "/runtime-root/usr/$sub/"; \
        fi; \
      done; \
      for sub in usr etc var; do \
        if [ -d "/tmp/x/$sub" ]; then \
          mkdir -p "/runtime-root/$sub"; \
          cp -a "/tmp/x/$sub/." "/runtime-root/$sub/"; \
        fi; \
      done; \
    done; \
    rm -rf /tmp/x /var/lib/apt/lists/* /var/cache/apt/archives/*.deb; \
    cp /etc/passwd /etc/group /etc/nsswitch.conf /runtime-root/etc/; \
    echo 'app:x:1000:1000:app:/app:/usr/sbin/nologin' >> /runtime-root/etc/passwd; \
    echo 'app:x:1000:' >> /runtime-root/etc/group; \
    chown 1000:1000 /runtime-root/app

# ============================================================
# deps：Go 依赖层（原生执行）
# ============================================================
FROM builder AS deps

ARG GOPROXY
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# ============================================================
# build：交叉编译 + 生成默认配置（原生执行）
# ============================================================
FROM deps AS build

WORKDIR /src

ARG VERSION
ARG COMMIT=unknown
ARG BUILDER=docker
ARG BUILDARCH
ARG TARGETARCH
ARG GOMEMLIMIT=off
ARG GOFLAGS

COPY . .

# COPY 会用仓库里的 go.sum 覆盖 deps 层生成的校验和文件，这里基于已下载的
# 模块缓存重新补齐，保证 go build（默认 -mod=readonly）不因缺校验和失败。
RUN go mod download

ENV CGO_ENABLED=1 \
    GOMEMLIMIT=${GOMEMLIMIT} \
    GOFLAGS=${GOFLAGS}

# 版本元数据 + 单架构交叉编译：
#   交叉时按方向选择 CC 与目标库路径（交叉工具链与 libpcap-dev:<arch> 已在 builder 就绪）
#   快照由仓库提交，构建环境无 protoc，不再烘焙
RUN set -eux; \
    make version-env VERSION="${VERSION}" COMMIT="${COMMIT}" BUILDER="${BUILDER}"; \
    . ./dist/build.env; \
    test -f internal/docs/ragdata/snapshot.json || { \
      echo "缺少 RAG 知识快照 internal/docs/ragdata/snapshot.json，请先 make rag-snapshot 后提交" >&2; \
      exit 1; \
    }; \
    cc=""; cgo_ldflags=""; \
    if [ "${BUILDARCH}" != "${TARGETARCH}" ]; then \
      case "${TARGETARCH}" in \
        arm64) cc=aarch64-linux-gnu-gcc; cgo_ldflags="-L/usr/lib/aarch64-linux-gnu" ;; \
        amd64) cc=x86_64-linux-gnu-gcc; cgo_ldflags="-L/usr/lib/x86_64-linux-gnu" ;; \
      esac; \
    fi; \
    CC="${cc}" CGO_CFLAGS="-I/usr/include" CGO_LDFLAGS="${cgo_ldflags}" \
      GOOS=linux GOARCH="${TARGETARCH}" \
      make build-release OUT=/out/zki VERSION="${VERSION}" COMMIT="${COMMIT}" \
        BUILD_TIME="${BUILD_TIME}" BUILDER="${BUILDER}"

# 生成镜像内置的默认配置：原生架构直接执行，跨架构用 qemu-user-static 执行，
# 避免在目标架构层跑 Go 二进制（QEMU 下极慢）。
RUN set -eux; \
    if [ "${BUILDARCH}" = "${TARGETARCH}" ]; then \
      /out/zki create default-config -o /out/default-config.toml; \
    else \
      "qemu-${TARGETARCH}-static" /out/zki create default-config -o /out/default-config.toml; \
    fi; \
    test -s /out/default-config.toml

# ============================================================
# runtime：目标架构运行基底 + 预准备的 rootfs（零 RUN）
# ============================================================
FROM ${RUNTIME_IMAGE} AS runtime

COPY --from=runtime-cross /runtime-root/ /

# ============================================================
# app：注入制品与 entrypoint，授予抓包 capability
# ============================================================
FROM runtime AS app

COPY --from=build /out/zki /usr/local/bin/zki
COPY --from=build /out/default-config.toml /app/default-config.toml

# entrypoint wrapper：
#   1. 运行时 capability 兼容性检测：
#      构建时已 setcap，但部分运行时（OrbStack/QEMU）exec 会返回 EPERM；
#      这里在启动时探测一次，不兼容则 setcap -r 并 fallback 到 root 运行。
#   2. 默认模式（CMD 传入 zki 子命令）→ 直接 exec zki
#   3. 外部工具（如 execute@docker）→ /bin/sh -c 执行命令数组
#   4. $1 非已知子命令时按 shell 命令处理
COPY <<'ENTRYPOINT' /entrypoint.sh
#!/bin/sh
set -eu

# 运行时 capability 兼容性检测：
# setcap 写入是确定性的（构建时完成），但部分运行时内核不支持 file capability，
# 非 root exec 会返回 EPERM。此时清除 capability 并 fallback 到 root 运行
# （牺牲最小权限换取服务可用性）。
CAP_INCOMPAT=0
if [ -x /usr/sbin/gosu ]; then
  if ! gosu app /usr/local/bin/zki --help >/dev/null 2>&1; then
    setcap -r /usr/local/bin/zki 2>/dev/null || true
    CAP_INCOMPAT=1
  fi
fi

is_zki() {
  case "$1" in
    ""|-*) return 0 ;;   # 空 or flag → 直接认为是 zki
    *)     /usr/local/bin/zki "$1" --help >/dev/null 2>&1 ;;
  esac
}

if [ "$CAP_INCOMPAT" -eq 0 ] && [ -x /usr/sbin/gosu ]; then
  # capability 生效：用 app 用户运行（非 root 抓包可用）
  if is_zki "${1:-}"; then
    exec gosu app /usr/local/bin/zki "$@"
  fi
  exec gosu app /bin/sh -c "$(printf '%s\n' "$@")"
fi

# capability 不兼容或无 gosu：用 root 运行
if is_zki "${1:-}"; then
  exec /usr/local/bin/zki "$@"
fi
exec /bin/sh -c "$(printf '%s\n' "$@")"
ENTRYPOINT

# 确定性授予抓包 capability（构建结果与运行时环境无关）。
# 部分运行时（OrbStack/QEMU）setcap 写入成功但 exec 返回 EPERM，
# 该兼容性由 entrypoint 在运行时处理，不在构建时探测。
RUN set -eux; \
    chmod 0755 /usr/local/bin/zki /entrypoint.sh; \
    setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/zki 2>/dev/null || true; \
    getcap /usr/local/bin/zki 2>/dev/null || echo "capability not visible (ok on some filesystems)"

WORKDIR /app
EXPOSE 8080 8081 19090

# 内置健康探针服务（19090）：/healthz 存活、/readyz 依赖就绪、/startupz 启动完成
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/entrypoint.sh"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

FROM app AS final-normal
