# 系统总览：组件、数据流与部署拓扑

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/capture`、`internal/reporter`、`internal/worker`、`internal/manager`、`internal/tagengine`、`internal/notify`、`internal/provider`

本页回答三个问题：系统由哪些进程组成、一次采集到告警的数据怎么流动、改动某处代码时应该先看哪个包。字段级细节不在此展开，见「领域模型」与生成参考页。

## 1. 一句话架构

zhongkui 是一个多租户的网络流量采集与分析平台：采集端抓包并生成流量快照，经 Reporter 写入原始库与对象存储；标签引擎按 Rego 规则给流量打标签，再按时间窗口聚合触发风险事件；通知服务把事件投递到配置的渠道。管理端 API 负责配置与查询，AI 运行时在同一套权限与命名空间边界内提供问答与检索。

## 2. 运行组件

| 组件 | 启动方式 | 职责 | 关键代码 |
| --- | --- | --- | --- |
| 管理端 API | `zki run web` | 提供 `/api/v1` 接口、鉴权、功能开关、配置与查询 | `internal/manager` |
| 后台 worker | `zki run worker` | 消费异步任务：数据源重载、标签引擎、流水线、选举等 | `internal/worker` |
| 采集端 | `zki run capture` | 基于 XDP/wire 抓包、生成流量快照并上报 | `internal/capture` |
| Reporter gRPC | `zki run grpc` | 接收采集端上报，写入原始库与对象存储 | `internal/reporter` |
| 标签引擎 | 随 worker 运行 | 执行 Rego 规则、按窗口聚合、产生标签事件 | `internal/tagengine` |
| 通知服务 | 随 worker/管理器调用 | 解析目标、渲染模板、投递并记录发送结果 | `internal/notify` |
| AI 运行时 | 随 web 进程按需启用 | 会话、工具调用、检索增强与流式输出 | `internal/ai` |

所有进程通过 `internal/provider` 统一装配依赖（数据库、Redis、etcd、S3、AI 运行时等），资源生命周期由注入器管理。

## 3. 两条主数据流

### 3.1 采集到告警

```text
采集端抓包
  → 生成流量快照（网络层 / HTTP 等模型）
  → Reporter 接收
      ├─ 原始数据写入 raw 库（ClickHouse）
      └─ 大字段（如报文体）写入 S3 兼容对象存储
  → 标签引擎读取目标模型、执行 Rego 规则
  → 打标结果写入标签记录
  → 按时间窗口聚合统计
  → 命中条件产生标签事件（风险）
  → 通知服务按事件模板渲染并投递到通知渠道
```

关键边界：

- 规则只能读取模型暴露的输入文档，加密字段对规则不可见。
- 窗口统计与事件判定在标签引擎内完成，通知只消费事件，不参与判定。
- 原始数据与管理数据分库：ClickHouse 承载高写入流量，PostgreSQL 承载配置与业务实体。

### 3.2 管理面读写

```text
浏览器 → /api/v1（鉴权 + 命名空间中间件）→ 视图层 In/Out → 领域服务/存储
```

- 每个请求都带命名空间语义；跨租户读取必须经统一的作用域函数解析。
- 视图层负责入参/出参结构与校验，不承载业务规则。
- 所有接口在注册期声明用途与入参/出参元信息，供文档与 AI 能力目录复用。

## 4. 存储与外部依赖

| 依赖 | 用途 | 配置段 |
| --- | --- | --- |
| PostgreSQL（core） | 配置、用户、规则、标签、通知、AI 元数据 | `[database.core]` |
| ClickHouse（raw） | 原始流量数据 | `[database.raw]` |
| Redis | 缓存与异步任务队列 | `[redis.cache]`、`[redis.pipeline]` |
| etcd | 配置与规则变更广播 | `[etcd]` |
| S3 兼容存储 | 抓包快照与大字段 | `[s3]` |
| gRPC Reporter | 采集端与 Reporter 通信 | `[grpc.server.reporter]`、`[grpc.client.reporter]` |

本地开发可用 `scripts/dev-containers.sh start` 启动同名容器，用 `scripts/dev-doctor.sh` 检查工具链、容器与端口是否就绪；`zki describe config` 打印全部配置项，`zki describe deps` 探测连通性。

## 5. 多租户与边界

- **命名空间是租户边界**：配置、规则、数据源、通知、AI 文档与记忆都按命名空间隔离。
- **系统命名空间**：代码声明的公共知识（接口目录、生成文档）写入内置系统命名空间，对所有租户只读可见；该命名空间禁止删除。
- **认证与授权**：管理端统一鉴权，接口级授权由策略中间件执行；AI 工具调用不绕过这套边界。
- **规则输入脱敏**：加密字段在进入 Rego 输入文档时序列化为空串，规则无法读取明文。

## 6. 代码地图

| 目录 | 职责 |
| --- | --- |
| `cmd/zki` | 二进制入口 |
| `internal/cmd` | Cobra 命令与子命令装配 |
| `internal/capture` | 抓包、快照生成、上报 |
| `internal/reporter` | 上报接收与落库 |
| `internal/network` | 网络层模型、指纹与检测器 |
| `internal/models` | 数据模型与存储映射 |
| `internal/tagengine` | Rego 规则、窗口统计、事件判定 |
| `internal/notify` | 通知模板、渲染、投递 |
| `internal/worker` | 异步任务与处理器注册 |
| `internal/manager` | HTTP 路由、中间件、视图层 |
| `internal/ai` | Agent 运行时、检索、记忆、工具 |
| `internal/docs` | 文档注册表、生成与检查 |
| `internal/provider` | 依赖注入与生命周期 |
| `internal/pkg` | 通用工具包（pipeline、enum、errors 等） |
| `web` | Vue 3 + Vuetify 3 管理端 |

> 说明：上表是手工维护的稳定地图；每个包的符号细节以 `docs/generated/reference/` 下的自动生成页为准。

## 7. 延伸阅读

- [领域模型：实体、关系与生命周期](domain-model.md)
- [采集架构：wire/XDP、快照与上报](../../generated/explanation/capture-architecture.md)
- [教程：看到你的第一条风险告警](../tutorials/first-alert.md)
- [异步任务与队列](../../generated/explanation/async-worker.md)
- [管理端分层与视图 In/Out 解耦](../../generated/explanation/manager-layering.md)
- [依赖注入与生命周期](../../generated/explanation/dependency-injection.md)
