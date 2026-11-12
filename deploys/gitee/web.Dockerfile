#
# zhongkui 前端镜像（Gitee CI 远程构建场景：假设 Web dist 静态产物已就绪）。
# 与 deploys/local/web.Dockerfile 的关系：
#   deploys/local/web.Dockerfile  本地构建使用：源码 → Node 工具链 → pnpm install → Vite/VitePress 编译 → 装配（一次成型）
#   deploys/gitee/web.Dockerfile  Gitee CI 远程构建使用：假设构建上下文（context）内已有 web/dist 和 docs/.vitepress/dist，
#                                  Dockerfile 仅做"nginx 基底 + 预制静态文件 → 最终镜像"的装配。
#
# 为什么拆分：
#   Gitee 的 image-build-plus@buildkitd 插件对 Dockerfile 层缓存的支持有限，
#   全栈 Dockerfile 每次都要重跑 pnpm install + Vite/VitePress 编译，构建资费高；
#   把 Web 构建下沉到 CI 脚本步骤（pnpm install + pnpm build，一次完成），
#   镜像构建只做文件 COPY + nginx 基底装配，层缓存命中率 100%。
#
# 构建上下文约定（CI 脚本步骤先执行，把产物放到仓库根目录下以下路径）：
#   web/dist/                   控制台（Vite）静态产物
#   docs/.vitepress/dist/       文档站（VitePress）静态产物
#   web/nginx.conf.template    nginx 配置模板（仓库源码自带，始终存在）
#
# 运行时基底与 deploys/local/web.Dockerfile 完全相同，最终产物行为一致。

# 基底镜像与镜像源可通过 --build-arg 覆盖（CI 中指向国内镜像加速地址）。
ARG NGINX_IMAGE=nginx:1.27-alpine

# ============================================================
# 运行环境：nginx 基底 + 运行时环境变量
# （与 web.Dockerfile 步骤 2 完全一致）
# ============================================================
FROM ${NGINX_IMAGE} AS runtime

ENV NGINX_ENVSUBST_FILTER=ZKI_API_UPSTREAM \
    ZKI_API_UPSTREAM=http://zki:8080

EXPOSE 80

# ============================================================
# 装配：nginx 模板 + 预制静态产物
# （对应 web.Dockerfile 步骤 5，唯一区别是静态文件从 --from=build 改为构建上下文直接 COPY）
# ============================================================
FROM runtime AS final

# nginx 基底自带 busybox wget，探测根路径即可（默认配置返回 404 但不影响进程存活）。
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost/ >/dev/null || exit 1

COPY web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY web/dist /usr/share/nginx/html
COPY docs/.vitepress/dist /usr/share/nginx/html/docs
