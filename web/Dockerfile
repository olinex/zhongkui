#
# zhongkui 前端镜像（支持交叉构建）。
# 构建上下文为仓库根目录；Vite 构建时把 API 基址固定为同源 /api/v1，
# 由 Nginx 反向代理到 ZKI_API_UPSTREAM（默认 http://zki:8080）。
#
# 前端产物为纯静态文件、与架构无关，Node 构建阶段固定在宿主机架构
# （--platform=$BUILDPLATFORM）上原生执行，跨架构构建（如 amd64 CI 构建
# arm64 镜像）时避免 QEMU 模拟 Node 导致的数倍减速；仅最终 nginx 阶段
# 使用目标架构。交叉构建需 BuildKit（docker buildx / DOCKER_BUILDKIT=1）。
#
#   docker build -f web/Dockerfile -t zhongkui-web:local .
#
# 可用构建参数：
#   VITE_DOCS_URL      控制台跳转文档站的地址，默认 /docs/
#   DOCS_BASE          文档站部署子路径，默认 /docs/
#   DOCS_CONSOLE_URL   文档站回跳控制台的地址，默认 /console（由 nginx 302 到 /）
#   NPM_REGISTRY       依赖下载源，默认 https://repo.huaweicloud.com/repository/npm
#

# 基础镜像可通过 --build-arg 覆盖（CI 中指向国内镜像加速地址），默认为 docker.io 官方镜像。
ARG NODE_IMAGE=node:22-bookworm-slim
ARG NGINX_IMAGE=nginx:1.27-alpine

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS build

# 公共镜像源与超时配置抽成 ARG，两个 Node 阶段各声明一次（ARG 不跨阶段继承）。
# 默认用华为云镜像；registry.npmmirror.com 在部分 CI 节点上频繁超时。
# 备选：https://mirrors.cloud.tencent.com/npm（腾讯云）、https://registry.npmjs.org（官方）
ARG NPM_REGISTRY=https://repo.huaweicloud.com/repository/npm
ENV npm_config_registry=${NPM_REGISTRY} \
    npm_config_fetch_retries=5 \
    npm_config_fetch_timeout=600000 \
    npm_config_fetch_retry_mintimeout=10000 \
    npm_config_fetch_retry_maxtimeout=60000

RUN corepack enable && corepack prepare pnpm@10 --activate

WORKDIR /app
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
# store 缓存挂载让重复构建复用已下载的包，缓存丢失时也能命中 registry 层缓存
RUN --mount=type=cache,target=/root/.local/share/pnpm/store,sharing=locked \
    pnpm install --frozen-lockfile
COPY web/ .

ARG VITE_ZHONGKUI_BASE_URL=/api/v1
ENV VITE_ZHONGKUI_BASE_URL=${VITE_ZHONGKUI_BASE_URL}

# 控制台顶栏的文档入口地址
ARG VITE_DOCS_URL=/docs/
ENV VITE_DOCS_URL=${VITE_DOCS_URL}

RUN pnpm build

# 文档站构建阶段：独立安装依赖并构建，产物只取静态文件，不带 node_modules
FROM ${NODE_IMAGE} AS docs-build

# Dockerfile 无法显式声明阶段依赖，这里复制 web 构建产物建立依赖边，
# 强制与 build 阶段串行：内存受限的构建环境（如 4GB Docker VM）下
# 两个 Node 构建并发会触发 OOM。
COPY --from=build /app/package.json /tmp/.web-build-done

ARG NPM_REGISTRY=https://repo.huaweicloud.com/repository/npm
ENV npm_config_registry=${NPM_REGISTRY} \
    npm_config_fetch_retries=5 \
    npm_config_fetch_timeout=600000 \
    npm_config_fetch_retry_mintimeout=10000 \
    npm_config_fetch_retry_maxtimeout=60000

RUN corepack enable && corepack prepare pnpm@10 --activate

WORKDIR /app
COPY docs/package.json docs/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store,sharing=locked \
    pnpm install --frozen-lockfile
COPY docs/ .

# 文档站部署在 /docs/ 子路径；DOCS_CONSOLE_URL 是文档站回跳控制台的地址，
# 默认 /console 由 Nginx 重定向到控制台根路径（见 nginx.conf.template）。
# 构建上下文不含 .git，关闭 lastUpdated，避免 VitePress 调用 git 失败。
ARG DOCS_BASE=/docs/
ARG DOCS_CONSOLE_URL=/console
ENV DOCS_BASE=${DOCS_BASE} \
    DOCS_CONSOLE_URL=${DOCS_CONSOLE_URL} \
    DOCS_LAST_UPDATED=false

RUN pnpm build

FROM ${NGINX_IMAGE}

ENV NGINX_ENVSUBST_FILTER=ZKI_API_UPSTREAM \
    ZKI_API_UPSTREAM=http://zki:8080

COPY web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
COPY --from=docs-build /app/.vitepress/dist /usr/share/nginx/html/docs

EXPOSE 80
