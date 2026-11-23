# Linux 开发环境部署

> 状态：已校对
> 责任人：@olinex
> 依据：`scripts/dev-containers.sh`、`scripts/install-tools.sh`、`Makefile`、`internal/cmd/testcase.go`

本文面向在 Linux 上参与开发、调试的工程师：用源码启动完整的 zhongkui 开发环境，完成本地依赖、配置、数据库初始化与管理员创建。

> 本文只覆盖开发环境。生产部署请按环境选择：[Linux 二进制部署](deploy-binary.md) · [Docker Compose 部署](deploy-docker-compose.md) · [Kubernetes Helm 部署](deploy-helm.md)。

## 1. 准备基础工具

克隆仓库只需要 Git 与 curl；其余依赖由仓库脚本安装。Debian / Ubuntu 上：

```bash
sudo apt-get update
sudo apt-get install -y git curl ca-certificates
```

其他发行版用对应包管理器安装 git 与 curl。

## 2. 获取源码并安装开发环境依赖

```bash
git clone https://gitee.com/olinex/zhongkui.git
cd zhongkui
scripts/install-tools.sh all
```

`all` 按顺序安装完整开发环境，需要提权的步骤会自动调用 sudo：

| 步骤 | 内容 |
| --- | --- |
| `base` | git、make、curl、ca-certificates、gnupg |
| `docker` | Docker Engine + Compose / buildx 插件（Linux 走 Docker 官方 apt 源；macOS 需预先安装 Docker Desktop） |
| `go` | Go 1.27.3（Linux 安装到 `/usr/local/go`） |
| `protobuf` | protoc 与 protoc-gen-go / protoc-gen-go-grpc |
| `ts` | Node 26 + pnpm 10（前端构建与调试） |
| `pcap` | libpcap 开发库（采集端） |
| `dev` | asynq、pkgsite、goconvey 等辅助工具 |

> 受限网络下，Go 工具安装可能因 `sum.golang.org` 不可达而失败。脚本会自动使用与镜像构建一致的模块代理，也可用 `GOPROXY` / `GOSUMDB` 环境变量覆盖，例如 `GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn scripts/install-tools.sh protobuf`。

只补装某一项时执行对应子命令，例如 `scripts/install-tools.sh ts`。脚本会把 `go` / `gofmt` 链接到 `/usr/local/bin`，当前 shell 可直接使用；`docker` 组需重新登录后生效。随后检查工具链：

```bash
scripts/dev-doctor.sh --tools-only
```

### Go 模块代理（受限网络）

脚本只在自身执行期间使用内置代理；项目后续的 `go run`、`go build`、`go test` 仍读取 Go 自己的配置。安装完成后建议把同一组代理写入 Go 配置，一次设置长期生效：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
go env GOPROXY GOSUMDB   # 复核
```

地址可替换为团队或云厂商提供的模块代理；恢复默认用 `go env -u GOPROXY GOSUMDB`。`scripts/install-tools.sh` 会尊重 `go env -w` 已配置的非默认值，不会覆盖。

所有命令都经同一入口 `cmd/zki/main.go` 暴露。下文用 `go run cmd/zki/main.go {子命令}` 表示未安装二进制时的执行方式；执行 `make build` 后即可改用生成的 `bin/zki` 直接替换该前缀。

环境相关的三个脚本分工如下：

| 脚本 | 职责 |
| --- | --- |
| `scripts/install-tools.sh` | 安装工具链（`all` 一次装完，也可只补装单项） |
| `scripts/dev-containers.sh` | 启停、删除依赖容器并初始化测试库 |
| `scripts/dev-doctor.sh` | 检查工具链、容器与端口是否就绪 |

## 3. 启动依赖容器

zhongkui 依赖 PostgreSQL（core 库）、ClickHouse（raw 库）、Redis、etcd 与 S3 兼容对象存储：

- `scripts/dev-containers.sh pull`：拉取上述依赖镜像。
- `scripts/dev-containers.sh start`：以 host 网络启动同名容器（PostgreSQL 口令为 `root_password`）。
- 调试结束后用 `scripts/dev-containers.sh stop` 停止，`scripts/dev-containers.sh remove` 一并删除容器。

网络受限导致拉取失败时，用一个变量统一替换镜像源即可（保留原镜像路径，Docker Hub 官方镜像自动补 `library/`）：

```bash
export DEV_IMAGE_REGISTRY=registry.example.com
scripts/dev-containers.sh pull
scripts/dev-containers.sh start
```

`DEV_IMAGE_REGISTRY` 可带路径前缀（如 `registry.example.com/mirror`）。若镜像源不代理 `quay.io`（etcd），再单独设置 `ETCD_IMAGE`。需要精确覆盖单个镜像时仍可用 `REDIS_IMAGE`、`ETCD_IMAGE`、`CLICKHOUSE_IMAGE`、`POSTGRES_IMAGE`、`RUSTFS_IMAGE`，其优先级高于 `DEV_IMAGE_REGISTRY`。

单个镜像的拉取默认不限制时长；网络较慢又希望避免长时间挂起时，可设置 `PULL_TIMEOUT=<秒>`（超时后该镜像记为失败，脚本以非零退出码结束）：

```bash
PULL_TIMEOUT=600 scripts/dev-containers.sh pull
```

也可以不改脚本、直接给容器运行时配置 registry mirror（Docker：`/etc/docker/daemon.json` 的 `registry-mirrors`；nerdctl/containerd：`hosts.toml`）后拉取。

容器启动后执行全量检查：

```bash
scripts/dev-doctor.sh
```

脚本会逐项检查工具链、容器运行时、5 个依赖容器、依赖端口（PostgreSQL 5432、ClickHouse 9000、Redis 6379、etcd 2379、S3 9002）与 `config.toml`（尚未创建时只告警）；存在失败项时退出码为 1，按提示修复后重新执行即可。

## 4. 准备配置

程序通过 Viper 读取名为 `config` 的 TOML 文件，搜索路径依次为 `/etc/zhongkui`、当前目录与上级目录；也可用全局 `--config` / `-c` 指定文件。仓库已将 `config.toml` 加入 `.gitignore`，本地配置不会误提交。

先生成带完整字段注释的默认配置（bootstrap 命令，不需要已有配置；目标文件已存在时加 `-f` 覆盖）：

```bash
go run cmd/zki/main.go create default-config -o config.toml
```

再按默认配置引用的路径生成开发密钥材料：

```bash
mkdir -p certs
go run cmd/zki/main.go create password-book -o certs/password-book.bin
chmod 600 certs/password-book.bin
go run cmd/zki/main.go create tls --name test --dir-path ./certs --ttl 8760h --dns-names localhost
```

最后把与本地依赖容器相关的占位值改为实际值，其余保持默认：

| 配置项 | 修改为 |
| --- | --- |
| `[crypto].parent_secret_key` | 32 字节以上随机值，例如 `openssl rand -hex 32` 的输出 |
| `[database.core].dsn` | `password=root_password`（库名保持 `postgres`） |
| `[database.raw].dsn` | `default:root_password@127.0.0.1:9000`（库名保持 `default`） |
| `[s3].accessKey` / `secretKey` | `rustfsadmin` / `rustfsadmin` |

各配置段的作用：

- `[api]`：管理端 HTTP 服务的监听地址与端口。
- `[database.core]` 与 `[database.raw]`：分别指向 PostgreSQL 与 ClickHouse。
- `[redis.cache]`、`[redis.pipeline]`：缓存与任务队列，默认指向 `localhost:6379`。
- `[etcd]`：规则与配置变更广播，默认 `127.0.0.1:2379`。
- `[s3]`：抓包快照的对象存储，默认 `127.0.0.1:9002`、桶 `capture`。
- `[grpc.server.reporter]` 与 `[grpc.client.reporter]`：采集端与 Reporter 服务之间的通信；仅在运行 `capture` 时需要补全客户端 `accessKeyID` / `accessKeySecret`。

SMTP、Reporter 凭证等占位仅在启用对应功能时填写。接口语言由环境变量 `ZKI_LANG` 控制（`zh-Hans` 或 `en`，默认 `zh-Hans`）。配置改完后先复核字段含义与依赖连通性，避免带着错误配置启动：

```bash
go run cmd/zki/main.go describe config
go run cmd/zki/main.go describe deps
```

## 5. 初始化数据库与管理员

- `go run cmd/zki/main.go migrate up`：执行数据库迁移（默认 `--db all`，同时迁移 core 与 raw 两库）。
- `go run cmd/zki/main.go create user`：交互式创建管理员账号，可用 `-u/--username`、`-p/--password`、`-e/--email`、`-n/--nicklename` 直接给定字段，并选择是否为超级管理员。

迁移与建号只需执行一次。

## 6. 启动服务

至少需要两个进程（各开一个终端）：

1. `go run cmd/zki/main.go run web`：启动管理端 HTTP 服务与健康检查端点。
2. `go run cmd/zki/main.go run worker`：启动后台任务消费者与各处理器（数据源重载、标签引擎、流水线、选举与各类 asynq 处理器）。

若同时要采集真实流量，另开终端运行 `go run cmd/zki/main.go run capture`；若需独立部署 Reporter gRPC 服务，可运行 `go run cmd/zki/main.go run grpc`。

## 7. 验证

打开管理端（`[api]` 配置的地址与端口），用上一步创建的管理员账号登录。全新部署且尚未导入流量时，风险列表为空属于预期结果——这恰好证明服务已就绪：数据库已迁移、后台任务已在消费、鉴权与接口均正常。随后即可导入流量或启用采集，观察标签与风险内容逐步出现。

需要排查时可用 `go run cmd/zki/main.go version` 确认版本，用 `describe config` / `describe deps` 复核配置与依赖状态。

## 8. 下一步

- 生产部署：按环境选择 [Linux 二进制部署](deploy-binary.md)、[Docker Compose 部署](deploy-docker-compose.md) 或 [Kubernetes Helm 部署](deploy-helm.md)。
- 功能验证：按[教程：看到你的第一条风险告警](first-alert.md)编写第一条 Rego 规则并确认告警链路。
