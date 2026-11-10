#
# zhongkui 后端镜像（多架构：amd64 / arm64；其他架构需在对应架构的构建节点上构建）。
# 构建上下文为仓库根目录；zki 二进制不在镜像内编译，直接装配预编译产物：
#   dist/bin/zki-linux-<arch>  在构建环境镜像（deploys/build/core.builder.Dockerfile）内
#   由 deploys/build/build-binary.sh 交叉编译产出，多架构一次编译、版本元数据一致；
#   CI 流水线编排见 .workflow/ci-build.yml，本地由 deploys/build/docker-image.sh 自动补齐。
#
# 目标平台由 BuildKit 的 TARGETARCH 自动注入，COPY 按架构选取对应二进制，无需手工传参：
#   docker buildx build --platform linux/amd64 -f deploys/build/core.Dockerfile -t zhongkui:local --load .
#   docker buildx build --platform linux/arm64 -f deploys/build/core.Dockerfile -t zhongkui:local-arm64 --load .
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploys/build/core.Dockerfile -t registry/zhongkui:v1.2.3 --push .
#
#   deploys/build/docker-image.sh v1.2.3                      # 本机原生架构，normal 变体（默认）
#   VARIANT=slim deploys/build/docker-image.sh v1.2.3         # slim 变体
#   PLATFORMS=linux/arm64 deploys/build/docker-image.sh v1.2.3
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

# 运行基底镜像与 apt 源可通过 --build-arg 覆盖（CI 中指向国内/内网地址）。
# 基础镜像为多架构镜像，--platform 指定的目标架构会自动选取对应 manifest。
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com
# 最终镜像变体：normal 或 slim（见文件头注释）
ARG IMAGE_VARIANT=normal

# 运行期公共基底：安装运行时依赖，创建非 root 运行用户。
# normal 变体直接继承本基底，slim 变体从本基底 COPY --from 组装最小 rootfs。
FROM ${RUNTIME_IMAGE} AS runtime-base

ARG APT_MIRROR
# TARGETARCH 由 BuildKit 按 --platform 自动注入（未指定时为构建机原生架构），
# 用于从构建上下文选取对应架构的预编译二进制。
ARG TARGETARCH

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

# 装配对应架构的预编译二进制；产物缺失时 COPY 直接失败，避免产出缺二进制的镜像。
COPY dist/bin/zki-linux-${TARGETARCH} /usr/local/bin/zki
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
    # 4) 镜像内置默认配置（供默认 CMD 使用）
    cp -a /app/default-config.toml "$root/app/default-config.toml"; \
    chmod 1777 "$root/tmp"

# normal 变体：保留 debian 基底的 shell 与系统工具，便于线上排障。
# 注入 entrypoint wrapper（放在仓库 deploys/build/docker-entrypoint.sh 里维护）：
#   1. 默认模式（CMD 传入 zki 子命令）：wrapper 直接 exec /usr/local/bin/zki
#   2. 外部工具（如 execute@docker）：传入 shell 命令数组，wrapper 用 /bin/sh -c 执行
#      这解决了 ENTRYPOINT 是二进制时 execute@docker 无法正确执行 shell 命令的问题。
FROM runtime-base AS final-normal

COPY deploys/build/docker-entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

WORKDIR /app
EXPOSE 8080 8081

ENTRYPOINT ["/entrypoint.sh"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

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
