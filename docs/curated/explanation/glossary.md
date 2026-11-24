# 术语表

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/models`、`internal/tagengine`、`internal/notify`、`internal/ai`、`internal/manager/views`

遇到不熟悉的词先查这里。术语按领域分组；「代码位置」给出权威定义处，字段细节以生成参考页为准。

## 1. 组织与平台

| 术语 | 英文/标识 | 含义 | 代码位置 |
| --- | --- | --- | --- |
| 命名空间 | namespace | 租户边界；配置、数据、AI 知识都按命名空间隔离 | `internal/models/core/namespace.go` |
| 系统命名空间 | system namespace | 内置公共知识区，存放接口目录与生成文档，所有租户只读可见，禁止删除 | `internal/pkg/base` |
| 功能开关 | feature flag | 控制某一能力是否对当前部署开放；关闭时前端展示不可用态 | `internal/manager/features` |
| 管理端 | manager | 面向运营与管理的 HTTP API 与前端 | `internal/manager` |
| 操作事件 | change event | 配置变更经 etcd 广播给各进程的事件 | `internal/ai`、`internal/pkg` |

## 2. 采集与流量

| 术语 | 英文/标识 | 含义 | 代码位置 |
| --- | --- | --- | --- |
| 采集器 | capturer | 一个抓包实例，按命名空间 + 名称 + 版本标识 | `internal/models/core/capturer.go` |
| 快照 | snapshot | 一次抓包结果的结构化流量记录 | `internal/capture` |
| 原始库 | raw database | ClickHouse，承载高写入的流量数据 | `internal/models/network` |
| 核心库 | core database | PostgreSQL，承载配置与业务实体 | `internal/models/core` |
| 对象存储 | S3 | 存放抓包快照与大字段（如报文体） | 配置段 `[s3]` |
| Reporter | reporter | 接收采集端上报并落库的 gRPC 服务 | `internal/reporter` |
| 传输模型 | transport model | 可被标签规则处理的目标模型，如 `HttpTransport` | `internal/models/network` |
| 输入文档 | input document | 规则实际看到的模型投影；敏感字段被剔除，需用 `crypto.reveal` 获取明文 | `internal/tagengine` |
| 指纹 | fingerprint | 从流量中提取的可用于识别与匹配的特征 | `internal/network/detectors` |

## 3. 标签与告警

| 术语 | 英文/标识 | 含义 | 代码位置 |
| --- | --- | --- | --- |
| 标签规则 | tag rule | 一段 Rego 源码，声明在何种条件下给流量打标签 | `internal/models/tag/tag_rule.go` |
| 规则版本 | rule version | 规则每次变更留下的历史快照，可回滚 | `internal/models/tag/tag_rule.go` |
| 打标 | tagging | 规则命中后给一行流量写入标签的结果 | `internal/tagengine` |
| 窗口统计 | window summing | 按时间窗口聚合标签命中的统计过程 | `internal/models/tag` |
| 事件规则 | event rule | 定义窗口、条件与通知组的规则 | `internal/models/tag` |
| 事件条件 | event condition | 事件规则中的判定表达式 | `internal/models/tag` |
| 标签事件 | tag event | 事件条件命中的一次风险记录 | `internal/models/tag` |
| 命中明细 | event hit | 事件记录对应的具体流量行 | `internal/models/tag` |
| 规则样例 | rule sample | 用于运行期校验的样例数据，按命名空间与模型隔离 | `internal/tagengine/base.go` |

## 4. 通知

| 术语 | 英文/标识 | 含义 | 代码位置 |
| --- | --- | --- | --- |
| 通知渠道 | notification channel | 渠道类型实例，如邮件或 Webhook | `internal/models/core/notification.go` |
| 通知目标 | notification target | 渠道下的具体收件方 | `internal/models/core/notification.go` |
| 通知组 | notification group | 一组通知目标；每个命名空间至多一个默认组 | `internal/models/core/notification.go` |
| 事件模板 | event template | 事件对应的标题/正文模板元信息 | `internal/notify/template.go` |
| 发送记录 | notification record | 一次投递的结果与负载快照，用于去重与审计 | `internal/models/core/notification.go` |

## 5. AI 能力

| 术语 | 英文/标识 | 含义 | 代码位置 |
| --- | --- | --- | --- |
| 会话 | session | 用户与助手的一段连续对话 | `internal/models/ai` |
| 领域事件 | domain event | AI 助手唯一的流式 UI 协议：服务端以 SSE 下发（回合 / 消息 / 内容块 / 工具 / 交互），前端按块渲染 | `internal/ai/assistant/`、`internal/ai/turns/` |
| 内容块 | block | 一条助手消息内的有序展示单元：思考、正文、工具调用、审批、问卷、下一步、压缩、中断提示 | `internal/ai/assistant/blocks.go` |
| 回合 | turn | 从用户提问（或一次动作续跑）到本轮结束的一次执行；由会话内的 TurnLoop 驱动 | `internal/ai/turns/loop.go` |
| TurnLoop | turn loop | eino ADK 的多轮会话驱动：持有输入生成、Agent 选择、事件投影、续跑映射与 checkpoint | `internal/ai/turns/loop.go` |
| 交互 | interaction | 需要用户参与并持久化的内容块：审批、问卷、推荐的下一步 | `internal/models/ai/agent_interaction.go` |
| 长期记忆 | memory | 跨会话保留的用户/租户事实，按命名空间隔离 | `internal/ai/memory/`（`store.go` / `summary.go` / `extract.go`） |
| 知识库 | knowledge base | 用户维护的文档集合，支持语义与关键词混合检索 | `internal/ai/knowledge/searcher.go` |
| 分块 | chunk | 文档检索的最小单元，携带向量与全文检索列 | `internal/models/ai` |
| 父子索引 | small-to-big | 用小块命中、按整篇文档返回的检索策略 | `internal/ai` 检索实现 |
| 工具 | tool | Agent 可调用的能力，分 builtin / client / MCP | `internal/ai/config/tool_specs.go` |
| 客户端工具 | client tool | 由前端在用户凭据下执行的工具，用 eino 中断挂起、用动作续跑 | `agentkit/client_tools.go` |
| 审核重要级 | audit level | 对一次客户端工具调用的风险分级：must / suggest / auto / none | `internal/ai/audit.go` |
| 审核阀门 | review valve | 会话级的人审策略：each / auto / none | `internal/models/ai` |
| 原生接口 | native API | 平台自身声明的 `/api/v1` 接口，登记在接口目录 | `internal/manager/views/catalog.go` |
| 接口目录 | API catalog | 接口用途与入参/出参元信息的注册表，供文档与 AI 使用 | `internal/manager/views/catalog.go` |
| 能力地图 | capability map | 「意图 → 接口」索引，由视图层能力声明派生并写入 `capability` 分区按需检索 | `internal/manager/views/capability.go`、`internal/ai/capability` |
| 上下文压缩 | compaction | 把过长历史摘要化以控制上下文预算，进度以 `message.compress` 事件更新压缩块 | `internal/ai/memory/`、`agentkit/compaction.go` |
| 挂起现场 | checkpoint | eino 序列化的引擎现场，续跑据此恢复；业务挂起元数据另存 `agent_runs` | `agentkit/checkpoint.go` |

## 6. 文档体系

| 术语 | 英文/标识 | 含义 | 代码位置 |
| --- | --- | --- | --- |
| 生成文档 | generated docs | 由代码声明派生、可重算的页面，带 `DO NOT EDIT` | `internal/docs` |
| 策展文档 | curated docs | 由人持有并审校的页面，AI 可起草但不可被生成器覆盖 | `docs/curated/` |
| 文档注册表 | registry | 文档大纲与生成清单的唯一登记处 | `internal/docs/registry.go` |
| 事实投影 | projection | 把声明源渲染成参考页的产物 | `internal/docs/export.go` |
| RAG 分区 | ingest partition | 生成文档、策展文档、接口、表、能力各自独立的入库分区 | `internal/pkg/docconv` |
