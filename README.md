# zhongkui（钟馗）

> 网络流量采集、标签计算与风险告警平台。

zhongkui 从网络流量出发，完成「采集 → 结构化 → 打标 → 窗口统计 → 风险告警 → 通知」的闭环，并提供多租户管理端与内置 AI 助手。在平台底座之上，重点打磨字段级数据加密、Rego 复杂规则、多源数据接入与 AI 编写辅助四项能力，适合需要自建流量分析与风险识别能力的团队。

- 许可证：Apache-2.0
- 文档语言：中文
- 文档站点：[docs/.vitepress](docs/.vitepress/config.mts)（`scripts/docs.sh dev` 本地预览）
- 代码规范与架构约束：[AGENTS.md](AGENTS.md)

## 核心能力（平台底座）

| 能力 | 说明 |
| --- | --- |
| 流量采集 | 基于 XDP/wire 抓包，生成结构化流量快照并可靠上报 |
| 协议与指纹 | 解析网络层与 HTTP 等传输模型，提取可用于匹配的指纹 |
| 风险事件 | 按时间窗口聚合标签命中，条件满足即产生风险事件 |
| 通知 | 邮件、Webhook 等渠道，支持命名空间级模板与发送去重 |
| 多租户 | 命名空间隔离配置与数据，系统命名空间承载公共知识 |

## 技术亮点

### 1. 严格的数据加密与保密规范

- **字段级加密**：敏感字段以 AES-GCM 加密存储，每个字段由独立派生的子密钥保护；写入自动加密、读取自动解密。
- **落盘即密文**：抓包分片经 Zstd + AES-GCM 流式管道写出，磁盘与对象存储中不出现明文载荷。
- **明文查看按字段授权**：流量审计默认返回密文与对象存储键；查看明文需具备 `plaintext:crypto:{model}:{field}` / `plaintext:s3:...` 粒度的 RBAC 权限。
- **规则与模型侧不可见**：加密字段进入 Rego 输入文档时序列化为空串；AI 会话、消息、记忆与知识库正文同样加密存储；用户口令只保存 bcrypt 哈希。

### 2. Rego 规则：复杂逻辑与数据处理

- **表达力**：规则以 Rego v1 编写，可组合布尔、集合、字符串与结构化数据判定，统一产出 `tags` 标签集合。
- **工程化校验**：保存前做语法与契约校验，并对整组规则做语义编译以拦截跨规则冲突；输入字段 lint 与编辑器补全由模型元信息自动推导。
- **外部数据参与判定**：规则可调用内置函数 `sqlite.query(sql, params)`，直接查询已接入的第三方数据源，实现跨源关联打标。
- **可验证、可回滚**：提供规则 dry-run 与样例校验；标签规则和标签事件规则均保留版本快照并支持回滚，事件条件支持 AND/OR/NOT 表达式、阈值与抑制窗口。

### 3. 外部数据源接入

- **插件化类型**：内置 Redis、MySQL、PostgreSQL、ClickHouse、MongoDB、etcd 与 SQLite 文件等数据源；实现 `DatasourcePlugin` 接口并自注册即可扩展新类型。
- **接入即校验**：创建前可做连通性探测与表结构自省，前端据此补全 schema；非法配置不会落库。
- **命名空间隔离**：每个数据源按命名空间注册为只读 SQLite 虚拟表，连接与凭据互不串扰；单次查询限制行数并禁止写操作。
- **可恢复**：已启用数据源由 worker 重载恢复注册，SQLite 文件原件经对象存储加密留存。

### 4. AI 助手：规则、技能与报告编写辅助

- **规则编写**：内置「风险标签 Rego 规则共创」技能，从风险场景澄清、字段与数据源能力核对，到起草、dry-run 评分，最终经审批创建规则。
- **技能编写**：内置「周期报告技能共创」技能，把报告口径、取数与结构固化为可无人值守运行的技能；自定义技能支持固定参数与输入字段。
- **报告编写**：周期计划按 cron 调度技能生成日报、周报、月报并落对象存储，保留历史运行记录与产出。
- **安全边界**：写操作以审批卡片确认，AI 工具调用同样受能力白名单、命名空间与 RBAC 约束。

## 架构一览

```text
采集端（XDP/wire）
   │  流量快照
   ▼
Reporter（gRPC） ──► ClickHouse（raw 库）
   │                   └─ 大字段 → S3
   ▼
标签引擎（Rego + 窗口统计） ──► 风险事件 ──► 通知服务
   ▲
   │  配置 / 规则 / 数据源
管理端 API（/api/v1） ◄── 浏览器（Vue 3 + Vuetify 3）
   │
   └─ 异步任务 / 配置广播：Redis、etcd
```

各组件职责、数据流与代码地图见[系统总览](docs/curated/explanation/system-overview.md)。

## 部署

三种方式都可以从零完成部署与首次登录，按环境选择：

| 方式 | 适用场景 | 文档 |
| --- | --- | --- |
| Linux 二进制部署 | Linux 物理机、云主机、单机长期运行 | [Linux 二进制部署](docs/curated/tutorials/deploy-binary.md) |
| Docker Compose 部署 | 单机快速试用、演示、内置全部依赖 | [Docker Compose 部署](docs/curated/tutorials/deploy-docker-compose.md) |
| Kubernetes Helm 部署 | 生产集群，连接外部数据库/缓存/对象存储 | [Kubernetes Helm 部署](docs/curated/tutorials/deploy-helm.md) |

二进制 Release 包内含 `core/`（amd64/arm64）、`web/html`（管理端 + 文档站）、`deploys/binary/`（配置与 systemd 示例）与许可证文件。

官方镜像：

- core（web/worker/grpc/capture 共用）：`zhongkui-cn-beijing.cr.volces.com/open-source/zhongkui-core`
- web（nginx 静态站点）：`zhongkui-cn-beijing.cr.volces.com/open-source/zhongkui-web`

镜像为公有，默认无需登录；如使用私有镜像代理，请先 `docker login <registry>`。

三篇部署文档都包含：pgvector 扩展安装、Reporter gRPC 默认启动、OpenTelemetry 接入、管理员创建、验证与常见问题。

## Linux 开发环境

前置：Git 与 curl。克隆仓库后执行一条命令安装其余依赖（Docker、Go 1.27.3、protoc、Node 26 + pnpm 10、libpcap 与 Go 辅助工具）：

```bash
git clone https://gitee.com/olinex/zhongkui.git && cd zhongkui
scripts/install-tools.sh all

# 1. 启动依赖（PostgreSQL / ClickHouse / Redis / etcd / S3）
scripts/dev-containers.sh start
scripts/dev-doctor.sh                # 检查工具链、容器与依赖端口是否就绪

# 2. 生成配置与本地密钥（再按教程替换数据库口令与 S3 凭证）
go run cmd/zki/main.go create default-config -o config.toml
mkdir -p certs
go run cmd/zki/main.go create password-book -o certs/password-book.bin
go run cmd/zki/main.go create tls --name test --dir-path ./certs --ttl 8760h --dns-names localhost

# 3. 执行数据库迁移并创建管理员
go run cmd/zki/main.go migrate up
go run cmd/zki/main.go create user

# 4. 启动管理端与后台 worker（各开一个终端）
go run cmd/zki/main.go run web
go run cmd/zki/main.go run worker
```

打开 `[api]` 配置的地址即可登录。完整步骤、验证方式与常见问题见[Linux 开发环境部署](docs/curated/tutorials/linux-dev.md)。

采集真实流量需要额外启动 `run capture`，并具备抓包权限；原理见[采集架构](docs/generated/explanation/capture-architecture.md)。

## 文档

- 文档总目录：[docs/index.md](docs/index.md)
- 开始使用：[Linux 开发环境部署](docs/curated/tutorials/linux-dev.md) · [Linux 二进制部署](docs/curated/tutorials/deploy-binary.md) · [Docker Compose 部署](docs/curated/tutorials/deploy-docker-compose.md) · [Kubernetes Helm 部署](docs/curated/tutorials/deploy-helm.md) · [教程：第一条风险告警](docs/curated/tutorials/first-alert.md)
- 核心概念：[系统总览](docs/curated/explanation/system-overview.md) · [领域模型](docs/curated/explanation/domain-model.md) · [术语表](docs/curated/explanation/glossary.md)
- 操作指南：[故障排查](docs/curated/how-to/troubleshooting.md) · [数据源插件](docs/curated/how-to/add-datasource-plugin.md) · [通知渠道](docs/curated/how-to/add-notify-channel.md)
- 参考：[参考索引](docs/generated/reference/index.md)

文档按来源隔离：`docs/generated/` 由代码声明生成、禁止手改；`docs/curated/` 由人持有并审校。约定见[文档体系架构与执行规划](docs/curated/explanation/documentation-architecture.md)。

## 构建与测试

```bash
make build       # 编译到 bin/zki
make lint        # 注释完备性检查
make test        # 单元测试
make docs-check  # 校验生成文档与代码声明是否一致
make ci          # 以上检查的合集

scripts/dist.sh v1.2.3   # 打包 Release：两架构二进制 + 管理端 + 文档站
```

## 目录结构

```text
cmd/zki/            二进制入口
internal/capture/   抓包、快照与上报
internal/reporter/  上报接收与落库
internal/network/   网络模型、指纹与检测
internal/tagengine/ Rego 规则、窗口统计与事件判定
internal/notify/    通知模板、渲染与投递
internal/manager/   HTTP 路由、中间件与视图层
internal/ai/        AI 助手运行时
internal/docs/      文档注册表、生成与检查
internal/models/    领域模型
web/                Vue 3 + Vuetify 3 管理端
docs/               文档（generated / curated）
```

## 兼容性

- 管理端与 worker：Linux、macOS。
- 采集端：Linux（XDP/BPF 需要内核与权限支持）。
- 数据库：PostgreSQL、ClickHouse；缓存与队列：Redis；协调：etcd；对象存储：S3 兼容实现。

## 贡献与安全

- 贡献指南：[CONTRIBUTING.md](CONTRIBUTING.md)
- 安全策略：[SECURITY.md](SECURITY.md)
- 行为准则：[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- 变更日志：[CHANGELOG.md](CHANGELOG.md)

主仓位于 Gitee，GitHub 为只读镜像；Issue 与 Pull Request 请在 Gitee 提交。

## 许可证

Apache License 2.0，详见 [LICENSE](LICENSE)。第三方组件、版本与许可证全文见 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。
