# syntax=docker/dockerfile:1.7
#
# zhongkui 后端镜像（原生架构）。
# 构建上下文为仓库根目录；如需注入版本号，先执行 `make dist` 生成 dist/build.env。
#
#   docker build -t zhongkui:local .
#
# 运行（配置文件与证书通过挂载提供）：
#   docker run --rm -p 8080:8080 -p 8081:8081 \
#     -v "$PWD/config.toml:/app/config.toml:ro" \
#     -v "$PWD/certs:/app/certs:ro" zhongkui:local

FROM golang:1.27.3-bookworm AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends libpcap-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENV GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=1

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN set -eu; \
    VERSION=unknown; COMMIT=unknown; BUILD_TIME=unknown; BUILDER=docker; \
    for f in dist/build.env build.env; do \
      if [ -f "$f" ]; then set -a; . "./$f"; set +a; fi; \
    done; \
    go build -trimpath \
      -ldflags "-s -w \
        -X zhongkuisec.com/zhongkui/internal/pkg/version.SemVer=${VERSION} \
        -X zhongkuisec.com/zhongkui/internal/pkg/version.Commit=${COMMIT} \
        -X zhongkuisec.com/zhongkui/internal/pkg/version.BuildTime=${BUILD_TIME} \
        -X zhongkuisec.com/zhongkui/internal/pkg/version.Builder=${BUILDER}" \
      -o /out/zki ./cmd/zki

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends libpcap0.8 ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/zki /usr/local/bin/zki

WORKDIR /app
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/zki"]
CMD ["run", "web", "--config", "/app/config.toml"]
