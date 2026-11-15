#
# zhongkui slim 变体镜像（多架构：amd64 / arm64），免编译。
#
# 从已构建的 normal 变体提取 zki 二进制与默认配置，
# 在 slim-prep（BUILDPLATFORM 原生层）重新授予抓包 capability，
# 解析动态库闭包，组装成 scratch 镜像所需的最小 rootfs。
#
# 注意：不直接沿用 normal 镜像中 zki 的 capability 状态。
# normal 的 app 阶段会在构建时探测 file capability 兼容性，
# 若构建环境不支持（如 QEMU / OrbStack）会清除 setcap。
# 该探测针对的是构建环境而非运行环境，结论对 slim 无意义，
# 因此 slim 在 slim-prep 原生层确定性地重新 setcap，运行时是否生效由内核决定。
#
# 阶段：
#   normal      中转源阶段（FROM ${NORMAL_IMAGE}）：COPY --from 不支持 ARG 展开，
#               先由 FROM 生成命名阶段，后续用 COPY --from=normal 引用
#   slim-tools  busybox-static，供 scratch 的 HEALTHCHECK 与紧急 shell 使用
#   slim-prep   安装 libpcap0.8 / CA / 时区 / libcap2-bin，收集动态库闭包与 NSS 模块，
#               重新授予 zki 抓包 capability，产出 /rootfs
#   final-slim  scratch + /rootfs
#
# 前提：normal 变体已构建并可通过 NORMAL_IMAGE 访问（本地 tag 或 registry）。
# 多平台构建时 NORMAL_IMAGE 须为已推送的多架构 manifest list。
#
#   scripts/image.sh slim v1.2.3
#   docker buildx build -f deploys/containers/slim.Dockerfile \
#     --build-arg NORMAL_IMAGE=zhongkui:v1.2.3 -t zhongkui:v1.2.3-slim --load .

ARG NORMAL_IMAGE
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com

# ============================================================
# normal：中转源阶段
# ============================================================
FROM ${NORMAL_IMAGE} AS normal

# ============================================================
# slim-tools：busybox-static
# ============================================================
FROM alpine:3.20 AS slim-tools
RUN apk add --no-cache busybox-static

# ============================================================
# slim-prep：组装最小 rootfs
# ============================================================
FROM ${RUNTIME_IMAGE} AS slim-prep

ARG APT_MIRROR

ENV DEBIAN_FRONTEND=noninteractive

# 安装 assemble 阶段需要的运行期库与数据（ldd 解析闭包、复制 CA 与时区）。
# libcap2-bin 用于在 slim-prep 原生层重新授予 zki 抓包 capability，
# 不依赖 normal 镜像构建时的探测结果（构建环境与运行环境可能不同）。
RUN set -eux; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update; \
    apt-get install -y --no-install-recommends libpcap0.8 ca-certificates tzdata libcap2-bin; \
    rm -rf /var/lib/apt/lists/*

# 从 normal 提取制品（COPY 会保留 security.capability 扩展属性，
# 但 slim-prep 会重新 setcap，不依赖 normal 的 capability 状态）
COPY --from=normal /usr/local/bin/zki /usr/local/bin/zki
COPY --from=normal /app/default-config.toml /app/default-config.toml

# busybox-static（scratch 无 shell，HEALTHCHECK 依赖它）。
# 注意：Alpine 的 busybox-static 包产出的是 /bin/busybox.static，
# 不能复制 /bin/busybox（那是动态链接 musl 的版本，scratch 无 musl 加载器）。
COPY --from=slim-tools /bin/busybox.static /tmp/busybox

# 组装 rootfs：二进制、动态库闭包、NSS 模块、CA/时区、最小 etc、默认配置、busybox
RUN set -eux; \
    mkdir -p /rootfs/tmp /rootfs/app /rootfs/etc/ssl/certs /rootfs/usr/local/bin \
             /rootfs/usr/share /rootfs/bin; \
    cp -a /usr/local/bin/zki /rootfs/usr/local/bin/zki; \
    # 重新授予抓包 capability：normal 构建时的探测可能因构建环境不兼容而清除，
    # 这里在 slim-prep 原生层确定性地 setcap，运行时是否生效由内核决定。
    setcap cap_net_raw,cap_net_admin=eip /rootfs/usr/local/bin/zki; \
    ldd /usr/local/bin/zki \
      | awk '{ if ($2 == "=>") print $3; else if ($1 ~ /^\//) print $1 }' \
      | grep '^/' | sort -u \
      | while read -r lib; do \
          mkdir -p "/rootfs$(dirname "$lib")"; \
          cp -L "$lib" "/rootfs$lib"; \
        done; \
    for m in libnss_files.so.2 libnss_dns.so.2 libnss_compat.so.2; do \
      p=$(ls /usr/lib/*-linux-gnu/"$m" 2>/dev/null | head -n 1 || true); \
      if [ -n "$p" ]; then \
        mkdir -p "/rootfs$(dirname "$p")"; \
        cp -L "$p" "/rootfs$p"; \
      fi; \
    done; \
    cp -a /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/; \
    cp -a /usr/share/zoneinfo /rootfs/usr/share/; \
    for f in nsswitch.conf passwd group services protocols; do \
      if [ -f "/etc/$f" ]; then cp -a "/etc/$f" "/rootfs/etc/"; fi; \
    done; \
    cp -a /app/default-config.toml /rootfs/app/default-config.toml; \
    cp -a /tmp/busybox /rootfs/usr/local/bin/busybox; \
    ln -sf /usr/local/bin/busybox /rootfs/bin/wget; \
    ln -sf /usr/local/bin/busybox /rootfs/bin/sh; \
    chmod 1777 /rootfs/tmp

# ============================================================
# final-slim：scratch + rootfs
# ============================================================
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

USER 1000:1000
WORKDIR /app
EXPOSE 8080 8081 19090

# busybox wget 探测健康探针服务（/bin/wget 由 slim-prep 创建为 busybox 符号链接）
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD /bin/wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/default-config.toml"]
