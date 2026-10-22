# 领域模型：实体、关系与生命周期

> 状态：草稿（AI 起草）
> 责任人：@olinex
> 依据：`internal/models/core`、`internal/models/network`、`internal/models/tag`、`internal/models/ai`

本页给出系统的实体地图：有哪些实体、它们如何关联、生命周期如何流转，以及字段细节应该去哪里查。字段与表结构的完整定义由生成参考页维护，本页不复制字段表。

## 1. 组织与身份

| 实体 | 作用 | 生命周期要点 |
| --- | --- | --- |
| `Namespace` | 租户边界，所有配置与数据的归属 | 创建后名称不可修改；内置系统命名空间受保护、禁止删除 |
| `User` | 登录主体 | 创建、改密、禁用；密码重置有独立记录 |
| `Group` | 用户分组，用于授权与可见性 | 与用户、策略关联 |
| `RbacPolicy` / `RbacPolicyTemplate` | 接口级授权策略 | 策略模板派生具体策略 |
| `AccessKey` / `AccessKeyPool` | 程序化访问凭据 | 有有效期与启停状态 |

命名空间是几乎所有实体的外键维度。跨租户操作必须显式解析作用域；系统命名空间是所有租户可见的只读公共区。

参考：[core 模型参考](../../generated/reference/internal/models/core.md)

## 2. 采集与流量数据

| 实体 | 作用 | 生命周期要点 |
| --- | --- | --- |
| `Capturer` | 采集器实例的元数据 | 以命名空间 + 名称 + 版本标识；注册、心跳、下线 |
| `Datasource` | 第三方数据源连接配置 | 归属命名空间；增删改经 etcd 广播热同步；凭据加密落库 |
| `HttpTransport` 等网络模型 | 一次流量快照的结构化表示 | 由采集端生成，写入 raw 库；大字段落对象存储 |
| `PacketMeta` / `BaseMeta` / `TagsMeta` 等 | 网络模型的公共元信息片段 | 作为模型字段被复用，不独立建表 |

流量实体是「读多写少、按时间增长」的数据：写入走 ClickHouse，查询按时间与维度过滤。加密字段在规则输入与查询展示中受脱敏约束。

参考：[network 模型参考](../../generated/reference/internal/models/network.md)、[采集架构](../../generated/explanation/capture-architecture.md)

## 3. 标签与告警

| 实体 | 作用 | 生命周期要点 |
| --- | --- | --- |
| `TagRule` | 一条 Rego 规则，绑定目标模型类型 | 归属命名空间；启停、编辑、删除 |
| `TagRuleVersion` | 规则的历史版本快照 | 每次变更产生版本，可回滚 |
| `TagEventRule` | 事件规则：窗口、条件、通知组 | 与标签规则配合，决定何时产生事件 |
| `TagEventCondition` | 事件条件表达式 | 属于事件规则，随规则版本演进 |
| `TagEventRecord` | 一次命中产生的事件记录 | 只追加；携带规则版本 |
| `TagEventRecordHit` | 事件命中明细（哪些行命中） | 与事件记录一对多 |
| `TagSumming` | 标签窗口聚合统计 | 按窗口滚动计算 |
| `TagRuleSample` | 规则运行期样例数据 | 按命名空间与模型隔离，供校验使用 |

关系要点：

- 一条标签规则可以有多个历史版本，但同一时刻只有当前版本参与求值。
- 事件规则引用标签规则与通知组；规则变更后，新产生的事件记录使用新版本。
- 事件记录与命中明细是审计与溯源的基础，不随规则删除而删除。

参考：[tag 模型参考](../../generated/reference/internal/models/tag.md)、[教程：看到你的第一条风险告警](../tutorials/first-alert.md)

## 4. 通知

| 实体 | 作用 | 生命周期要点 |
| --- | --- | --- |
| `NotificationChannel` | 渠道类型实例（如邮件、Webhook） | 归属命名空间；凭据加密 |
| `NotificationTarget` | 渠道下的具体投递目标 | 可加入多个通知组 |
| `NotificationGroup` | 目标集合 | 每个命名空间至多一个默认组 |
| `NotificationGroupTarget` | 组与目标的多对多关联 | 组内目标不重复 |
| `NotificationTemplate` | 命名空间级模板覆盖 | 未配置时回退到 i18n 默认模板 |
| `NotificationRecord` | 发送记录与负载快照 | 同一事件对同一目标至多一条 |

参考：[如何新增通知渠道与模板](../how-to/add-notify-channel.md)、[core 模型参考](../../generated/reference/internal/models/core.md)

## 5. AI 能力

| 实体 | 作用 | 生命周期要点 |
| --- | --- | --- |
| `LlmModel` / `LlmBinding` | 模型登记与档位绑定 | 绑定决定运行期使用哪个模型；维度与能力需匹配 |
| `McpServer` / `LlmTool` | 外部 MCP 服务与工具登记 | 工具启用受命名空间与配置控制 |
| `AgentParam` | 运行参数与提示词 | 有出厂值与运行值，可恢复出厂 |
| `AgentSession` | 会话 | 归属用户与命名空间；可压缩、可归档 |
| `AgentMessage` / `AgentMessageBlock` | 消息与结构化块 | 消息按轮次追加；块承载工具调用、审批、引用等 |
| `AgentMemory` | 长期记忆 | 按命名空间隔离；可重嵌入 |
| `KnowledgeDocument` / `KnowledgeChunk` | 知识库文档与分块 | 文档可加密；分块携带向量与全文检索列 |
| `AgentRun` / `AgentRunEvent` | 一次运行的现场与事件流 | 支持挂起、审批、续跑、取消 |
| `AgentSessionCompaction` | 会话压缩结果 | 旧消息被摘要替换，保留可追溯记录 |

参考：[AI Agent 功能架构详解](agent-architecture.md)、[LLM Agent + RAG 设计](../../generated/explanation/agent-rag-design.md)、[ai 模型参考](../../generated/reference/internal/models/ai.md)

## 6. 关系图

```text
Namespace 1 ── n Capturer
Namespace 1 ── n Datasource
Namespace 1 ── n TagRule 1 ── n TagRuleVersion
Namespace 1 ── n TagEventRule 1 ── n TagEventCondition
TagEventRule n ── 1 NotificationGroup
TagEventRule 1 ── n TagEventRecord 1 ── n TagEventRecordHit
NotificationGroup n ── n NotificationTarget n ── 1 NotificationChannel
NotificationTarget 1 ── n NotificationRecord
Namespace 1 ── n KnowledgeDocument 1 ── n KnowledgeChunk
User 1 ── n AgentSession 1 ── n AgentMessage 1 ── n AgentMessageBlock
AgentSession 1 ── n AgentRun 1 ── n AgentRunEvent
Namespace 1 ── n AgentMemory
```

## 7. 生命周期与一致性

- **配置类实体**（规则、数据源、通知）以命名空间为隔离单位，变更经 etcd 广播到消费进程，最终一致而非强同步。
- **数据类实体**（流量、标签、事件）以追加为主，按时间查询；删除规则不影响既有事件记录。
- **AI 实体** 与会话绑定用户与命名空间；知识库文档区分人工文档与生成分区，同步只读写各自分区。
- **跨实体删除** 由外键与业务保护共同约束，例如系统命名空间禁止删除，避免级联清空共享知识。

## 8. 字段查哪里

- 表结构与字段含义：`docs/generated/reference/internal/models/<域>.md`（当前为包级参考；按表拆分的字段页在 P1 生成）。
- 接口入参/出参：`docs/generated/reference/api/`（P1 生成）。
- 枚举取值：`docs/generated/reference/enums.md`（P1 生成）。
- 配置项：`docs/generated/reference/config.md`（P1 生成）。
