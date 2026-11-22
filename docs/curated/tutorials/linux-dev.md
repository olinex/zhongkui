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

> 受限网络下，Go 工具安装可能因 `sum.golang.org` 不可达而失败。脚本默认使用与镜像构建一致的模块代理，也可用 `GOPROXY` / `GOSUMDB` 环境变量覆盖后重试，例如 `GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn scripts/install-tools.sh protobuf`。

只补装某一项时执行对应子命令，例如 `scripts/install-tools.sh ts`。安装完成后重新登录（或执行 `. /etc/profile.d/go.sh`），使 Go 的 PATH 与 docker 组生效，然后检查工具链：

```bash
scripts/dev-doctor.sh --tools-only
```

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

容器启动后执行全量检查：

```bash
scripts/dev-doctor.sh
```

脚本会逐项检查工具链、容器运行时、5 个依赖容器、依赖端口（PostgreSQL 5432、ClickHouse 9000、Redis 6379、etcd 2379、S3 9002）与 `config.toml`（尚未创建时只告警）；存在失败项时退出码为 1，按提示修复后重新执行即可。

## 4. 准备配置

程序通过 Viper 读取名为 `config` 的 TOML 文件，搜索路径依次为 `/etc/zhongkui`、当前目录与上级目录；也可用全局 `--config` / `-c` 直接指定文件路径。关键在于各依赖段要与本地容器对齐：

- `[api]`：管理端 HTTP 服务的监听地址与端口。
- `[database.core]` 与 `[database.raw]`：分别指向 PostgreSQL 与 ClickHouse。
- `[redis.cache]`、`[redis.pipeline]`：缓存与任务队列，须指向容器暴露的 Redis。
- `[etcd]`：规则与配置变更广播。
- `[s3]`：抓包快照的对象存储。
- `[grpc.server.reporter]` 与 `[grpc.client.reporter]`：采集端与 Reporter 服务之间的通信。

接口语言由环境变量 `ZKI_LANG` 控制（`zh-Hans` 或 `en`，默认 `zh-Hans`）。可用 `describe config` 打印全部配置项及其说明（说明即配置项的 `comment` tag），用 `describe deps` 探测核心外部依赖的连通性，避免带着错误配置启动。

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
