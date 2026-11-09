#
# zhongkui 前端镜像（原生架构）。
# 构建上下文为仓库根目录；Vite 构建时把 API 基址固定为同源 /api/v1，
# 由 Nginx 反向代理到 ZKI_API_UPSTREAM（默认 http://zki:8080）。
#
# 文档站（docs/，VitePress）在同一镜像内独立构建，产物挂载到 /docs/，
# 与 Go 后端镜像解耦，避免文档体积进入核心进程镜像。
#
#   docker build -f web/Dockerfile -t zhongkui-web:local .
#
# 可用构建参数：
#   VITE_DOCS_URL      控制台跳转文档站的地址，默认 /docs/
#   DOCS_BASE          文档站部署子路径，默认 /docs/
#   DOCS_CONSOLE_URL   文档站回跳控制台的地址，默认 /console（由 nginx 302 到 /）
#

# 基础镜像可通过 --build-arg 覆盖（CI 中指向国内镜像加速地址），默认为 docker.io 官方镜像。
ARG NODE_IMAGE=node:22-bookworm-slim
ARG NGINX_IMAGE=nginx:1.27-alpine

FROM ${NODE_IMAGE} AS build

ENV npm_config_registry=https://registry.npmmirror.com
RUN corepack enable && corepack prepare pnpm@10 --activate

WORKDIR /app
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
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

ENV npm_config_registry=https://registry.npmmirror.com
RUN corepack enable && corepack prepare pnpm@10 --activate

WORKDIR /app
COPY docs/package.json docs/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
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
