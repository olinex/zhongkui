# 快速开始：从零运行 zhongkui

> 状态：已校对
> 责任人：@olinex
> 依据：`internal/cmd/testcase.go`

从安装、配置到启动服务，完成一次最小可运行的部署。

本文是一篇从零开始的教程：假设你已拿到源码仓库，按顺序完成依赖准备、配置、初始化与启动， 最终在管理端看到一个可用的（内容为空的）风险列表，即表示采集、识别、存储与展示整条链路已经打通。

所有命令都经同一入口 cmd/zki/main.go 暴露。下文用 `go run cmd/zki/main.go {子命令}` 表示未安装二进制时的执行方式；执行 `make build` 后即可改用生成的 `bin/zki` 直接替换该前缀。

## 1. 准备工具链

先确认本机 Go 版本与 go.mod 的 `go` 指令一致。随后安装项目脚本工具与抓包所需的系统依赖：

- `scripts/install-tools.sh dev`：安装开发辅助工具（asynq、pkgsite、goconvey 等）。
- `scripts/install-tools.sh pcap`：安装采集侧依赖（pcap 相关系统库）；只有需要本地跑 `run capture` 时才必须。

## 2. 启动依赖容器

zhongkui 依赖 PostgreSQL（core 库）、ClickHouse（raw 库）、Redis、etcd 与 S3 兼容对象存储：

- `scripts/dev-deps.sh pull`：拉取上述依赖镜像。
- `scripts/dev-deps.sh start`：以 host 网络启动同名容器（PostgreSQL 口令为 `root_password`）。
- 调试结束后用 `scripts/dev-deps.sh stop` 停止，`scripts/dev-deps.sh remove` 一并删除容器。

## 3. 准备配置

程序通过 Viper 读取名为 `config` 的 TOML 文件，搜索路径依次为 `/etc/zhongkui`、当前目录与上级目录； 也可用全局 `--config` / `-c` 直接指定文件路径。关键在于各依赖段要与本地容器对齐：

- `[api]`：管理端 HTTP 服务的监听地址与端口。
- `[database.core]` 与 `[database.raw]`：分别指向 PostgreSQL 与 ClickHouse。
- `[redis.cache]`、`[redis.pipeline]`：缓存与任务队列，须指向容器暴露的 Redis。
- `[etcd]`：规则与配置变更广播。
- `[s3]`：抓包快照的对象存储。
- `[grpc.server.reporter]` 与 `[grpc.client.reporter]`：采集端与 Reporter 服务之间的通信。

接口语言由环境变量 `ZKI_LANG` 控制（`zh-Hans` 或 `en`，默认 `zh-Hans`）。 可用 `describe config` 打印全部配置项及其说明（说明即配置项的 `comment` tag）， 用 `describe deps` 探测核心外部依赖的连通性，避免带着错误配置启动。

## 4. 初始化数据库与管理员

- `go run cmd/zki/main.go migrate up`：执行数据库迁移（默认 `--db all`，同时迁移 core 与 raw 两库）。
- `go run cmd/zki/main.go create user`：交互式创建管理员账号，可用 `-u/--username`、`-p/--password`、 `-e/--email`、`-n/--nicklename` 直接给定字段，并选择是否为超级管理员。

迁移与建号只需执行一次。

## 5. 启动服务

至少需要两个进程（各开一个终端）：

1. `go run cmd/zki/main.go run web`：启动管理端 HTTP 服务与健康检查端点。
2. `go run cmd/zki/main.go run worker`：启动后台任务消费者与各处理器（数据源重载、标签引擎、 流水线、选举与各类 asynq 处理器）。

若同时要采集真实流量，另开终端运行 `go run cmd/zki/main.go run capture`； 若需独立部署 Reporter gRPC 服务，可运行 `go run cmd/zki/main.go run grpc`。

## 6. 验证

打开管理端（`[api]` 配置的地址与端口），用上一步创建的管理员账号登录。 全新部署且尚未导入流量时，风险列表为空属于预期结果——这恰好证明服务已就绪： 数据库已迁移、后台任务已在消费、鉴权与接口均正常。随后即可导入流量或启用采集， 观察标签与风险内容逐步出现。

需要排查时可用 `go run cmd/zki/main.go version` 确认版本，用 `describe config` / `describe deps` 复核配置与依赖状态。
