#
# zhongkui slim 变体镜像（多架构：amd64 / arm64），免编译，从 normal 变体提取 zki 二进制。
#
# 构建流程：
#   步骤 1  normal      中转源阶段：FROM ${NORMAL_IMAGE} AS normal（COPY --from 不支持 ARG 展开，
#                       必须先经 FROM 生成命名阶段再 COPY --from=normal）
#   步骤 2  slim-tools  busybox-static（scratch 镜像 HEALTHCHECK 用，约 800KB）
#   步骤 3  slim-prep   Debian 运行时依赖层：装 libpcap0.8 / CA / 时区，
#                       从 normal 阶段 COPY zki（含 setcap 扩展属性）与 default-config.toml，
#                       跑 core-slim-assemble.sh 组装最小 rootfs
#   步骤 4  final-slim  scratch 基底，注入 rootfs，产出 slim 变体
#
# 前提：normal 变体已先构建并可通过 NORMAL_IMAGE 参数访问（本地 tag 或远程 registry）。
# 构建时 BuildKit 会根据 --platform 自动从 NORMAL_IMAGE 的 manifest list 中选取对应架构的 zki。
#
# 用法示例：
#   # 1. 先构建 normal 变体
#   docker buildx build --platform linux/amd64 -f deploys/containers/normal.Dockerfile \
#     --build-arg VERSION=v1.2.3 -t zhongkui:v1.2.3 --load .
#
#   # 2. 再基于 normal 变体构建 slim（无需 --load / --push normal 到远程，本地 tag 即可）
#   docker buildx build --platform linux/amd64 -f deploys/containers/slim.Dockerfile \
#     --build-arg NORMAL_IMAGE=zhongkui:v1.2.3 -t zhongkui:v1.2.3-slim --load .
#
#   # 多平台构建（NORMAL_IMAGE 需为多架构 manifest list，已 --push）
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploys/containers/slim.Dockerfile \
#     --build-arg NORMAL_IMAGE=registry.example.com/zhongkui:v1.2.3 \
#     -t registry.example.com/zhongkui:v1.2.3-slim --push .
#
#   # 使用仓库脚本（NORMAL_IMAGE 可省略，默认取同版本号的 local normal tag）
#   VARIANT=slim deploys/scripts/core-docker-image.sh v1.2.3
#
# 镜像特点（约 13MB）：
#   - 基于 scratch，不含 shell / 包管理器，攻击面最小
#   - 非 root 用户（uid=1000:gid=1000）运行
#   - zki 通过 setcap 已授予抓包 capability，无需 root
#   - HEALTHCHECK 由 busybox-static 提供 wget 实现（normal 镜像的 zki 已自带默认配置）
#
# 运行（与 normal 变体完全一致）：
#   docker run --rm -p 8080:8080 -p 8081:8081 zhongkui:v1.2.3-slim
#   docker run --rm -p 8080:8080 -p 8081:8081 \
#     -v "$PWD/config.toml:/app/config.toml:ro" \
#     zhongkui:v1.2.3-slim run web --config /app/config.toml

# ============================================================
# 参数声明（顶部，BUILDARCH / TARGETARCH 在 FROM 前不可用，
# 但 slim-prep FROM debian 后会由 BuildKit 自动注入，
# COPY --from=${NORMAL_IMAGE} 会根据 buildx --platform 自动选取对应架构的 zki）
# ============================================================
# normal 变体的镜像地址（必需）：本地 tag 或远程 registry。
# 多平台构建时须为包含多架构 manifest 的镜像（已 --push 或 --output 到 registry）。
ARG NORMAL_IMAGE
# slim-prep 阶段的 Debian 基底（用于执行 ldd 收集动态库闭包）
ARG RUNTIME_IMAGE=debian:bookworm-slim
# apt mirror（CI 环境覆盖）
ARG APT_MIRROR=mirrors.aliyun.com

# ============================================================
# 步骤 1：normal 源阶段（FROM 支持 ARG 展开，COPY --from 不支持，故中转）
# ============================================================
# COPY --from 不接受 ${ARG} 变量展开，会把字面量当作镜像名报 invalid reference format。
# 解决：先用 FROM ${NORMAL_IMAGE} AS normal 生成一个命名阶段，后续 COPY --from=normal
# 引用阶段名即可，FROM 指令是支持 ARG 变量展开的。
FROM ${NORMAL_IMAGE} AS normal

# ============================================================
# 步骤 2：slim-tools — busybox-static
# ============================================================
# scratch 基底没有 wget / sh，HEALTHCHECK 需要一个可执行工具。
# busybox-static 单文件二进制（约 800KB），同时提供 wget（健康探针）与 sh（紧急 shell）。
FROM alpine:3.20 AS slim-tools
RUN apk add --no-cache busybox-static

# ============================================================
# 步骤 3：slim-prep — 组装最小 rootfs
# ============================================================
# 本阶段职责：
#   - 安装 libpcap0.8 / ca-certificates / tzdata：供 core-slim-assemble.sh 内
#     ldd 收集动态库闭包、复制 CA 证书、复制时区数据使用
#   - 从 normal 阶段 COPY zki 二进制：保留 setcap 写入的 security.capability 扩展属性
#   - 从 normal 阶段 COPY default-config.toml：默认 CMD 依赖
#   - 跑 core-slim-assemble.sh 产出 /rootfs/ 目录
#
# 本阶段不含编译环境，无 Go 工具链、无交叉工具链、无源码。
FROM ${RUNTIME_IMAGE} AS slim-prep

ARG APT_MIRROR

# apt 缓存目录（仅一次 apt install，缓存收益不显著，不挂 cache mount 保持简洁）
RUN apt-get update \
    && sed -i \
        "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" \
        /etc/apt/sources.list 2>/dev/null || true \
    && apt-get install -y --no-install-recommends \
        libpcap0.8 ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

# 从 normal 阶段提取制品（COPY --from 根据 buildx --platform 自动选对应架构）
# cp -a 会保留 setcap 扩展属性，无需重新 setcap
COPY --from=normal /usr/local/bin/zki /usr/local/bin/zki
COPY --from=normal /app/default-config.toml /app/default-config.toml

# busybox（由 slim-tools 阶段提供，先 COPY 进来再由脚本统一写入 rootfs）
COPY --from=slim-tools /bin/busybox /tmp/busybox
COPY deploys/scripts/core-slim-assemble.sh /tmp/

# 组装 rootfs：二进制、动态库闭包、NSS 模块、CA/时区、默认配置、busybox
RUN bash /tmp/core-slim-assemble.sh /rootfs /tmp/busybox

# ============================================================
# 步骤 4：final-slim — scratch + rootfs
# ============================================================
# 直接注入 slim-prep 产出的最小 rootfs，无 apt、无 shell、无构建层残留。
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

USER 1000:1000
WORKDIR /app
EXPOSE 8080 8081 19090

# 用 busybox wget 探测 normal 镜像内置的健康探针服务端口 19090/healthz
# /bin/wget 在 slim-prep 阶段已创建为 busybox 的 symlink
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD /bin/wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/default-config.toml"]
