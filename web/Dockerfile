#
# zhongkui 前端镜像（原生架构）。
# 构建上下文为仓库根目录；Vite 构建时把 API 基址固定为同源 /api/v1，
# 由 Nginx 反向代理到 ZKI_API_UPSTREAM（默认 http://zki:8080）。
#
#   docker build -f web/Dockerfile -t zhongkui-web:local .

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
RUN pnpm build

FROM ${NGINX_IMAGE}

ENV NGINX_ENVSUBST_FILTER=ZKI_API_UPSTREAM \
    ZKI_API_UPSTREAM=http://zki:8080

COPY web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
