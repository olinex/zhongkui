# Docker Compose 部署

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`deploys/compose/docker-compose.yml`、`deploys/compose/config.example.toml`、`deploys/compose/.env.example`、`deploys/compose/otel-collector.yaml`、`internal/pkg/config/config.go`、`internal/pkg/otel/environments.go`、`internal/cmd/user.go`

本文用仓库自带的 Compose 文件，在一台机器上拉起 zhongkui 的全部组件与依赖。适合试用、演示和单机小规模使用；生产建议使用 Helm 或托管依赖。

## 1. 适用场景与前置条件

- Docker 24+，Docker Compose v2（`docker compose version`）
- 2 核 4 GB 起；磁盘按流量保留周期预留
- 可访问公网镜像仓库 `zhongkui-cn-beijing.cr.volces.com`（公有镜像，默认无需登录）
- 默认占用端口：80（Web）、8080（API）、8081（gRPC）、19090（API 健康检查）、5432、9000、8123、6379、2379、9001、9002、4317、4318；均可在 `.env` 中改宿主机端口

组件拓扑：

```text
浏览器
  │ :80
  ▼
zhongkui-web (nginx) ──/api/──► zhongkui-api (run web) ──► PostgreSQL / ClickHouse / Redis / etcd / RustFS
                                    ▲
zhongkui-worker (run worker) ───────┘
zhongkui-grpc (run grpc) ◄── 采集端上报（可选 capture）
```

## 2. 获取 Compose 文件并准备变量

无需克隆源码，直接从 Gitee 下载 Compose 部署所需的四个文件。示例使用 `master` 分支；固定版本时把 `REF` 换成发布 tag（如 `v0.0.0-alpha.1`）：

```bash
REF=master
BASE="https://gitee.com/olinex/zhongkui/raw/${REF}/deploys/compose"
mkdir -p zhongkui-compose && cd zhongkui-compose
for f in docker-compose.yml config.example.toml .env.example otel-collector.yaml; do
  curl -fsSL "$BASE/$f" -o "$f" ||
    { echo "下载失败：$BASE/$f（该 REF 可能还不包含 deploys/compose）" >&2; exit 1; }
done
ls -1A
```

> 若返回 404，说明所选 `REF` 还没有 `deploys/compose`（例如旧 tag 或尚未合入的分支）。先用浏览器确认 `https://gitee.com/olinex/zhongkui/tree/<REF>/deploys/compose` 可访问，再把 `REF` 固定为包含该目录的发布 tag。

也可以在 `https://gitee.com/olinex/zhongkui/tree/${REF}/deploys/compose` 页面逐个下载。

```bash
cp config.example.toml config.toml
cp .env.example .env
vi .env
```

`config.toml` 含环境相关配置，不要提交回仓库（上游仓库已忽略该文件）。

如果修改了 `ZKI_WEB_PORT`（默认 80），请同步把 `config.toml` 的 `[api].frontendBaseURL` 改为 `http://localhost:<端口>`。

至少替换：

| 变量 | 说明 |
| --- | --- |
| `POSTGRES_PASSWORD` | PostgreSQL 口令 |
| `CLICKHOUSE_PASSWORD` | ClickHouse 口令 |
| `REDIS_PASSWORD` | Redis 口令 |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | 对象存储账号 |
| `ZKI_PARENT_SECRET_KEY` | 主密钥，建议 `openssl rand -hex 32` |
| `ZKI_VERSION` | 镜像版本；生产固定为发布 tag |

生成随机值示例：

```bash
openssl rand -hex 16   # 数据库/Redis/S3 口令
openssl rand -hex 32   # 主密钥
```

镜像为公有，默认无需登录：

```bash
docker pull zhongkui-cn-beijing.cr.volces.com/open-source/zhongkui-core:latest
```

如果所在网络要求通过私有镜像代理拉取，请先 `docker login <你的镜像仓库>`。

### 自定义镜像源（网络受限）

依赖镜像默认从 Docker Hub / quay.io 拉取。网络受限时任选一种方式：

1. 修改 `.env` 中的 `IMAGE_REGISTRY`（保留后续路径，官方镜像已带 `library/`）：

```dotenv
IMAGE_REGISTRY=registry.example.com
# 若镜像源不代理 quay.io，etcd 需单独指定
ETCD_IMAGE=registry.example.com/coreos/etcd:v3.6.5
```

   zhongkui 自身镜像不受 `IMAGE_REGISTRY` 影响，改用 `ZKI_CORE_IMAGE` / `ZKI_WEB_IMAGE` 指向私有代理地址。

2. 或给 Docker 配置 registry mirror（`/etc/docker/daemon.json` 的 `registry-mirrors`）后重启 Docker，Compose 文件无需修改。

调整后重新拉取并重建：

```bash
docker compose pull
docker compose up -d --force-recreate
```

## 3. 启动

```bash
docker compose up -d
docker compose ps
```

首次启动按以下顺序自动完成：

1. `postgres`、`clickhouse`、`redis`、`etcd`、`rustfs` 启动并通过健康检查。
2. `s3-init` 等待 RustFS 就绪并创建桶 `capture`。
3. `zhongkui-init` 生成密码本 `/data/password-book.bin` 与 Reporter TLS 证书，并修正数据卷属主。
4. `zhongkui-migrate` 执行数据库迁移。
5. `zhongkui-api`、`zhongkui-worker`、`zhongkui-grpc` 启动。
6. `zhongkui-web` 启动并反代到 API。

查看迁移结果：

```bash
docker compose logs -f zhongkui-migrate
```

看到迁移成功输出后按 `Ctrl+C` 退出日志跟踪。

## 4. 验证

```bash
curl -fsS http://127.0.0.1:19090/healthz && echo api-ok
curl -fsS http://127.0.0.1:19090/readyz  && echo api-ready
docker compose exec zhongkui-grpc wget -qO- http://127.0.0.1:19090/healthz && echo grpc-ok
docker compose port zhongkui-grpc 8081
```

浏览器打开：

```text
http://localhost
```

> 请使用 `localhost` 访问。配置中的 Cookie 域是 `localhost`，用 `127.0.0.1` 访问会导致登录态无法保持。

## 5. 创建管理员

```bash
docker compose exec -e ZKI_SUPERUSER=true zhongkui-api \
  zki --config /app/config.toml create user \
  -u zkadmin \
  -p 'ZkAdmin2026!' \
  -n 系统管理员账号 \
  -e admin@example.com
```

密码需包含大小写、数字与 `@$!%*?&#` 之一；昵称至少 6 个字符。示例口令仅用于演示，登录后请立即修改。

## 6. 常用运维

```bash
# 查看全部日志
docker compose logs -f

# 查看单个组件
docker compose logs -f zhongkui-api
docker compose logs -f zhongkui-worker
docker compose logs -f zhongkui-grpc

# 重启单个组件
docker compose restart zhongkui-api

# 修改配置后重建
vi config.toml
docker compose up -d

# 停止但保留数据
docker compose down

# 停止并删除全部数据（谨慎）
docker compose down -v
```

备份建议：

```bash
docker compose exec -T postgres pg_dump -U postgres -d zhongkui > backup-$(date +%F).sql
docker run --rm -v zhongkui_zhongkui-data:/data -v "$PWD":/backup alpine \
  tar -czf /backup/zhongkui-data-$(date +%F).tgz -C /data .
```

> 数据卷名称以 `docker volume ls` 实际输出为准；`down -v` 会删除数据库、对象存储与密码本，密码本丢失后历史密文无法解密。

## 7. Reporter gRPC 与可选采集端

gRPC 默认随栈启动，端口 8081，容器内健康检查 19090。采集端默认不启动：

```bash
docker compose --profile capture up -d
```

采集端使用主机网络与 `NET_RAW`/`NET_ADMIN`，仅支持 Linux。首次接入前需要：

```bash
# 创建访问密钥（记录 access-key-id / access-key-secret）
docker compose exec zhongkui-api zki --config /app/config.toml create access-key \
  -u zkadmin -d "capture client" --enabled --ttl 8760h

# 创建采集器记录；eth0 替换为真实网卡
docker compose exec zhongkui-api zki --config /app/config.toml create capturer \
  --namespace default --name test --version v1d \
  --device eth0 --snap-len 1600 --promisc --enabled
```

创建后把 access-key 填入当前目录的 `config.toml` 的 `[grpc.client.reporter.auth]`：

```toml
[grpc.client.reporter.auth]
enabled = true
authorize = 'accesskeySecret'
accessKeyID = '<输出的 access-key-id>'
accessKeySecret = '<输出的 access-key-secret>'
```

并确认 `[capture.resource]` 与上面的 namespace/name/version 一致，再重启 capture：

```bash
docker compose --profile capture up -d --force-recreate zhongkui-capture
```

## 8. pgvector（仅在改用外部 PostgreSQL 时需要）

Compose 默认使用 `pgvector/pgvector:pg17`，已内置扩展，无需额外操作。若把 PostgreSQL 换成外部实例，请任选一种方式安装/启用：

1. **Docker**：继续使用 `pgvector/pgvector:pg17`。
2. **Debian / Ubuntu（需 PGDG 源）**：`postgresql-*-pgvector` 不在发行版默认源中，先按 [Linux 二进制部署](deploy-binary.md) 2.1 节启用 PGDG 源，再安装：
   ```bash
   sudo apt install -y postgresql-17-pgvector
   ```
   PostgreSQL 主版本不是 17 时替换包名。
3. **源码编译**：
   ```bash
   # postgresql-server-dev-17 同样来自 PGDG；也可改用发行版自带的 postgresql-server-dev-all
   sudo apt install -y build-essential postgresql-server-dev-17 git
   git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git
   cd pgvector && make && sudo make install
   ```
4. **云托管**：在控制台启用 `vector` 扩展，或执行 `CREATE EXTENSION IF NOT EXISTS vector;`。

验证：

```sql
SELECT extversion FROM pg_extension WHERE extname = 'vector';
```

## 9. OpenTelemetry（可选）

默认 `OTEL_ENABLED=false`。启用内置 Collector：

```bash
# 1. 编辑 .env
#    OTEL_ENABLED=true
#    OTEL_COLLECTOR_ENDPOINT=http://otel-collector:4317

# 2. 启动 Collector 并重建应用容器
docker compose --profile otel up -d

# 3. 查看遥测数据（debug 导出，日志即数据）
docker compose logs -f --tail=100 otel-collector
```

接入外部 OTLP Collector / APM：

```dotenv
OTEL_ENABLED=true
OTEL_COLLECTOR_ENDPOINT=http://<collector-host>:4317
OTEL_EXPORTER_OTLP_HEADERS=uptrace-dsn=http://<token>@<host>:14318?grpc=4317
```

说明：

- 三个服务分别以 `OTEL_SERVICE_NAME=zhongkui-api` / `zhongkui-worker` / `zhongkui-grpc` 上报。
- 采样默认 `always_on`；高流量可改为 `OTEL_SAMPLER_TYPE=head` 并设置 `OTEL_SAMPLER_RATIO=0.1`。
- 端点不可达不会阻塞启动，只会丢失遥测；务必用 Collector 日志或 APM 服务列表确认。
- 本地自检也可设 `OTEL_PROVIDER=stdout`，无需 Collector。

## 10. 首次使用

1. 登录后确认命名空间（默认 `default`）。
2. 查看流量页面；没有数据说明采集端未接入。
3. 配置通知渠道与通知组（可选）。
4. 按教程编写第一条规则：[看到你的第一条风险告警](first-alert.md)。

## 11. 升级

```bash
# 1. 修改 .env 中的 ZKI_VERSION 为目标版本
vi .env

# 2. 拉取新镜像并重建
docker compose pull
docker compose up -d

# 3. 如需强制重跑迁移
docker compose run --rm zhongkui-migrate
```

升级前请备份 PostgreSQL、对象存储与密码本。迁移通常向前兼容，但数据库结构回滚需要单独的备份方案。

## 12. 常见问题

| 症状 | 检查与处理 |
| --- | --- |
| `docker compose up` 报变量未设置 | `.env` 缺少 `ZKI_PARENT_SECRET_KEY` 等必填项；对照 `.env.example` 补齐 |
| 镜像拉取失败 | 检查网络与代理；按第 2 节「自定义镜像源」修改 `IMAGE_REGISTRY` / `ETCD_IMAGE`，或为 Docker 配置 registry mirror |
| `postgres` 启动失败，报 `driver failed programming external connectivity` / `port is already allocated` | 宿主机端口被占用（常见于同机已有二进制部署或开发容器）。`ss -lntp \| grep -E '5432\|9000\|6379\|2379\|9002\|19090'` 找到占用者后停止，或在 `.env` 改 `POSTGRES_HOST_PORT` 等宿主端口变量；端口未被占用时执行 `sudo systemctl restart docker` 后重试 |
| RustFS 报权限错误 | `rustfs-perms` 未执行或数据卷属主不是 10001；`docker compose down` 后重新 `up`，或手动 `chown -R 10001:10001` |
| `redis` 一直 `waiting` / 始终 unhealthy | 旧版 Compose 的 healthcheck 拿不到 `REDIS_PASSWORD`（`docker inspect` 中可见 `NOAUTH`）；更新 compose 文件，或给 `redis` 服务补 `environment: { REDIS_PASSWORD: "${REDIS_PASSWORD}" }` |
| `zhongkui-init` 报 `/usr/local/bin/zki: Operation not permitted` | 该服务覆盖了镜像 entrypoint，跳过 file capability 兼容性处理；当前 Compose 文件已内置 `setcap -r` 兜底，使用旧文件时在 init 命令开头补一行 `setcap -r /usr/local/bin/zki` 或重新下载最新 compose 文件 |
| 迁移失败提示 vector | PostgreSQL 镜像不是 pgvector，或外部库未安装扩展；见第 8 节 |
| 页面能开但登录失败 | 使用 `http://localhost` 而非 `127.0.0.1`；检查 `[api.auth.cookie].domain` |
| 桶不存在/写入失败 | `s3-init` 未完成；`docker compose logs s3-init`，必要时手动 `up -d --force-recreate s3-init` |
| 8081 无监听 | `docker compose ps zhongkui-grpc`；查看日志与迁移是否完成 |
| 宿主机 19090 被占用 | 只发布 api 的 19090；如需改端口，修改 compose 的 ports 映射 |
| OTel 无数据 | 确认 `.env` 中 `OTEL_ENABLED=true` 且已启动 `--profile otel`；查看 Collector 日志 |

更多排查路径见[故障排查](../how-to/troubleshooting.md)。
