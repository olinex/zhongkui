# Kubernetes Helm 部署

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`deploys/helm/zhongkui/Chart.yaml`、`deploys/helm/zhongkui/values.yaml`、`deploys/helm/zhongkui/templates/deployment-api.yaml`、`deploys/helm/zhongkui/templates/deployment-grpc.yaml`、`deploys/helm/zhongkui/templates/deployment-worker.yaml`、`deploys/helm/zhongkui/templates/job-migrate.yaml`、`deploys/helm/zhongkui/templates/deployment-otel-collector.yaml`、`internal/pkg/health/server.go`、`internal/pkg/otel/environments.go`

本文用仓库自带的 Helm Chart 把 zhongkui 部署到 Kubernetes，并连接外部 PostgreSQL、ClickHouse、Redis、etcd 与 S3。Chart 不内置这些依赖，避免生产环境被单机依赖绑定。

Chart 随每个版本发布到 OCI 仓库，部署时**无需 clone 源码**：

```
oci://zhongkui-cn-beijing.cr.volces.com/open-source/charts/zhongkui
```

## 1. 适用场景与前置条件

- Kubernetes 1.25+
- Helm 3.12+
- 可用的 Ingress Controller（可选，但通常需要）
- 外部依赖：PostgreSQL 17（含 pgvector）、ClickHouse 25.3、Redis 8、etcd 3.6、S3 兼容对象存储
- 集群节点可拉取 `zhongkui-cn-beijing.cr.volces.com` 镜像

Chart 渲染的资源：

| 资源 | 名称（release=zhongkui） | 说明 |
| --- | --- | --- |
| Deployment | `zhongkui-api` | 管理端 API，8080 |
| Deployment | `zhongkui-worker` | 后台任务与标签引擎 |
| Deployment | `zhongkui-grpc` | Reporter gRPC，8081（默认启用） |
| Deployment | `zhongkui-web` | nginx 前端，80 |
| DaemonSet | `zhongkui-capture` | 采集端，默认关闭 |
| Job | `zhongkui-migrate` | pre-install/pre-upgrade 迁移 |
| Deployment/Service | `zhongkui-otel-collector` | 可选内置 OTel Collector |
| Ingress | `zhongkui` | 可选 |

先创建命名空间（后续命令都使用 `zhongkui`）：

```bash
kubectl create namespace zhongkui
```

## 2. 准备外部依赖

### 2.1 建库、建桶

```sql
-- PostgreSQL（管理员账号）
CREATE DATABASE zhongkui;
\c zhongkui
CREATE EXTENSION IF NOT EXISTS vector;
```

```bash
# ClickHouse
clickhouse-client --host <ch-host> --password '<password>' \
  --query "CREATE DATABASE IF NOT EXISTS zhongkui"

# S3（以 S3 兼容服务为例；云对象存储可用控制台）
# 若使用本仓库 Docker 启动的 RustFS，账号与口令均为 rustfsadmin
export AWS_ACCESS_KEY_ID=rustfsadmin
export AWS_SECRET_ACCESS_KEY=rustfsadmin
# --region 必须与服务端一致：RustFS/MinIO 默认 us-east-1，云对象存储填对应区域
aws --endpoint-url https://<s3-endpoint> --region <s3-region> s3 mb s3://capture
```

Redis 使用两个逻辑库：cache 用 DB 0，pipeline 用 DB 1；若启用了密码，需要同时提供。

### 2.2 pgvector 扩展（可选安装路径）

迁移脚本会创建 `vector(1536)` 列并执行 `CREATE EXTENSION IF NOT EXISTS vector`。任选一种方式：

1. **Docker**：使用 `pgvector/pgvector:pg17` 作为数据库镜像。
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
4. **云托管 PostgreSQL**：在控制台启用 `vector` 扩展，或用 `rds_superuser` / 管理员账号执行：
   ```sql
   CREATE EXTENSION IF NOT EXISTS vector;
   ```

验证：

```sql
SELECT extversion FROM pg_extension WHERE extname = 'vector';
```

## 3. 镜像拉取凭证（可选）

`zhongkui-core` 与 `zhongkui-web` 为公有镜像，默认无需 `imagePullSecrets`，本步骤可以跳过。

仅当镜像被同步到私有仓库（或你的集群要求统一走私有镜像代理）时，才需要创建 `imagePullSecret`：

```bash
kubectl -n zhongkui create secret docker-registry volc-cr \
  --docker-server=<your-registry> \
  --docker-username='<username>' \
  --docker-password='<password>'
```

并在 values 中追加：

```yaml
imagePullSecrets:
  - name: volc-cr
```

## 4. 创建加密 Secret（安装前必须存在）

Chart 不创建密码本。先生成密码本、主密钥与（可选的）Reporter TLS 证书，再一次性创建 Secret：

```bash
# 1. 密码本：65536 字节随机数据
openssl rand 65536 > password-book.bin

# 2. 主密钥：保存到安全位置，后续不要更换
PARENT_SECRET_KEY="$(openssl rand -hex 32)"
echo "$PARENT_SECRET_KEY" > parent-secret-key.txt

# 3. Reporter TLS 证书（可选；grpc.tls.enabled=true 时需要）
#    DNS 名必须与 grpc.tls.domain 一致，默认 zhongkui-grpc
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout reporter-key.pem -out reporter-cert.pem \
  -subj "/CN=zhongkui-grpc" \
  -addext "subjectAltName=DNS:zhongkui-grpc"

# 4. 创建 Secret
kubectl -n zhongkui create secret generic zhongkui-crypto \
  --from-file=password-book.bin \
  --from-file=reporter-cert.pem \
  --from-file=reporter-key.pem \
  --from-literal=parent-secret-key="$PARENT_SECRET_KEY"
```

> 请同时备份 `password-book.bin`、`parent-secret-key.txt` 与数据库备份。密码本与主密钥一旦用于加密就不能更换，否则历史密文无法解密。

## 5. 准备 values 文件

创建 `values-prod.yaml`：

```yaml
image:
  tag: v1.2.3
webImage:
  tag: v1.2.3

environment: production

config:
  api:
    frontendBaseURL: "https://zhongkui.example.com"
    cookieDomain: "zhongkui.example.com"
    cookieSecure: true
    corsAllowOrigins:
      - "https://zhongkui.example.com"
  database:
    coreDSN: "host=pg.internal port=5432 user=zk password=<PASSWORD> dbname=zhongkui sslmode=require TimeZone=Asia/Shanghai"
    rawDSN: "clickhouse://zk:<PASSWORD>@ch.internal:9000/zhongkui"
  redis:
    cacheAddr: "redis.internal:6379"
    cachePassword: "<PASSWORD>"
    pipelineAddr: "redis.internal:6379"
    pipelinePassword: "<PASSWORD>"
  etcd:
    endpoints:
      - "etcd.internal:2379"
  s3:
    endpoint: "s3.internal:9000"
    bucket: capture
    region: ""
    useSSL: true
    streamingUpload: true
    accessKey: "<ACCESS_KEY>"
    secretKey: "<SECRET_KEY>"

web:
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: zhongkui.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: zhongkui-tls
        hosts:
          - zhongkui.example.com

grpc:
  enabled: true
  tls:
    enabled: false
```

说明：

- `config.database.*`、S3、Redis、SMTP 口令会进入 Chart 渲染的 Secret，不写入 ConfigMap。
- `crypto.existingSecret` 默认 `zhongkui-crypto`，对应第 4 步。
- `cookieDomain` 必须与 Ingress 域名一致；HTTPS 时 `cookieSecure: true`。
- `internalBaseURL` 由 Chart 自动设置为 `http://<fullname>-api:8080`，无需手工填写。

## 6. 安装

Chart 已发布到 OCI 仓库，无需 clone 源码。仓库公开可读时可直接拉取；私有仓库需先登录一次：

```bash
helm registry login zhongkui-cn-beijing.cr.volces.com
```

查看当前 Chart 的默认 values：

```bash
helm show values oci://zhongkui-cn-beijing.cr.volces.com/open-source/charts/zhongkui --version 1.2.3
```

安装（`--version` 必填，用于锁定版本；Chart 的 `appVersion` 即镜像 tag）：

```bash
helm upgrade --install zhongkui \
  oci://zhongkui-cn-beijing.cr.volces.com/open-source/charts/zhongkui \
  --version 1.2.3 \
  -n zhongkui --create-namespace \
  -f values-prod.yaml
```

> 需要固定镜像 tag 时显式传 `--set image.tag=v1.2.3 --set webImage.tag=v1.2.3`；为空时默认使用 Chart 的 `appVersion`。
>
> 开发调试可改用源码目录：`helm upgrade --install zhongkui deploys/helm/zhongkui -n zhongkui --create-namespace -f values-prod.yaml`。

安装顺序：

1. `pre-install` hook：创建 hook ConfigMap → 执行迁移 Job。
2. 迁移成功后再创建正式资源（ConfigMap、Secret、Deployment、Service、Ingress）。
3. 各 Deployment 启动后通过 `19090` 探针变为 Ready。

> 迁移 Job 在 pre-install 阶段运行，数据库必须已可达。若失败，安装会中止；修复依赖后重新执行 `helm upgrade --install`。

## 7. 验证

```bash
kubectl -n zhongkui get pods
kubectl -n zhongkui get svc
kubectl -n zhongkui logs job/zhongkui-migrate
```

预期看到 `zhongkui-api-*`、`zhongkui-worker-*`、`zhongkui-grpc-*`、`zhongkui-web-*` 四类 Pod 均为 Running/Ready。

本机访问（未启用 Ingress 或需要临时调试）：

```bash
kubectl -n zhongkui port-forward svc/zhongkui-web 8080:80
# 浏览器打开 http://127.0.0.1:8080
```

探针端点：

| 端点 | 说明 |
| --- | --- |
| `GET /healthz` | 存活 |
| `GET /readyz` | 依赖就绪 |
| `GET /startupz` | 启动完成 |

## 8. 创建管理员

```bash
kubectl -n zhongkui exec -it deploy/zhongkui-api -- \
  env ZKI_SUPERUSER=true \
  zki --config /etc/zhongkui/config.toml create user \
  -u zkadmin \
  -p 'ZkAdmin2026!' \
  -n 系统管理员账号 \
  -e admin@example.com
```

密码需包含大小写、数字与 `@$!%*?&#` 之一；昵称至少 6 个字符。示例口令仅用于演示，登录后请立即修改。

## 9. 可选：采集端 DaemonSet

采集端需要主机网络与抓包权限，仅在 Linux 节点运行。先在平台创建 access-key 与 capturer：

```bash
kubectl -n zhongkui exec -it deploy/zhongkui-api -- \
  zki --config /etc/zhongkui/config.toml create access-key \
  -u zkadmin -d "capture client" --enabled --ttl 8760h

kubectl -n zhongkui exec -it deploy/zhongkui-api -- \
  zki --config /etc/zhongkui/config.toml create capturer \
  --namespace default --name test --version v1d \
  --device eth0 --snap-len 1600 --promisc --enabled
```

把 access-key 写入 `values-prod.yaml`：

```yaml
config:
  grpcClient:
    accessKeyID: "<ACCESS_KEY_ID>"
    accessKeySecret: "<ACCESS_KEY_SECRET>"
  capture:
    namespace: default
    name: test
    version: v1d

capture:
  enabled: true
  privileged: false
  hostPath: /sys/fs/bpf
```

重新 `helm upgrade`。如果节点内核/XDP 仍报权限不足，可将 `capture.privileged` 设为 `true`。

若启用 `grpc.tls.enabled`，采集端使用 `grpc.tls.domain`（默认 `<fullname>-grpc`）校验证书，证书 DNS 名必须一致。

## 10. OpenTelemetry（可选）

默认 `otel.enabled=false`。接入外部 Collector / APM：

```yaml
otel:
  enabled: true
  provider: otlp
  protocol: grpc
  endpoint: "http://otel-collector.observability:4317"
  headers: "uptrace-dsn=http://<token>@<host>:14318?grpc=4317"
  compression: gzip
  samplerType: always_on
  samplerRatio: 1.0
  tailOnError: true
```

或使用 Chart 内置 Collector（debug 导出，日志即数据）：

```yaml
otel:
  enabled: true
  endpoint: "http://zhongkui-otel-collector:4317"

otelCollector:
  enabled: true
```

验证：

```bash
kubectl -n zhongkui logs deploy/zhongkui-otel-collector -f
```

说明：

- api/worker/grpc 分别以 `OTEL_SERVICE_NAME=zhongkui-api|worker|grpc` 上报。
- `otel.enabled=true` 且 `provider=otlp` 时，`otel.endpoint` 为空会让 `helm template` 直接报错，避免静默丢弃遥测。
- 采样：低流量用 `always_on`；高流量用 `head` + `samplerRatio`；只关心错误用 `tail`。

## 11. 升级、回滚与卸载

```bash
# 升级（迁移由 pre-upgrade hook 自动执行）
helm upgrade zhongkui \
  oci://zhongkui-cn-beijing.cr.volces.com/open-source/charts/zhongkui \
  --version 1.2.4 \
  -n zhongkui -f values-prod.yaml

# 查看历史与回滚应用版本
helm history zhongkui -n zhongkui
helm rollback zhongkui <REVISION> -n zhongkui
```

注意：

- `helm rollback` 只回滚 Kubernetes 资源，**不会回滚数据库迁移**；回滚前必须备份数据库。
- `helm uninstall` 不会删除外部依赖与 `zhongkui-crypto` Secret；如需彻底清理需手动删除。
- 迁移 Job 每次 `pre-upgrade` 都会重新执行，幂等。

## 12. 常见问题

| 症状 | 检查与处理 |
| --- | --- |
| 安装卡在 pre-install | `kubectl logs job/zhongkui-migrate`；多为 DSN 错误、数据库不可达或 pgvector 缺失 |
| Pod 报 `secret "zhongkui-crypto" not found` | 第 4 步未执行或命名空间不一致 |
| Pod 报 `password book` 相关错误 | Secret 中缺少 `password-book.bin`，或 `crypto.passwordBookFile` 名称不一致 |
| `readyz` 返回 503 | `kubectl describe pod` 查看依赖项；检查 DSN、Redis、etcd、S3 连通性 |
| Ingress 502 | `ZKI_API_UPSTREAM` 指向的 Service 名/端口错误；默认 `zhongkui-api:8080` |
| 登录后掉线 | `cookieDomain` 与访问域名不一致；HTTPS 时确认 `cookieSecure: true` |
| gRPC 无法连接 | `grpc.enabled=false`，或 Service/端口不一致；`kubectl get svc zhongkui-grpc` |
| OTel 无数据 | `otel.enabled` 未开启、端点为空的渲染错误，或 Collector 不可达；查看 Collector 日志 |
| 多副本读取密码本失败 | 密码本来自同一个 Secret，不应使用 RWO PVC；确认所有副本挂载同一 Secret |

更多排查路径见[故障排查](../how-to/troubleshooting.md)。
