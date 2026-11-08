# zhongkui（钟馗）

> 网络流量采集、标签计算与风险告警平台。

zhongkui 从网络流量出发，完成「采集 → 结构化 → 打标 → 窗口统计 → 风险告警 → 通知」的闭环，并提供多租户管理端与内置 AI 助手。适合需要自建流量分析与风险识别能力的团队。

- 许可证：Apache-2.0
- 文档语言：中文
- 文档站点：[docs/.vitepress](docs/.vitepress/config.mts)（`scripts/docs.sh dev` 本地预览）
- 代码规范与架构约束：[AGENTS.md](AGENTS.md)

## 核心能力

| 能力 | 说明 |
| --- | --- |
| 流量采集 | 基于 XDP/wire 抓包，生成结构化流量快照并可靠上报 |
| 协议与指纹 | 解析网络层与 HTTP 等传输模型，提取可用于匹配的指纹 |
| 标签规则 | 用 Rego 编写规则，对流量打标签，支持版本与回滚 |
| 风险事件 | 按时间窗口聚合标签命中，条件满足即产生风险事件 |
| 通知 | 邮件、Webhook 等渠道，支持命名空间级模板与发送去重 |
| 多租户 | 命名空间隔离配置与数据，系统命名空间承载公共知识 |
| AI 助手 | 会话、长期记忆、知识库检索、工具调用与人工审批 |

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

## 快速开始

前置：Go 1.27.3+、Docker（用于本地依赖容器）。

```bash
# 1. 启动依赖（PostgreSQL / ClickHouse / Redis / etcd / S3）
scripts/dev-deps.sh start

# 2. 执行数据库迁移并创建管理员
go run cmd/zki/main.go migrate up
go run cmd/zki/main.go create user

# 3. 启动管理端与后台 worker（各开一个终端）
go run cmd/zki/main.go run web
go run cmd/zki/main.go run worker
```

打开 `[api]` 配置的地址即可登录。完整步骤、验证方式与常见问题见[快速开始教程](docs/curated/tutorials/quick-start.md)。

采集真实流量需要额外启动 `run capture`，并具备抓包权限；原理见[采集架构](docs/generated/explanation/capture-architecture.md)。

## 文档

- 文档总目录：[docs/index.md](docs/index.md)
- 开始使用：[快速开始](docs/curated/tutorials/quick-start.md) · [第一条风险告警](docs/curated/tutorials/first-alert.md)
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
