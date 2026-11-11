#
# zhongkui 前端镜像（多架构：产物为纯静态文件、与架构无关，仅 nginx 基底按目标架构拉取）。
# 构建上下文为仓库根目录，静态产物在镜像构建阶段内编译，不再依赖预编译的 web/dist。
#
# 编译链（builder → deps → build）固定在构建平台（$BUILDPLATFORM）上执行一次：Vite / VitePress
# 只产出 JS/CSS/HTML，与目标架构无关，因此不存在需要交叉编译的产物；多架构构建只让 runtime
# 阶段按目标架构拉取 nginx 基底镜像，再 COPY 同一份制品。若不加该约束，BuildKit 会为每个目标
# 平台各求值一次编译链，arm64 分支将以 QEMU 模拟重跑 Node 构建（实测多花约 22 分钟）。
#
# 五个构建步骤（与 deploys/build/core.Dockerfile 对齐）：
#   步骤 1  builder   前端编译环境：Node 22 + pnpm 10（corepack 启用，镜像源可覆盖）。
#   步骤 2  runtime   最终运行环境：nginx 基底 + 运行期环境变量。
#   步骤 3  deps      Node 包环境：只引入 package.json / pnpm-lock.yaml 并 pnpm install，
#                     依赖层与源码层分离，源码变更不会触发重新安装依赖。
#   步骤 4  build     制品构建：引入源码，串行编译控制台（web/）与文档站（docs/）静态产物。
#                     类型检查不在镜像内执行，由 CI 门禁承担（scripts/web.sh type-check）。
#   步骤 5  装配      把两份静态产物与 nginx 配置注入运行环境。
#
# 本地手工构建（需 BuildKit，Docker 23+ 默认启用；构建平台由 $BUILDPLATFORM 自动填充）：
#   docker build -f deploys/build/web.Dockerfile -t zhongkui-web:local .
#   docker build -f deploys/build/web.Dockerfile --build-arg VITE_DOCS_URL=/docs/ -t zhongkui-web:local .
#
# Vite 构建时把 API 基址固定为同源 /api/v1，由 Nginx 反向代理到 ZKI_API_UPSTREAM；
# 文档站部署在 /docs/ 子路径，回跳控制台地址 /console 由 nginx 302 到 /（见 nginx.conf.template）。
#
# 运行期环境变量：
#   ZKI_API_UPSTREAM   后端 API 地址，默认 http://zki:8080
#

# 基底镜像与镜像源可通过 --build-arg 覆盖（CI 中指向国内镜像加速地址）。
ARG NODE_IMAGE=node:22-bookworm-slim
ARG NGINX_IMAGE=nginx:1.27-alpine
ARG NPM_REGISTRY=https://repo.huaweicloud.com/repository/npm
# 控制台与文档站的构建期参数（Vite / VitePress 在编译期固化）。
ARG VITE_ZHONGKUI_BASE_URL=/api/v1
ARG VITE_DOCS_URL=/docs/
ARG DOCS_BASE=/docs/
ARG DOCS_CONSOLE_URL=/console

# ============================================================
# 步骤 1：前端编译环境（Node + pnpm）
# ============================================================
# 固定到构建平台：编译链只原生执行一次，deps / build 继承该平台，
# 两个目标架构的 final 阶段共用同一份静态产物。
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS builder

# 公共镜像源与超时配置：默认华为云镜像；registry.npmmirror.com 在部分 CI 节点上频繁超时。
# 备选：https://mirrors.cloud.tencent.com/npm（腾讯云）、https://registry.npmjs.org（官方）
ARG NPM_REGISTRY
ENV npm_config_registry=${NPM_REGISTRY} \
    npm_config_fetch_retries=5 \
    npm_config_fetch_timeout=600000 \
    npm_config_fetch_retry_mintimeout=10000 \
    npm_config_fetch_retry_maxtimeout=60000

# 钉死 pnpm 版本：避免每次构建都重新解析 "pnpm@10" 的可用版本，也让该层可稳定命中缓存。
RUN corepack enable && corepack prepare pnpm@10.34.9 --activate

WORKDIR /src

# ============================================================
# 步骤 2：最终运行环境（nginx）
# ============================================================
FROM ${NGINX_IMAGE} AS runtime

ENV NGINX_ENVSUBST_FILTER=ZKI_API_UPSTREAM \
    ZKI_API_UPSTREAM=http://zki:8080

EXPOSE 80

# ============================================================
# 步骤 3：Node 包环境（依赖层独立缓存）
# ============================================================
# 控制台与文档站是两个独立的 pnpm 项目，各自只引入清单与锁文件后安装依赖。
FROM builder AS deps

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./web/
COPY docs/package.json docs/pnpm-lock.yaml ./docs/

RUN cd web && pnpm install --frozen-lockfile
RUN cd docs && pnpm install --frozen-lockfile

# ============================================================
# 步骤 4：引入源码并编译静态产物
# ============================================================
# 控制台与文档站串行构建：内存受限环境下两个 Node 构建并发会触发 OOM。
# 构建上下文不含 .git，关闭 lastUpdated，避免 VitePress 调用 git 失败。
# 镜像内只跑生产构建：vue-tsc 与 vite build 并发会争抢 2 核 CPU，类型检查改由 CI 门禁承担。
FROM deps AS build

ARG VITE_ZHONGKUI_BASE_URL
ARG VITE_DOCS_URL
ARG DOCS_BASE
ARG DOCS_CONSOLE_URL

ENV VITE_ZHONGKUI_BASE_URL=${VITE_ZHONGKUI_BASE_URL} \
    VITE_DOCS_URL=${VITE_DOCS_URL} \
    DOCS_BASE=${DOCS_BASE} \
    DOCS_CONSOLE_URL=${DOCS_CONSOLE_URL} \
    DOCS_LAST_UPDATED=false

COPY . .

RUN echo ">>> 构建控制台（web/）" && cd web && pnpm build-only
RUN echo ">>> 构建文档站（docs/）" && cd docs && pnpm build

# ============================================================
# 步骤 5：把制品注入最终运行环境
# ============================================================
FROM runtime AS final

COPY web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/web/dist /usr/share/nginx/html
COPY --from=build /src/docs/.vitepress/dist /usr/share/nginx/html/docs
