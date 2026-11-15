#
# zhongkui slim 变体镜像（多架构：amd64 / arm64），免编译。
#
# 从已构建的 normal 变体提取 zki 二进制与默认配置，
# 在 slim-prep 重新授予抓包 capability，
# 解析动态库闭包，组装成 scratch 镜像所需的最小 rootfs。
#
# 构建耗时注意（本文件刻意避开两类慢源，曾致构建 8 分钟）：
#   - 不执行 apk：CI 集群访问 dl-cdn.alpinelinux.org 极慢（单架构曾耗时 4 分钟），
#     busybox 与 musl 加载器直接 COPY 自 alpine 基础镜像，零网络、零 RUN；
#   - apt 最小化：仅安装 libcap2-bin（供 setcap）。CA / 时区 / 账户等数据文件
#     全部从 normal 提取（normal 已含目标架构原生版本），避开 QEMU 下
#     update-ca-certificates 等慢触发器（arm64 曾因此耗时 58s）。
#
# 注意：不直接沿用 normal 镜像中 zki 的 capability 状态。
# normal 已在原生构建层确定性 setcap，COPY 会保留 security.capability 扩展属性，
# 但中间环节（registry / 文件系统）可能丢失 xattr，因此 slim-prep 仍确定性
# 重新 setcap 兜底，运行时是否生效由内核决定。
#
# 阶段：
#   normal      中转源阶段（FROM ${NORMAL_IMAGE}）：COPY --from 不支持 ARG 展开，
#               先由 FROM 生成命名阶段，后续用 COPY --from=normal 引用
#   slim-tools  alpine 基础镜像（零 RUN），仅提供 /bin/busybox 与 musl 加载器，
#               供 scratch 的 HEALTHCHECK 与紧急 shell 使用
#   slim-prep   安装 libcap2-bin，从 normal 提取制品与 CA / 时区 / 账户数据，
#               收集动态库闭包（glibc，不含 libpcap），重新授予 zki 抓包
#               capability，产出 /rootfs
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
ARG ALPINE_IMAGE=alpine:3.20
ARG APT_MIRROR=mirrors.aliyun.com

# ============================================================
# normal：中转源阶段
# ============================================================
FROM ${NORMAL_IMAGE} AS normal

# ============================================================
# slim-tools：busybox + musl 加载器（零 RUN，不访问 dl-cdn）
# ============================================================
# alpine 的 /bin/busybox 是动态链接 musl 的版本，scratch 无 musl 加载器，
# 因此需同时复制 /lib/ld-musl-*.so.1（路径随架构不同，用通配符覆盖）。
FROM ${ALPINE_IMAGE} AS slim-tools

# ============================================================
# slim-prep：组装最小 rootfs
# ============================================================
FROM ${RUNTIME_IMAGE} AS slim-prep

ARG APT_MIRROR

ENV DEBIAN_FRONTEND=noninteractive

# 仅安装 libcap2-bin（提供 setcap）：在 slim-prep 重新授予 zki 抓包 capability，
# 并把 setcap 一并放进 rootfs 供 entrypoint 运行时清除不兼容的 file capability。
# CA / 时区 / 账户 / NSS 等数据文件不通过 apt 安装，直接从 normal 提取。
# libpcap 已在 normal 构建时静态链接进 zki，此处无需安装 libpcap0.8。
RUN set -eux; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update; \
    apt-get install -y --no-install-recommends libcap2-bin; \
    rm -rf /var/lib/apt/lists/*

# 从 normal 提取制品（COPY 会保留 security.capability 扩展属性，
# slim-prep 仍会重新 setcap 兜底）。
# gosu 供 entrypoint 将 zki 降权到 app 用户运行（避免容器内 root 运行 zki）。
COPY --from=normal /usr/local/bin/zki /usr/local/bin/zki
COPY --from=normal /usr/sbin/gosu /usr/sbin/gosu
# zki.deps 是 normal 在 UPX 压缩前导出的 ldd 输出，用于收集动态依赖
# （UPX 压缩后 ldd 无法识别动态链接）
COPY --from=normal /out/zki.deps /tmp/zki.deps
# 默认配置直接放进 rootfs（slim-prep 活环境无需使用）
COPY --from=normal /app/default-config.toml /rootfs/app/default-config.toml
# normal 的 passwd/group 已含 app 用户，供 gosu 降权使用；直接放进 rootfs
COPY --from=normal /etc/passwd /rootfs/etc/passwd
COPY --from=normal /etc/group /rootfs/etc/group
# CA 与时区数据与架构无关，normal 已就绪，直接放进 rootfs
COPY --from=normal /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ca-certificates.crt
COPY --from=normal /usr/share/zoneinfo /rootfs/usr/share/zoneinfo

# busybox（scratch 无 shell，HEALTHCHECK 依赖它）与 musl 加载器
COPY --from=slim-tools /bin/busybox /rootfs/usr/local/bin/busybox
COPY --from=slim-tools /lib/ld-musl-*.so.1 /rootfs/lib/

# 组装 rootfs：动态库闭包、NSS 模块、最小 etc、busybox 符号链接、
# setcap（供 entrypoint 运行时清除不兼容的 file capability）
RUN set -eux; \
    mkdir -p /rootfs/tmp /rootfs/app /rootfs/usr/local/bin \
             /rootfs/usr/sbin /rootfs/bin; \
    cp -a /usr/local/bin/zki /rootfs/usr/local/bin/zki; \
    # 重新授予抓包 capability：作为中间环节丢 xattr 的兜底，运行时是否生效由内核决定。
    setcap cap_net_raw,cap_net_admin=eip /rootfs/usr/local/bin/zki; \
    # 收集 zki 的动态依赖闭包。UPX 压缩后 ldd 无法识别，
    # 改用 normal 导出的 zki.deps（压缩前的 ldd 输出）。
    awk '{ if ($2 == "=>") print $3; else if ($1 ~ /^\//) print $1 }' /tmp/zki.deps \
      | grep '^/' | sort -u \
      | while read -r lib; do \
          mkdir -p "/rootfs$(dirname "$lib")"; \
          cp -L "$lib" "/rootfs$lib"; \
        done; \
    # setcap 供 entrypoint 运行时清除不兼容的 file capability；
    # 其依赖 libcap.so.2（libc/ld 已由 zki 的闭包覆盖）。
    cp -a /usr/sbin/setcap /rootfs/usr/sbin/setcap; \
    for lib in $(ldd /usr/sbin/setcap | awk '{ if ($2 == "=>") print $3 }' | grep '^/'); do \
      mkdir -p "/rootfs$(dirname "$lib")"; \
      cp -L "$lib" "/rootfs$lib"; \
    done; \
    # gosu 供 entrypoint 将 zki 降权到 app 用户运行（Go 静态链接，无额外依赖）。
    cp -a /usr/sbin/gosu /rootfs/usr/sbin/gosu; \
    chmod 0755 /rootfs/usr/sbin/gosu; \
    for m in libnss_files.so.2 libnss_dns.so.2 libnss_compat.so.2; do \
      p=$(ls /usr/lib/*-linux-gnu/"$m" 2>/dev/null | head -n 1 || true); \
      if [ -n "$p" ]; then \
        mkdir -p "/rootfs$(dirname "$p")"; \
        cp -L "$p" "/rootfs$p"; \
      fi; \
    done; \
    for f in nsswitch.conf services protocols; do \
      if [ -f "/etc/$f" ]; then cp -a "/etc/$f" "/rootfs/etc/"; fi; \
    done; \
    ln -sf /usr/local/bin/busybox /rootfs/bin/wget; \
    ln -sf /usr/local/bin/busybox /rootfs/bin/sh; \
    chmod 1777 /rootfs/tmp; \
    chmod 0755 /rootfs/usr/sbin/setcap

# ============================================================
# final-slim：scratch + rootfs
# ============================================================
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

# entrypoint wrapper：
#   1. 以 root 运行，探测 file capability 兼容性（需要 CAP_SETFCAP 才能 setcap -r）。
#   2. 兼容则用 gosu 降权到 app 用户运行 zki（非 root + 抓包 capability）。
#   3. 不兼容（OrbStack/QEMU）则 setcap -r 清除后再 gosu app 运行 zki
#      （非 root 但无抓包能力，服务仍可用）。
COPY <<'ENTRYPOINT' /entrypoint.sh
#!/bin/sh
set -eu

run_zki() {
  exec /usr/sbin/gosu app /usr/local/bin/zki "$@"
}

if /usr/local/bin/zki --help >/dev/null 2>&1; then
  run_zki "$@"
fi

# file capability 不兼容，清除后 retry
/usr/sbin/setcap -r /usr/local/bin/zki 2>/dev/null || true
run_zki "$@"
ENTRYPOINT

# entrypoint 以 root 启动（仅用于 setcap -r），随后用 gosu 降权到 app 用户运行 zki，
# zki 进程本身非 root，降低容器逃逸风险。
WORKDIR /app
EXPOSE 8080 8081 19090

# busybox wget 探测健康探针服务（/bin/wget 由 slim-prep 创建为 busybox 符号链接）
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD /bin/wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
CMD ["run", "web", "--config", "/app/default-config.toml"]
