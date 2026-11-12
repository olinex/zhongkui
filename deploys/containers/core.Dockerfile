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
#   步骤 4  build    制品构建：引入源码，调 deploys/scripts/core-build-multiarch.sh 完成版本元数据生成
#                    + 按 TARGETARCH 编排交叉编译，输出 /out/zki。
#   步骤 5  装配     把 /out/zki 注入运行环境，产出 normal / slim 两个变体。
#
# 目标平台由 BuildKit 的 TARGETARCH 自动注入，按架构选取二进制与交叉工具链，无需手工传参：
#   docker buildx build --platform linux/amd64 -f deploys/containers/core.Dockerfile \
#     --build-arg VERSION=v1.2.3 -t zhongkui:local --load .
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploys/containers/core.Dockerfile \
#     --build-arg VERSION=v1.2.3 -t registry/zhongkui:v1.2.3 --push .
#
#   deploys/scripts/core-docker-image.sh v1.2.3                      # 本机原生架构，normal 变体（默认）
#   VARIANT=slim deploys/scripts/core-docker-image.sh v1.2.3         # slim 变体
#   PLATFORMS=linux/amd64,linux/arm64 deploys/scripts/core-docker-image.sh v1.2.3 --push
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
# 固定在构建机架构上运行，原生编译 BUILDARCH；同时预装反向架构的交叉工具链，
# 保证后续 build 阶段在多平台构建时（--platform linux/amd64,linux/arm64）
# 能交叉编译另一个架构：
#   amd64 构建机 → gcc-aarch64-linux-gnu + libpcap-dev:arm64（交叉 arm64）
#   arm64 构建机 → gcc-x86-64-linux-gnu + libpcap-dev:amd64（交叉 amd64）
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS builder

ARG APT_MIRROR
# BUILDARCH / TARGETARCH 由 BuildKit 自动注入（构建机架构 / 目标架构）。
ARG BUILDARCH
ARG TARGETARCH

# 把脚本提前 COPY 进来，才能在本层调用；脚本体积 < 4KB，不影响层缓存大小。
COPY deploys/scripts/core-setup-build-env.sh /tmp/

RUN bash /tmp/core-setup-build-env.sh \
      --host-arch "${BUILDARCH}" \
      --mirror "${APT_MIRROR}"
# 注意：本阶段只装交叉工具链（apt），不设置任何 Go 相关 ENV。
# Go 代理/校验（影响 go mod download 结果）放在 deps 阶段，
# 编译参数（CGO/GOMEMLIMIT/GOFLAGS）放在 build 阶段——
# 这样改动某类参数只会失效对应阶段的缓存，不会连带 builder 的交叉工具链层一起失效。

# ============================================================
# 步骤 2：最终运行环境（目标架构）
# ============================================================
# 安装运行时依赖并创建非 root 运行用户；normal 变体直接继承本基底，
# slim 变体从这里挑选文件组装最小 rootfs。
FROM ${RUNTIME_IMAGE} AS runtime

ARG APT_MIRROR

COPY deploys/scripts/core-setup-runtime-env.sh /tmp/

# gosu 用于 normal 变体降权（比 sudo 轻量）；libcap2-bin 提供 setcap，
# 给 zki 二进制授予抓包所需的 CAP_NET_RAW/CAP_NET_ADMIN，让非 root 用户也能抓包。
# 实际安装 + 用户创建委托给 core-setup-runtime-env.sh。
RUN bash /tmp/core-setup-runtime-env.sh --mirror "${APT_MIRROR}"

# ============================================================
# 步骤 3：Go 包环境（依赖层独立缓存）
# ============================================================
# 只引入 go.mod / go.sum：依赖不变时该层直接命中缓存，源码变更不会触发重新下载。
FROM builder AS deps

ARG GOPROXY
# go mod download 需要代理与校验服务；放在这里而非 builder 顶部，
# 是为了让 CI 切换 GOPROXY 时只失效本阶段及之后的缓存，不牵连 builder 的 apt 交叉工具链层。
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# ============================================================
# 步骤 4：引入源码并构建制品
# ============================================================
# 调 deploys/scripts/core-build-multiarch.sh 完成版本生成 + 单架构编译：
#   make version-env → dist/build.env（COMMIT / BUILDER 由 --build-arg 注入）
#   make build-release → 快照由仓库已提交版本提供（构建环境无 protoc，不烘焙），
#                       按 TARGETARCH 配 CC/CGO 交叉编译
# 只编译单一 TARGETARCH，输出到 /out/zki。
FROM deps AS build

WORKDIR /src

ARG VERSION
ARG COMMIT=unknown
ARG BUILDER=docker
ARG BUILDARCH
ARG TARGETARCH
# 仅 go build 需要的编译参数。ARG 不产生层，放这里声明；
# 对应的 ENV 下移到 go build 执行前（见下方），
# 使 GOFLAGS/GOMEMLIMIT 变更只失效 go build，不影响 COPY . . 与 go mod download 缓存。
ARG GOMEMLIMIT=off
ARG GOFLAGS

COPY . .

# COPY 会用仓库里的 go.sum 覆盖步骤 3 生成的校验和文件，这里基于已下载的模块缓存
# 重新补齐 go.sum，保证后续 go build（默认 -mod=readonly）不会因缺少校验和而失败。
RUN go mod download

# 仅 go build 阶段需要的编译参数 ENV 放在此处，紧邻 go build：
# 变更 CGO_ENABLED / GOMEMLIMIT / GOFLAGS 只失效本层及其后的 go build，
# 不影响上方 COPY . . 与 go mod download 的缓存。
ENV CGO_ENABLED=1 \
    GOMEMLIMIT=${GOMEMLIMIT} \
    GOFLAGS=${GOFLAGS}

# 复用仓库脚本 deploys/scripts/core-build-multiarch.sh：版本生成 + 按架构编排 CC/CGO 交叉编译参数。
# --skip-apt 跳过 apt 安装（builder 阶段已预装交叉工具链），
# --arch / --out 指定 TARGETARCH 到 /out/zki。
RUN COMMIT="${COMMIT}" BUILDER="${BUILDER}" \
    bash deploys/scripts/core-build-multiarch.sh \
      --skip-apt \
      --arch "${TARGETARCH}" \
      --out /out/zki \
      "${VERSION}"

# ============================================================
# 步骤 5：把制品注入最终运行环境
# ============================================================
# 先装配通用应用层（二进制 + 抓包 capability + 内置默认配置），
# 再由 normal / slim 两个变体分别产出最终镜像。
FROM runtime AS app

COPY --from=build /out/zki /usr/local/bin/zki
RUN chmod 0755 /usr/local/bin/zki

COPY deploys/scripts/core-cap-setup.sh /tmp/

# 给 zki 授予抓包所需的 CAP_NET_RAW/CAP_NET_ADMIN，让非 root 用户也能抓包。
# 部分运行时（QEMU/OrbStack）setcap 写入成功但 exec 时 EPERM，
# 脚本用 gosu app 做兼容性探测，失败则主动清除 capability 扩展属性。
RUN bash /tmp/core-cap-setup.sh /usr/local/bin/zki app

# 生成镜像内置的默认配置：该命令依据代码中的配置结构体序列化 TOML，无需外部 config.toml，
# 产物 /app/default-config.toml 供默认 CMD 使用；生产环境可挂载自有配置覆盖。
RUN /usr/local/bin/zki create default-config -o /app/default-config.toml \
    && test -s /app/default-config.toml

# normal 变体：保留 debian 基底的 shell 与系统工具，便于线上排障。
# 注入 entrypoint wrapper（仓库 deploys/scripts/core-docker-entrypoint.sh）：
#   1. 默认模式（CMD 传入 zki 子命令）：wrapper 直接 exec zki
#   2. 外部工具（如 execute@docker）：传入 shell 命令数组，wrapper 用 /bin/sh -c 执行
#      这解决了 ENTRYPOINT 是二进制时 execute@docker 无法正确执行 shell 命令的问题。
FROM app AS final-normal

COPY deploys/scripts/core-docker-entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

WORKDIR /app
EXPOSE 8080 8081 19090

# 项目内置健康探针服务（端口 19090）暴露 k8s 语义端点：
#   /healthz  live 探测：进程存活且未进入关机
#   /readyz   ready 探测：依赖（etcd 等）均就绪
#   /startupz startup 探测：启动完成
# 这里用 /healthz 做 liveness probe，进程崩/探针崩都会让 wget 失败。
# scratch 的 slim 变体没有 shell 和 wget，不加 HEALTHCHECK。
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/entrypoint.sh"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

# slim 变体的 HEALTHCHECK 辅助工具：busybox-static 单文件二进制（约 800KB），
# 提供 wget / sh 等 applet，scratch 基底无 shell 时作为唯一可执行工具。
FROM alpine:3.20 AS slim-tools
RUN apk add --no-cache busybox-static

# slim 变体的最小文件集装配：二进制、动态库闭包、CA 证书、时区数据与最小 etc 配置。
FROM app AS slim-prep

# busybox 由 slim-tools 阶段提供，先 COPY 进来（RUN 内无法跨阶段取文件），
# 再由 core-slim-assemble.sh 统一写入 rootfs。
COPY --from=slim-tools /bin/busybox /tmp/busybox
COPY deploys/scripts/core-slim-assemble.sh /tmp/

RUN bash /tmp/core-slim-assemble.sh /rootfs /tmp/busybox

# slim 变体：基于 scratch，不含 shell/包管理器，体积最小、攻击面最小。
# 以 app 用户（uid=1000）非 root 运行；zki 通过 setcap 已授予抓包 capability。
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

USER 1000:1000
WORKDIR /app
EXPOSE 8080 8081 19090

# slim 变体用 busybox wget 探测健康探针服务端口 19090/healthz，
# 与 normal 变体完全一致的探针逻辑；busybox 静态二进制在 /usr/local/bin/busybox，
# /bin/wget 为其 symlink（cp 时已创建）。
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD /bin/wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

# 按 IMAGE_VARIANT 选择最终镜像；变体名非法时此处找不到目标阶段，构建直接失败。
FROM final-${IMAGE_VARIANT} AS final
