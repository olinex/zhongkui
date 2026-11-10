#
# zhongkui 前端镜像（多架构：产物为纯静态文件、与架构无关，仅 nginx 基底按目标架构拉取）。
# 构建上下文为仓库根目录；静态产物不在镜像内编译，直接装配预编译产物：
#   web/dist（控制台）、docs/.vitepress/dist（文档站）在前端构建环境镜像
#   （deploys/build/web.builder.Dockerfile）内由 deploys/build/build-web.sh 产出；
#   CI 流水线编排见 .workflow/ci-build.yml。
#
# 本地手工构建（产物缺失时先经构建环境镜像编译）：
#   docker run --rm -v "$PWD:/root/workspace" \
#     -v zhongkui-pnpm-store:/root/.local/share/pnpm/store \
#     -e HOST_UID=$(id -u) -e HOST_GID=$(id -g) \
#     zhongkui-web-builder:node22 deploys/build/build-web.sh
#   docker build -f deploys/build/web.Dockerfile -t zhongkui-web:local .
#
# Vite 构建时把 API 基址固定为同源 /api/v1，由 Nginx 反向代理到 ZKI_API_UPSTREAM；
# 文档站部署在 /docs/ 子路径，回跳控制台地址 /console 由 nginx 302 到 /（见 nginx.conf.template）。
#
# 运行期环境变量：
#   ZKI_API_UPSTREAM   后端 API 地址，默认 http://zki:8080
#

# nginx 基底镜像可通过 --build-arg 覆盖（CI 中指向国内镜像加速地址）。
ARG NGINX_IMAGE=nginx:1.27-alpine

FROM ${NGINX_IMAGE}

ENV NGINX_ENVSUBST_FILTER=ZKI_API_UPSTREAM \
    ZKI_API_UPSTREAM=http://zki:8080

COPY web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY web/dist /usr/share/nginx/html
COPY docs/.vitepress/dist /usr/share/nginx/html/docs

EXPOSE 80
