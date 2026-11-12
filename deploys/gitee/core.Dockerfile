#
# zhongkui 后端镜像（Gitee CI 远程构建场景：假设预制二进制制品已就绪）。
# 与 deploys/local/core.Dockerfile 的关系：
#   deploys/local/core.Dockerfile  本地构建使用：源码 → Go 工具链 → 交叉编译 → 装配（一次成型）
#   deploys/gitee/core.Dockerfile  Gitee CI 远程构建使用：假设构建上下文（context）内已有预制二进制，
#                                  Dockerfile 仅做"运行时基底 + 预制文件 → 最终镜像"的装配。
#
# 为什么拆分：
#   Gitee 的 image-build-plus@buildkitd 插件对 Dockerfile 层缓存的支持有限，
#   全栈 Dockerfile 每次都要重跑 Go 编译链，构建资费高；
#   把 Go 编译下沉到 CI 脚本步骤（复用缓存、一次完成），
#   镜像构建只做文件 COPY + 装配，层缓存命中率 100%。
#
# 构建上下文约定（CI 脚本步骤先执行，把产物放到仓库根目录下以下路径）：
#   dist/bin/zki-core-linux-amd64   amd64 二进制（normal/slim 变体由 TARGETARCH 自动选取）
#   dist/bin/zki-core-linux-arm64   arm64 二进制（同上）
#
# 运行时基底与 deploys/local/core.Dockerfile 完全相同，最终产物行为一致；
# 仅缺少 builder / deps / build 三个源码编译阶段。
#
# 镜像变体与 deploys/local/core.Dockerfile 一致（通过 --build-arg IMAGE_VARIANT 控制）：
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f deploys/gitee/core.Dockerfile \
#     --build-arg IMAGE_VARIANT=normal \
#     -t registry/zhongkui-core:v1.2.3 --push .

# 基础镜像与软件源可通过 --build-arg 覆盖（CI 中指向国内/内网地址）。
ARG RUNTIME_IMAGE=debian:bookworm-slim
ARG APT_MIRROR=mirrors.aliyun.com
ARG IMAGE_VARIANT=normal

# ============================================================
# 运行环境：debian 基底 + 运行时依赖 + 非 root 用户
# （安装 + 用户创建委托给 deploys/scripts/core-setup-runtime-env.sh，与 deploys/local/core.Dockerfile 共用）
# ============================================================
FROM ${RUNTIME_IMAGE} AS runtime

ARG APT_MIRROR

COPY deploys/scripts/core-setup-runtime-env.sh /tmp/

RUN bash /tmp/core-setup-runtime-env.sh --mirror "${APT_MIRROR}"

# ============================================================
# 通用应用层：预制二进制注入 + setcap + 默认配置生成
# （对应 core.Dockerfile 步骤 5 的 app stage，唯一区别是 COPY 源从 --from=build 改为构建上下文）
# ============================================================
FROM runtime AS app

# BuildKit 自动注入 TARGETARCH（amd64 / arm64），按目标架构选取正确的预制二进制。
# CI 需预先产出 dist/bin/zki-core-linux-amd64 与 dist/bin/zki-core-linux-arm64 两份。
ARG TARGETARCH
COPY dist/bin/zki-core-linux-${TARGETARCH} /usr/local/bin/zki
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

# ============================================================
# normal 变体：debian 基底 + entrypoint + HEALTHCHECK
# （与 core.Dockerfile final-normal 完全一致）
# ============================================================
FROM app AS final-normal

COPY deploys/scripts/core-docker-entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

WORKDIR /app
EXPOSE 8080 8081 19090

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/entrypoint.sh"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

# ============================================================
# slim 变体的 HEALTHCHECK 辅助工具：busybox-static（单文件二进制，约 800KB）
# （与 core.Dockerfile slim-tools 完全一致）
# ============================================================
FROM alpine:3.20 AS slim-tools
RUN apk add --no-cache busybox-static

# ============================================================
# slim 变体的最小文件集装配（对应 core.Dockerfile slim-prep）
# 与 normal 变体的唯一区别：从构建上下文 COPY 二进制（而非 --from=build）
# ============================================================
FROM app AS slim-prep

# busybox 由 slim-tools 阶段提供，先 COPY 进来（RUN 内无法跨阶段取文件），
# 再由 core-slim-assemble.sh 统一写入 rootfs。
COPY --from=slim-tools /bin/busybox /tmp/busybox
COPY deploys/scripts/core-slim-assemble.sh /tmp/

RUN bash /tmp/core-slim-assemble.sh /rootfs /tmp/busybox

# ============================================================
# slim 变体：基于 scratch（与 core.Dockerfile final-slim 完全一致）
# ============================================================
FROM scratch AS final-slim

COPY --from=slim-prep /rootfs/ /

USER 1000:1000
WORKDIR /app
EXPOSE 8080 8081 19090

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD /bin/wget -qO- http://localhost:19090/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/default-config.toml"]

# 按 IMAGE_VARIANT 选择最终镜像；变体名非法时此处找不到目标阶段，构建直接失败。
FROM final-${IMAGE_VARIANT} AS final
