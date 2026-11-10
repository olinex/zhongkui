#
# zhongkui 构建环境镜像：CI 与本地统一的 Go 编译 / 交叉编译环境。
# 只封装工具链与系统依赖、不含源码；日常构建直接复用 registry 中的镜像，
# 仅在 Go 版本、libpcap、交叉工具链等底层依赖变化时重建推送（见 .workflow/ci-builder.yml，
# tag 跟随 Go 版本命名，如 go1.27.3）。
#
# 镜像按架构分别构建（processorType: amd64 / arm64），并预装反向架构的交叉工具链，
# 保证流水线无论调度到哪种架构的构建节点都能产出两种架构的二进制：
#   amd64 镜像：libpcap-dev + gcc-aarch64-linux-gnu + libpcap-dev:arm64（交叉出 arm64）
#   arm64 镜像：libpcap-dev + gcc-x86-64-linux-gnu + libpcap-dev:amd64（交叉出 amd64）
#
# 用法（本地手工编译多架构二进制）：
#   docker run --rm -v "$PWD:/root/workspace" \
#     -e VERSION=v1.2.3 -e COMMIT=$(git rev-parse --short HEAD) -e BUILDER=$(whoami) \
#     zhongkui-builder:go1.27.3 deploys/build/build-binary.sh
#
ARG GOLANG_IMAGE=golang:1.27.3-bookworm
FROM ${GOLANG_IMAGE}

ARG APT_MIRROR=mirrors.aliyun.com
ARG GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
# BUILDARCH 由 BuildKit 自动注入（构建机架构），用于选择反向架构的交叉工具链；
# 非 BuildKit 构建时默认按 amd64 处理。
ARG BUILDARCH

RUN set -eu; \
    for f in /etc/apt/sources.list /etc/apt/sources.list.d/debian.sources; do \
      if [ -f "$f" ]; then \
        sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" "$f"; \
      fi; \
    done; \
    apt-get update \
    && apt-get install -y --no-install-recommends libpcap-dev ca-certificates \
    && if [ "${BUILDARCH:-amd64}" = "amd64" ]; then \
         dpkg --add-architecture arm64; \
         apt-get update; \
         apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu libpcap-dev:arm64; \
       else \
         dpkg --add-architecture amd64; \
         apt-get update; \
         apt-get install -y --no-install-recommends gcc-x86-64-linux-gnu libpcap-dev:amd64; \
       fi; \
    rm -rf /var/lib/apt/lists/*

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=1

# 工作目录与 Gitee Go 的代码挂载目录一致，源码（仓库根）挂载于此。
WORKDIR /root/workspace

# 不设 ENTRYPOINT：execute@docker 等外部步骤直接向容器传入 shell 命令执行。
