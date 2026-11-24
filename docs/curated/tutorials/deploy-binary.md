# Linux 二进制部署

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`deploys/binary/config.example.toml`、`deploys/binary/zk-web.service`、`deploys/binary/zk-worker.service`、`deploys/binary/zk-grpc.service`、`deploys/binary/otel.env.example`、`deploys/binary/nginx.conf.template`、`scripts/dist.sh`、`internal/cmd/user.go`、`internal/cmd/migrate.go`

本文面向第一次接触 zhongkui 的工程师：在一台 Linux 机器上，用二进制 + systemd 跑起管理端（web）、后台任务（worker）与 Reporter gRPC，再配 nginx 提供浏览器访问。所有命令都可以直接复制执行，`<...>` 需要替换。

> 获取方式二选一：下载 Release 包，或克隆源码后执行 `scripts/dist.sh <version>` 生成同样的包。两种方式得到的 `zki-<version>.tar.gz` 结构一致，后续步骤完全相同。

## 1. 适用场景与前置条件

适合物理机、云主机、单机验证环境。默认端口与目录：

| 组件 | 进程 | 端口 | 说明 |
| --- | --- | --- | --- |
| 管理端 API | `zki run web` | 8080 | 浏览器经 nginx 访问 |
| Reporter gRPC | `zki run grpc` | 8081 | 采集端上报入口 |
| 后台 worker | `zki run worker` | 无对外端口 | 消费异步任务、标签引擎 |
| 健康探针 | 每个进程各一个 | 19090 / 19091 / 19092 | web / worker / grpc，必须错开 |
| 前端 | nginx | 80 | 静态站点 + `/api/` 反代 |

目录规划：

| 路径 | 用途 |
| --- | --- |
| `/usr/local/bin/zki` | 二进制 |
| `/etc/zhongkui/config.toml` | 主配置 |
| `/etc/zhongkui/otel.env` | OpenTelemetry 环境变量（可选） |
| `/etc/zhongkui/password-book.bin` | 密码本（0600） |
| `/etc/zhongkui/certs/` | Reporter TLS 证书 |
| `/var/lib/zhongkui/` | 运行目录（临时文件） |
| `/usr/share/nginx/html/` | 前端静态文件 |

依赖版本（已验证组合）：

- PostgreSQL 17，且安装 **pgvector** 扩展
- ClickHouse 25.3
- Redis 8
- etcd 3.6
- S3 兼容对象存储（RustFS / MinIO / 云对象存储）
- 前端静态产物已包含在 Release 包中，部署时不需要 Node
- 仅部署 Release 包时不需要 Go、Node、Docker；从源码生成安装包见 [Linux 开发环境部署](linux-dev.md) 与 3.2

资源建议：最小 2 核 4 GB；正式环境按流量与保留周期扩容。

### 1.1 运行时工具（可选）

- 基础工具（下载、解压、校验与端口检查）：

```bash
sudo apt update
sudo apt install -y curl tar sed coreutils iproute2
```

- nginx：第 8 节安装。
- PostgreSQL 客户端（2.2 建库与 13 节备份）：

```bash
sudo apt install -y postgresql-client
```

- AWS CLI v2（2.2 命令行创建 S3 桶；也可用云控制台）：

```bash
sudo apt install -y unzip
curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-$(uname -m).zip" -o awscliv2.zip
unzip -q awscliv2.zip
sudo ./aws/install
aws --version
```

- `clickhouse-client`（2.2 建库）：未安装时可在 ClickHouse 控制台执行建库语句，或改用其 HTTP 接口。

> 仅部署 Release 包时不需要 Go、Node、Docker；从源码生成安装包的工具链见 [Linux 开发环境部署](linux-dev.md) 与 3.2。

## 2. 准备依赖服务

二进制部署连接外部 PostgreSQL、ClickHouse、Redis、etcd 与 S3。已有托管服务时直接看 2.1 / 2.2；需要单机快速准备时，可使用发行版包或云服务，也可参考 [Docker Compose 部署](deploy-docker-compose.md) 启动依赖（需要 Docker，安装见 [Linux 开发环境部署](linux-dev.md)）。

### 2.1 pgvector 扩展（可选安装路径）

迁移脚本会执行 `CREATE EXTENSION IF NOT EXISTS vector` 并创建 `vector(1536)` 列。如果 PostgreSQL 没有 pgvector，`migrate up` 会失败。任选一种方式：

1. **Debian / Ubuntu（PGDG 源）**：`postgresql-*-pgvector` 不在发行版默认源中，直接安装会报 `Unable to locate package`。先启用 PostgreSQL 官方 PGDG 源：

   ```bash
   sudo apt install -y postgresql-common
   sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh
   sudo apt update
   # 包名中的版本号必须与已安装的 PostgreSQL 主版本一致（此处以 17 为例）
   sudo apt install -y postgresql-17-pgvector
   ```

   没有 `postgresql-common` 自带脚本，或脚本无法使用时，可手动写入源：

   ```bash
   sudo install -d /usr/share/postgresql-common/pgdg
   sudo curl -fsSL -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
     https://www.postgresql.org/media/keys/ACCC4CF8.asc
   . /etc/os-release
   echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt ${VERSION_CODENAME}-pgdg main" \
     | sudo tee /etc/apt/sources.list.d/pgdg.list
   sudo apt update
   sudo apt install -y postgresql-17-pgvector
   ```

   PostgreSQL 主版本不是 17 时替换包名（如 `postgresql-16-pgvector`）；仍可用 `apt-cache search pgvector` 确认。若源添加后仍搜不到包，说明当前系统代号不在 PGDG 支持范围，改用下面的源码编译方式。
2. **源码编译（发行版没有包）**：
   ```bash
   # 同样需要 PGDG 源提供 postgresql-server-dev-17；也可换成发行版自带的 postgresql-server-dev-all
   sudo apt install -y build-essential postgresql-server-dev-17 git
   git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git
   cd pgvector && make && sudo make install
   ```
3. **云托管 PostgreSQL**：在控制台启用 `vector` 扩展，或用具备 `rds_superuser` / 管理员权限的账号执行：
   ```sql
   CREATE EXTENSION IF NOT EXISTS vector;
   ```

验证（库必须已创建；尚未建库时可先用默认的 `postgres` 库验证）：

**登录 PostgreSQL**

```bash
# 本机安装的 PostgreSQL：默认 peer/socket 认证，无需口令
sudo -u postgres psql

# 远程或密码认证（托管实例、非本机部署）：按提示输入口令
psql -h 127.0.0.1 -p 5432 -U postgres -d postgres

# 非交互执行单条 SQL
PGPASSWORD='<POSTGRES_PASSWORD>' psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -c '<SQL>'
```

**检查扩展包与启用状态**

```bash
# 1) 扩展包是否已安装到 PostgreSQL：有行表示已安装
sudo -u postgres psql -d postgres -c \
  "SELECT name, default_version, installed_version FROM pg_available_extensions WHERE name = 'vector';"

# 2) 当前库是否已启用：有行表示已启用，extversion 为版本号（如 0.8.0）
sudo -u postgres psql -d postgres -c \
  "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
```

远程实例把 `sudo -u postgres psql -d postgres` 换成上面的 `psql -h ...` 连接方式即可。结果判读：

| 查询 | 有结果 | 无结果 |
| --- | --- | --- |
| `pg_available_extensions` | 扩展包已安装 | 回到 2.1 安装 pgvector |
| `pg_extension` | 当前库已启用，输出如 `0.8.0` | 在目标库执行 `CREATE EXTENSION IF NOT EXISTS vector;` |

切换目标库：psql 内执行 `\c zhongkui`，或连接时用 `-d zhongkui`。创建扩展需要数据库超级用户权限，权限不足时用管理员账号执行。

### 2.2 托管依赖：建库与建桶

使用外部 PostgreSQL / ClickHouse / S3 时，先完成：

```sql
-- PostgreSQL，用管理员账号执行
CREATE DATABASE zhongkui;
\c zhongkui
CREATE EXTENSION IF NOT EXISTS vector;
```

```bash
# ClickHouse
clickhouse-client --host 127.0.0.1 --password '<CLICKHOUSE_PASSWORD>' \
  --query "CREATE DATABASE IF NOT EXISTS zhongkui"

# S3（以 RustFS/MinIO 为例；云对象存储可在控制台建桶）
# 若 S3 是本仓库用 Docker 启动的 RustFS（scripts/dev-containers.sh 或 Compose），
# 账号与口令均为 rustfsadmin；其他服务用控制台或初始化时生成的凭据
export AWS_ACCESS_KEY_ID=rustfsadmin
export AWS_SECRET_ACCESS_KEY=rustfsadmin
# --region 必须与服务端一致：RustFS/MinIO 默认 us-east-1；
# 不指定时 AWS CLI 会带上本机配置的区域，可能报 InvalidLocationConstraint
aws --endpoint-url http://127.0.0.1:9002 --region us-east-1 \
  s3 mb s3://capture
```

> 建桶用的凭据必须与 `config.toml` 的 `[s3].accessKey` / `secretKey` 一致。代码不会自动创建 S3 桶，桶不存在会导致快照与大字段写入失败。未安装 AWS CLI 时，可直接在云对象存储控制台创建桶，或按 1.1 安装 AWS CLI v2。未安装 `clickhouse-client` 时，可在 ClickHouse 控制台执行建库语句，或改用其 HTTP 接口。若指定 `--region` 后仍报 `InvalidLocationConstraint`，检查环境变量 `AWS_REGION` / `AWS_DEFAULT_REGION` 与 `~/.aws/config` 中的 region 是否被覆盖。

## 3. 安装二进制

### 3.1 下载 Release 制品

从 Gitee Release 下载对应版本的压缩包：

```bash
# 以 v0.0.0-alpha.1 为例
VERSION=v0.0.0-alpha.1
curl -LO "https://gitee.com/olinex/zhongkui/releases/download/${VERSION}/zki-${VERSION}.tar.gz"

# Release 不提供单独的 .sha256 文件；下载后自行计算哈希，
# 与 Release 说明中公布的校验值比对（若未公布，可记录本次哈希用于后续核对）
sha256sum "zki-${VERSION}.tar.gz"

mkdir -p "$HOME/zki-${VERSION}"
tar -xzf "zki-${VERSION}.tar.gz" -C "$HOME/zki-${VERSION}"
cd "$HOME/zki-${VERSION}"
```

解压后的目录结构：

```text
core/
  zki_linux_amd64
  zki_linux_arm64
web/
  html/                # 管理端静态产物
    docs/              # 文档站静态产物
deploys/
  binary/
    config.example.toml
    nginx.conf.template
    zk-web.service
    zk-worker.service
    zk-grpc.service
    zk-capture.service
    otel.env.example
LICENSE
NOTICE
THIRD_PARTY_LICENSES.md
README.md
CHANGELOG.md
```

以下所有步骤都在该目录中执行，命令中的 `core/...`、`web/...`、`deploys/binary/...` 均相对此目录。按 CPU 架构选择 `core/zki_linux_<arch>`，安装到 `/usr/local/bin/zki`。

### 3.2 从源码生成同一个包（可选）

如需自行构建安装包，先按 [Linux 开发环境部署](linux-dev.md) 执行 `scripts/install-tools.sh all` 安装 Go 1.27.3+、Docker（buildx）与 pnpm 10，然后在仓库根目录执行：

```bash
scripts/dist.sh v0.0.0-alpha.1

mkdir -p "$HOME/zki-v0.0.0-alpha.1"
tar -xzf "dist/zki-v0.0.0-alpha.1.tar.gz" -C "$HOME/zki-v0.0.0-alpha.1"
cd "$HOME/zki-v0.0.0-alpha.1"
```

基础镜像无法直接从 Docker Hub 拉取时，可在调用 `scripts/dist.sh` 时注入 `GOLANG_IMAGE` / `RUNTIME_IMAGE`；具体见 CONTRIBUTING 的发布流程。

脚本生成的包与 Release 包结构一致。解压后从 3.3 继续，后续步骤与使用 Release 包完全相同。

### 3.3 安装到系统

```bash
# 选择与当前机器匹配的架构
case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *) echo "不支持的架构: $(uname -m)" >&2; exit 1 ;;
esac

sudo install -m 0755 "core/zki_linux_${ARCH}" /usr/local/bin/zki
sudo useradd --system --home-dir /var/lib/zhongkui --shell /usr/sbin/nologin zki
sudo mkdir -p /etc/zhongkui/certs /var/lib/zhongkui/tmp
sudo chown -R zki:zki /etc/zhongkui /var/lib/zhongkui
sudo chmod 750 /etc/zhongkui
zki version
```

## 4. 编写配置

```bash
sudo cp deploys/binary/config.example.toml /etc/zhongkui/config.toml
sudo chown root:zki /etc/zhongkui/config.toml
sudo chmod 640 /etc/zhongkui/config.toml
sudo vi /etc/zhongkui/config.toml
```

必须修改的字段：

| 配置段 | 字段 | 说明 |
| --- | --- | --- |
| `[crypto]` | `parent_secret_key` | 替换为 32 字节以上随机值；务必先备份 |
| `[database.core]` | `dsn` | PostgreSQL 地址、口令、库名 |
| `[database.raw]` | `dsn` | ClickHouse 地址、口令、库名 |
| `[redis.cache]` / `[redis.pipeline]` | `addr` / `password` | Redis 地址与口令 |
| `[etcd]` | `endpoints` | etcd 地址列表 |
| `[s3]` | `endpoint` / `accessKey` / `secretKey` / `bucket` | 对象存储；仓库 Docker 启动的 RustFS 用 `rustfsadmin` / `rustfsadmin` |
| `[grpc.client.reporter.tls]` | `domain` | 与下一步生成的证书 DNS 名一致 |
| `[grpc.client.reporter.auth]` | `accessKeyID` / `accessKeySecret` | 采集端上报凭证，后续创建 |
| `[api]` | `frontendBaseURL` / `[api.auth.cookie].domain` | 浏览器实际访问地址 |
| `[api.secure.cors]` | `allowOrigins` | 浏览器访问来源 |

> 命令行（`describe deps`、`migrate`、`create user`）与 systemd 服务读取同一份配置。为保证两者一致，建议把真实值直接写入 `config.toml`（文件权限 0640，仅 root 与 zki 可读），不要只写进环境文件。

如果希望把口令单独放在环境文件（高级用法），三个 unit 已配置 `EnvironmentFile=-/etc/zhongkui/zk.env`；执行 CLI 时需要用同样的变量：

```bash
sudo -u zki sh -c 'set -a; . /etc/zhongkui/zk.env; set +a; zki --config /etc/zhongkui/config.toml migrate up'
```

## 5. 初始化密钥

```bash
# 密码本：65536 字节随机数据，权限必须 0600
sudo -u zki /usr/local/bin/zki --config /etc/zhongkui/config.toml \
  create password-book -o /etc/zhongkui/password-book.bin
sudo chmod 600 /etc/zhongkui/password-book.bin

# Reporter TLS 证书；DNS 名必须与 grpc.client.reporter.tls.domain 一致
sudo -u zki /usr/local/bin/zki --config /etc/zhongkui/config.toml \
  create tls \
  --name reporter \
  --dir-path /etc/zhongkui/certs \
  --ttl 8760h \
  --dns-names reporter \
  --dns-names localhost
```

> 主密钥与密码本一旦用于加密就 **不能更换**，否则历史密文无法解密。请立即把 `/etc/zhongkui/password-book.bin`、`config.toml` 中的主密钥与数据库备份一起保存到安全位置。

检查配置与依赖连通性：

```bash
sudo -u zki zki --config /etc/zhongkui/config.toml describe deps
```

`describe deps` 的每一项都应为通过；有失败项时先修依赖，不要继续。

## 6. 迁移数据库并创建管理员

```bash
sudo -u zki zki --config /etc/zhongkui/config.toml migrate up
```

创建超级管理员（密码必须含大小写、数字与特殊字符，昵称至少 6 个字符）：

```bash
sudo -u zki env ZKI_SUPERUSER=true \
  zki --config /etc/zhongkui/config.toml create user \
  -u zkadmin \
  -p 'ZkAdmin2026!' \
  -n 系统管理员账号 \
  -e admin@example.com
```

> 示例口令仅用于演示，请在首次登录后立即修改。

## 7. 启动服务

```bash
sudo cp deploys/binary/zk-web.service /etc/systemd/system/
sudo cp deploys/binary/zk-worker.service /etc/systemd/system/
sudo cp deploys/binary/zk-grpc.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now zk-web zk-worker zk-grpc
sudo systemctl status zk-web zk-worker zk-grpc --no-pager
```

同机部署时，三个 unit 已分别设置 `ZKI_HEALTH_PORT=19090/19091/19092`，不要把三个进程的探针端口设成同一个，否则后启动的进程无法绑定。

查看日志：

```bash
sudo journalctl -u zk-web -f
sudo journalctl -u zk-worker -f
sudo journalctl -u zk-grpc -f
```

## 8. 部署前端

Release 包已包含构建好的管理端与文档站（`web/html`）以及 nginx 模板（`deploys/binary/nginx.conf.template`），无需再安装 Node：

```bash
sudo apt install -y nginx
sudo mkdir -p /usr/share/nginx/html
sudo rm -rf /usr/share/nginx/html/*
sudo cp -a web/html/. /usr/share/nginx/html/

# 模板中的 ${ZKI_API_UPSTREAM} 替换为本机 API 地址
sed 's|\${ZKI_API_UPSTREAM}|http://127.0.0.1:8080|g' deploys/binary/nginx.conf.template \
  | sudo tee /etc/nginx/conf.d/zhongkui.conf >/dev/null

sudo rm -f /etc/nginx/conf.d/default.conf
sudo nginx -t
sudo systemctl enable --now nginx
sudo systemctl reload nginx
```

安装完成后：管理端为 `http://<主机>/`，文档站为 `http://<主机>/docs/`。

## 9. 验证

```bash
curl -fsS http://127.0.0.1:19090/healthz && echo web-ok
curl -fsS http://127.0.0.1:19091/readyz  && echo worker-ok
curl -fsS http://127.0.0.1:19092/healthz && echo grpc-ok
ss -lntp | grep -E '8080|8081|1909[0-2]'
sudo -u zki zki --config /etc/zhongkui/config.toml describe deps
```

浏览器打开 `http://<服务器IP或域名>/`，用管理员账号登录。全新环境风险列表为空是正常的，表示服务已就绪但还没有流量。

## 10. 下一步

部署完成后，从[教程第 0 章：教程总览与系列路线](00-roadmap.md)开始，按推荐路线依次完成流量接入、标签规则与告警链路。

## 11. 接入 OpenTelemetry（可选）

默认 `OTEL_ENABLED=false`，不上报任何数据。复制示例并按需修改：

```bash
sudo cp deploys/binary/otel.env.example /etc/zhongkui/otel.env
sudo chown root:zki /etc/zhongkui/otel.env
sudo chmod 640 /etc/zhongkui/otel.env
sudo systemctl restart zk-web zk-worker zk-grpc
```

接入已有 OTLP Collector / APM：

```dotenv
OTEL_ENABLED=true
OTEL_PROVIDER=otlp
OTEL_OTLP_PROTOCOL=grpc
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4317
OTEL_EXPORTER_OTLP_COMPRESSION=gzip
OTEL_SAMPLER_TYPE=always_on
```

本地自检（无后端）：

```dotenv
OTEL_ENABLED=true
OTEL_PROVIDER=stdout
```

三个 unit 已各自设置 `OTEL_SERVICE_NAME=zhongkui-web|worker|grpc`，在 Collector / Uptrace 中应能看到三类服务。OTLP 端点不可达不会阻塞启动，只会丢失遥测。

如使用仓库自带的 Uptrace 脚本，注意其打印的环境变量缺少 `OTEL_ENABLED=true`，需要手动补上，并把 UI 中创建项目得到的 DSN 写入 `OTEL_EXPORTER_OTLP_HEADERS`。

## 12. 可选：接入采集端

采集端只在 Linux 支持，需要抓包权限。先创建访问密钥与采集器记录：

```bash
# 记录输出的 access-key-id / access-key-secret，填入配置的 [grpc.client.reporter.auth]
sudo -u zki zki --config /etc/zhongkui/config.toml create access-key \
  -u zkadmin -d "capture client" --enabled --ttl 8760h

# 创建采集器记录；device 与 BPF 表达式按真实网卡与流量调整
sudo -u zki zki --config /etc/zhongkui/config.toml create capturer \
  --namespace default --name test --version v1d \
  --device eth0 --bpf-expr 'tcp port 80 or tcp port 443' \
  --snap-len 1600 --read-timeout 5000 --promisc --enabled
```

确认配置中的 `[capture.resource]` 与上面一致，并把访问密钥填入 `[grpc.client.reporter.auth]`：

```toml
[grpc.client.reporter.auth]
enabled = true
authorize = 'accesskeySecret'
accessKeyID = '<输出的 access-key-id>'
accessKeySecret = '<输出的 access-key-secret>'
```

### 12.1 采集端与 Reporter gRPC 不同机

采集端不要求与 `zk-grpc` 部署在同一台服务器。`create access-key` / `create capturer` 仍在平台侧（能访问数据库的机器）执行，采集机只需要：

1. 一份可读取的 `config.toml`，把 gRPC 地址指向平台侧：

   ```toml
   [grpc.client.reporter]
   host = '10.0.0.12'        # Reporter gRPC 所在主机
   port = 8081
   serviceConfig = {}

   [grpc.client.reporter.tls]
   enabled = true
   domain = 'reporter'       # 必须与证书 DNS 名一致
   cert = ''
   certFilePath = '/etc/zhongkui/certs/reporter-cert.pem'

   [grpc.client.reporter.auth]
   enabled = true
   authorize = 'accesskeySecret'
   accessKeyID = '<输出的 access-key-id>'
   accessKeySecret = '<输出的 access-key-secret>'
   ```

2. 把平台侧生成的 `reporter-cert.pem`（自签证书即服务端证书；使用 CA 时放 CA）复制到采集机的 `/etc/zhongkui/certs/`。
3. 确保采集机能访问平台侧 `8081`，且 `domain` 与证书 SAN 一致。
4. 采集机不需要启动 `zk-web` / `zk-worker` / `zk-grpc`，只启用 `zk-capture.service`。

然后启动采集端：

```bash
sudo cp deploys/binary/zk-capture.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now zk-capture
sudo journalctl -u zk-capture -f
```

## 13. 升级、备份与回滚

升级前：

```bash
sudo systemctl stop zk-worker zk-grpc zk-web
sudo -u postgres pg_dump -d zhongkui > /backup/zhongkui-$(date +%F).sql
# ClickHouse 按既有备份方案执行；同时备份 /etc/zhongkui 与对象存储
```

替换二进制并迁移：

```bash
# 在解压后的新版本目录中执行
# 选择与当前机器匹配的架构
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
sudo install -m 0755 "core/zki_linux_${ARCH}" /usr/local/bin/zki

# 若前端或文档有更新，同步替换静态文件
sudo rm -rf /usr/share/nginx/html/*
sudo cp -a web/html/. /usr/share/nginx/html/

sudo -u zki /usr/local/bin/zki --config /etc/zhongkui/config.toml migrate up
sudo systemctl start zk-web zk-worker zk-grpc
```

注意：`migrate down` 可能删除数据，生产环境回滚前必须先备份；数据库结构回滚与应用版本回滚要配套。

## 14. 常见问题

| 症状 | 检查与处理 |
| --- | --- |
| 启动即退出 | `journalctl -u zk-web -n 100`；先跑 `describe deps`，确认配置与依赖 |
| `migrate up` 报 vector 相关错误 | PostgreSQL 未安装 pgvector；按 2.1 安装后在目标库执行 `CREATE EXTENSION IF NOT EXISTS vector` |
| 大字段/快照写入失败 | S3 桶不存在或凭据错误；确认 `bucket` 已创建、`endpoint` 不含 `http://` |
| `aws s3 mb` 报 `InvalidLocationConstraint` | AWS CLI 带了与服务端不一致的 region；命令加 `--region us-east-1`（RustFS/MinIO），并检查 `AWS_REGION` / `AWS_DEFAULT_REGION` |
| 登录后立即掉线 | `[api.auth.cookie].domain` 与浏览器地址不一致；改为实际域名或 IP |
| 只有 web 健康检查可用 | worker/grpc 健康端口冲突；确认 19090/19091/19092 各不相同 |
| 浏览器页面正常但接口 502 | 检查 `/etc/nginx/conf.d/zhongkui.conf` 是否由 `deploys/binary/nginx.conf.template` 生成，且 `/api/` 已反代到 `127.0.0.1:8080` |
| 采集端无数据 | 检查网卡、capability、access-key 与 `[capture.resource]`；见[采集架构](../../generated/explanation/capture-architecture.md) |

更多排查路径见[故障排查](../how-to/troubleshooting.md)。
