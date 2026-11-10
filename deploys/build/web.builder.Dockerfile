#
# zhongkui 前端构建环境镜像：CI 与本地统一的 Node / pnpm 静态产物编译环境。
# 只封装工具链、不含源码；日常构建直接复用 registry 中的镜像，
# 仅在 Node / pnpm 版本等底层依赖变化时重建推送（见 .workflow/ci-builder.yml，
# tag 跟随 Node 版本命名，如 node22）。
#
# 前端产物为纯静态文件、与架构无关；镜像按构建节点架构分别构建（processorType），
# 保证流水线无论调度到哪种架构节点都能原生执行 Node 构建（不经 QEMU）。
#
# 用法（本地手工编译静态产物）：
#   docker run --rm -v "$PWD:/root/workspace" \
#     -v zhongkui-pnpm-store:/root/.local/share/pnpm/store \
#     -e HOST_UID=$(id -u) -e HOST_GID=$(id -g) \
#     zhongkui-web-builder:node22 deploys/build/build-web.sh
#
ARG NODE_IMAGE=node:22-bookworm-slim
FROM ${NODE_IMAGE}

# 公共镜像源与超时配置：默认华为云镜像；registry.npmmirror.com 在部分 CI 节点上频繁超时。
# 备选：https://mirrors.cloud.tencent.com/npm（腾讯云）、https://registry.npmjs.org（官方）
ARG NPM_REGISTRY=https://repo.huaweicloud.com/repository/npm
ENV npm_config_registry=${NPM_REGISTRY} \
    npm_config_fetch_retries=5 \
    npm_config_fetch_timeout=600000 \
    npm_config_fetch_retry_mintimeout=10000 \
    npm_config_fetch_retry_maxtimeout=60000

RUN corepack enable && corepack prepare pnpm@10 --activate

# 工作目录与 Gitee Go 的代码挂载目录一致，源码（仓库根）挂载于此。
WORKDIR /root/workspace

# 不设 ENTRYPOINT：execute@docker 等外部步骤直接向容器传入 shell 命令执行。
