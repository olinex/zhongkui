# Trae Project Rules - zhongkui

## Project Overview

- **Module**: `zhongkuisec.com/zhongkui` (Go 1.27.3)
- **Type**: 网络流量抓包与数据分析平台
- **Stack**: Go (Gin + GORM + gRPC + Cobra + Viper + OpenTelemetry) + Vue 3 + Vuetify 3

## Development Status 开发阶段

封闭首版本开发阶段（closed first-version development）：数据库 schema、项目架构等可自由重构，无需考虑向后兼容与既有数据迁移。

- **版本管理**：SemVer 2.0.0 + annotated tag 为唯一版本来源；`v1.0.0` 前允许破坏性变更（升 minor），`patch` 仅修复；开发构建只注入 Commit，release 构建经 `make build-release VERSION=...` 注入 SemVer；发布流程见 CONTRIBUTING「版本与发布」。
  - **正式版本**：`vX.Y.Z`，如 `v1.2.3`
  - **预发布版本**：`vX.Y.Z-{label}.{n}`，label 取值及语义：
    - `alpha`：内部测试，功能仍在迭代，API 可能随时变更；格式 `vX.Y.Z-alpha.1`、`vX.Y.Z-alpha.2` …
    - `beta`：功能冻结，邀请外部用户试用，专注修复 bug；格式 `vX.Y.Z-beta.1`、`vX.Y.Z-beta.2` …
    - `rc`（Release Candidate）：候选发布版，若测试通过即可作为正式版发布；格式 `vX.Y.Z-rc.1`、`vX.Y.Z-rc.2` …
  - **版本号演进规则**：`v1.2.3-alpha.1` → `v1.2.3-alpha.2` → `v1.2.3-beta.1` → `v1.2.3-rc.1` → `v1.2.3`（预发布期修复 bug 不升 Y/Z，只递增 `.n`；功能变更或范围调整升 minor 或 patch 后从 alpha.1 重启）
  - **预发布 tag 必须带预发布后缀**，正式发布时创建不带后缀的 tag；同一 X.Y.Z 的预发布 tag 与正式 tag 可以并存

---

## Go Backend Conventions

### Imports

Three-block grouping, blank-line separated, alphabetical within each block: 标准库 / 第三方 / 项目内部。

```go
import (
	// 标准库
	"context"
	"fmt"

	// 第三方
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	"go.uber.org/zap"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)
```

### Naming Conventions

| Category | Style | Example |
|----------|-------|---------|
| Exported types/functions | PascalCase | `JwtAuthenticator`, `GetUser` |
| Unexported functions | camelCase | `filePath`, `newViperProvider` |
| Constructor functions | `NewXxx` | `NewJwtAuthenticator` |
| Getter functions | `GetXxx` | `GetUser`, `GetAuth` |
| Receiver variables | Single lowercase letter of type | `(j *JwtAuthenticator)`, `(u *User)` |
| Package-level constants/variables | UpperCamelCase | `AnonymousUsername`, `AuthCtxName`, `Prefix` |
| Package-level error sentinels | `Err` prefix | `ErrUserNotExist`, `ErrJwtBadToken` |
| Package-level zero-value variables | camelCase | `anonymous` |
| 标识符 `ID` 后缀 | 全大写 `ID`（Go 字段 / json tag / 前端标识符一致） | `DocumentID`、json `documentID`、前端 `documentID`、`sessionID` |

- **`ID` 后缀前后端一致**：凡是「标识符」语义的字段与变量，后缀一律全大写 `ID`——Go 结构体字段 `DocumentID`、其 json tag `documentID`、前端 TS / 模板标识符 `documentID` 三者同名同构，禁止出现 `documentId` / `docId` 这类小写 `d` 写法（`Id` 仅允许作为外部官方 API 的既有拼写，如 Vue 的 `useId`）。

### Error Handling

- Use project `errors` package（`internal/pkg/errors`，包裹 `github.com/pkg/errors`）：
  - `errors.New(msg)` / `errors.Newf(format, args...)` — 创建错误
  - `errors.WithStack(err)` — 包裹第三方/stdlib 错误（最常见）
  - `errors.Is(err, target)` — 错误链比较
- Sequential error checks use `if-else` cascading style; error 始终是最后一个返回值。
- 面向用户的业务错误详情必须走 i18n：需要展示给用户的拒绝原因（如档位绑定失败）一律经 `response.DetailContext` 构造（`CreateBadQueryError` / `CreateSchedulerTaskError` / `CreateBadBodyMessageError` / `CreateLlmBindingIncompatibleError` 等以 `msgID + args` 传参），键写入 `internal/pkg/translate/locales/{zh-Hans,en}.toml` 的 `[error]` 表并保持中英齐备；禁止把 `errors.New`/`errors.Newf` 产出的英文文案直接作为 `details` 下发（`CreateBadBodyError(err)` 仅用于「原因本身是诊断信息、无需本地化」的场景，如端点不可达、凭据无效）。

```go
if cryptoKey, err := j.cfg.ExtractCryptoKey(j.kd); err != nil {
	return "", CreateAccidentError(err)
} else if tokenStr, err := token.SignedString(cryptoKey); err != nil {
	return "", ErrJwtBadToken
} else {
	return j.appendPrefix(tokenStr), nil
}
```

### Comments

注释一律使用中文，禁止中英双语或纯英文（`// Package`、`// Deprecated:` 等由 Go 工具链识别的固定前缀按语言约定保留英文）；所有导出类型/函数/常量/结构体字段必须有注释，文件必须有 `// Package xxx` 文档注释。

```go
// UserClaims 自定义 JWT 声明结构
// 继承 jwt.RegisteredClaims 标准声明，并添加用户相关字段
type UserClaims struct {
	jwt.RegisteredClaims
	UnionID serializer.HexUUID `json:"unionID"`
}
```

- **强制手段**：注释完备性由自建 analyzer 检查，入口为 `make lint`（`cmd/commentlint` 驱动 `internal/lint/commentlint` 的 `analysis.Analyzer`，标准 `go/analysis` 驱动，诊断输出 `文件:行:列: 说明`，发现缺口时退出码 3），提交前与 CI 必跑，禁止只靠人工检查。
- **检查范围**：导出类型 / 函数 / 方法 / 常量 / 变量、导出结构体的字段（**含非导出字段**，字段带 `comment:"` tag 或行尾注释均视为已有注释）、导出接口的方法，以及包注释。
- **包注释按目录聚合**：同目录任一文件带包注释即视为该包已有包注释；被构建约束排除的同目录源文件同样计入（包注释允许落在 `//go:build linux` 这类实现文件中），外部测试包 `xxx_test` 的包注释由主包承载、不单独检查。
- **豁免规则**：包子句前的注释带生成标记（标准 `// Code generated <id> DO NOT EDIT.` 及 `DO NOT EDIT!!!` 等非标准生成头）的文件整体跳过，正文里提到标记不算生成文件；`_test.go` 中 `Test` / `Benchmark` / `Example` / `Fuzz` 前缀函数跳过（测试替身的导出方法与导出类型仍须注释）；内嵌字段、内嵌接口与多名字声明中「部分导出」的字段（如 `A, b string`）不检查。
- **口径与实现的单一来源**：检查口径的权威实现是 `internal/lint/commentlint/analyzer.go`，其行为契约由 `analyzer_test.go` 的 `analysistest` 用例（`testdata/src/`）锁定；新增/调整豁免规则必须同时改实现、测试用例与本条目。

### Struct Organization

- 用结构体嵌入实现组合：`BaseMeta` → `TimestampMeta` → `CaptureMeta` → `AsPacket`
- 字段按逻辑分组，组间空行分隔；字段须带相应的 GORM/JSON/mapstructure tag

### Database Index Naming

Composite indexes / unique constraints use `uq`/`idx` prefix:

| Index type | Format | Example |
|-----------|--------|---------|
| Unique index / constraint | `uq_{table}_{column1}_{column2}` | `uq_capturers_namespace_name_name_version` |
| Regular index | `idx_{table}_{column}` | `idx_tag_rule_versions_rule_id` |

- PostgreSQL 中索引名 schema 级唯一，必须带表名，否则跨表同名索引报 `relation already exists`
- 列顺序必须与 SQL migration 定义一致；GORM 组合索引列按结构体字段声明顺序生成，字段顺序须对齐（如 `uq_tag_rules_namespace_name_model_name` 要求 `NamespaceName` 先于 `Model` 先于 `Name`）
- 同时适用于 `internal/migrations` SQL 与 `internal/models` 中 GORM `uniqueIndex` / `index` tag

### 字段级加密存储（Field Encryption）

- **加密列用 `serializer.Crypto`**：敏感字段声明为 `*serializer.Crypto` 并带 `secure:"mode=\"aes-gcm\",offset=...,length=..."` tag，SQL 列类型 `BYTEA` 且可空（`001_init.sql` 的 `COMMENT` 标注 `stored encrypted with the crypto serializer`）；写路径赋值 `serializer.NewCrypto([]byte(...))`，读路径取 `string(field.Value())`（`Value()` 对 nil 安全）。密钥派生路径为 `FieldTag` → `GetKeyDerivation` → `DeriveChildKey(offset,length)`。
- **`offset` 全局唯一且不越界**：须满足 `offset + max(key.MinKeyLength, length) <= key.PasswordBookSize`，否则密钥派生回绕甚至 panic；新增/调整 `secure` 字段由 `internal/pkg/crypto/secure_offset_test.go`（AST 扫描 `internal/models`）守卫唯一性与边界。
- **AI 会话全链路一并加密**：同一隔离域内的会话内容不得留下明文旁路，`agent_messages.payload/citations_json`、`agent_interactions.payload`、`agent_sessions.summary`、`agent_session_compactions.summary`、`agent_runs.pending_calls/citations`、`agent_messages.content/draft_json` 一律按上述口径加密。`agent_runs.pending_calls/citations` 原为 `JSONB`，加密后落库为可空 `BYTEA`（失去 DB 侧 JSONB 路径查询能力，现有读写仅整列存取，无此需求）；`agent_sessions.title` 保留明文（列表展示与检索语义）。
- **摘要字段用 `serializer.Hash` + 带密钥 HMAC**：用于去重/陈旧判定的摘要字段声明为 `serializer.Hash` 并带 `secure:"mode=\"hmac-sha256\",offset=...,length=..."` tag（SQL 列 `CHAR(64)`，ClickHouse `String`），写路径赋值 `serializer.NewHashDigest(...)`，读路径与出参取 `field.Digest()`；禁止裸 `sha256`（防离线爆破）。序列化器只做 string/[]byte 互转、nil 安全与 JSON，并在 `Value` 时校验 tag mode 必须为 `hmac-sha256`。摘要须在写库前预计算（确定性，可等值匹配与建唯一索引），故不可复用可逆且随机的 `serializer.Crypto`。一次性摘要用 `crypto.DigestForField`；正文等体积大、无法一次性持有的流式摘要用 `crypto.HashKeyForField` 派生密钥后自行 `hmac.New(sha256.New, key)` 计算，且摘要与 S3 对象键同源时须同批改为 HMAC，避免明文摘要经列值泄漏内容指纹。
- **`map` 更新绕过序列化器**：`Updates(map[string]any{...})` / `Update(col, v)` 不经 field serializer，明文直写；加密字段必须走「结构体 + `Select(列...)`」更新以触发序列化器（先例见 `internal/ai/memory/store.go`、`internal/manager/views/ai/knowledge.go`、`internal/worker/handlers/knowledge/parse.go`）。
- **`Scan` 到本地结构体不解密**：检索/汇总行结构体须内嵌模型结构体（如 `struct{ ai.KnowledgeChunk; Score float64 }`）以继承 serializer tag，否则静默拿到密文。
- **已接受的残留明文**：全文检索列 `tsvector`（`search`）、向量列 `embedding`、`title` 保持明文可检索语义，不得写入需加密的正文；业务 Out 直接返回明文字段，无需注册 `plaintextModels`（后者仅用于前端持有密文时的 RBAC 解密查看）。
- **checkpoint 负载加密**：`agent_checkpoints.payload` 为 eino 引擎现场的加密列（crypto 序列化器），与 `agent_runs` 的业务挂起元数据成对存在；新架构下不存在运行事件表。

### AI 回合生命周期与连接解耦（TurnLoop）

- **脱管上下文 + 禁止 gin 逃逸**：每个会话由 `turns.Manager`（`internal/ai/turns/manager.go`）持有一个 `adk.TurnLoop[*ChatItem, M]`；`New` 以 `context.WithoutCancel(ctx)` 派生进程级上下文，`genericSessionLoop.start` 在独立 goroutine 中 `loop.Run(detached)`。运行上下文只承载普通 `context.Context`，**禁止持有 `*gin.Context`**（handler 返回后不可再用）。浏览器断开（关抽屉 / 刷新 / 断网）只 detach SSE 订阅，回合继续执行；`turns.Manager` 经 `Deps.Agents()` 在使用时解析当前 `agentkit.Runtime`，跨配置热重载存活。
- **单会话单活跃回合 + 抢占**：`turns.Manager.Chat` 以会话 `turnMu` 串行化启动；新提问经 `loop.Push(item, adk.WithPreempt(adk.AfterToolCalls))` 在工具调用后的安全点抢占旧回合，旧流以 `turn.preempted` 结束、新回合重新 `turn.started`；禁止按 HTTP 连接生命周期取消回合。
- **领域事件内存缓冲 + 可重连 SSE**：`OnAgentEvents` 把 `TypedAgentEvent[M]` 投影为领域事件写入每会话有界内存缓冲（`internal/ai/turns/buffer.go`），SSE 只是订阅者，绝不与生成绑定。契约：`POST /agent/:sessionID/chat`（启动 / 抢占）、`GET /agent/:sessionID/render`（历史快照 + 当前缓冲后结束）、`POST /agent/:sessionID/action`（审批 / 问卷续跑）、`POST /agent/:sessionID/abort`、`GET /agent/:sessionID/events?after=`（运行中重连）。所有流均为领域事件的 SSE 行（`id:` 游标 + `data: <json>`），事件类型在 JSON 信封内，不使用 SSE `event:` 行。
- **刷新后恢复**：`turns.Manager.State` 返回 `Running` / `RunID` / `Interrupted` / `PendingInterruptIDs` / `UserTurns` / `LastErr`；`RunID` 经 `/status` 的 `liveRun.runID` 下发，前端刷新 / 重开抽屉 / 另一标签页打开运行中会话时据此订阅 `/events?after=` 续播（缺失会使页面永久停留在「生成中」）。`/render` 从 `agent_messages.payload`、`agent_interactions`、`agent_sessions.pending_interrupt_id` 重建历史消息与内容块，再叠加当前回合缓冲快照。缓冲只在内存中，进程重启后仅能恢复历史与 checkpoint 现场。
- **回合落库**：`OnAgentEvents` 在回合收敛时把结果持久化为 `agent_messages.payload`（eino 原生消息 JSON，`message_kind` 为 `chat` / `responses`）与 `agent_interactions`（审批 / 问卷 / 下一步），并推进消息序号；挂起时另写 `agent_sessions.pending_interrupt_id` 与 `agent_runs`（`checkpoint_id` / `slot` / `pending_calls`（含 `interrupt_id`）/ `citations` / `anchor_message_id` / `status`）。落库一律走 `context.WithoutCancel(ctx)`，禁止只在 done 时一次性追加、禁止把中断提示文案拼进正文。
- **checkpoint 是唯一引擎现场**：`agent_checkpoints` 以会话键 `session:<id>` 保存 TurnLoop/Agent 现场；挂起保留，正常完成 / 取消 / 失败删除。续跑用 `TurnLoop` 的 `Store` + `CheckpointID` 或 `Runner.ResumeWithParams`，**不回放** `agent_runs.messages`。
- **中止**：`turns.Manager.Abort` 调 `loop.Stop(adk.WithImmediate())`，等待循环收敛、以 `turn.completed(cancelled)` 结束缓冲，并作废挂起现场与 checkpoint；无在途回合时幂等。
- **运行阶段枚举**：`AgentRunPhase`（running/done/failed/cancelled/suspended）仍用于助手消息 `phase`，在 `internal/models/ai/enum.go` 唯一注册。
- **AI 表真外键 + 级联删除**：`agent_messages` / `agent_memories` / `agent_runs` / `agent_interactions` / `agent_session_compactions` 的 `session_id` / `namespace_name` 一律为真外键并显式 `ON DELETE CASCADE`，主键统一 `BIGSERIAL`；禁止以哨兵值做软引用。`agent_checkpoints` 按 `checkpoint_id` 独立寻址，由会话生命周期显式清理。
- **消息有序性以 `seq` 为准**：`agent_messages.seq` 为会话内单调序号（由 `agent_sessions.message_seq` 在单事务内原子分配），唯一索引 `uq_agent_messages_session_seq (session_id, user_id, seq)`；消息列表 / 历史窗口 / 滚动摘要游标一律按 `seq` 读写；`agent_interactions.seq` 同样会话内单调，定义交互的真实产生顺序。
- **消息读写三重限定**：消息的查询、更新与追加一律以 `session_id + namespace_name + user_id` 定位（写入时命名空间取自经校验的会话）；命名空间不符按会话 / 消息不存在处理，禁止只按 `session_id + user_id` 读写消息。
- **表结构唯一准据**：AI 表结构以 `internal/migrations/core/001_init.sql` 为唯一准据。
- **只列活跃会话**：`ListSessions` 仅返回 `status='active'` 的会话。

### `internal/ai` 分层与依赖规则

`internal/ai` 按职责拆分为单向依赖的子包，根包只承担运行时装配（`Runtime` / `Components` / 探活入口），不得再承载领域逻辑：

| 包 | 职责 | 允许依赖 |
| --- | --- | --- |
| `internal/ai` | 运行时装配（`Runtime` / `Components`）与探活入口 | 以下全部 |
| `internal/ai/contract` | 跨能力共享的线协议与值对象（无行为）及其枚举注册 | `internal/models`、`internal/pkg` |
| `internal/ai/port` | 跨包小接口（`Embedder` / `ChatResolver` / `ChatGenerator` / `ChatModelNamer`）与向量化用量助手 | eino、`contract`、`internal/models`、`internal/pkg` |
| `internal/ai/config` | 行为参数与工具元信息注册表 | `contract`、`internal/pkg` |
| `internal/ai/llmconfig` | LLM 运行配置读取与快照（模型/绑定/参数）及工具装配（含 MCP 拨号与降级） | `contract`、`config`、`llminfra`、`internal/ai/tools`（含 `mcp`）、`internal/models`、`internal/pkg` |
| `internal/ai/change` | 配置变更事件负载、etcd 广播与监听器构造 | `internal/pkg/etcd`、`internal/pkg/logging`、`internal/pkg/errors` |
| `internal/ai/llminfra` | LLM 基础设施（探活、embedding 维度校准与兼容、MCP 探活） | `contract`、`port`、`internal/pkg/llm`、`internal/pkg/database`、`internal/ai/tools/mcp` |
| `internal/ai/retrieval` | 检索结果类型与融合/去重/排序算法 | eino/schema、`internal/models`、`internal/pkg` |
| `internal/ai/audit` | 审核分级策略与批量评估器 | `contract`、`config`、`port`、`internal/models` |
| `internal/ai/interaction` | 交互状态派生与操作守卫 | `internal/models`、`internal/pkg` |
| `internal/ai/session` | 会话聚合持久化（会话/原生消息/交互/挂起元数据/压缩检查点/回滚分叉） | `contract`、`internal/models`、`internal/pkg` |
| `internal/ai/knowledge` | 知识索引、混合检索、整篇展开、dry-run 评估、分块概览、检索适配器 | `retrieval`、`contract`、`config`、`port`、`internal/ai/tools`（仅契约） |
| `internal/ai/memory` | 长期记忆存储/检索、摘要与压缩、主题与抽取 | `retrieval`、`contract`、`config`、`port`、`internal/ai/tools`（仅契约）、`internal/models` |
| `internal/ai/msgops` | 泛型消息构造/提取/规范化（`*schema.Message` ⇄ `*schema.AgenticMessage`） | eino/schema、`internal/pkg` |
| `agentkit` | ADK Agent / Runner、middleware、client 工具、checkpoint、压缩/指标/下一步/提示词优化 | eino、eino-ext、`contract`、`config`、`audit`、`retrieval`、`knowledge`、`capability`、`internal/ai/tools`、`internal/models`、`internal/pkg` |
| `internal/ai/assistant` | AI 助手 SSE 事件协议（信封 / 内容块 / 回合载荷与事件构造器） | `contract`、`retrieval`、`internal/models` |
| `internal/ai/turns` | TurnLoop 会话驱动、抢占/中止、事件缓冲、checkpoint 续跑 | `agentkit`、`assistant`、`msgops`、`knowledge`、`contract`、`config`、`session`、`internal/models`、`internal/pkg` |

硬性约束：

- 任何子包禁止 import 根包 `internal/ai`；`session` / `knowledge` / `memory` 禁止 import `agentkit` / `turns` / `assistant`。
- `agentkit` 只通过自身 `Deps` 声明的组合接口使用模型、记忆与工具能力，生产装配在根包完成；`turns` 只通过 `Deps` 取用运行时与会话存储。
- 同一份数据全模块只允许一个类型定义：`PendingCall`、`ToolResumeResult`、`BusinessData`、`MemoryHit`、`Interaction` 等共享值类型一律归 `contract`；领域事件与内容块载荷（`assistant.*`）只在 `assistant` 声明，不得在视图层另建平行协议结构。
- 新增能力时先判定归属包再落笔：协议/值类型进 `contract`，纯算法进 `retrieval` 或能力包，跨包接口进 `port`，禁止在根包、`agentkit` 或 `turns` 内就地新增领域实现。

### Dependency Injection

`samber/do/v2` 固定三层模式：私有 provider 工厂 → `InjectXxx` → `InvokeXxx`；`MustInvokeXxx`（panic 版）仅用于启动关键依赖。

```go
// 第一层：私有 provider 工厂
func newViperProvider() do.Provider[*viper.Viper] { ... }

// 第二层：注入函数
func InjectViper(i do.Injector) { do.Provide(i, newViperProvider()) }

// 第三层：带错误的调用
func InvokeViper(i do.Injector) (*viper.Viper, error) { ... }

// 第三层b：panic 版调用（仅用于启动关键依赖）
func MustInvokeViper(i do.Injector) *viper.Viper { ... }
```

所有 provider 位于 `internal/provider/`，统一在 `root.go` 的 `PersistentPreRunE` 注入。

### Configuration

- Viper 绑定用 `mapstructure` tag；默认值用 `default` tag（`github.com/creasty/defaults`）
- `comment` tag 必须用中文书写，作为 `describe config`（`DescribeConfig`）渲染的唯一事实来源；可选英文翻译加在 `internal/pkg/translate/locales/en.toml` 的 `config.<path>`（如 `[config.i18n.rootPath]`），未命中回退中文
- 类型安全提取：`config.ExtractConfig[T]()`；配置前缀常量 `Prefix = "capture"`（`internal/capture/config.go`）

### CLI (Cobra)

- 命令树字段即翻译键本身：所有命令的 `Short`/`Long`/`Example` 与 flag usage **必须**写成 i18n key（`cmd.<path>.short|long|example`、`flag.<path>.<name>`），禁止在代码中硬编码任何语言的字面量文案；`Use` 字段保持命令名/语法不变
- 翻译统一接入 go-i18n（`internal/pkg/translate`）：运行文案用 `cli.<topic>` 键；代码中一律调用 `translate.T(key, data)`（缺失时返回 key 本身），禁止自定义带 fallback 的翻译辅助函数（已移除 `cliT`）
- go-i18n 数据定界符约定：translate 层全局注入 `[[ / ]]` 定界符（`localize.go` 的 `dataDelimParser`），普通翻译消息的数据占位符统一写作 `[[.X]]`；`{{ }}` 仅用于 text/template 渲染的 notify.* 模板正文（其源串不含 `[[`，go-i18n fast-path 原样透出）。新增带数据占位的翻译文案必须用 `[[.X]]`，勿用 `{{.X}}`
- 回退语言固定为 `zh-Hans`：`locales/zh-Hans.toml` 必须覆盖全部 cmd.*/flag.*/cli.* 键；`locales/en.toml` 维护英文翻译，未命中时经 go-i18n 回退到 zh-Hans；两个文件都缺失时返回 key 本身
- 键完整性由 `internal/cmd/i18n_test.go` 的 `TestCommandTreeKeysComplete` 兜底：遍历命令树断言每个 Short/Long/Example/flag usage 在 zh-Hans 下可解析（解析结果不等于 key），新增命令/标志必须同步补翻译文件
- `prompt:"..."` 标签提示由 `internal/pkg/cliinput` 按 `cli.input.prompt.<StructName>.<FieldName>` 经 `internal/pkg/translate` 的 `T` 函数翻译，中文文案维护在 `locales/zh-Hans.toml`、英文在 `en.toml`；cliinput 内部固定键为 `cli.input.prompt.confirm-suffix` / `cli.input.prompt.comma-separated-suffix` / `cli.input.mismatch`（前缀常量 `CliI18nPrefix`）
- 语言切换：环境变量 `ZKI_LANG`（`en` / `zh-Hans`，默认 `zh-Hans`），统一由 `internal/pkg/base/environments.go` 的 `GlobalEnv.Lang`（`GlobalPrefix` 为 `zki` + `split_words:"true"` 映射为 `ZKI_LANG`）提供，CLI 经 `base.Env.Lang` 读取
- 翻译入口 `internal/cmd/i18n.go`：命令树本地化在 `Execute()` 的 `localizeCmdTree()`（将字段值作为 key 经 `translate.T` 就地替换）
- 覆盖 cobra 内置英文命令：`internal/cmd/help.go`（中文 help）、`internal/cmd/completion.go`（中文 completion），根命令 `init()` 中 `RootCmd.SetHelpCommand(helpCmd)` 注册
- `--help` 不触发 `PersistentPreRunE`，故语言切换必须放在 `Execute()` 入口的 `localizeCmdTree()` 中
- **长文案走外部文件引用**：locale TOML 中长文本（尤其 HTML 邮件正文）写作 `key = "@相对路径"`，由 `internal/pkg/translate/bundle.go` 的 `resolveExternalLocales` 在解析前就地展开为同目录文件内容；内置（`//go:embed locales`，须递归以含 `locales/emails/*.html`）与磁盘两来源统一经 `fs.FS` 处理，改模板只编辑外部文件、无需改 TOML
- **长耗时 LLM 任务的进度与汇总**：CLI 中会调用 LLM（尤其是 embedding）的长耗时任务，必须提供「运行期实时进度条 + 结束后明细汇总表」：进度条展示已完成/总数、百分比、处理速度与预计剩余时间；结束后以表格逐条展示条目、动作、耗时、输入 token、输出 token，并附合计行
- **表头与空任务**：任务开始时一次性打印目标命名空间、使用的模型名与待处理总数；待处理总数为 0 时不创建进度条，改为打印「内容已是最新，无需处理」
- **上报契约**：进度与用量经可选上报接口（`internal/ai/contract` 的 `SyncReporter`）注入；应用运行期（HTTP 视图层、worker）传 nil，保持零输出、零行为变化——进度输出只属于 CLI
- **用量降级**：embedding 供应商未返回 usage 时，token 列以 `-` 展示，绝不因此报错或中断
- **用量来源**：token 用量经上下文级 eino 回调（`callbacks.InitCallbacks`）采集，禁止改动进程级全局回调（`callbacks.InitCallbackHandlers` / `AppendGlobalHandlers`）
- **同步进度文案 i18n**：相关运行文案统一走 `translate.T`，键归属 `cli.sync.progress.*`，`zh-Hans.toml` 必备、`en.toml` 同步维护，数据占位统一 `[[.X]]`

### Protobuf

- `proto3`，文件在 `api/`；包名 `{service}.v1`；枚举值 UPPER_CASE（`CREATED` 等）；注释用 `///`；生成代码输出到 `internal/pkg/protobuf/`

### General Go Rules

- `context.Context` 始终是第一个参数
- 函数类型别名用于行为契约：`type LiveFunc = func(ctx context.Context, task *Task) error`
- 单方法接口定义在使用方（consumer side）
- goroutine 生命周期用 `errgroupx`（`internal/pkg/errgroupx`，errgroup 扩展，错误带 `goroutine#N` 溯源）
- 自定义序列化类型实现 `driver.Valuer` + `sql.Scanner`
- 能推断时不写显式类型参数：`etl.NewPipeline(extract, ...)` 而非 `etl.NewPipeline[any](extract, ...)`
- 配置结构体的默认值必须依赖 `github.com/creasty/defaults` 的 `default` tag，并在**反序列化前**调用 `defaults.Set(&cfg)` 填充，再由实际传入数据（如 JSON 解码）覆盖，缺省字段保持默认值；禁止用 if-else 手写默认值分支（如 `if port == 0 { port = 6379 }`）。该规则同样适用于不含 Viper 绑定、仅经 JSON 反序列化的配置结构体。

### View Layer In/Out Decoupling 视图层解耦强约束

- 强约束：视图层（`internal/manager/views`）的 In/Out 结构体**禁止直接引用模型层**（`internal/models` 下的 `core` / `network` / `tag`）的结构体 —— 不得通过结构体嵌入（struct embedding）引用，不得作为字段类型引用；即使两个结构体字段完全相同，也必须独立定义。
- 允许出现在 In/Out 结构体字段中的类型：基本类型、标准库类型（如 `time.Time`）、第三方类型（gopacket/layers、`gorm.io/datatypes.JSON` 等）、`internal/pkg` 共享类型（`serializer.*`、`utils.BaseTotalOut` 等）、视图层自定义类型（如 `TcpOptionOut` / `LayerInfoOut`）。
- 模型层专属类型（`network.LayerLevel` / `network.TCPOptions` / `network.LayerStack` 等）同样不得出现在 In/Out 结构体字段中，一律用视图层等价类型替换（如 `uint8`、`[]TcpOptionOut`、`[]LayerInfoOut`）。
- 数据流：处理器以模型查询数据库，再经显式映射函数（`newXxxOut`，入参为模型指针）逐字段转换为独立 Out；JSON 字段名（json tag）须与映射来源字段保持一致，避免破坏前端契约。
- **`internal/ai` 的 JSON 协议载体类型豁免**：上述禁令只约束**模型层**（`internal/models`）。`internal/ai` 中**声明了 json tag、且被设计为 JSON 协议载体**的结构体类型（线形态的唯一权威定义，同时充当 SSE 事件载荷、加密列 JSON 形状与 API 出参三处契约）**允许**直接作为 In/Out 字段类型——镜像它们只会产生必须与线载荷逐字节一致的平行结构体，与「单一事实来源」及「实时流与刷新渲染一致」两条约束相冲突，故显式豁免并登记先例：`retrieval.RetrievalResult`（`AgentMessageItemOut.citations`）、`contract.QuestionnaireQuestion` / `contract.QuestionnaireAnswer`（`AgentQuestionnaireOut.questions` / `.answers`）、`knowledge.DryRunResult`（`DryRunOut` 类型别名）。反之，`internal/ai` 中的**值类型别名**（如 `contract.AuditLevel` / `contract.ToolExecOwner`）与**基础类型别名**（如 `contract.BusinessData`）与模型层枚举同构、无协议载体语义，一律镜像为视图层等价类型（`string` / `map[string]any`），不得直通。
- 例外：**局部匿名扫描容器**（`var rows []struct { network.Xxx; utils.BaseTotalOut }`）仅作 GORM 扫描用途，不属于 In/Out 结构体，允许内嵌模型，沿用 notification 模块既有模式。

### 长期记忆（Long-Term Memory）

- **隔离维度固定为「命名空间 + 属主用户 + Agent 类型 + 来源会话」**：`agent_memories` 的记忆只在同属主、同来源会话内可见，跨越用户或会话互不检索。唯一索引 `uq_agent_memories_scope_hash` 与检索索引 `idx_agent_memories_namespace_kind` 均须包含 `user_id` 与 `session_id`；存储层的精确去重、语义合并与向量/关键词两路检索的 Where 及 `ON CONFLICT` 目标列一律含 `user_id`，禁止只按 namespace + agent_kind 过滤。
- **无会话运行不生成记忆**：无会话运行（`session_id` 为空）时 `MemoryStore.Write` 拒绝写入、`Retrieve` 直接返回空、Agent 不注入记忆运行时（`memoryToolContext` 原样返回 ctx）也不检索；`session_id` 已是真外键（`ON DELETE CASCADE`），数据库拒绝悬空引用（不再有 `session_id = 0` 哨兵），避免落进任何会话都检索不到的孤儿桶。
- **管理端不提供新增/编辑**：记忆由 Agent 运行期经 `memory_write` 工具生成，管理接口仅保留列表 / 详情 / 删除 / 启停，`AgentMemoryCreateIn` / `AgentMemoryUpdateIn` 及其路由、RBAC 权限点与前端入口一律不得存在。
- **检索去重复用知识库文本 n-gram（4-gram Jaccard、阈值 `memory.dedupeThreshold`）**；写入去重是 sha256 精确去重 + pgvector 余弦语义合并，两者语义不同，禁止混用。

### 上下文压缩（Context Compaction）

- **容量口径按档位区分**：token 容量随绑定模型走，取自模型注册表 `llm_models.context_tokens`（创建 / 编辑 **chat** 模型时由用户提供，必填正整数；embedding 模型不涉及、留 0），**不再是 agent 参数**；agent 经运行时快照按当前有效档位读取该容量（`Snapshot.SlotContextLimit`，绑定不可用 / 非对话档位为 0，即关闭占用展示与自动压缩）。触发比例仍为参数 `session.compactThresholdRatio`；会话占用取该会话**最近一条有用量记录的助手消息的真实 `PromptTokens`**（无助手消息为 0）——这是**自动压缩与压缩前度量的基线口径**（`ContextTokens`），展示口径另见下「压缩后占用」条；占用比例 = 占用 / **当前有效档位**容量，有效档位按既有解析规则（显式 `slot` → 会话 `target_model` → `agent.defaultSlot`）。
- **占用比例随档位变化**：切换档位即按新档位容量重算比例；前端容量取自 `AgentStatusOut.slots[].contextTokenLimit`，切换不额外请求。
- **自动触发条件**：`turns.Manager` 在 `GenInput` 持久化本轮用户消息**之前**按有效档位感知占用；容量 >0 且 `占用/容量 >= compactThresholdRatio` 时**强制推进滚动摘要**（折叠窗口外、尚未被摘要覆盖的全部消息，忽略 `memory.summaryMinEvictedMessages` 下限），并把 `summary_upto_seq` 推进到折叠上界；失败仅告警、绝不中断生成。
- **压缩呈现**：自动 / 手动压缩都以 `message.compress` 事件流式更新同一条助手消息的压缩块（进行中增量 → `压缩前 … · 已折叠 N 条`），压缩后占用由 `tokens_after` 估算即刻带回（见下「压缩后占用」条）；手动压缩端点 `/ai/session/:id/compact` 直接输出 SSE。静默滚动无实时块，仅在刷新 / 重进会话时随历史回显。
- **历史压缩记录随检查点持久化并回显**：压缩检查点（`agent_session_compactions`）除游标与摘要外，另落库展示度量 `tokens_before` / `folded_count` / `tokens_after`（由 `CompactionMetrics` 经 `CompressSession → CompactStream → UpdateSummaryAndCheckpoint` 传入；`tokens_after` 由 `CompactStream` 内部算得）；**三种推进原因（rolling / auto / manual）都必须落真实度量——取值与是否展示解耦**，其中 `tokens_after` 只对用户可见的压缩推进（auto / manual）落值。消息列表接口据 `upto_seq` 把记录锚定到「被折叠的最后一条消息」（`AgentMessageListGet` 按 `seq` 归并，**含静默滚动 rolling**），前端 `loadMessages` 映射为完成态压缩卡片（透传真实 `reason` 与 `tokens_after`）复用同一压缩卡片（`.zk-panel--subtle` + `.zk-chat-compress--failed` 失败态）渲染——刷新 / 重进会话后即见。静默滚动**无实时卡片**（`refreshSummary` 只落库不报事件），故仅在刷新 / 重进会话时随历史回显；自动 / 手动则实时与历史皆可见，三类共用同一卡片与文案。
- **压缩后占用（即时估算与展示口径）**：压缩本身不产生新的真实测量，故压缩完成即刻的占用由后端给出**即时估算** `tokens_after` 并随检查点落库：`estimatedCompactedTokens` 先按摘要模型用量算（压缩前占用 − 摘要 prompt tokens + 摘要 completion tokens），**用量缺失或其估值不构成缩减**时回落为**按字节折算**（新摘要与被折叠内容按 `buildMessageTranscript` 口径的字节差 ÷ 经验比 3 字节 / token），两种口径都不可用才留 0（前端回落为仅展示压缩前占用与折叠条数）。**展示占用**（`ContextUsage` / `ContextUsageBySession`，会话列表 / 详情出参的 `contextTokens`）取「末次真实测量」与「最近一条 `tokens_after > 0` 的压缩」中**更晚落库**者——压缩晚于末次测量（压缩后尚无生成落定）时即取该估算（压缩完成即刻下降、刷新后不回退），此后任一生成的真实 `PromptTokens` 随其助手消息行落定即自动接管；**压缩基线（`ContextTokens`）恒取真实测量**，绝不用上一次的估算度量下一次压缩（否则估算偏差逐次累积）。前端占用环直接消费会话列表项的 `contextTokens`（不自持压缩后估算的内存覆盖态），压缩块在该检查点有估算时展示「压缩前 X → 压缩后 Y（占用 Z%）」。
- **边界语义**：仅**助手回复**可作为压缩边界（用户消息不可作为压缩起点）；边界口径为「第 N 条用户消息（从 1 起，仅计 role=user、按 seq 升序）之后的**首条助手消息** seq」，折叠该轮及其更早的未摘要消息，更晚消息不受影响。
- **手动压缩端点**：`POST /ai/session/:id/compact` 为 **SSE 流式**（请求体可选 `assistantMessageID` 指定折叠边界，不传保持全量语义），正常以流 EOF 收敛；越界 / 助手消息不存在沿用既有本地化 404（`ErrMessageNotFound` 语义）；摘要以脱管上下文（`context.WithoutCancel`）完成并落库，**客户端断连不影响持久化**。
- **复用既有滚动摘要**：压缩即前缀折叠（`agent_sessions.summary` + `summary_upto_seq`）；摘要流式生成由 flash 档位模型 `Stream` 逐块产出。压缩**不删除**消息，只前移上下文起点；每次推进摘要（静默滚动或压缩）在同一事务内追加一条**压缩检查点**（表 `agent_session_compactions`，`summary` 经 crypto 加密、随会话与命名空间级联删除）。
- **回滚感知的 start bound**：`TruncateMessagesFromUserMessage` 在删除消息的同一事务内，若截断点落在已压缩区间（`summary_upto_seq ≥` 截断起始 seq），恢复「`upto_seq` < 截断起始 seq」的最近检查点的摘要与游标，并作废 `upto_seq ≥` 截断点的检查点；无可用检查点时清空摘要、`summary_upto_seq` 置 0；压缩期间抽取的长期记忆**不受**回滚影响。
- **会话删除为真实（硬）删除**：`DeleteSession` 经 `Unscoped` 硬删会话行，连同消息 / 交互 / 压缩检查点 / 长期记忆 / 挂起元数据 / checkpoint 一并整行删除（不再软删除）。

### RAG 检索（内置工具编排）

- **会话内检索改为内置工具**：不存在图内固定检索节点（旧五节点 `compose.Graph` 已随旧 agent 包删除），会话内的知识检索一律由内置工具 `knowledge_search` 按需发起——调用时机与查询语句由模型自行编排，可依上一轮结果改写查询多次调用。新增任何「模型按需取数」的能力都走同一范式（内置工具 + ADK 工具循环），不得回退为固定编排节点。
- **工具入参仅 `query`**：`knowledge_search` 的入参 schema 只暴露 `query`（必填）；命名空间（`turns.ChatInput` / `agentkit.Request.NamespaceName`）与文档限定（`Request.DocIDs`）由本次回合上下文经 `tools.WithKnowledgeRuntime` 注入到 `internal/ai` 侧适配器（`knowledge.Searcher`），模型入参**不得**携带或改变租户/文档范围——这是多租户安全边界，topK / 融合 / 权重等沿用 `retrieval.*` 参数，模型同样不可越权。
- **引用经采集器回传**：工具命中由运行期知识采集器（`internal/ai/knowledge/collector.go` 的 `Collector`，在 `turns` 构造回合上下文时创建、互斥锁保护）累积，回合收敛时由 `OnAgentEvents` 取 `binding.collector.Snapshot()` 写入运行结果（实时 `message.citations` 与 `agent_runs.citations` 同源）；禁止在工具 goroutine 内直接写共享状态。模型未调用工具时引用为空。
- **参数治理**：单条命中内容截断沿用 `agent.retrievedDocSnippetLimit`（语义为 `knowledge_search` 单条命中截断长度）；`agent.retrievalSectionTitle` 已删除，用户消息不再内嵌检索知识段（只由业务数据段 + 提问段拼成）。
- **父子索引（small-to-big，父块 = 整篇文档）**：`Retriever.Retrieve` 在重排得最终 TopK 后调用 `expandDocuments`——按 `document_id` 取回命中分块所属文档的整篇检索文本（`loadDocFullTexts`，按 `chunk_index` 升序拼接「标题路径 + 正文」），同文档仅保留首条命中并替换为整篇全文、标记 `RetrievalResult.Complete=true`，同文档其余命中删除。展开发生在重排之后：向量 / 关键词 / 重排都作用于语义聚焦的小分块，仅最终返回内容提升为整篇，无需改 schema 或通用分块器（表格的原子性保留）。
  - **整篇展开预算 `retrieval.expandDocLimit`**（rune 数，默认 26000）：按结果顺序**逐篇累计**展开，单篇超过该值或累计后超出该值的文档**退回分块**而非截断整篇；非正数关闭展开。默认值取「覆盖全部 199 篇生成文档（最大者 `http-transport/list` 约 25042 rune）」，与 `agent.maxToolResultBytes` 量级配套——若调低，须同步核验 `agent.maxToolResultBytes` 仍能容纳「整篇展开 + 其余分块命中」的工具结果 JSON，否则会在 `agentkit/middleware_tool.go` 处被二次截断成非法 JSON。
  - **`Complete` 表达「不再截断」**：为 true 时消费方一律原样返回、不做片段截断——`internal/ai/tools/knowledge.go` 的 `knowledge_search`（`hit.Complete` 时不走 `truncateSnippet`）、`internal/ai/knowledge/evaluate.go` 的 `evalSnippet` 均据此豁免 `SnippetLimit`。`Complete` 经 `schema.Document.MetaData` 与 `ToRetrievalResults` 往返，但**不参与线序列化**（`json:"-"`），前端契约不变。
  - **失败不中断**：取数失败按「增强逻辑失败绝不中断检索」处理——`r.warnf` 告警后返回原分块结果（与使用统计回写同策略）。
- **不得预设知识库内容范畴**：知识库由用户自行上传维护，内容随命名空间而异，任何一侧都不得假定它是「平台知识」或某一领域资料。工具描述（`knowledgeSearchTool.Info`）、参数说明与系统提示（`agent.systemPrompt`）一律用中性措辞——如「检索用户维护的知识库中的文档片段」「当回答需要引用知识库中的既有资料时调用」；描述检索能力时须写明「语义与关键词混合检索」，不得简化为「语义相似度检索」。系统提示 `agent.systemPrompt` 需引导模型按需调用 `knowledge_search`，并按命中条目的 `citation` 编号在正文相应位置标注 `[n]`。
- **三层链路与出厂默认**：内置工具元信息（`internal/ai/config` 的 `ToolSpecs`）、出厂行（`002_fixtures.sql` 的 `llm_tools` 段）、实现注册表（`internal/ai/tools` 的 `Register`）三处必须同步，由 `internal/ai/config/tools_fixture_test.go` 双向守卫；`knowledge_search` 是只读基础能力，出厂 `enabled=true`，管理员仍可在「工具」页停用，停用后不再进入模型可用工具清单。
- **通用内置工具按能力类别分文件、纯算法下沉 pkg**：数学 / 时间 / 文本 / 编码 / 数据等通用计算能力，其**纯算法按类别分文件实现于 `internal/pkg/utils`**（`math.go` / `equation.go` / `datetime.go` / `text.go` / `encoding.go` / `data.go`，只含算法与结果结构体，不 import Eino），`internal/ai/tools` 只留**同名的 6 个薄适配器**（仅 `args` / schema / `Info` / `InvokableRun` / `init()` 的 `Register`：解析 JSON 入参、按需补缺省、调 utils 导出函数、包工具名前缀错误、序列化结果），每个文件在 `init()` 登记本类别工具。如此分层的理由是让与业务无关的纯计算能力可被其它模块直接复用，并把依赖方向固定为 `internal/ai → internal/pkg` 以减少导入环——故 `internal/pkg/utils` 不得反向依赖 `internal/ai`，也不得 import Eino（`github.com/cloudwego/eino/...`）。粒度遵循「一能力一工具」（工具名 `snake_case`、全局唯一），不按类别聚合为单个带 `operation` 的多能力工具。这些工具均为无状态、纯计算、无副作用、无租户边界的只读能力，故与 `knowledge_search` 同策略出厂 `enabled=true`（管理员仍可在「工具」页停用），同样受上述内置工具三处同步守卫约束。依赖取舍：表达式求值复用既有依赖 `github.com/casbin/govaluate`，UUID 用 `github.com/google/uuid`，其余用标准库，`datetime_calc` 内嵌 `time/tzdata` 兜底 IANA 时区。
- **JS 运行时是独立组件、脚本按注册注入**：`internal/pkg/jsengine` 提供进程内唯一的 goja 运行时（`Engine`：`Start` 预热、`Do` 互斥访问、`Shutdown` 释放），装配与生命周期由 `internal/provider/jsengine.go` 统一管理（provider 构造时 `Start`，服务启动路径经 AI 运行时解析即完成预热，关闭阶段由 samber/do 调用其 `Shutdown`；不进入 AI 的命令行子命令不承担加载开销）。业务包在自己的 `init` 里通过 `Register`（需要浏览器桩或 Go 绑定的场景）或 `RegisterScript`（纯嵌入式源码）登记注入项，引擎启动时按登记顺序执行一次；业务语义（浏览器桩、mermaid bundle 等）不得写进引擎本身。注册项名称唯一，空名与重名在登记期 panic（fail fast）；引擎启动后新增的注册项不会回灌已启动的实例，故注册一律放在 `init`。
- **图表校验走官方解析器（jsengine + 官方 bundle）**：`mermaid_check` 不自行维护语法规则，其校验能力在 `internal/pkg/mermaid`——把官方 `assets/mermaid.min.js` 注册进上述共享 JS 引擎并调用 `mermaid.parse`，单二进制内完成、不依赖 Node/Chromium；`internal/ai/tools/mermaid.go` 经 `tools.Deps.JS` 取引擎、只做适配，`llmconfig.Store` 以 `SetJSEngine` 承接，装配层用 `ai.WithJSEngine` 注入。该 bundle **必须与 `web/package.json` 的 `mermaid` 版本同源**（当前 12.1.0，MIT），升级前端 mermaid 时同步替换，否则校验口径与前端渲染分叉。goja VM 非并发安全，单引擎内串行（单次 parse 约 50ms）；解析前把 ```mermaid 围栏与标签内 HTML 标签做等长空白替换以绕开「解析期需要真实 DOM」的限制，行号/列号不漂移；官方报错行号在 mermaid 12.1.0 下比源码行号大 1，`Check` 已校正。
- **dry-run 不走工具**：检索试运行（`Retriever.DryRun` 与视图层 dry-run 端点）保持「确定性检索 → 生成 → RAGAS 评估」链路不变，仍用于评估 RAG 本身，不经 LLM 工具编排。
- **引用须携带 `namespaceName` 以支持溯源**：`RetrievalResult` 新增 `namespaceName` 字段（json `namespaceName`），自检索 SQL → Eino `MetaData` → SSE `done` → 消息落库（`CitationsJSON`）全线贯通，任一环节不得丢弃；前端据此判断来源命名空间并把引用标题渲染为指向独立知识文档详情页的超链接。
- **正文引用标注按来源编号对齐**：`knowledgeCollector` 对命中按（命名空间 + 文档 ID + 分块序号）去重，并分配本次运行内稳定的 1 起编号，经 `KnowledgeHit.Citation` → `knowledge_search` 结果的 `citation` 字段提供给模型；系统提示要求模型在正文中用 `[n]` 标注来源。前端 `AssistantMarkdown` 把 `[n]` 改写为指向第 n 条引用文档详情页的上标链接（跳过代码块与行内代码，编号越界时移除该标注、缺失命名空间时保持原文），来源列表同号展示，二者据此一一对应。编号必须由采集器分配且在后续检索追加时保持不变，不得由模型自行编号。引用编号按轮次隔离：新提问从 1 重新编号，历史正文与摘要中的旧 `[n]` 在送入模型前剥离（代码段除外），禁止跨轮复用编号。

### 内置技能（Builtin Skills）

- **`agent_skills` 注册行 + 代码正文、跨命名空间**：内置技能是 `agent_skills` 表中 `source='builtin'`、固定落在 `base.SystemNamespace` 的注册行——`name` / 明文 `description` / `enabled` 由 `002_fixtures.sql` 注入，`content` 为 NULL，技能正文由 `internal/ai/skill/builtin.go` 的 `BuiltinSkills` 代码注册表按 `name` 提供；与内置工具「`llm_tools` 注册行 + 代码实现」的组织方式同构。`source` 取值 `custom | builtin`（枚举类 `agentSkillSource`）。当前代码注册表登记了「周期报告技能共创」技能 `periodic_report_cocreate`（正文见 `internal/ai/skill/builtin_periodic_report_cocreate.go`，只覆盖日报/周报/月报类报告技能，涵盖周期与时间窗口、数据取数契约、指标口径、满分 100 的评分标准与最终创建指引；不承担通用技能共创）与「风险标签 Rego 规则共创」技能 `risk_tag_rego_rule`（正文见 `internal/ai/skill/builtin_risk_tag_rego_rule.go`，覆盖风险场景澄清、模型字段与数据能力核对、Rego 契约、validate 与 dry-run 验证、满分 100 的评分标准与审批后创建；不承担标签事件规则或通用 Rego 咨询）；新增内置技能必须同时追加一条 `BuiltinSkill` 与一条 fixture 注册行。`agent_skills.description` 为明文列，`content` 仍为加密列（内置行为 NULL）。
- **统一技能选择 `skillID`**：内置与自定义技能都落在 `agent_skills` 并有行 ID，聊天请求与选择器一律用 `skillID` 表达一次选择，不再使用任何 `builtin:<name>` / `custom:<id>` 引用前缀。`AgentChatIn.skillID` 非 0 时技能须存在且启用（自定义技能还须属于当前命名空间），内置正文按行的 `name` 取自代码注册表。
- **技能按轮次锚定、不绑定会话**：技能归属「一轮用户提问」而非 `agent_sessions`——同一会话的后续轮次可换用其他技能，也可不选技能。`turns` 在追加用户消息时把本轮 `SkillID` 与 `SkillName` 快照写入该行 `agent_messages.skill_id/skill_name`（仅用户消息有值），`turn.started` 的 `userMessage`、`/render` 的 `snapshot.message` 与消息列表出参一并回传，前端用户气泡据同一字段展示本轮技能（即时气泡与刷新后的历史气泡同源）。`skill_id` 可空且带外键 `REFERENCES agent_skills(id) ON DELETE SET NULL`——未选技能以 NULL 表达（模型层用 `*uint`，`ai.SkillIDColumn` / `ai.SkillIDFromColumn` 在 0 与 NULL 间转换），技能常规删除为软删除故历史引用保持；`skill_name` 是写入时的名称快照而非实时关联，技能后续重命名 / 停用 / 删除不影响历史展示；会话内不得再引入任何「整会话唯一技能」字段，也不得以 0 充当「无技能」外键值。
- **技能写入能力与参数完整性**：AI 侧开放 `ai.skill.create`（写入）与 `ai.skill.detail`（只读），不开放 `ai.skill.update`——创建必须一次成功，不允许用更新接口补救半截正文。`periodicCapable`（是否可周期执行）、`inputFields`（输入字段）与 `fixedFields`（固定字段 / 固定参数）均由用户或调用方直接填写，平台不做模型评审：创建与更新接口整体覆盖这三个字段，并对周期技能强制要求「必须且只能有一个 `name=nowTime`、`type=date` 的输入字段」，同时要求固定参数包含 `timeOffset` 与 `timeWindow`。`api_invoke` 参数统一要求顶层 `complete: true`（与 `capability` / `params` / `pathParams` 同级），由模型在全部字段完整输出后置位；`prepareApiInvokeCall` 先 `json.Unmarshal` 原入参（截断参数会直接解析失败），再在能力 / 路径 / params 校验通过后检查该标志，缺失或为 false 时返回可回灌的错误文本，模型须重新发起同一能力的完整调用，平台不会挂起或执行半截参数。JSON 对象键序无语义，不得用正则假设 `complete` 位于参数末尾。不要假设 client 工具参数有固定字符上限：eino 会在内存中拼接完整流式参数后才进入工具；若模型提前结束，同一流中不存在后续 token，只能靠完整性标志识别并重试，不能靠「再组装一次」恢复。
- **管理语义对齐内置工具**：技能列表同时返回当前命名空间的自定义技能与全局内置技能，内置行以 `source='builtin'` 直接表达只读语义（不再下发独立 readonly 字段）；内置技能可启停（`/:id/status`），但编辑（`/:id/update`）与批量删除（`/delete`）对内置行一律拒绝（删除为整批拒绝），前端不提供内置行的编辑/删除入口。前端来源列经 `enumLabelOf(agentSkillSource)` 渲染并以锁形图标标识只读。
- **单点装配与守卫**：内置技能以注册行（名称 / 明文描述 / 启停）为准、正文取自代码注册表，二者合入既有 `turns.ChatInput` 技能文本字段，复用「当前技能」提示词段，不改 `turns` / `agentkit` 契约；`validateBuiltinSkills` 在 `init` 期 fail-fast，`internal/ai/skill/builtin_test.go` 守卫代码注册表，`builtin_fixture_test.go` 双向守卫「代码注册表名 ↔ 系统命名空间 fixture 行名」，防止只加一侧。
- **选择器合并语义**：`POST /api/v1/ai/skill/options` 先返回 `source='builtin'` 且已启用的注册行（`id` 升序，即 fixture 注入顺序），再返回当前命名空间已启用的自定义技能（`created_at` 倒序），不返回正文；停用的内置技能既不可选也不注入。

### 注释生成文档入 RAG（generated 分区）

- **仅由命令行全量扫库更新、禁止人工维护**：叙述页由代码注释派生（`internal/docs` 的 `Registry()` 自动生成条目，经 `internal/docs.GeneratedNarrativeDocuments` 渲染），禁止人工维护第二份说明；运行期不再隐式写库，仅由 `zki sync docs` 走常规工具箱（DB + embedding）全量扫库同步，只入 RAG 不落盘（与「自省注入的知识库不可被随意修改」的既有约定一致）。
- **固定写内置系统命名空间**：叙述页与原生接口目录同属代码声明的公开共享数据，`zki sync docs` 与 `zki sync apis` 一致，固定同步至 `base.SystemNamespace`（无 `--namespace` 参数），经 `base.NamespaceScope` 对所有命名空间的检索可见。
- **分区隔离**：叙述页落在 `format='generated'`（`docconv.FormatGenerated`）的专用知识域，与人工业务文档分区隔离；同步（`SyncGeneratedDocs`，接收调用方传入的文档集与目标 format）只读写该分区，不得影响人工文档。
- **幂等同步**：按新增 / 更新 / 删除差异计划幂等执行——正文未变的条目零写入（不重复向量化），条目下线即清理，连续两次执行且源码未变时第二次不产生任何写入。
- **检索范围**：`knowledge_search` 不限定 format，可命中叙述页与人工文档。
- **投影边界（参考页不入 RAG）**：进 RAG 的是**单篇**叙述页；`docs/generated/reference/**`（逐包参考档与参考索引）与 `docs/index.md`、`docs/nav.json` 是随 `zki create docs` 生成、带 `DO NOT EDIT` 的人读产物，**不入 RAG**，避免同一份说明以整页粒度重复索引。

### 原生接口知识目录（注册期声明 + RAG 同步）

- **注册期单一声明点**：全部原生接口的 OPTIONS 元信息必须经 [catalog.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/manager/views/catalog.go) 的 `RegisterApi` / `RegisterListApi[M,N]` 声明；禁止在 helper 之外直接调用 `StructMetaGinHandler` / `ListMetaGinHandler`（守卫测试扫描 `internal/manager/views/**` 非测试源码拦截裸用）。
- **fail-fast**：helper 的 `description`（单行中文用途，取自该接口处理器注释）去空白后为空即 panic；声明缺失用途的接口不允许进入 RAG。
- **声明内容**：helper 在向 gin 注册 OPTIONS 元信息路由的同时，把「完整路径（`group.BasePath()` + 相对路径）、各方法入参 / 出参元信息、单行用途、RAG 开关」记入进程内注册表，键为 `(HTTP 方法, 完整路径)`，重复登记以最后一次为准，导出按（路径, 方法）确定性排序。
- **RAG 开关**：`WithRagEnabled(bool)` 逐接口控制，缺省纳入；声明为不纳入的接口从同步文档集排除，下次同步时其 `native_api` 分区条目被幂等删除。
- **离线装配**：[catalog.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/manager/catalog.go) 的 `CollectApiCatalog()` / `CollectApiKnowledgeDocs()` 以空操作中间件构造最小 `views.RegisterContext`，在无 DB / Redis / Etcd 环境下取得与生产一致的目录；在线装配（`RouteRegister`）与离线装配必须共用同一份域注册清单 `registerViewRoutes(rc)`，新增 / 移除视图域只改该函数，避免两份清单漂移。
- **RAG 分区**：接口知识落在专用分区 `docconv.FormatNativeApi`（值 `native_api`，已登记进 `documentFormat` 枚举并具备中英 label，见 [docconv.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/pkg/docconv/docconv.go)），与叙述页分区 `generated` 及人工文档严格隔离；同步只读写本分区。
- **子命令**：`zki sync apis`（[sync.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/cmd/sync.go)）把注册期声明的接口目录渲染为 Markdown 知识文档后，复用 `Knowledge.SyncGeneratedDocs` 幂等同步到内置系统命名空间（`base.SystemNamespace`）的 `native_api` 分区——接口目录是代码声明的公开共享数据，故**不需要** `--namespace`；AI 运行时未启用时以本地化错误终止，不触碰数据；正文未变零写入，接口下线或退出 RAG 即清理。
- **文档渲染形态（检索可读性约束）**：`renderApiKnowledgeDoc` 段落顺序固定为「用途 / 请求 / 请求示例 / 请求头入参 / 入参 / 响应头出参 / 出参 / 响应示例」，四类结构体节各自自包含（同一节内某类型只定义一次，跨节各自重复，便于分块检索时无需依赖整篇展开即可读到本节的类型定义）。每节渲染为「顶层字段表 + 具名嵌套类型子节」：子节标题为 `### <字段路径>（<类型全名>）`，字段路径对切片/数组追加 `[]`、map 追加 `{}`（如 `options[]`），递归展开深度上限 `nestedSectionMaxDepth`；`time.Time` 等标量结构体不展开，类型名集合防环。枚举列内联该枚举类全部取值（`<类名>：<值1>、<值2>`，未注册时退化为仅类名）；`Enum` / `EnumType` 与 `default` 均取自既有标签，不新增标签。
- **示例生成必须确定性**：[catalog_example.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/manager/views/catalog_example.go) 的 `BuildJSONExample` 逐字节幂等——示例正文参与文档内容摘要（HMAC-SHA256）比对，用 map 组装对象（遍历顺序随机）会导致每次同步都判定「内容已变更」而全量重嵌入。一律按结构体字段声明顺序手工拼装；取值只用类型化占位，不臆造业务样例——枚举字段取枚举类首个 value、有 `default` 标签取该默认值（注意 `utils.ParseScalar` 整数返回 `int`、浮点返回 `float64`，两者都要处理）、字符串 `"string"`、数值 `0`、布尔 `false`、`time.Time` 固定字面量、对象递归展开、数组取单元素示例；深度上限 `jsonExampleMaxDepth` + 类型名集合防环。
- **预算分页（分块可读性约束）**：字段表与示例代码块按预算切分，使单个分块即可读到完整内容、无需依赖整篇展开——`writeFieldTable` 逐行累积，超 `fieldTableMaxRunes`（480 rune，含表头预算）即另起一张表（重复表头，保证每张表自包含）；`writeExampleSection` 经 `splitJSONExampleBlocks` 在顶层字段边界分组，每组收口为独立合法 JSON 对象且 ≤ `exampleBlockMaxRunes`（480 rune）。两处常量与切分逻辑须保持确定性（同一输入逐字节幂等）。已知不足：单个顶层 key 包裹巨型嵌套数组（如 `{"items":[...]}`）时无法在示例层继续拆分，此类示例依赖整篇展开交付。
- **接口详情走 RAG**：接口目录经 `zki sync apis` 进入 `native_api` 分区后，模型查询接口用途与字段统一走 `knowledge_search` 的 `format=native_api`；不再维护第二份接口快照或专用描述工具。

### AI 能力地图（capability）

- **唯一声明点**：能力在视图层随接口声明，经 `views.WithCapability(...)` 写入接口目录；`views.CapabilityItems()` 从同一份目录派生能力快照。新增能力只改对应视图域的 `RegisterApi` / `RegisterListApi` 调用，禁止在提示词、前端或其他包另维护第二份清单。
- **引用天然同源**：能力的 method/path 直接取自接口声明，不再手写；`internal/manager/catalog_capability_test.go` 强制校验 ID 唯一、字段齐备、访问性质合法，以及与接口目录一致。
- **只读优先、写入须审批**：能力地图以 `AccessRead` 为主；敏感、诊断或仅用于编辑器辅助的只读接口标为 `Invokable=false`（可被检索知晓但 `api_invoke` 拒绝）。写入能力只有在「客户端工具挂起 + 用户审批卡片」链路就绪后，才可标为 `AccessWrite` 且 `Invokable=true`；当前 `ai.skill.create`（技能创建）与 `tag.rule.create`（标签规则创建）按此登记，`tag.rule.validate` / `tag.rule.dry-run` / `tag.rule.model.metadata` / `tag.rule.functions` / `tag.rule.sqlite.meta` 因「风险标签 Rego 规则共创」需要闭环验证而开放为可调用只读能力，新增写能力或开放编辑器辅助接口须逐个评审。
- **按需检索**：能力地图与每条能力明细经 `zki sync capabilities` 写入 `capability` 分区；模型判断平台能力时调用 `knowledge_search` 并传 `format=capability`。整张地图不再作为 system message 常驻，系统提示只附加一句固定的能力发现指引（`agentCapabilityHint`）。
- **运行期白名单**：`manager.RouteRegister` 在 `registerViewRoutes` 之后经 `Runtime.SetCapabilities` 注入能力快照，仅供 `api_invoke` 做可调用白名单校验（`Invokable` 单点决定，只读与写入能力均按此判定），不进入模型上下文。
- **人机同源**：`zki create docs` 经 `manager.CollectCapabilities()` 生成 `docs/generated/reference/capability-map.md`，内容与 `capability` 分区的整体地图同源，禁止另写一份能力说明。
- **能力知识分区**：`capability.KnowledgeDocs(manager.CollectCapabilities())` 生成「一篇整体地图 + 每条能力一篇明细」，`zki sync capabilities` 只写 `docconv.FormatCapability`（值 `capability`）分区；标题即稳定去重键，不得人工维护第二份能力说明。
- **接口详情查询复用 RAG**：不另建接口快照或专用工具；接口知识已由 `zki sync apis` 进入 `native_api` 分区，模型需要用途、入参或出参字段时，用 `knowledge_search` 并传 `format=native_api`。`format` 仅允许 `generated` / `native_api` / `schema` / `capability` 四类生成分区，空值表示全部分区；检索在向量路与关键词路 SQL 中统一按 `knowledge_documents.format` 过滤。
- **api_invoke（client，读写）**：模型只能提供能力 ID、`pathParams` 与 `params`；后端在挂起前完成白名单解析（能力必须 `Invokable`（`AccessRead` 或 `AccessWrite`）标记为可调用、路径参数受限字符集、请求体 ≤64 KiB 且必须是 JSON 对象），并把 `method/path/params` 规范入参写入挂起现场；写入能力会真实改变数据，调用文案要求模型仅在用户明确确认后发起，且所有 client 工具（含 api_invoke）一律走人工审批卡片。列表查询的 `filterBy.timestampFrom/To` 在缺省时镜像为 `createdAtFrom/To`（`created_at` 为分区键，缺失会被接口 400 拒绝并退化为全分区扫描）。能力快照本身就是接口白名单（与接口目录同源），其端点存在性由 `internal/manager/catalog_capability_test.go` 守卫，运行期不再另查接口目录。前端只执行该规范入参，不得信任模型给的任意方法或 URL。

### 内置系统命名空间（system，代码声明的公开共享数据）

- **定位与单一常量**：`base.SystemNamespace = "system"`（[consts.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/pkg/base/consts.go)）承载**由代码声明、对所有租户公开共享**的数据，与 `base.DefaultNamespace` 并列为内置命名空间。占用者为原生接口目录（`native_api`）、数据表（`schema`）、平台能力（`capability`）与注释叙述页（`generated`）。禁止在任何位置写 `"system"` 字面量。
- **只存一份，禁止复制**：共享数据**不得**按命名空间各复制一份（那会重复索引、重复消耗 embedding token）；一律单份落在 system 作用域，经检索并入生效。
- **真实行 + 种子化**：`knowledge_documents` / `knowledge_chunks` 的 `namespace_name` 为 `NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE`，故 system 必须有真实命名空间行；该行作为结构性内置行由 `001_init.sql` 紧随 `namespaces` 建表后种子化（值须与 `base.SystemNamespace` 一致，不新增独立迁移文件），其 webhook 占位通知渠道由 `002_fixtures.sql` 第 7 段「为每个已存在命名空间补齐占位渠道」自动覆盖（维持「每个命名空间必有占位渠道」不变式）。
- **检索并入**：检索在调用方限定命名空间之外自动并入 system 作用域，由公用函数 [base.NamespaceScope](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/pkg/base/namespace.go) 统一解析（空命名空间不追加，保持「空命名空间不命中」语义；system 自身不重复）；凡是按命名空间取数的跨租户操作都应复用它，向量路与关键词路两处 SQL 必须一并使用。`WithDocIDs` 的文档限定保持不变，显式限定文档时不穿透 system。
- **对前端完全隐藏**：`system` 不出现在命名空间列表（[admin_namespace.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/manager/views/admin/admin_namespace.go) 的 `NamespaceListGet` 过滤 `name <> system`）与用户可访问命名空间（[user.go](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/manager/views/user/user.go) 的 `visibleNamespaces` 过滤），前端无需改动。
- **只读开放为例外**：上述「对前端完全隐藏」的**例外**——系统命名空间以**只读**方式开放给知识库页的「系统知识库（只读）」视图：后端新增专用只读接口组 `/api/v1/ai/doc/system/*`（`list` / `:id/get` / `:id/chunks`），恒读 `base.SystemNamespace`、不提供任何写方法（权限点登记于 [002_fixtures.sql](file:///Users/olin/Documents/workspace/golang/zhongkui/internal/migrations/core/002_fixtures.sql)）；仍**不进命名空间列表、不可写、不可删除**。
- **系统视图整体只读**：知识库管理页在「当前命名空间」与「系统知识库（只读）」视图间切换；系统视图下一律隐藏写操作与批量选择，仅保留查看详情 / 分块。
- **前端禁止硬编码 `"system"`**：任意位置一律不得写 `"system"` 字面量；知识文档详情路由页 `web/src/pages/[language]/ai/document/[id].vue`（新标签打开、只读）须据 `?namespace=` 与当前活动命名空间是否一致，选用「常规读接口」或「系统只读接口」。
- **禁止删除**：`NamespaceBatchDelete` 对 system 直接拒绝（业务码 `1405` / `ErrSystemNamespaceProtected`），避免经外键级联清空共享文档；不修改 `core.Cache` 本身（`NamespaceExist` 等仍须能识别该真实命名空间）。

### 向量与模型对齐（sync embeddings / sync embedding-dimensions）

- **命令分组**：两条命令同属 `zki sync` 组、互为并列：`zki sync embedding-dimensions --dimensions <n>` 只改 schema（列维度），`zki sync embeddings --namespace <ns>` 只改内容（重嵌入向量）。`embedding-dimensions` 不是 goose 迁移、也不产迁移文件，故不归 `zki migrate`。
- **分工**：列维度重建（`ALTER TYPE` + 清空 + 重建 HNSW）属 `zki sync embedding-dimensions`（须先解绑 embedding 档位）；内容重嵌入属 `zki sync embeddings --namespace <ns>`（以当前 embedding 模型重算知识库文档分块向量 `Knowledge.ReindexNamespace` 与全部 active 长期记忆向量 `MemoryStore.ReindexEmbeddings`）。两者互不替代。
- **范围与边界**：`sync embeddings` 按命名空间隔离，只重算 `knowledge_chunks` 的向量与 `agent_memories.embedding`，不改文档/记忆正文，也不重写 `search`（tsvector）列；`--namespace` 必填。内置系统命名空间的共享文档（如原生接口目录）不随租户命名空间重嵌入，需显式执行 `zki sync embeddings --namespace system` 对齐；`zki sync apis` 在正文变更时会自行重新向量化。
- **破坏性提醒**：`sync embedding-dimensions` 的 DDL 会 `DELETE` 整张 `knowledge_chunks` 与 `agent_memories`（不只是清向量），故维度变更后长期记忆已无内容可重嵌入（只能重新积累），知识库文档由 `knowledge_documents` 原文经 `sync embeddings` 重建；「同维度换模型」时迁移不触发清表，此时长期记忆与知识库文档都需要 `sync embeddings` 对齐。
- **索引维度由运维独占**：前端不提交 `dimensions`（Create / Update / Test 入参均无该字段），`llm_models.dimensions` 仅为绑定校验时探测写入的只读缓存；绑定或修改已绑 embedding 档位时经 `verifyEmbeddingCompat`（`ProbeEmbeddingDimensions` 两步探测：先原生维度、再按列维度定向）校验其与 `knowledge_chunks.embedding` 列维度一致，不匹配返回 400 并指引先执行 `zki sync embedding-dimensions`；目标维度仅由该命令 `--dimensions` 显式指定，不做隐式推断。

### 客户端工具与人工审批（client 工具挂起 / 恢复）

- **禁止服务凭据直连**：需要以用户自身身份执行的调用（如平台原生接口）禁止由后端以服务凭据直连，一律走「后端下发工具调用请求 → 前端以用户自身凭据执行 → 结果回传续跑」。
- **client 来源工具**：`internal/ai/contract` 的 `ClientToolInfos` 注册表只登记 schema、不构建本地执行器，`contract.IsClientTool` 判定工具是否属该来源；`ask_user` 与 `api_invoke` 由 `agentkit` 直接实现为 eino 工具。命名空间由运行上下文注入，模型入参不得参与租户选择。
- **命中即中断（不本地执行）**：`ask_user` / `api_invoke` 实现为 `tool.InvokableTool`（`agentkit/client_tools.go`、`api_invoke.go`），首次调用 `tool.StatefulInterrupt` 挂起；`turns` 的 `OnAgentEvents` 落 `agent_runs`（`checkpoint_id` / `pending_calls`（含 `interrupt_id`）/ `citations` / `anchor_message_id`）与 `agent_interactions`（`approval` / `questionnaire`），并在事件流中以对应内容块与 `interaction` 事件结束本轮。禁止由手工 `toolTurn` 攒 pending。
- **恢复原子认领**：`/agent/:sessionID/action` 经 `turns.Manager.Action` 原子认领挂起现场（`ClaimAgentRun`，重复提交 / 归属不符有专用哨兵并映射业务码 1300~1301），把动作映射为 `ResumeParams.Targets`，工具经 `tool.GetResumeContext` 取回决策 / 答案后继续；工具结果按 `agent.maxToolResultBytes` 截断。挂起现场不设过期，任意时间可恢复。
- **续跑回填原交互块**：审批 / 问卷载荷存于 `agent_interactions.payload`，提交后按 `status=answered` 回填决策与答案并随历史重放（`RenderAssistant` 的 `buildAssistantSnapshots`）；领域事件是前端展示的唯一来源，不得以阶段事件代替块更新。
- **单点声明**：状态取值（`pending/consumed`，`internal/models/ai` 的 `AgentRunStatus`）与失败类别（`internal/ai/contract` 的 `ErrAgentRun*` 哨兵，`internal/ai/session` 保留同值别名）必须单点声明，禁止就地字面量。
- **新增 / 修改工具三处同步**：必须同步 `internal/ai/config` 的 `ToolSpecs` 元信息注册表、实现（builtin 为 `internal/ai/tools`，client 为 `internal/ai/contract` 的 schema 注册表）与 `002_fixtures.sql` 出厂行，由守卫测试 `internal/ai/config/tools_fixture_test.go` 双向守卫。
- **前端审批契约**：前端收到待决策的审批块（`kind=approval`、`awaitingApproval=true`）必须进入「等待审批」态而非已完成 / 失败；审批面板须展示工具名与调用参数；批准后以用户自身凭据执行，拒绝时不得发起任何调用；同一轮多个调用必须一次性整体提交（运行现场一次性消费）；动作请求前返回的业务错误（无挂起现场 / checkpoint 缺失 / 无权恢复）须以后端本地化错误提示用户。
- **两轴合成（审核重要级 × 审核阀门）**：`approvalRequired` 由**四档审核重要级** `auditLevel`（`must` 必须审核 / `suggest` 建议审核 / `auto` 可自动审核 / `none` 无需审核，由后端评估产出）与**三档会话级审核阀门** `reviewValve`（`each` 逐一审核 / `auto` 自动审核 / `none` 无需审核，持久化于 `agent_sessions.review_valve`）合成得出。
- **判定矩阵（后端单点）**：`approvalRequired = level==must || valve==each || (valve==auto && level∉{auto,none})`；「必须审核」为绝对下界（任何阀门下均需人工），「逐一审核」阀门下任何重要级均需人工，「自动审核」仅 `auto` / `none` 可自动，「无需审核」仅 `must` 需人工。规则唯一实现在 `internal/ai/audit` 的 `NeedsHumanReview`，前端只消费下发的 `approvalRequired`，不得重复实现该规则。
- **评估独立且保守回落**：重要级由后端在挂起前以**一次**独立 LLM 调用对本次全部待执行调用批量分类产出（优先 `flash` 档位、不可用回退本次运行档位）；评估失败、输出无法解析、条数不匹配或档位非法时一律回落 `must` 并告警，绝不中断挂起。
- **默认阀门 `each`**：`models/ai.DefaultReviewValve` 取 `each` 且与 `agent_sessions.review_valve` 列默认值一致，保持引入本特性前「一律人工审核」的既有行为，不产生静默行为变更。
- **定位为判定单点而非安全兜底**：client 工具由前端持用户凭据执行，后端不从技术上阻止前端绕过；后端合成出的 `approvalRequired` 仅供告知与审计留痕（`agent_runs.pending_calls` 记录每条调用的 `auditLevel` 与 `approvalRequired`），真正的闸门在前端。
- **枚举归属**：`auditLevel` / `evalStage` / `evalStageStatus` 归 `internal/ai/contract`，`diagnosisCategory` 归 `internal/ai/knowledge`，`reviewValve` 归 `internal/models/ai`（会话模型列取值空间），各自在归属包 `init()` 注册，前端经 `enum.auditLevel.*` / `enum.reviewValve.*` 渲染。
- **免审批调用仍须前端执行并自动回传**：`approvalRequired=false` 只免除人工点击，不代表后端可执行——client 调用（如 `api_invoke`）在挂起块到达后仍须由前端执行，并把结果随 `approval.decide` **自动提交**，否则会话会停在「等待授权」。提交闸门以「该审批块是否仍有未决策调用」为准（免审批调用的 `awaitingApproval` 会被块状态推导为 false，不能作为闸门），且批次内任一调用仍在前端执行时不得提前提交。
- **`ask_user`（主动提问）豁免人工审批**：`ask_user` 是 client 来源的问卷类工具（`internal/ai/contract` 的 `IsQuestionnaireTool` 单点判定），其调用**不弹审批面板**、**不参与 `auditLevel × reviewValve` 合成**——`auditLevel` 恒 `none`、`approvalRequired` 恒 `false`（即使会话阀门为 `each`）。前端据问卷块（`kind=questionnaire`、`awaiting=true`）渲染问卷面板；callID 仍登记在 `pendingCalls` 中，该轮全部调用在问卷作答后一次性提交（`questionnaire.submit`）。
- **交互块按时序内联**：审批 / 问卷 / 下一步块由 `agent_interactions.seq` 与消息内块序号决定其在内容块序列中的位置，与思考 / 正文块按真实产生顺序交错；前端按 `blocks` 顺序渲染，不得按固定阶段顺序重排或自行切分边界。
- **思考 / 正文各段按产生顺序投影**：`streamAssistantEvents` 在思考 / 正文段落切换时各自建立独立 `reasoning` / `content` 块，因此「每轮工具调用前的说明」与「末轮最终正文」各在其真实位置；块序号的唯一分配点是 `internal/ai/turns` 的投影器，实时流与历史重建同构。
- **思考独立成块**：历史回放与实时流一致，思考（`ReasoningContent`）单独一个 `reasoning` 块，不得以 `*reasoning*` + 引用块合并进正文 Markdown——否则前端只能按普通正文渲染，丢失可折叠思考块，且块序号与实时流不一致。
- **实时历史与持久历史同构**：实时投影与 `/render` 快照共用同一套块构建逻辑（`StreamResult` → `buildAssistantSnapshots`），块类型、顺序、工具调用状态与用量口径一致；前端以服务端历史为准丢弃本地提问回显（`pendingUserEcho`）。
- **问卷内联 + 输入区禁用**：会话因问卷待作答而阻塞，但**输入区不再被问卷卡片遮蔽 / 替换**——输入区常驻、仅整体禁用（问卷挂起时禁用正文输入、档位 / 阀门 / 思考等全部控件），并展示一条提示行把用户视线引回消息流中内联的问卷。作答 / 取消后输入区自动恢复可用。
- **问卷卡片自身须保持完全可交互**：**不得把会话级 `busy` 传给问卷卡片**；挂起状态由 `turns.Manager.State.Interrupted` 与 `PendingInterruptIDs` 表达，用它闸住卡片会把「提交 / 取消」一并锁死。问卷提交是同步动作，面板随 `status` 变为 `answered` 卸载，无需「提交中」禁用态。
- **一轮多条 `ask_user` 合并为一块问卷**：一轮内多条 `ask_user` 调用的题目在挂起时按调用顺序**合并**为同一个问卷块（该轮落一条 `questionnaire` 交互）；提交时前端按题目归属（各调用 `arguments` 中声明的题目 id）把答案**拆回各条调用**，由 `turns.mapActionTargets` 按 callID / interruptID 逐条构造 `ResumeParams.Targets`。题目 id 重复会使答案无法归属，该条调用按工具失败回灌（`duplicate ask_user question id: ...`），让模型修正后重试。
- **同一回合内多轮提问各成独立交互**：模型可在拿到上一轮答案后再次调用 `ask_user`；每一轮各落一条 `agent_interactions`（`seq` 先后即轮次先后），前端每轮渲染成独立问答框，最新一轮位于底部。作答续跑只把本轮交互更新为 `answered`，更早各轮保留历史答案；重新作答与重新审批共用「回退到待处理交互」入口（见下条），不再截断重提问。
- **重新审批 / 重新回答（交互回退）不重跑整轮回答**：`POST /agent/:sessionID/interaction/reopen` 按定位键找到交互并回退——审批按卡片上的 `callIDs`、问卷按卡片上的 `interruptIDs`。后端删除该交互锚点之后的全部消息与更晚交互，把该交互重置为 `pending`（审批清空决策、问卷清空作答并置 `awaiting=true`），并以挂起时留底的图现场快照（`agent_checkpoints` 键 `redo:<会话ID>:<交互ID>`）重建 `agent_runs` 现场与 `pending_interrupt_id`——状态回到「等待中 / 审批或作答」，用户重新决策 / 作答后照常续跑。快照在挂起现场即将被续跑 / 放弃前留底（`turns.Manager.Action`、`Chat`、`Abort` 三处，同一挂起的审批与问卷各留一份，且已存在不覆盖），因为续跑会覆盖同一 `session:<id>` 检查点；回退点已被滚动摘要覆盖时拒绝（`ErrRedoUnavailable` → 404）。问卷的调用清单取自该挂起现场（`agent_runs.pending_calls`），因为问卷载荷本身只存题目与作答。
- **交互块携带定位元信息**：审批块载荷额外携带 `approvalCallIDs`（逗号分隔的 callID），问卷块载荷额外携带 `questionnaireInterruptIDs`（逗号分隔的根中断 ID）；已决策 / 已作答块没有按钮，前端无法再从按钮上下文取回 ID，交互回退据此仍能精确定位持久化交互。
- **挂起锚点不变式**：挂起时保留本回合助手消息行作为锚点（`phase=suspended`、`agent_runs.anchor_message_id`），问卷 / 审批交互的 `message_id` 指向该行；续跑终态化写回同一行，故同一回合恒只有一条助手消息。
- **挂起现场无过期**：等待授权 / 作答时没有大模型调用在跑，现场生命周期只随会话删除；`turns.Manager.Chat` 遇到新提问会显式作废挂起现场、取消 pending 交互并删除 checkpoint。
- **截断与回滚须作废挂起现场**：`TruncateMessagesFromUserMessage` 在同一事务内把该会话全部 pending 现场置为已消费（不限锚点是否落在截断区间），`AgentMessageTruncate` 再经 `turns.Manager.StopSession` 停掉内存回合并删除 checkpoint，避免刷新后把已删除回合重建为孤立气泡。

### 领域事件投影（Agent-to-UI）

- **唯一 UI 协议**：AI 助手的全部服务端输出统一为领域事件（信封 `{v, seq, type, sessionID, runID?, ts, payload}`），SSE 以 `id:` 游标 + `data: <json>` 行下发，事件类型在信封内。事件目录以 `internal/ai/assistant/types.go` 为唯一权威：`snapshot.begin|message|end`、`resume.begin|reset`、`turn.started|preempted|completed|error`、`message.started|persisted|citations|compress`、`block.created|delta|updated`、`tool.call.updated`、`interaction.waiting|resumed`。
- **投影规则唯一实现**：`turns.streamAssistantEvents` 与 `assistantProjector` 把 ADK 事件投影为内容块（`reasoning` / `content` / `tool_calls` / `approval` / `questionnaire` / `next_steps` / `compress` / 中断提示）；块序号的唯一分配点在该投影器。正文与思考按段落各成一块并流式追加增量，工具调用为 `tool_calls` 块内按 callID 原位更新（`status=start` → `done` / `failed`，带回 `elapsedMs` 与结果）。
- **历史与挂起重建**：`RenderAssistant` 读 `agent_messages.payload` 与 `agent_interactions` 经 `buildAssistantSnapshots` 重建历史消息与块，下发 `snapshot.begin` / `snapshot.message` / `snapshot.end`；在途回合的缓冲事件追加在快照之后。`/render` 的输出与实时流同构，前端刷新 / 重进会话渲染一致。
- **交互持久化**：审批 / 问卷 / 下一步落 `agent_interactions`（`kind` / `status` / `seq` / `payload` 加密），不落块表；交互 `seq` 与锚点消息共同决定其与正文块交错的真实顺序。
- **上下文隔离**：内容块是**展示事实**——不进模型提示词、不参与压缩摘要折叠、不改变 `agent_messages.payload` 的模型历史口径；`next_steps` 兜底辅助调用读取会话摘要、最近对话与本轮回答，块本身不回灌上下文。
- **前端渲染**：前端只消费 `snapshot.message` / `block.*` 的 `blocks` 列表并按块类型渲染既有组件（思考折叠块、Markdown、工具调用组、审批面板、问卷、下一步、引用、压缩卡片），不维护 surface store、不做时间线反解析。
- **已决策 / 已作答的本地收敛态须留底**：挂起块在续跑实时事件中可能先于服务端终态被重建，前端按 `callID` 保存审批决策 / 结果（`approvalOverrides`）、按问卷块序号保存作答态（`questionnaireOverrides`），重建时回填；「等待审批」提示由「是否仍有未决策调用」推导，不得直接沿用块自身的 `awaiting`，否则已拒绝 / 已作答的块会回退成未处理态。两个本地快照在开始新一轮 / 会话重载时清空。
- **回滚级联**：`TruncateMessagesFromUserMessage` / `DeleteSession` 在同一事务内删除消息与交互（外键级联 / 显式删除），无孤儿块；压缩不删消息、不改变已有交互顺序。

### Agent 交互状态机（Agent Interaction State Machine）

- **状态是派生量而非状态列**：会话交互状态（四主状态）由既有事实经**全函数**推导，唯一实现在 `internal/ai/interaction` 的 `StateOf(facts)`，不新增可变状态列、不由前端推导、也没有「迁移 / 解锁」状态写路径（派生即得、重算即改）。事实装配在视图层只有唯一入口 `internal/manager/views/ai/interaction.go` 的 `agentInteractionFacts`（读内存在途活动、`agent_runs` 挂起现场、`CountUserTurns`、`session.maxUserTurns`、最近助手终局），出参 `InteractionSnapshot`（`state` / `substate` / `locked` / `userTurns` / `maxUserTurns`）供会话详情、会话列表与 `POST /status` 快照共用；禁止任一端点另写一套状态判断，禁止前端自行拼状态。
- **四主状态 × 子状态取值**：主状态 `waiting`（等待中）/ `running`（运行中）/ `paused`（暂停）/ `locked`（已锁定）；子状态 `approval` / `answer`（等待中）、`generating` / `thinking` / `tool_calling` / `compacting`（运行中，**四项**为准）、`success` / `failed` / `aborted`（暂停）；`locked` 无子状态（其子状态为空串）。两取值空间在 `internal/models/ai/enum.go` 注册为枚举类 `agentInteractionState` / `agentInteractionSubstate`，取值一律由后端返回，前端不得自声明字面量。
- **推导优先级（严格短路）**：`running > waiting > locked > paused`。在途活动（生成或手动压缩）存活 → `running`（子状态取活动上报的阶段）；否则存在挂起现场或 `pending` 运行现场 → `waiting`（挂起调用为问卷类 `ask_user` → `answer`，否则 → `approval`）；否则有效轮次 ≥ 上限 → `locked`；否则 → `paused`（子状态由最近一条助手消息终局映射：`done→success`、`failed→failed`、`cancelled→aborted`、无助手消息或尚未终结 → `success`，故空会话即 `暂停/成功`）。全函数保证任意时刻必落且仅落一个状态（无空档、无并列、无「未定义」态）。
- **子状态归一兜底**：残缺 / 未知的运行子状态经 `normalizeRunningSubstate` 兜底为 `generating`，未知挂起种类经 `normalizePendingSubstate` 兜底为 `approval`，未知 / 空的助手终局经 `pausedSubstateOf` 归为 `success`，使事实残缺时仍是合法状态。
- **会话内至多一个在途回合（生成与压缩共用互斥）**：由 `turns.Manager` 的会话 `state.current` + `turnMu` 在进程内保证；新回合启动前先 retire 旧 TurnLoop（`Abort` + `Wait`）。`running` 期间拒绝发送新提问以外会破坏现场的操作；`waiting` 期间没有大模型调用在跑，仅拒绝压缩，允许截断 / 作废挂起现场后重开（由 `CanRollback` / `CanRerunUserInput` 放行）。
- **静默中止（context cancel + TurnLoop.Stop）**：`running` 任一子状态均可中止；`turns.Manager.Abort` 调 `loop.Stop(adk.WithImmediate())` 并等待 `loop.Wait()`，取消信号沿运行上下文即时中断上游模型流式请求与工具执行；随后以 `stream.CloseNotice("cancelled", …)` 下发 `turn.completed(cancelled)` 收敛缓冲、落锚点消息 `cancelled`，并作废 pending 现场与 checkpoint。**取消返回后不再产生任何领域事件**。
- **等待中中止（无超时）**：`waiting` 时调 `SessionStore.AbortPendingRun`（现场置 `consumed`、锚点助手消息置 `cancelled`）并清空 `pending_interrupt_id`，幂等。挂起现场不设过期、不参与 TTL 清理：等待授权 / 作答时没有大模型调用在跑，生命周期只随会话删除。
- **进程重启后的存活性收敛**：TurnLoop 只存在于进程内，进程重启后内存回合并发丢失；会话状态由 `POST /status`（`turns.Manager.State`）按 `pending_interrupt_id` + `agent_runs` + 最近助手消息 `phase` 推导。若登记仍显示运行中但内存无活动，按孤儿处理：仅当最近一条助手消息仍在途（phase 为空）或挂起（suspended）时写回 `failed`，已 `done` / `failed` / `cancelled` 的历史不得覆盖。收敛不删除消息、记忆与压缩检查点。
- **轮次上限与派生解锁**：上限取自全局参数 `session.maxUserTurns`（int、group=`session`、出厂值 20；`<=0` 表示不设上限、永不锁定）。有效轮次 = 会话内当前存在的 `role=user` 消息条数（`SessionStore.CountUserTurns`，被回滚删除的不计入）；「发送前」与「每轮终局时」都按同一口径判定锁定，避免「已满仍可发」/「发完不锁」。锁定是派生态、不写状态列：`已锁定` 非死路，回滚使有效轮次降到上限以下即**自动回 `暂停`**，无额外解锁写路径。
- **回滚类操作的边界与守卫**：新提问、回滚到某条用户提问**发送前**（`TruncateMessagesFromUserMessage` / `AgentMessageTruncate`）、压缩到某条**助手回答**（`AgentSessionCompact`）、回退到助手待输入回合（问卷 / 授权），一律先经交互守卫判定，守卫只依据 `StateOf`、与前端闸门同源：`CanSend` 仅 `paused` 允许；`CanCancel` 仅 `running` / `waiting` 允许（等价「停止按钮可用」）；`CanRollback` / `CanRerunUserInput` 允许 `paused` / `locked` / `waiting`（仅 `running` 拒绝；等待中截断 / 重开会作废当前挂起现场）；`CanCompact` 仍仅 `paused` / `locked` 允许（压缩是另一次在途活动，等待中应先作答 / 授权或回滚）。压缩的折叠下界 = 当前 `session.summary_upto_seq`、上界 = 选中助手回答 seq（`compactFoldSet`），边界按助手消息定位（`AssistantSeqAtMessage`）而非按轮次；回滚跨压缩点时经 `latestCompactionBefore` 回退到 `upto_seq < 回滚点` 的最近检查点、并作废 `upto_seq ≥ 回滚点` 的检查点。
- **被拒时的业务码口径（唯一映射点）**：守卫拒绝哨兵在视图层 `rejectGuardError` 一处映射——`ErrInteractionLocked` → **400 + 业务码 1305**「已达轮次上限，请回滚后再试」；`ErrInteractionBusy` → **409 + 业务码 1306**「当前会话状态不允许该操作」。领域层 `internal/ai/interaction` 只暴露语义哨兵，不关心 HTTP 状态码与文案，各端点不得各写一套。
- **事实变更后重算并暴露**：所有改动上述事实的操作（发送 / 挂起 / 批准 / 回答 / 完成 / 失败 / 中止 / 回滚 / 压缩 / 轮次变化）完成后，视图层经 `interactionSnapshot` 重算；有内存回合时由其领域事件缓冲继续输出终局事件，无内存回合时状态随 HTTP 快照出参下发，使状态与事实不发生漂移。

### LLM 提示词与工具文案（中文约束）

- **一律中文**：凡送入模型或供模型决策的自然语言文本统一使用中文——出厂提示词（`002_fixtures.sql` 的 `agent.systemPrompt`、`retrieval.rerankSystemPrompt`、`session.topicSystemPrompt`、`memory.summarySystemPrompt`、`memory.extractSystemPrompt`、`eval.*SystemPrompt`、`agent.nextStepsSystemPrompt`）、用户消息模板中的段标题与标签（`agentSummarySectionTitle`、`agentMemorySectionTitle`、`agentBusinessSectionTitle`、`agentQuestionSectionTitle`、`buildRerankPrompt`、`buildEvalChunkPrompt`、`buildClaimsPrompt`、`buildSummaryPrompt`、`buildSuggestionsPrompt`、`buildNextStepsPrompt` 及其语言指令），以及内置工具的 `Info.Desc` 与参数说明（`internal/ai/tools`）。禁止新增英文提示词或英文工具描述。
- **Prompt 构造必须使用 eino message template**：所有送入模型的 system / user 消息一律经 eino 的 message template 渲染——`schema.Message(tpl).Format(ctx, vars, schema.GoTemplate)` 或 `prompt.FromMessages(schema.GoTemplate, ...)`；禁止用 `fmt.Fprintf`、`strings.Builder`、`+` 拼接提示词正文。可编辑提示词正文、检索分块、历史消息、业务数据、错误文本等只能作为模板变量注入，不得参与二次解析；固定段落与分块清单用模板的 `{{if}}` / `{{range}}` 表达。模板声明集中在各包的 `prompt.go`（先例：`agentkit/prompt.go`、`internal/ai/knowledge/prompt.go`），调用处只组装变量并检查渲染错误。工具 `Info.Desc` / 参数说明等静态元数据不属于消息模板，可继续使用常量。
- **结构性标识保持英文**：JSON 键（`content` / `draft` / `results` / `memories` / `judgments` / `claims` / `suggestions` 等）、枚举取值（`memory.extractSystemPrompt` 的 `kind` 取值与 `memoryKindNames()` 返回的记忆类别）、工具名（`knowledge_search` / `memory_write`）、诊断类别标识（`coverage` / `ranking` / `generation` / `context` / `other`）与消息角色（`user` / `assistant` / `system`）是解析契约，必须保持英文原样，不得翻译。
- **输出语言由提示词声明或代码追加**：回答语言以「跟随用户提问语言」为原则（在 `agent.systemPrompt` 内声明）；`eval.suggestionsSystemPrompt` 的建议正文语言由 `suggestionsSystemPrompt` 在代码侧追加 BCP-47 语言标签指令，禁止把语言约束写进可编辑的提示词正文（否则管理员编辑提示词时会丢失）。
- **错误信息仍用英文**：工具与内部流程返回的 `error` 面向排障而非终端用户（同 `internal/ai/tools` 包注释），保持英文，不属于本约束所指的「提示词」范畴。
- **出厂值只对新装库生效**：提示词经 `002_fixtures.sql` 以 `ON CONFLICT (key) DO NOTHING` 种子化，调整文案仅影响新装库；已部署库的存量 `value` / `default_value` 不回填，由管理员在参数页改写。
- **`agent_params` 是参数值唯一来源**：只读列 `default_value`（仅 `002_fixtures.sql` 注入、接口不可写）与生效列 `value`；代码内零提示词 / 出厂值字面量（`internal/ai` 注册表只留 key / group / kind / comment）。`harness.Load` 启动强校验「注册表键集合 == 库内行集合」且值可按 kind 解析，缺键即 panic 并提示 `migrate up`；跨键关系（融合权重非负且不同时为零、`agent.maxToolResultBytes` 不小于 `retrieval.expandDocLimit`）由 `config.ValidateRelations` 在启动期与参数写入接口落库前各校验一次；「恢复默认」= 将 `value` 回写 `default_value`（不删行）。

### LLM 客户端超时（流式 vs 非流式）

- **禁止以整请求超时约束流式生成**：构造对话客户端时必须经 `einoopenai.ChatModelConfig.HTTPClient` 注入自定义 `*http.Client`（`internal/pkg/llm.NewChatClient` → `newChatHTTPClient`），**不得**使用 `ChatModelConfig.Timeout`——后者在未提供 HTTPClient 时被 eino-ext 落为 `&http.Client{Timeout: t}`，而该超时覆盖整个请求生命周期（含响应正文读取），对 SSE 流式生成同样生效，任何耗时超过 `t` 的长回答都会在读取正文阶段被中断，报 `failed to receive stream chunk: context deadline exceeded (Client.Timeout or context cancellation while reading body)`（曾在检索试运行「生成回答」阶段复现）。
- **`ModelConfig.Timeout` 的语义按调用方式分流**：流式 `Stream` 中该值仅用作连接建立与响应头（首字节）上限（`DialContext` / `TLSHandshakeTimeout` / `ResponseHeaderTimeout`），整体生成时长交由调用方 context（如 HTTP 请求生命周期）约束；非流式 `Generate` 中该值经 `context.WithTimeout` 施加整请求截止时间。`t <= 0` 表示不施加任何超时。
- **非流式超时重试为全 Agent 通用热配置**：`ChatClient.Generate` / `AgenticClient.Generate` 命中超时（`context.DeadlineExceeded` 或 HTTP 首字节/i/o 超时）时按 `agent.llmTimeoutRetryEnabled` / `agent.llmTimeoutRetryMaxRetries` / `agent.llmTimeoutRetryMultiplier` 重试，第 n 次重试的截止时间为「模型超时 × 倍数^n」；参数存于 `agent_params`，前端参数页修改并经 etcd 广播重建运行时后立即生效。流式 `Stream` 不施加整请求超时，故不参与该重试（重试流式中途会重复输出）；流式仅保留既有 429 类瞬时错误重试。
- **回归测试**：`internal/pkg/llm/client_test.go` 的 `TestChatClient_StreamExceedsModelTimeout`（服务端停顿远超模型超时后仍读完全部增量）与 `TestNewChatHTTPClient_NoTotalTimeout`（断言 `http.Client.Timeout == 0`）为守卫，禁止改回整请求超时。
- **禁止在 `NewChatClient` 注入 `response_format`**：同一份档位客户端被结构化输出（重排、记忆抽取、审核分级、dry-run 评估、Agent 主对话）与自由文本输出（会话主题 `session.topicSystemPrompt`、滚动摘要 `memory.summarySystemPrompt`）共用，而 OpenAI 兼容供应商在 `response_format=json_object` 下会校验消息中必须出现 "json" 字样，自由文本提示词不含该字样会被整请求以 400 拒绝（曾在会话创建时复现：`'messages' must contain the word 'json' in some form, to use 'response_format' of type 'json_object'`，主题生成被静默降级为主题兜底、滚动摘要直接失败）。eino 的 `model.Option` 没有按次设置 `response_format` 的入口，故结构化输出**一律由各系统提示词显式声明所要求的 JSON 形状保证**，且各解析函数必须容忍 ```json 围栏包裹（`stripJSONFence` / `parseRerankIndices` / `parseExtractedMemories` / `parseAuditLevels`）。

### 枚举统一注册表（enum 注册表 / search-generate 规范）

- **单一数据来源**：所有枚举值统一注册进 `internal/pkg/enum` 注册表（`enum.Register(enum.Type{Name, Entries})` 或 `enum.RegisterSource(name, entriesSource)`，条目 `enum.Entry{Key, Value}`），每个枚举类（类名如 `channelType`、`httpMethod`）有且仅有一个注册点，类内 key 互斥唯一、value 互斥唯一。数据库内存储该字段枚举值时一律存 **value**（不可存 key）。
- **声明与注册一律在取值空间的归属包**：每个枚举类的唯一注册点落在**取值空间的归属包**——即该取值空间的定义处，声明形态两类：
  - 取值空间固定的枚举：取值空间就地声明在该包内（类型化取值常量 + 取值序列），并在包内 `init()` 经 `enum.EntriesOf` + `enum.Register` 在注册时快照条目；若包内已有该取值空间的常量/清单（如 `base.SupportLanguages`、`base.EffectAllow`、`core.TemplateSource*`、`tag.EventStatus*`、`vtab.redisKind*`、`ai.AllSlots`），则**直接复用**为条目来源，不重复声明常量。当前归属：`language` / `policyEffect` → `internal/pkg/base`；`httpMethod` / `httpVersion` → `internal/models/network`；`notificationSource` / `notificationRecordStatus` → `internal/models/core`；`tagAlertStatus` → `internal/models/tag`；`datasourceStruct` → `internal/pkg/sqlitevtab/vtab`；`llmModelKind` / `llmAPIMode` / `chunkUnitKind` → `internal/pkg/llm`；`llmSlot`（取值空间 `Slot` / `AllSlots`）→ `internal/models/ai`；`llmToolSource` → `internal/models/ai`；`agentKind` / `agentRunStatus` / `memoryKind` / `memoryStatus` → `internal/models/ai`；`mcpTransport` → `internal/ai/tools/mcp`；`evalStage` / `evalStageStatus` → `internal/ai/contract`；`diagnosisCategory` → `internal/ai/knowledge`。**禁止**任何集中的纯声明包（如曾经的 `internal/enums`）：类必须与它的取值空间同包，跨层共用类由消费方 import 归属包引用其常量。
  - 业务注册表来源（channelType / notificationEvent / datasourceType / tagRuleModel / tagEventDimension）：值空间随业务注册表在代码内固定，由**所属业务包**（message / notify / vtab / tagengine）经 `enum.RegisterSource(类名, func() []enum.Entry)` 声明条目来源——条目不在注册时快照，而在每次读取（`Get` / `List`）时从业务注册表实时派生，避免包内多 `init()` 的文件顺序影响条目完整性；与 `/channel/types`、`/template/events`、`/datasource/types`、tagengine 注册表共用单一来源。视图层不再组装任何枚举类。
- **归属判据是取值空间的拥有者，而非存储位置**：*被模型列存储 ≠ 归 model*。取值空间仅作为某个模型列的值域存在时，归该模型包（如 `knowledgeDocStatus` → `internal/models/ai`、`notificationRecordStatus` → `internal/models/core`、`tagAlertStatus` → `internal/models/tag`）；若该词汇由某个非 model 子系统定义或使用（如 `documentFormat` 由 `internal/pkg/docconv` 的 `Format` 定义、`mcpTransport` 由 `internal/ai/tools/mcp` 的传输实现使用），则归该子系统所属包，**不得**因「存在同名模型列」而迁入 model——后者会迫使 `internal/pkg` 反向依赖 `models`，或就地重复声明常量造成定义漂移。编排层（`internal/worker`、`internal/manager`）不设枚举注册点：记录的生命周期状态是持久化行的值域，归其模型包，驱动状态迁移的 handler/view 只是消费方。
- **字段与枚举类绑定**：`search.toml` `[[enums]]` 仅作「结构体类型 + 字段 JSON 名 → 枚举类名」绑定引用（`type` / `field` / `set`），`search-generate` 生成进视图层 `enum_bindings.go` 的 `enumBindings`（绑定视图层结构体字段名，属视图层关注点），meta.go 反射输出 `FieldMeta.EnumType`；运行时字段（如 `NotificationTemplateCreateIn.event`）经 `views.AddEnumBinding(typeName, field, className)`（`enum_registry.go`）在模块 `init()` 补充。`set` 类名仅为引用，不携带可选项——可选项唯一来源是 enum 注册表。
- **i18n label 键**：统一前缀 `enum.<类名>.<key>`（如 `enum.notificationEvent.tag_event_record`、`enum.httpMethod.GET`），只允许经 **key** 翻译 label，禁止经 value 翻译；中英文 `zh-Hans.toml` / `en.toml` 结构一一对应，新增枚举条目（含注册表来源的新渠道/事件）必须同步补两语言键，未命中回退显示 key 本身。**禁止**任何 `model.*.option.*` 平铺或分组键。键完整性由 `internal/manager/views/enum_i18n_test.go` 的 `TestEnumClassesHaveBilingualLabels` 兜底：遍历 `enum.List()` 断言每个类每个条目的 label 键在每种支持语言下均可解析（解析结果不等于 key），新增枚举类/条目必须同步补两语言翻译文件。
- **统一枚举接口**：`POST /admin/enum/get`（按 `{type, lang}` 返回单类条目）与 `POST /admin/enum/list`（按 `lang` 返回全量类条目），出参条目均为 `{key, value, label}`，label 跟随请求语言。前端按（枚举类, 语言）组合缓存（`stores/enums.ts`），仅后端进程重启才可能更新，页面间不重复请求。
- **前端消费契约**：类名常量集中在 `utils/enums.ts` 的 `EnumClass`；`enumFieldTypeOfMeta` 从 meta `bodyOutput` 取字段 `enumType`，`ensureEnum` / `enumEntriesOf` / `enumLabelOf`（value/key 双解析优先 value）/ `enumOptionsOf` 统一经枚举 store；表头/筛选（SearchTable.vue、AdaptiveCell.vue）与页面下拉一律按 `enumType` 定位枚举类，禁止手工维护选项字面量。通知模板 `t` 函数类感知：`t "<类名>" <key|value>` 优先按 value 匹配再按 key 匹配，经 `enum.<类名>.<key>` 渲染 label。
- **目标模型枚举（tagRuleModel）**：Key 为 `tagengine.ModelKey` 推导的短展示键（camelCase，如 `httpTransport`），Value 为完整类型键（数据库存储语义，驱动 `WHERE model = ?` 匹配 tag_summings）；`TagEventRecordHit.Model` 落库存**完整类型键（value）**（`toEventHit` 直存 `hit.Model`、不做 value→key 预转换，与 `TagRule.Model` / `TagSumming.Model` / `TagEventCondition.Model` 一致），展示 label 由消费方经 `tagRuleModel` 枚举类按 `enum.tagRuleModel.<ModelKey>` 推导，通知模板 `t "tagRuleModel" .Model` 与展示经同一枚举类收敛；S3 明文查看接口的模型 `switch` 经 `tagengine.ModelKey` 幂等匹配（兼容完整类型键/枚举键两种入参）。禁止 snake_case 与完整类型键作为展示键双写。
- **前端禁止声明枚举字面量**：枚举的取值集合、默认值与展示文案**一律由后端接口返回**，前端不得在任何文件中书写枚举取值数组或取值常量——包括 `const SLOT_ORDER = ["flash", "standard", "advanced", "embedding"]`（取值集合）与 `const DEFAULT_SLOT = "standard"`（默认值）这类形式。理由：字面量一旦落在前端就会与后端注册表/参数配置漂移（曾出现助手页把默认档位写死 `standard`，运维把参数 `agent.defaultSlot` 改为 `advanced` 后该页仍按 `standard` 生成）。取值集合与默认值经接口下发（搜索类走 `enumType` / 参数类走 `options` + `enumType` / 状态类走 `defaultSlot` 等），选项文案经 `enum.<类名>.<key>` 渲染，需要固定展示顺序时以接口返回顺序为准，前端不再自行排序。
- **参数注册表的枚举取值**：参数注册表（`internal/ai` 的 `ParamSpec`）中取值空间属于某个枚举类的参数，必须同时声明 `Options`（封闭取值清单：可选项与写入校验的唯一来源，由枚举/业务注册表**派生**得出，禁止重复书写字面量）与 `EnumType`（枚举类名，仅供消费方按 `enum.<类名>.<key>` 渲染 label）；`param/list` 出参带 `enumType`，前端据此经枚举 store 渲染下拉项文案，未声明 `EnumType` 时字面量本身即文案（如 `retrieval.fusion`）。枚举类名与取值清单的一致性由 `internal/manager/views/llm/param_enum_test.go` 的守卫测试兜底。

### 字段校验标签（validator 自定义 tag）

- **单一登记点**：全部自定义校验规则的 tag → 校验函数登记表唯一维护在 `internal/pkg/validator` 的 `Funcs()`，`internal/provider/validator.go` 与各测试 setup（`views/base_test.go`、`views/*/testmain_test.go`、`views/*/setup_i18n_test.go`）一律经 `validator.Funcs()` 取用，禁止任何调用方重复抄写 tag 清单（漏抄会让该 tag 在对应场景下被判为未注册规则，表现为绑定失败而非编译错误）。
- **能力扩展范式**：新增规则一律「`TagXxx` 常量 + `IsValidXxx(string) bool` 谓词 + `ValidateXxx(fl validator.FieldLevel) bool` 适配器」，在 `Funcs()` 登记，并在中英文 locale（`internal/pkg/translate/locales/zh-Hans.toml` / `en.toml`）的 `[validator]` 段补 `validator.<tag>` 文案（缺失即 `MustLocalizeMessage` panic）。
- **通用字段级 tag（禁止就地实现同等能力）**：
  - `enum=<枚举类名>`：取值必须命中 `internal/pkg/enum` 注册表中该类的某个 **value**（按 value 严格匹配，不接受 key）；类名未注册属编程缺陷，直接 panic。
  - `httpUrl`：绝对 `http` / `https` 地址（scheme 与 host 均非空）；可选时配 `omitempty`。
  - `jsonObject`：所有取值均为字符串的 JSON 对象文本（字面量 `null`、数组、非字符串取值一律不合法）；可选时配 `omitempty`。
  - `trimmed`：字符串首尾无空白。后端**只判定、不静默裁剪**——裁剪会让前端提交原值与实际落库值不一致；由此前端对同名单行输入必须 trim，文本域（`v-textarea`）豁免（见「String Input Trimming 字符串输入去空白」）。
- **就地校验禁令**：视图层 In 结构体的字段合法性**一律由绑定标签表达**，禁止书写 `normalizeXxx` / `parseXxx` 这类仅做 trim、枚举白名单、URL 或 JSON 形态判断的私有方法；处理器只做「绑定 → 业务约束（唯一性、存在性等）→ 落库」。数据层解码（如密文列解出的 JSON 文本转 `map[string]string`）不属于校验，保留在处理器内。
- **枚举取值一律经 `enum=<类名>` 绑定，禁止就地书写取值白名单**：取值空间已注册为枚举类的字段，其校验必须写作 `enum=<类名>`；**禁止**写作 `oneof=<值1> <值2> ...` 这类就地枚举白名单——白名单一旦落在用法处即与枚举注册表形成第二份取值事实，注册表新增取值时不会同步（先例：`AgentMemoryListGet` 过滤项曾把 `agentKind` 写成 `oneof=knowledge rego`，漏掉注册表中已有的 `general`；`memoryStatus` 亦曾写作 `oneof=active archived`）。本约束同样覆盖 `search.toml` 为生成过滤项声明的 `binding`（写作 `binding = "enum=<类名>"`），因生成的 `FilterByItem` 同为视图层字段校验的载体；非枚举的内部固定取值（分页档位、排序字段名、过滤动作符等，如 `oneof=eq`、`oneof=10 20 50 100`）不受此限。
- **条件必填**：同一 In 结构体存在多种用法时（如「带 id 时其余字段被忽略」），用 `required_if=<字段> <值>` + `omitempty` 表达条件必填，而非无条件的 `required`。
- **分层校验职责**：形状校验（必填 / 长度 / 范围 / 格式 / 枚举 / 条件必填）只在进入边界的结构体上用 `binding` 声明并执行一次；业务不变量（唯一性、存在性、租户归属、状态迁移）留在领域 / 服务层用哨兵错误表达；nil 依赖等编程前置条件属于内部错误，不得映射为字段校验错误。
- **契约类型单一事实源**：HTTP In 在视图层，worker 载荷在 handler 包，LLM 协议在拥有该协议的 AI 功能包，配置在配置结构体；`internal/models` 模型不携带 `binding` 标签。同一领域操作被多入口调用时，规则收敛到共享输入契约，禁止各入口复制规则。
- **校验视图（validation view）**：模型类型、共享协议类型、`map[string]json.RawMessage`、`json.RawMessage`、LLM 原始文本等无法携带标签的公开类型，在其消费边界定义私有 `xxxRule` / `xxxView` 结构体，装载数据后经 `validator.Check` 校验再映射回目标类型；禁止在调用点堆砌空值 / 长度 / 枚举 / URL / JSON 形态的 if 分支。
- **统一校验入口**：HTTP 之外的 worker / CLI / LLM 输出 / 自由形状数据一律经 `internal/pkg/validator` 的 `Check` / `CheckPartial`，不得直接调用 `validator.Validate.Struct`；`Check` 返回未包装的 `validator.ValidationErrors`，HTTP 层 `CreateBadBodyError` 仍可直接类型断言并本地化；模型 / 任务的纠错摘要统一经 `Describe`。
- **JSON 协议帧统一解析**：模型输出的裸 JSON 对象一律经 `DecodeObject`（严格拒绝代码围栏与前后缀文字）或 `DecodeObjectLenient`（容忍单层代码围栏）；禁止各包自写 `HasPrefix("{")` / `stripJSONFence`。可选对象与可选任意 JSON 值的「空串 / 空白 / null 视为未提供」语义统一经 `DecodeOptionalObject` / `DecodeOptionalValue`，禁止各包重复书写空值与 null 判断。顺序固定为「帧解析 → 结构体形状校验 → 业务处理」。
- **动态 schema**：规则依赖运行期 schema 时，复用 `validator` 导出的纯谓词（如 `IsValidJSONTypedValue`、`IsValidEnum`）或专用校验视图，禁止在业务包重复 `switch` / 取值白名单。
- **静态守卫**：`internal/pkg/validator` 的 AST 守卫测试扫描 `internal/` 全部 `binding` 标签，未登记且非内置的 tag 直接失败；同一包另以 AST 守卫禁止 `internal/pkg/validator/json.go` 之外自行用 `strings.HasPrefix("{")` / `HasSuffix("}")` / `TrimSuffix("```")` 判断 JSON 帧。新增自定义 tag 仍须在 `Funcs()` 登记并补中英文 locale。
- **执行一次**：下游只消费「已校验类型」，不得重复实现同一形状规则；类型注释标注「字段已由调用边界校验」。

### 文档分类与导出（Diátaxis + gomarkdoc）

- **四分类、两来源、唯一清单**：Diátaxis 只描述页面种类（`tutorial` / `how-to` / `reference` / `explanation`，见 `internal/docs/category.go`），导航分区（`Section`）描述读者旅程；来源（`Source`）只有 `comment`（代码声明派生）与 `curated`（人策展，AI 可起草）两种。全部条目（分类/分区/顺序/标题/摘要/角色/场景/责任人/来源/AI 摄入/来源包/产物路径/渲染档）唯一登记在 `internal/docs/registry.go` 的 `Registry()`，新增或调整文档一律改此处。
- **目录按来源隔离（硬边界）**：`docs/generated/` 只由生成器写入，每个文件首行带 `DO NOT EDIT`；`docs/curated/` 只由人写入（AI 起草的初稿也归此目录）。生成器只能写 `docs/generated/`、`docs/index.md`、`docs/nav.json`，路径白名单与「覆盖前校验生成标记」由 `internal/docs/export.go` 的 `writeGenerated` 强制；CI 在生成后要求 `git diff --exit-code -- docs/curated` 为空。
- **文档入口**：`zki create docs` 生成 `docs/index.md`（总目录 + 阅读路径 + 按角色索引）与 `docs/nav.json`（站点侧边栏与链接检查的机器可读大纲），链接相对 `docs/` 书写；项目根 `README.md` 是手写产品门面，生成器禁止写入；文档只维护中文，不提供英文版本。
- **策展文档（AI 起草 + 人审校）**：`docs/curated/**` 每页必须带三行元信息头——`> 状态：草稿（AI 起草）` 或 `> 状态：已校对`、`> 责任人：@账号`、`> 依据：` 反引号包裹的仓库相对路径；状态为「已校对」且责任人明确才可合并。页面必须从 `docs/templates/` 的模板起草，模板中的「起草须知」必须在定稿前删除。策展页只写意图与取舍：字段表、接口签名、枚举取值、命令参数表一律链接生成页，禁止复制。
- **单一事实来源**：概念与任务正文不再由 package 注释承载。package 注释只写「包职责 / 文件分工 / 边界 / 不变量」，任务步骤写入 `docs/curated/`；生成条目由注册表声明并在 `docs/generated/explanation/` 落盘，禁止手改。
- **投影页（Projection）**：正文由调用方按单一事实来源渲染，导出器只落盘并套用统一页头（分类固定为参考），输出必须位于 `generated/` 下；投影页不入 RAG；`Options.Projections` 为空时导出行为与不传时一致。
- **每个包至多一条叙述条目**：同一包的 package 注释只对应一篇叙述页，避免同包多页内容重复、被 RAG 重复索引。
- **导出与漂移检查**：`zki create docs`（`internal/cmd/docs.go`）一次导出叙述页、参考页、参考索引、`docs/index.md` 与 `docs/nav.json`；`zki create docs --check` 导出到临时目录并逐文件比对生成域，任何缺失、多余或内容不同都以非零退出（`make docs-check`）。**抽取或生成失败不得静默**：出错必须经 `pterm.Error` 报错并返回错误使命令非零退出。
- **叙述档渲染约束（重要）**：叙述档在 `internal/docs/render.go` 中额外覆盖 `text` 模板（`narrativeTextTemplate`）关闭纯文本转义，因此叙述来源的 package 注释必须写成「原样 Markdown 且安全」——命令 / 路径 / 配置键 / 标识符一律用反引号包裹（否则 `_`、`*` 等会被 Markdown 解析成斜体 / 加粗）；禁止 `# ` 单井号标题（Go 文档注释只识别单井号为 heading，会渲染成固定层级并与注册表注入的 H1 标题重复、层级倒挂），顶层小节用 `## `、嵌套用 `### `；避免会被当作 HTML 标签吞掉的裸尖括号占位符。
- **参考档保持内置转义**：参考档不覆盖 `text` 模板，沿用 gomarkdoc 内置转义——这对符号文档里的字面量 `#`、尖括号是正确行为，不得关闭。
- **守卫测试**：`internal/docs/registry_test.go` 校验分类与分区合法、分区内顺序唯一、来源与根目录一致、责任人非空、来源包存在、孤儿页（生成与策展分别检查）、生成文件带标记、策展页元信息头与依据路径、叙述条目元信息完整；改注册表或文档后必须过此测试。

### 密码重置（忘记密码）安全链路

- **公共路由与开关**：`POST /api/v1/user/password/forgot`（申请）与 `POST /api/v1/user/password/reset`（凭令牌重置）均为公共路由（无需认证）；可发性由功能开关注册表判断（`features.PasswordResetKey`）。
- **防枚举**：`forgot` 无论账号与邮箱是否匹配一律返回成功；仅命中的 `Active` 用户才生成令牌、发信并落库。命中后的技术性失败（令牌生成 / 落库 / 入队失败）同样返回该统一成功响应，差异只写服务端日志，避免响应差异成为「账号名↔邮箱」配对的枚举 oracle。
- **申请限流**：`forgot` 按**请求来源地址**（`c.ClientIP()`）维度经 Redis 固定窗口限流（通用件 `internal/pkg/redis.FixedWindowLimiter`，维度键为来源地址的 SHA-256 摘要，避免明文地址落键）。维度不取「账号名 + 邮箱」：字典攻击会不断更换组合（每猜一个组合都是新键），按组合计数永不触发。上限与窗口取 `PasswordReset.RateLimit` / `PasswordReset.RateLimitWindow`，超限返回业务码 `CodeTooManyRequests`；Redis 不可用时放行并告警（限流属纵深防御，不阻断正常找回）。**前提**：`c.ClientIP()` 的可信度取决于受信代理配置——`manager.Config` 顶层 `api.trustedProxies`（受信代理 IP / 网段列表，元素类型 `serializer.CIDR`：裸地址补 `/32` / `/128`，掩码外的位归一化为零）在 `RouteRegister` 中经 `engine.SetTrustedProxies` 生效，**默认空列表即不信任任何代理**（`ClientIP` 退化为 TCP 对端地址，不采信 `X-Forwarded-For`）；反向代理部署时须显式填入代理网段，否则来源地址可被伪造而绕过限流。非法网段在配置提取阶段（`serializer.CIDR` 文本解析）即报错使启动失败；per-IP 亦无法约束僵尸网络等分布式来源。
- **令牌只存摘要**：`user_password_resets.token_hash` 存 SHA-256 十六进制（`CHAR(64)`，唯一索引）；原始高熵令牌只出现在邮件链接，库泄露不可直接使用。
- **单次有效**：重置成功时在同一事务内把该用户全部未使用令牌置 `used = true`（多封邮件仅一次可用）；校验条件为 `token_hash = ? AND used = false AND expired_at > now`。
- **链接格式**：`{FrontendBaseURL}/{lang}/auth/reset-password?token=...`，语言段取 `base.DefaultLanguage`；`FrontendBaseURL` 是 `manager.Config` 顶层项目级配置。
- **不可用语义**：SMTP 未启用或 `host` / `from` 缺失时关闭 `PasswordResetKey`，`forgot` 返回业务码 `CodePasswordResetUnavailable`，不生成令牌、不发信。

### Agent 业务数据边界（前端取数透传）

- **不持有登录态、不取业务数据**：AI 通道不引入取数能力，业务数据一律由前端以用户登录会话调用既有授权读接口取得，组装为上下文随生成请求透传。
- **后端只做边界与清洗**：后端仅施加尺寸 / 条数边界与 `SanitizeBusinessData` 清洗，原样注入 LLM，不自行查询业务库；`businessData` 不落库。
- **越权规避**：如此据以规避水平越权与提权；若未来新增后端侧取数，必须走用户会话 + 既有鉴权，禁止在 AI 包内直连业务库。

### 标签引擎与 rego（Tag Engine）

- **Rego 契约单一事实**：标签规则必须 `package zki` 并产出 `tags` 规则（`contains` / 赋值），引擎查询 `data.zki.tags`；写入必经两层校验——`ValidateRego` / `InspectRego`（语法 + 契约）与 `ValidateGroup`（`CompileGroup` 组级语义，拦截跨规则冲突）。
- **目标模型注册表为单一来源**：可打标模型在 `internal/tagengine/registry.go` 的 `Target`（`Type` + `Dimensions`）登记；`TagRule.Model` / `TagSumming.Model` / `TagEventCondition.Model` 与命中明细 `Model` 一律存**完整类型键**（`pkgpath.TypeName`），展示 label 经 `tagRuleModel` 枚举类按 `enum.tagRuleModel.<ModelKey>` 推导，禁止 snake_case 双写。
- **ID 类型锚点**：所有目标模型须有唯一 `ID` 字段且类型为 `serializer.HexUUID`（注册时反射校验，违规 panic），保证 `tag_summings.ids`（`Array(UUID)`）跨模型统一。
- **运行期样例校验是写入硬门禁**：创建 / 更新 / 启用 / 回滚必过 `ValidateGroupRuntime`；样例存 `tag_rule_samples`，按（命名空间, 模型）隔离、由 `EnsureModelSamples` / `ListModelSamples` 懒供给；空组、未注册模型、无样例的降级语义见 `internal/tagengine/base.go` 包注释。
- **第三方数据源按命名空间池**：`vtab.Pools` 为每个命名空间独立持池，数据源以本地名注册（无 `{namespace}__` 前缀）；`Engine` 求值前按命名空间把池绑定到 ctx（`WithDatasourcePool`），builtin `sqlite.query` 从 ctx 取池；同名数据源跨命名空间不复用连接与凭据。
- **`sqlite.query` 全局注册**：经 `rego.RegisterBuiltin2` 同时注册 `ast` 与 `topdown`，使校验 / dry-run / 样例校验 / 识别全路径可用；生产识别时内置函数报错令规则体不满足（安全闭合），dry-run 与样例校验启用 `StrictBuiltinErrors` 让错误浮出。
- **标签事件检测语义**：`Detect` 将全部启用条件组装为单条 UNION ALL 查询，`SUM(cnt) > threshold` 为命中；`GroupEvents` 中 `MatchAll = false` 任一条件命中即触发、`= true` 需全部条件命中；非法表达式条件跳过而不报错。

### 数据源插件体系（Datasource）

- **插件自注册**：新增数据源类型实现 `vtab.DatasourcePlugin` 并在该类型所在文件的 `init()` 调 `vtab.RegisterDatasourceType` 自注册（重复 Key panic，启动期快速失败），禁止按类型分发 switch 或维护平行类型清单。
- **类型键唯一来源**：类型键常量（`DatasourceTypeRedis` 等，见 `internal/pkg/sqlitevtab/vtab/register.go`）是 `Datasource.Type` 的唯一来源，模型层不再复定义。
- **schema 必填与预检边界**：表型数据源（mysql / postgres / clickhouse / sqlite_file）与 mongo 的 `schema` 必填，注册期不再自动自省；redis / etcd 结构固定由实现给出。`/datasource/test`、`/datasource/introspect` 为纯预检——不落库、不注册 vtable，失败置响应体而非 HTTP 5xx。
- **重载以存库 config 为准**：`Reload` / `SyncOne` 以存库 config 重调 `Pools.Register`；存量缺 `schema` 的行重载必然失败，需重建。
- **命名空间隔离与池化**：每个命名空间独立 `SQLitePool`，vtable 以裸名 `{name}` 注册、凭据不串扰；第三方连接经 `internal/pkg/resourcepool` 句柄池化、取数即还，池 Key 不含密码。
- **超集契约**：表型下推仅为优化，返回必须是真实匹配行的超集（由 sqlite 逐行复查）。
- **原件加密留存**：`sqlite_file` 原件经 `s3.ObjectCache` 加密留存，`Materialize` 按需拉取解密；未启用 S3 时注册报错。

### 采集与流量（Capture & Traffic）

- **抓包统一经 `wire.Wire`**：以 `Name` / `LinkType` / `SetBPFFilter` / `ZeroCopyReadPacketData` / `Close` 为契约，由 `wire.NewWire` 选路（Linux XDP Native → Generic → afpacket；Darwin 与其他走 libpcap）；禁止直接 `pcap.OpenLive`。
- **先落盘再上传**：Zstd + AES-GCM 流式写出 `.zkcn` 分片，对象键 `pcap/{namespace}/{name}/{version}/{unixmilli}_{sessionID}.zkcn`。
- **平台一致性经 gRPC**：以 `reporter_v1` 的 Watch / Feedback 交互；配置存 etcd、元数据存 DB，写操作需双写。
- **状态观测键**：`{GLOBAL}/reporter/{namespace}/{name}/{version}/sts/{sessionID}`（带租约）；一资源一行，取 `timestamp` 最大的会话。
- **流量审计只读**：独立路由组 `/api/v1/traffic`，只读查询 raw 库（ClickHouse），强制入库时间范围（`created_at` 分区裁剪），`timestamp` 采集时间选填；加密字段以 base64 密文、S3 body 以对象键返回，明文查看走 `/api/v1/plaintext/crypto` 与 `/api/v1/plaintext/s3`。
- **网络堆栈定位**：`network_stacks` 的 `payloadStartOffset` / `payloadEndOffset` 与 `HttpTransport.AppTailer` 的方向 offset 相交；堆栈以 `traceId` 关联。
- **ClickHouse 过滤走跳过索引**：高基数字段 `bloom_filter(0.02) GRANULARITY 4`，低基数字段 `set(N) GRANULARITY 1`；不扩展 `ORDER BY`。

### 动作接口批量化（Batch Operations）

- **集合化动作路径**：可集合化动作（`delete` / `enable` / `disable` / `activate` / `deactivate`）统一 `POST /{resource}/{action}`、标识符入 body；`join-group` / `leave-group` / `password/reset` / `frontend-config/reset` 等非集合化动作保留路径标识符（见 Key Rules #7）。
- **入参 `views.FlexList[T]`**：由 `internal/manager/views/batch.go` 反序列化，同时接受标量与数组；`views.ValidFlexLen` 约束长度 1..100，越界返回 400。字段名按资源而定：user 用 `unionIds`（`serializer.HexUUID`），group / namespace 用 `names`，capturer / tag-rule / tag-rule-sample / tag-event-rule 用 `ids`。
- **出参 `views.BatchResultOut`**：`{Succeeded, Failed []BatchFailureOut}`，经 `views.BatchExecute` 逐条独立执行、失败不中断并汇总；HTTP 200 仅表示请求已处理，不代表全部成功。
- **幂等语义**：不存在项与已处于目标状态项计入 `succeeded`；引用冲突 / 校验失败 / DB 错误计入 `failed`。命名空间类资源恒按中间件注入的 `namespace_name` 隔离，不随 body 传入。
- **权限点与自省**：权限点同步为去标识符段路径（`api:{FullPath}` + `POST` / `OPTIONS`，见 `rbac_policy_templates` / `rbac_policies`）；每个动作接口注册 `OPTIONS` 自省（`WithBodyInput` + `WithBodyOutput[BatchResultOut]`）。
- **前端**：`SearchTable.vue` 传 `batchActions` 时启用勾选列与批量栏（暴露 `clearSelection` / `selectItems`）；单条行操作与批量走同一接口（body 传标量或单元素数组）。

### 列表字段投影（fields / detail）

- **请求契约**：全部 `POST /xxx/list` 支持可选 `fields: []string`（camelCase，与列表项 JSON 键一致）；请求非空即投影，缺失 / 为空退回类型化全字段 Out。
- **后端投影**：searcher 实现 `utils.FieldProjector`（`GetFields()` + `ResolveSelectColumns()`）；`BuildSearchDataQuery` 在解析出非空列时 `Select(columns...)`（count 不受影响），`BuildSearchQuery` 投影时在投影列尾追加 `COUNT(*) OVER() AS total` 以保持 total 契约。
- **`ResolveSelectColumns` 语义**：`GetFields()` 为空 → 返回 nil（不投影）；白名单（`search.toml` 的 `selectFields`）为空 → 返回空列；否则按白名单解析（未知字段静默忽略、去重，白名单含 `id` 时恒含 `id`）。
- **`selectFields` 完整性**：`search.toml` 中全部 `ListGet` 类型必须配置（fieldName 为 Out 的 JSON 键、column 为数据库列、含 `id`；`NetworkStackListGet` 无 `id`），且须覆盖该类型 Out 的全部 JSON 键，由视图层完整性测试（反射 `GetOptions().Fields` 与 Out 键集合双向比对）fail-closed 兜底。
- **同行列禁止循环回填**：列表项字段凡取自**本行**的数据库列，必须写进该类型 `selectFields` 由 SQL 直接选列；禁止在 handler 中 `Find` 后用 `for` 循环为每条 item 赋值。原因：`ResolveSelectColumns` 会按白名单裁剪 `Select`，漏配的列不会被查出，循环补值不仅掩盖漏配，还常把「假定值」（如调用方命名空间）当真值返回。此前 `KnowledgeDocumentListGet.namespaceName` 即此误（漏配白名单 + 循环回填调用方命名空间），已改为白名单选列。
- **跨行 / 聚合补全（允许，占位列）**：字段值不在本行（关联表名称、关联表聚合计数，如 `channelName`、`targetCount`）时，允许 handler 第二趟批量补全：去重外键 → 一次性 `IN` 查询 / `GROUP BY` 聚合 → map 回填，禁止逐条查询（N+1）。此类字段仍须在 `selectFields` 声明，`column` 取本表**真实存在**的列作占位（如 `channelName -> channel_id`、`targetCount -> id`），并在 `search.toml` 邻近处加注释说明「由处理器第二趟回填」。
- **输出形态**：投影路径经 `utils.ProjectSearchItems[T]` / `utils.ProjectItemFields` 产出 `utils.SearchProjectedOut{total, items []map[string]any}`（每项键 = 请求字段 ∩ 白名单 ∪ `id`）；`SearchOptions.Fields` 经 `OPTIONS` 自省下发。
- **流量三类型的 `id` 与详情**：`HttpTransport` / `PacketIPv4TCP` / `PacketIPv6TCP` 新增 `id`（`serializer.HexUUID`，写路径生成）；行详情 / 收藏样例 / 堆栈下钻改走 `POST /traffic/{type}/detail`（入参 `{namespace, id}`，未命中 404）拉全字段。
- **前端**：`SearchTable.vue` 的 `projectFields`（默认 `(visible) => ["id", ...visible]`）决定随请求发送的 `fields`；行操作依赖隐藏列时页面须覆写补齐。
- **列展示语义复用 OPTIONS 自省**：JSON / 加密 / S3 / hex / 字节数等展示语义不是数据类型——在 Out 结构体上以 `form:"kind=json|crypto|s3|hex|decByte"` 声明（统一 `k=v` 写法，冒号写法会 panic）；语义随 `meta.bodyOutput.fields[].kind` 下发，前端 `SearchTable` 经 `utils/meta` 的 `fieldMetaOfMeta` / `fieldKindSetsOfMeta` / `fieldRenderTypeOfMeta` 写入表头、分组与渲染类型。加密 / S3 列由 `SearchTable` 自动渲染 `EncryptedValueCell` 并经 `view-plaintext` 事件交由页面解密，页面不得再为单列写 `cell-<key>` 插槽；纯展示列交 `AdaptiveCell`。不得把 `type` 伪造成 `json`，也不得在前端硬编码字段清单或字段类型映射重复实现展示。

---

## Vue Frontend Conventions

### Tech Stack

Vue 3.5 (Composition API) + Vuetify 3 (Material Design 3) + Pinia + vue-i18n + vue-router + Axios + ECharts + vee-validate (yup)

### File Naming

| Category | Style | Example |
|----------|-------|---------|
| Vue components | PascalCase | `AppHeader.vue`, `TempEditableDialog.vue` |
| Page components | lowercase-hyphen | `user.vue`, `task.vue` |
| TypeScript utilities | camelCase | `base.ts`, `i18n.ts` |
| Stores | camelCase | `app.ts`, `user.ts` |
| API modules | camelCase | `admin.ts`, `base.ts` |
| Type definition files | `@types/` dir | `@types/admin.ts` |

### Script Setup

`<script setup lang="ts">` 必须带 `name` attribute；Vue API（`ref`、`computed`、`watch`、`onMounted` 等）已自动导入，禁止手动 import。

```vue
<script setup lang="ts">
const searchTableRef = useTemplateRef("searchTableRef");
</script>
```

- **`ID` 后缀 prop 的模板绑定**：prop 声明为 `documentID` 时模板写 `:documentID=`，不得写成连字符化的 `:document-i-d=`——连续大写字母会被逐字母拆词，可读性极差。`eslint.config.js` 已为 `vue/attribute-hyphenation` 配置 `ignore: ["documentID", "recordID"]`，新增同类 prop 时同步追加忽略项。

### Imports

按带注释标签的块组织，类型导入一律用 `import type`：

```ts
// 第三方
import consola from "consola";

// 插件 / 工具
import { registerPlugins } from "@/plugins";

// 组件
import App from "./App.vue";

// 类型
import type { App } from "vue";
```

### API Layer

- 基础 HTTP 客户端：`axios.create()` 于 `apis/base.ts`
- 函数命名 `{resource}{Operation}`（`userListGet`）；类型命名 `{Entity}{Action}In|Out`（`SearchIn`）
- 全部使用 POST；URL 模式 `/{resource}/{identifier}/{action}`（`/admin/user/{unionID}/get`）
- 统一响应处理：`getResponse()` 回调，含 `success` / `failed` / `noResponse` / `noRequest`
- JWT 经自定义 `X-Jwtoken` 头传递（前端常量 `X_JWT_HEADER_NAME`，后端默认 `headerName: "X-Jwtoken"`，同时签发 Cookie `jwtoken`）
- 全局登录态捕获：后端仅在 JWT 无效 / 过期 / 用户不存在时返回 HTTP 401，而多数调用点省略 `failed` 回调（枚举预取、功能开关加载等），`getResponse` 会静默吞掉错误、用户只看到空数据。故 `apis/base.ts` 的响应拦截器在 401 时广播 `AUTH_FAILED_EVENT`，由 `router/index.ts` 订阅：**仅当已登录（`userStore.exists`）且不在 401 / 登录页时**复位会话（`signOut`）并跳转 401 页面（带 `redirect` 保留原路径）。拦截器只广播、不复位不跳转，避免 `apis` 层反向依赖 `router` / `stores/user`；订阅注册在 router 模块（先于应用挂载求值），以免漏掉首次导航阶段发起的请求。403（`CodeAuthorizeFailed`，权限不足）不参与跳转——权限不足不等同于会话失效

### i18n

- YAML 文件在 `src/locales/`：`en.yaml` / `zhHans.yaml`；点号层级 key（`model.user.field.username`）
- 模板：`{{ $t("key.path") }}`；脚本：`i18n.global.t("key.path")`
- 文案内嵌**字面花括号**（如 JSON 示例 `{"Authorization":"Bearer xxx"}`）必须用 vue-i18n 字面量语法包裹：写成 `{'{"Authorization":"Bearer xxx"}'}`。裸 `{` 会被消息编译器当作插值占位符，构建期报 `Invalid token in placeholder`（code 2）；双写 `{{...}}` 同样非法（code 9 `Not allowed nest placeholder`）。YAML 标量须加引号（单引号风格下把内层 `'` 双写为 `''`，以 prettier 输出为准）
- 语言切换通过 `[language]` 动态路由段

### i18n 模型翻译组织规范（model.*）

语言文件以 `model` 顶层键组织模型翻译，结构如下：

- `model.__base`：跨模型通用键，按用途分子结构：
  - `field`：通用字段（如 `date`、`action`、`field`、`startTime`、`value`、`encrypted`、`timestampRange`、`createdAtRange`）
  - `content`：通用文案（如 `condition`、`doubleCheck`）
  - `value`：通用值（如 `blank`、`null`、`undefined`）
- `model.<entity>`：每个模型实体一个块（`user`、`task`、`ipv4Tcp` 等），块内固定子结构：
  - `name`：模型名称（必填）
  - `field`：模型字段，键与 API JSON 字段一致（必填）
  - `type`：模型枚举/子类型，键为枚举值（仅当模型具有类型属性时）
- 不设模型组层级：同属一个业务域的数据类型（如流量审计的 ipv4-tcp / ipv6-tcp / http-transport）是三个独立模型实体 `model.ipv4Tcp` / `model.ipv6Tcp` / `model.httpTransport`，与 `user`、`tagEventCondition` 等同级，各自含完整 `name` + `field` 子结构；组名（如 `traffic`）不得作为 model 下的中间层，页面级通用标签（如时间范围筛选）归入 `model.__base.field`
- 字段键命名：一律与 API JSON 字段名对齐，采用开头小写驼峰命名法（`createdAt`、`sessionId`、`ipv4Src`、`reqHdrUserAgent`）；网络模型（`internal/models/network` 下 PacketIPv4TCP / PacketIPv6TCP / HttpTransport 等）的 Go json tag、前端语言包字段键、权限点模板（`plaintext:{crypto|s3}:{model}:{field}`）与明文查看接口的 field 参数必须使用同一套 camelCase 键，仅数据库列名（gorm `column` tag、ClickHouse SQL、`search.toml` 的 `column`）保留 snake_case
- 块内可用 `# 注释` 对字段分组（如 `# ---- 列设置全量字段（默认隐藏）----`）
- 禁止裸键：`model.<entity>` 下不得出现脱离 name/field/type 子结构的孤立键（曾出现 `model.network.httpTransport` 裸键，已并入 `model.httpTransport`）
- 中英文语言文件结构必须一一对应，新增/删除/重命名键需同步维护

### Pinia Stores

Options Store 风格（`state` + `actions`）：

```ts
export const useAppStore = defineStore("app", {
  state: () => ({
    theme: localManager.getTheme(),
    language: localManager.getLanguage(),
  }),
  actions: {
    asLanguage(language?: SupportLanguage) { ... },
  },
});
```

### Form Validation

- `vee-validate` + `yup`，校验 schema 放 `@/schemas/`
- `useForm` + `defineField` 模式，配 `vuetifyCfg` 适配器；提交逻辑用 `handleSubmit` 包裹
- 对话框表单的复位统一放在**打开动作**里（与填充默认值 / 回填详情同一处），不在关闭时复位：复位本就是「准备本次打开的数据」的一部分，挂在打开路径上就天然覆盖关闭按钮 / ESC / 点击遮罩 / 程序化关闭等所有关闭方式，且不会在关闭淡出动画中把表单清空。创建类表单更必须如此——首次打开之前不存在任何关闭事件，默认值只能由打开动作写入。
- 复位须用原子 API `resetForm({ values })`（或 `resetForm()`），一次性重置值 + `errors` / `touched` / `dirty` / `validated` / `submitCount`；不要逐字段 `resetField`，它不重置表单级校验状态，且要求枚举全部字段，容易漏项。
- 注意字段状态归**调用方组件**所有：`defineField` 在页面/面板的 setup 中调用，故对话框关闭不会卸载字段，残留的值与校验状态必须由调用方显式复位。
- 对话框组件（`TempEditableDialog`）只负责开关与自身可编辑态（任何关闭途径都要退出可编辑态），不承担表单复位职责。

### String Input Trimming 字符串输入去空白

- 所有单行文本输入框（用户名、邮箱、昵称、名称、标题、Cron 表达式、搜索关键字等）在提交或作为查询条件前**必须去除首尾空白字符**（trim），避免用户误输入不可见的空白字符导致登录失败、查询无结果或创建出看似相同实则不同的记录。
- 强制豁免（禁止 trim）：
  - **密码字段**：密码可合法包含首尾空格，trim 会改变凭据本身
  - **多行文本域（`v-textarea`）**：内容中的空白具有语义（模板正文、表达式/代码、长描述）
  - **校验性输入**（如 `ConfirmDialog` 的双重确认随机串）：仅用于精确比对，不属于数据输入
- 推荐实现方式：优先在 `@/schemas/` 的 yup schema 上对单行字符串字段统一使用 `.transform((v) => v?.trim())` 做规范化，避免散落在各页面提交逻辑中；未走 schema 的搜索/筛选字符串输入（`AdaptiveField`、`ScrollSearchField` 等）同样应在使用前 trim。

### 样式体系分层（Style Layering）

本文件是前端设计系统的**唯一信任源**（完整口径与不变式见文末「单一信任源（Single Source of Truth）」一节）。下列源文件是规范的**实现载体**：

| 载体 | 职责 |
|------|------|
| `web/src/plugins/vuetify.ts` | 主题色（`theme.colors`）与组件默认值（`defaults`） |
| `web/src/styles/tokens.scss` | 设计令牌：字体 / 字号 / 字重 / 间距 / 圆角 / 边框 / 高度 / 宽度 / 阴影 |
| `web/src/styles/main.scss` | 全局样式与 `.zk-*` 复用外壳类 |
| `web/src/styles/settings.scss` | Vuetify SASS 覆写（`$border-radius-root` / `$body-font-family` 等） |
| `web/src/components/base/AppDialog.vue` | 对话框骨架 |
| `web/src/components/base/ExpansionPanelTitle.vue` | 展开面板标题区骨架 |
| `web/src/components/base/ResizeHandle.vue` | 可拖拽边框手柄（握把视觉 + 指针拖拽逻辑） |
| `web/src/utils/editorHighlight.ts` | 编辑器语法高亮配色 |
| `web/src/utils/chartTheme.ts` | 图表主题合成 |
| `web/src/utils/mermaidTheme.ts` | 助手 Markdown 内 mermaid 图表主题、强调节点配色与判断节点连线吸附 |
| `web/src/layouts/{MainLayout,CentralLayout}.vue`、`web/src/components/base/AppHeader.vue` | 布局骨架 |

样式按**四层**组织，上层只能引用下层，禁止越层就地造值：

| 层 | 位置 | 职责 | 允许内容 |
|----|------|------|----------|
| 1 令牌层 | `web/src/styles/tokens.scss` | **唯一数值来源** | `$zk-*`（SASS）+ `--zk-*`（CSS 变量）双导出：字体族、字号档位、字重、控件高度、工具栏高度、表格行高、4px 间距栅格、圆角四档、hairline 边框、对话框宽度四档、容器宽度、内容尺寸约束、可拖拽握把、编辑器外壳、面板表面透明度、z-index、过渡时长、阴影 |
| 2 主题层 | `web/src/plugins/vuetify.ts` | **唯一颜色来源** + 组件默认值 | `theme.colors`（导出为 `--v-theme-*`）+ `defaults`（组件 `variant` / `density` / `rounded` / `elevation` / `maxWidth`） |
| 3 外壳层 | `web/src/styles/main.scss` | 跨组件复用外观 | `.zk-*` 复用类 + Vuetify / CodeMirror 内部类覆盖 |
| 4 组件层 | `*.vue` 的 `<style scoped>` | **仅**布局定位 | flex / grid、尺寸约束、溢出、定位 |

要改视觉先改对应的唯一来源：

- **数值**（字号 / 间距 / 圆角 / 边框 / 阴影 / 高度 / 宽度档位）→ `web/src/styles/tokens.scss`，新增视觉数值必须先在此登记令牌
- **颜色**（含语义色与文字色）→ `plugins/vuetify.ts` 的 `theme.colors`，消费方式 `rgb(var(--v-theme-xxx))`
- **跨组件复用外观**（面板 / 编辑器外壳 / 区块标题 / 提示文本）→ `styles/main.scss` 的 `.zk-*` 类
- **编辑器语法高亮配色** → `utils/editorHighlight.ts`（`LIGHT_PALETTE` / `DARK_PALETTE` / `sqlHighlightFor`），禁止新增第二套配色
- **助手 Markdown 内 mermaid 图表配色** → `utils/mermaidTheme.ts`（`mermaidConfigFor` / `decorateMermaidSvg`），禁止在组件里就地写图表色值。强调节点由模型在源码里用 `classDef` 的 `zkKey` / `zkWarning` / `zkError` 标注（约定同时写在 `mermaid_check` 工具说明里），`decorateMermaidSvg` 把它们统一映射为 primary / warning / error 强调色；连线走向沿用 mermaid 原生渲染，不做端点吸附或路径改写

外壳层（`main.scss`）的 `.zk-*` 类按用途分组，下表为职责摘要，**完整清单以 `styles/main.scss` 为准**：

| 族 | 类 | 职责 |
|----|-----|------|
| 边框 | `.zk-hairline` / `-top` / `-right` / `-bottom` | 四边或单边 1px hairline 分隔 |
| 面板 | `.zk-panel`（+ `--subtle`）、`.zk-code-preview`、`.zk-interactive-card`、`.zk-tabs-embedded` | 表面容器（hairline + `radius-md`）；弱化面板用 `on-surface` 6% 底 + `radius-sm`（代码块、键值行、预览）；可点卡片 hover 转 `primary` 描边；嵌入标签页压平内层 `v-card` |
| 排版 | `.zk-section-title` / `.zk-section-subtitle` / `.zk-editor-title` / `.zk-hint` / `.zk-error-text` / `.zk-label-caps` / `.zk-mono-emphasis` / `.zk-metric` / `.zk-step-badge` | 区块标题、次级标题（主标题旁的同级弱化标题，如对话框标题后的实体名）、编辑器标题、提示、字段错误、分组标签、技术标识强调、指标值、步骤徽标 |
| 编辑器 | `.zk-editor`（+ `.is-error`）/ `.zk-editor-header` | CodeMirror 外壳（主题感知 hairline + `radius-sm`）与标题行 |
| 请求态 | `.zk-request-loading` / `.zk-request-spinner` | `v-request` 加载态：隐藏按钮原内容并注入 spinner 覆盖层 |
| 布局 | `.zk-page-container` / `.zk-centered-layout` / `.zk-centered-surface` / `.zk-filter-form` / `.zk-breadcrumbs` / `.zk-refresh-group` | 页面容器、居中骨架、筛选表单（压掉空提示区以保持单行）、面包屑（兼任页面上方顶隙、居中于 AppBar 与内容表面之间）、刷新分体按钮组 |
| 交互态 | `.zk-drag-over` / `.zk-row-hover` / `.zk-row-selected` | 拖拽悬停行、可点击行悬停（primary 6%）、常驻选中行（展开面板为「描边盒 + 盒内平铺条目」，盒由外层容器提供，见「展开面板族」） |
| AI 对话 | `.zk-chat-bubble`（+ `--user` / `--assistant`）、`.zk-chat-composer`（+ `--blocked`）、`.zk-chat-composer-status`（+ `--locked`）、`.zk-chat-queue-item` / `-text`、`.zk-chat-actions`（+ `--persistent`）、`.zk-chat-next-step`（+ `--disabled`）/ `-main`、`.zk-chat-compress--failed`、`.zk-chat-citation-index` / `-link` / `-ref` / `-citations-toggle`、`.zk-chat-parallel`、`.zk-chat-nav-btn` / `-count`、`.zk-chat-session-row` / `-main`、`.zk-chat-option`（+ `--disabled`）、`.zk-chat-markdown`、`.zk-chat-slot` / `-valve` / `-usage-percent` / `-composer-input` | AI 助手会话区：气泡、输入区与待发队列、消息操作行、推荐下一步、引用 / 压缩 / 并行调用、块间跳读、会话列表行、问卷选项、Markdown 预览 |
| 网络栈对比 | `.zk-stack-packet`（+ `-request` / `-response`）、`.zk-stack-header`（+ `-primary` / `-secondary`）、`.zk-stack-compare-header` / `-empty`、`.zk-clause-card` | 包卡片（灰色左侧标记条）、协议头摘要 chip、两列对齐占位、筛选子句卡片 |

同文件还注册非 `.zk-` 辅助：`.font-mono`、CodeMirror tooltip 覆写（`.cm-tooltip` / `.cm-function-info`）与示例 / 函数悬停块（`.cm-sample-hover*` / `.cm-function-hover*`）。

覆盖 Vuetify 组件类必须带 `.v-application` 前缀：

- `vite-plugin-vuetify` **延迟注入**组件样式，注入位置在 `styles/main.scss` 之后；因此外层用**单个** `.zk-*` 类去覆盖 Vuetify 组件类（与 `.v-container` / `.v-card` 等同为单类权重）**不可靠**——同权重下后者胜出，声明会被静默吞掉。
- 一律写成 `.v-application .zk-xxx`（现有先例：`.v-application .v-card`、`.v-application .zk-page-container`）。
- 反例：`.zk-page-container { max-width: 1440px }` 会被 `.v-container` 的媒体查询 `max-width`（1280 / 1920 / 2560px）覆盖，令牌在视口 ≥ 1920px 时静默失效。
- **例外——对话框 / 浮层内部禁止带 `.v-application` 前缀**：`VDialog` 内容经 `v-overlay-container` 挂载到 `body` 下，**不在 `.v-application` 祖先链内**，带前缀的选择器会完全不匹配（曾出现 `.v-application .zk-dialog-toolbar …` 静默失效，且同一原因导致对话框 `.v-card` 拿不到全局 hairline 描边）。这类规则改以**严格高于目标 Vuetify 规则的选择器权重**取胜，与注入顺序无关：如 `.zk-dialog-toolbar .v-toolbar__content`（0-2-0）覆盖 `.v-toolbar__content`（0-1-0）、`.zk-dialog-toolbar .v-toolbar__content > .v-toolbar-title`（0-3-0）覆盖 Vuetify 同构规则（0-2-0）。

量化边界：

- 颜色只取 `rgb(var(--v-theme-*))` 或 `--zk-*`，不得出现 hex / `rgb(数字…)` / `grey-*`
- 圆角只有四档：`--zk-radius-sm`(4px) / `--zk-radius-md`(6px) / `--zk-radius-lg`(12px) / `--zk-radius-pill`
- 间距只走 4px 栅格（`--zk-space-1..6` = 4/8/12/16/24/32px）；Vuetify 密度工具类（如 `pa-4` / `pa-md-6`）映射同一节奏（`pa-4` = 16px、`pa-md-6` = 24px），自定义 margin / padding 一律取令牌，禁止自由像素值
- 字号只取令牌档位：`--zk-font-size-caption`(12px) / `body`(14px) / `subtitle`(16px) / `title`(20px) / `heading`(24px)；字重只取 `--zk-font-weight-regular|medium|semibold`（400/500/600）
- 对话框宽度只有 `sm` / `md` / `lg` / `xl` 四档（`--zk-dialog-width-sm|md|lg|xl`）
- 控件高度、工具栏高度、表格行高一律取令牌（`--zk-control-height-compact|default`、`--zk-toolbar-height-compact`、`--zk-table-row-height-compact`、`--zk-table-header-height-compact`、`--zk-panel-title-height-compact`）
- 边框一律用 hairline 令牌（`--zk-hairline-width` / `--zk-hairline-opacity`，亮色主题强化值 `--zk-hairline-opacity-strong`）或 `.zk-hairline*` 外壳类
- 全屏居中页（认证页 / 错误页）的表面宽度固定 `--zk-centered-surface-width`(420px)，不随视口放大；高度上限由 `100dvh` 减去 Vuetify 布局偏移（`--v-layout-top` / `--v-layout-bottom`）推导，超出时表面内部滚动。禁止用 `vw` / 百分比表达宽度；高度也必须与内容无关（`height: 100%` 会与 `v-main` 的 `flex: 1 0 auto` 形成循环，永不收缩）

正例：

```vue
<template>
  <!-- 视觉来自外壳类与主题令牌，组件只负责布局 -->
  <div class="zk-panel pa-4 d-flex flex-column ga-2">
    <span class="zk-section-title">{{ $t("...") }}</span>
    <span class="zk-hint">{{ $t("...") }}</span>
  </div>
</template>

<style scoped>
/* 仅布局定位 */
.panel-body {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: var(--zk-space-2);
  overflow: auto;
}
</style>
```

反例：

```vue
<style scoped>
.panel {
  /* 反例：颜色字面量，主题切换后失效 */
  border: 1px solid rgb(0 0 0 / 24%);
  /* 反例：令牌外字号 */
  font-size: 13px;
  /* 反例：令牌外圆角（只允许 4 / 6 / 12 / pill） */
  border-radius: 8px;
}
</style>
```

### 设计原则（Design Principles）

本产品是运维人员在压力下使用的网络流量审计平台，界面遵循三条原则：

1. **简洁（Simplicity）**——一屏只做一件事。布局紧凑但可预期；扁平表面 + 细边框替代重阴影；禁止装饰性渐变与花饰。
2. **易用（Ease of use）**——紧凑的输入控件与表格，导航尽量让位（rail 抽屉悬停展开）。高频操作一步可达，任何状态变化都同时用**文字 / 图标**表达，而不只靠颜色。
3. **信息清晰（Clear information delivery）**——技术性取值（ID、哈希、hex、字节数）一律用等宽字体 + 表格数字（tabular numerals），保证列与哈希对齐。语义色只表达**状态**，不用于装饰。

### 色板与主题（Color Palette）

**颜色的唯一来源是 `web/src/plugins/vuetify.ts` 的 Vuetify 主题**，一律经 `rgb(var(--v-theme-*))` 消费；令牌层（`tokens.scss`）刻意**不定义任何颜色**，以保证亮 / 暗两套主题始终同步。

| Token | Light | Dark | 用途 |
|-------|-------|------|------|
| `background` | `#E6EAF0` | `#0E1116` | 页面底色（亮色刻意压暗，让白色表面浮起） |
| `surface` | `#FFFFFF` | `#151A21` | 卡片 / sheet / AppBar / tooltip 背景 |
| `surface-variant` | `#E2E7EF` | `#1D242E` | tooltip 描边、次级表面 |
| `primary` | `#2563EB` | `#3B82F6` | 主操作、选中、导航激活态 |
| `secondary` | `#64748B` | `#94A3B8` | 次操作、中性强调 |
| `success` | `#16A34A` | `#22C55E` | 成功状态 |
| `warning` | `#D97706` | `#FBBF24` | 警告状态 |
| `error` | `#DC2626` | `#EF4444` | 错误 / 破坏性状态 |
| `info` | `#0891B2` | `#22D3EE` | 信息状态 |
| `border` | `#D5DCE7` | `#262E3A` | 自定义色，供 `color="border"` 使用（输入框 / chip / badge 填充） |
| `outline` | `#B4C0D2` | `#37404F` | 自定义色，供 `color="outline"` 使用 |
| `on-background` | `#1F2937` | `#E5E7EB` | `background` 上的主文本 |
| `on-surface` | `#1F2937` | `#E6E8EC` | `surface` 上的主文本 |
| `on-surface-variant` | `#4B5563` | `#9CA3AF` | 次级文本（说明、表头） |
| `on-primary` | `#FFFFFF` | `#FFFFFF` | `primary` 上的文本 / 图标 |
| `on-secondary` | `#FFFFFF` | `#0F172A` | `secondary` 上的文本 / 图标 |
| `on-success` | `#FFFFFF` | `#052E16` | `success` 上的文本 / 图标 |
| `on-warning` | `#FFFFFF` | `#422006` | `warning` 上的文本 / 图标 |
| `on-error` | `#FFFFFF` | `#FFFFFF` | `error` 上的文本 / 图标 |
| `on-info` | `#FFFFFF` | `#083344` | `info` 上的文本 / 图标 |

**边框与描边**：主题额外注册了 `border` / `outline` 两个自定义色，仅用于 `color="border"` / `color="outline"` 场景；**分隔线与卡片描边禁止硬编码 hex**，一律用 `on-surface` 半透明 tint（与 Vuetify 自身的 border-opacity 语义一致）：

```scss
border: var(--zk-hairline-width) solid rgb(var(--v-theme-on-surface), var(--zk-hairline-opacity));
```

| Hairline 令牌 | 值 | 用途 |
|---------------|-----|------|
| `--zk-hairline-width` | 1px | 分隔线、卡片 / 抽屉 / AppBar 描边 |
| `--zk-hairline-opacity` | 0.12 | 主值，暗色主题使用 |
| `--zk-hairline-opacity-strong` | 0.16 | 亮色主题强化值，让白卡片与浅灰页底分离 |

外壳层已注册 `.zk-hairline` / `.zk-hairline-top` / `.zk-hairline-right` / `.zk-hairline-bottom` 与全局 `.v-card` 规则，可复用场景不必重述边框。

**对比度**：文本颜色只取 `on-surface`（主文本）与 `on-surface-variant`（次级 / 说明文本），两者在各自主题的 `surface` 上均满足 WCAG AAA（≥ 4.5:1）：

| 颜色 | 亮色（`surface` `#FFFFFF`） | 暗色（`surface` `#151A21`） |
|------|------------------------------|------------------------------|
| `on-surface`（主文本） | ≈ 14:1 | ≈ 14:1 |
| `on-surface-variant`（次级 / 说明文本） | ≈ 7.5:1 | ≈ 7.0:1 |

### 字号与排版（Typography）

字体栈定义在 `tokens.scss`（`$zk-font-family` / `$zk-font-mono`，导出为 `--zk-font-family` / `--zk-font-mono`）：

```scss
$zk-font-family: "Roboto", "PingFang SC", "Hiragino Sans GB",
  "Microsoft YaHei", "Noto Sans SC", system-ui, -apple-system, "Segoe UI",
  sans-serif;

$zk-font-mono: ui-monospace, "SF Mono", sfmono-regular, menlo, consolas,
  "Liberation Mono", monospace;
```

`styles/settings.scss` 把同一字体族接到 Vuetify 的 `$body-font-family` / `$heading-font-family`，全部组件自动继承。

**字号档位**（禁止在组件里写裸 `font-size`）：

| 令牌 | 值 | 角色 |
|------|-----|------|
| `--zk-font-size-caption` | 0.75rem (12px) | 提示、表头、时间戳与次级元信息 |
| `--zk-font-size-body` | 0.875rem (14px) | 正文、表格单元格、字段标签 |
| `--zk-font-size-subtitle` | 1rem (16px) | 区块标题（`.zk-section-title`） |
| `--zk-font-size-title` | 1.25rem (20px) | 块标题 |
| `--zk-font-size-heading` | 1.5rem (24px) | 页面标题 |
| `--zk-editor-font-size` | 0.8125rem (13px) | CodeMirror 编辑器正文 |

**字重**：`--zk-font-weight-regular`(400) / `--zk-font-weight-medium`(500) / `--zk-font-weight-semibold`(600)。

**控件与 chrome 高度**：

| 令牌 | 值 | 用途 |
|------|-----|------|
| `--zk-control-height-compact` | 40px | 输入控件，`density="compact"`（全站默认） |
| `--zk-control-height-default` | 48px | 输入控件，`density="default"` |
| `--zk-toolbar-height-compact` | 48px | 紧凑工具栏（对话框标题栏） |
| `--zk-table-row-height-compact` | 36px | 紧凑数据表行高 |
| `--zk-table-header-height-compact` | 40px | 紧凑数据表表头高 |

**已由 `main.scss` 集中承载的排版规则**（页面不得重复声明）：

| 场景 | 取值 | 位置 |
|------|------|------|
| 正文基线 | body 字号 + `font-variant-numeric: tabular-nums` | `main.scss` `body` |
| 数据表表头 | caption 字号、600、uppercase、`letter-spacing: 0.05em`、`on-surface-variant` | `main.scss` `.v-data-table th` |
| 区块标题 | `.zk-section-title`（subtitle + 600 + `on-surface`） | `main.scss` |
| 提示文本 | `.zk-hint`（caption + `on-surface-variant`） | `main.scss` |
| 字段错误文本 | `.zk-error-text`（caption + `error`） | `main.scss` |
| 分组标签 | `.zk-label-caps`（caption + 600 + uppercase + `on-surface-variant`） | `main.scss` |
| 技术性取值 | `.font-mono`（等宽栈 + tabular numerals） | `main.scss` |

行高跟随 Vuetify 组件默认值，不另立一套 line-height scale。

**无效值占位符**：字段在当前上下文无意义 / 不适用时（如非向量档位的「向量维度」、未填写的可选值），统一以 em dash「—」占位（配 `class="zk-hint"`），与正常取值区分。该字符经 `utils/base.ts` 的 `INVALID_PLACEHOLDER` 引用，禁止就地写 `0` / 空串 / 其它符号充当无效值。

**`.font-mono` 用途**：ID（union ID / rule ID / task ID）、哈希、十六进制值、字节 / 包计数：

```vue
<span class="font-mono">{{ packet.hex }}</span>
```

正文自身已设 `font-variant-numeric: tabular-nums`，数字默认即对齐。

### 圆角、阴影与扁平化（Radius / Shadow / Elevation）

圆角**只有四档**（`tokens.scss`），`8px` 之类一律违规，须就近取令牌：

| 令牌 | 值 | 典型用途 |
|------|-----|----------|
| `$zk-radius-sm` / `--zk-radius-sm` | 4px | 小元素、代码块、滚动条滑块、编辑器外壳 |
| `$zk-radius-md` / `--zk-radius-md` | 6px | 输入控件、按钮、面板（同时是 Vuetify `$border-radius-root`） |
| `$zk-radius-lg` / `--zk-radius-lg` | 12px | 卡片、对话框 |
| `$zk-radius-pill` / `--zk-radius-pill` | 999px | 胶囊（badge、计数标记、步骤徽标） |

- Vuetify 根圆角在 `styles/settings.scss` 覆写为 **6px**（`$border-radius-root: 6px`），所有输入框与按钮自动继承。
- 卡片经 `main.scss` 的全局 `.v-card` 规则渲染为 **12px**。
- 对话框经 `vuetify.ts` 默认值取 `rounded="lg"`（12px）；菜单取 `rounded="md"`（6px）。

**扁平是默认。** 卡片与 AppBar 用 `elevation="0"` + 1px hairline 描边替代重阴影：

```scss
.v-application .v-card {
  border: var(--zk-hairline-width) solid rgb(var(--v-theme-on-surface), var(--zk-hairline-opacity));
  border-radius: var(--zk-radius-lg);
}
```

`VCard: { elevation: 0 }`、`VToolbar: { density: "compact", elevation: 0 }`、`VAppBar: { elevation: 0 }` 均为 `vuetify.ts` 默认值，用法处无需再写 `elevation`。AppBar 的分隔线来自 `.zk-hairline-bottom`（`AppHeader.vue`）。

仅菜单 / 气泡 / 拖拽浮层这类罕见的抬升层使用阴影令牌（按暗色主题调校）：

| 令牌 | 值 |
|------|-----|
| `$zk-shadow-sm` | `0 1px 2px rgb(0 0 0 / 20%), 0 1px 3px rgb(0 0 0 / 30%)` |
| `$zk-shadow-md` | `0 2px 4px rgb(0 0 0 / 24%), 0 4px 12px rgb(0 0 0 / 32%)` |

优先 `$zk-shadow-sm`；禁止在同一元素叠加多层阴影。

### 组件与变体约定（Component & Variant）

- **默认值集中声明**：凡 `plugins/vuetify.ts` 的 `defaults` 已覆盖的属性（`variant` / `density` / `rounded` / `elevation` / `maxWidth` / `size` / `inset`），用法处**禁止**重复声明。默认值全清单见本节末「组件默认值清单」，与 `plugins/vuetify.ts` 的 `defaults` 对象逐项对应。
- **开关统一内嵌轨道**：全站 `v-switch` 由 `defaults` 固定为 `size: "small"` + `inset: true`（内嵌轨道胶囊 + 缩小滑块的经典形态），用法处**禁止**再声明 `size` / `inset`——否则会在同一页面内出现细轨道与胶囊轨道并存的两种形态。`density` 不在其中：抽屉工具条、条件卡片等紧凑容器内仍按容器高度就地声明 `density="compact"`。先例反例：`pages/[language]/admin/capturer.vue`（曾与 `notification/*` 各页开关形态不一）。
- **一个功能族一个变体基线**：同一功能族的组件（全部对话框、全部 CodeMirror 编辑器、全部信息面板、全部 chip、全部日期时间字段）共享同一骨架与同一视觉基线；需要变体时必须**成对定义**（如 `size: "sm" | "md" | "lg" | "xl"`）并在样式层集中实现，禁止各组件内自写一套。
- **输入族变体基线**：所有输入类组件（`VTextField` / `VSelect` / `VAutocomplete` / `VCombobox` / `VTextarea` / `VNumberInput` / `VFileInput` / `VField`）必须**全部**在 `plugins/vuetify.ts` 的 `defaults` 中登记 `variant: "outlined"` + `density: "compact"`。理由：Vuetify 这些组件**自带的默认 variant 是 `filled`**，漏登记不会报错，只会静默退化为灰色填充，与同族 outlined 控件并排时明显分裂（`VNumberInput` / `VFileInput` 曾长期如此）。新增或升级 Vuetify 输入组件时必须同步补登记，并在本节末「组件默认值清单」中更新对应行——该清单与 `defaults` 对象必须逐项一致。
- **扩展面板单展开**：`VExpansionPanels` 已由 `plugins/vuetify.ts` 的 `defaults` 固定为 `variant: "accordion"`，语义是**同一时刻只展开一项**，用法处**禁止**声明 `multiple`。理由：`multiple` 与 `variant` 在 Vuetify 内是**正交**的两组属性（`variant` 只落 `--variant-accordion` 样式类，是否允许多开只由 `multiple` 决定），一旦声明就只保留手风琴外观、却突破其单开语义，出现多个面板同时展开；同一手风琴内的 `v-model` 也必须是**单值**（无展开项时为空串），不得声明为数组。先例：`components/admin/AgentParamPanel.vue`、`pages/[language]/ai/knowledge.vue`。
- **展开面板标题区**：全部 `v-expansion-panels` 的标题一律走 `web/src/components/base/ExpansionPanelTitle.vue`（图标头像 + `.zk-section-title` 标题 + `.zk-hint` 说明；右侧 `actions` 槽插在组件统一渲染的 chevron 之前），**禁止**在用法处自写 `v-expansion-panel-title` 结构或手写 chevron。状态类图标需要旋转动效（运行中 / 加载态）时走组件的 `spin` prop（复用 `@mdi/font` 的 `.mdi-spin` 动画），用法处不得另挂图标。先例：`pages/[language]/profile.vue`、`components/admin/AgentParamPanel.vue`、`components/admin/RegoDryRunPanel.vue`、`components/admin/TagEventRuleDryRunPanel.vue`、`pages/[language]/ai/knowledge.vue`、`components/forms/DatasourceGuidePanel.vue`。
- **标题行元信息收敛为 icon + tooltip**：阶段 / 条目级的运行元信息（模型名、耗时、Token 用量、失败原因等）**禁止**常驻标题行——长文本会挤占并压缩标题可读性。一律收敛为标题行 `actions` 槽内的 info 图标（`mdi-information-outline`，`size="x-small"`，`color="primary"`），hover 经 `v-tooltip` 按需展示、逐行左对齐；无内容时不渲染图标。tooltip 宽度用 `max-width` 属性限制（浮层内容为行内样式，CSS 类覆盖无效）。阶段级元信息（尤其模型名）必须由流式阶段事件（`ai.StageEvent`）随每次阶段状态实时携带，**禁止**只依赖收尾汇总（`DryRunMeta.Calls`）——否则运行中 hover 取不到值。明细每一项统一写作「**标签: 值**」（含模型、耗时、输入 / 输出 / 合计 Token、失败原因），不得混用「标签 值」的空格分隔——同一组文案（如 `metaTotalTokens`）在 tooltip 与汇总脚注等多处复用时，写法必须一致，否则读者无从判断字段边界。先例：`pages/[language]/ai/knowledge.vue`（检索试运行阶段元信息 + 汇总脚注）+ `internal/ai/knowledge/evaluate.go` 的 `evaluate` 经 `emit` 统一注入 `StageEvent.Model`。
- **展开面板族 = 描边盒 + 盒内平铺条目**：全站 `v-expansion-panels` 一律呈现「一个 hairline 描边盒，盒内以面板间 1px 分隔线分条」，条目自身无边框、无悬浮阴影。描边盒**只能由外层容器提供，且恰好一层**（`.zk-panel` 包裹 div、`TabView` 卡片、`.zk-editor` 外壳三种皆可，按所在页面结构择一）；`v-expansion-panels` 自身**禁止**再带 `.zk-panel` / `.zk-editor` / `.zk-hairline` 等描边类（曾出现面板组自身带 `.zk-panel`、外层参考栏也带 `.zk-panel` 的双层框，以及「有阴影 / 无边框平铺 / 有边框」三种观感并存）。`.v-expansion-panel__shadow` 抬升阴影已由 `styles/main.scss` 全局 `display: none`；`.v-expansion-panel-title__icon` 亦由 `styles/main.scss` 全局补 `align-items: center`（Vuetify 未声明该属性，图标为固定尺寸无法被 stretch，会退化为顶部对齐，致 `x-small` 元信息图标与展开箭头垂直错位）。两处覆盖均用「两个类」选择器（0-2-0）以在浮层内同样命中，用法处不得覆写或重新引入。其余外观（标题 `16px 24px` / 48px、展开态 64px；内容 `8px 24px 16px` 内边距；手风琴 variant 下展开项 `margin-top: 0`）保持 Vuetify 默认，禁止在用法处覆写内边距 / 底色 / hover 遮罩。标题区存在唯一变体：`ExpansionPanelTitle` 的 `size="sm"`——收起态压到单行（最小高度取令牌 `--zk-panel-title-height-compact`，纵向内边距 4px，标题与说明并排、头像缩至 24px），只收纵向与最小高度、横向仍为 24px，以维持「标题头像左边缘 ↔ 展开后正文左边缘」的对齐；样式实现在 `styles/main.scss` 同一区块的 `.zk-expansion-title--sm`（展开态需连带 `.v-expansion-panel--active` 祖先才压得过其 `min-height: 64px`），仅供**默认收起、仅作收纳入口**的折叠块使用（`components/ai/AssistantDrawer.vue` 的思考过程——它绝大多数时候不被展开），其余面板一律保留 `md` 默认值。该思考过程的展开区（思考正文）自带限高滚动，上限取专用令牌 `--zk-reasoning-max-height`（全站长文本预览上限的 1/3，**禁止**改回 `--zk-preview-max-height`——思考是流式增量长文本，独占一屏高度会把正文挤出视口）；流式增长期间每段增量到达后由 `scrollReasoningToBottom` 把该滚动区贴到底部，使最新思考始终可见（消息区的 `scrollToBottom` 到不了它内部）。先例：`pages/[language]/profile.vue`（包裹 div 提供盒）、`components/admin/AgentParamPanel.vue`（TabView 卡片提供盒）、`components/admin/{Rego,TagEventRule}DryRunPanel.vue`（`.zk-editor` 外壳提供盒）、`components/forms/DatasourceGuidePanel.vue`（外层参考栏提供盒，根节点已去掉自身描边）。
- **对话框**：一律走 `web/src/components/base/AppDialog.vue` 骨架（紧凑工具栏「图标 + 标题 + 关闭图标按钮」+ `VDivider` + 可滚动主体 + 可选右对齐页脚）；宽度只取 `size` 四档（映射 `--zk-dialog-width-sm|md|lg|xl`），禁止自由 px。例外：全屏 + 底部过渡形态的 `NetworkStackDialog.vue` 保留原生 `VDialog`。
- **可拖拽边框**：可拖拽边框一律走 `web/src/components/base/ResizeHandle.vue`（`v-model` 尺寸 + `side` 四边 + `placement` 内 / 外 + `min` / `max`），禁止在用法处自写指针拖拽逻辑。组件贴合父容器指定边，父容器须 `position: relative`；`side` 为 `left` / `right` 调宽、`top` / `bottom` 调高；`placement` 取 `inside`（默认，命中条贴容器边框内侧，占容器内容区 8px）或 `outside`（命中条越过边框贴到容器外侧，不占内容区）——`outside` 要求该方向上的祖先**不得裁剪溢出**（如 `VNavigationDrawer` 的内容区为 `overflow-x: hidden`，在其上写 `outside` 会被整个裁掉、既看不见也点不到），故仅用于本身不裁剪、且外侧有可视空间的容器。常驻握把短条的**朝向随轴向切换**，让用户静态即可分辨能力边界——**左右边框为竖条（可拖宽，光标 `col-resize`）**、**上下边框为横条（可拖高，光标 `row-resize`）**，悬停 / 拖拽时转 `primary`；握把的尺寸与常态不透明度取令牌 `--zk-resize-grip-length` / `--zk-resize-grip-thickness` / `--zk-resize-grip-opacity`，禁止在组件外自定义握把外观。先例：`components/ai/AssistantDrawer.vue`（左缘调宽，`min` 320 / `max` 取视口与设计上限收敛值）。
- **长耗时任务对话框的关闭语义**：承载长耗时任务（检索试运行等）的对话框，运行中必须 `:persistent="running"`——点背景 / ESC 不关闭，避免等待期间误触丢失进度；X / 关闭按钮仍可主动关闭（`persistent` 只拦截点击外部与 ESC，不拦截 `v-model` 变更）。任务状态须挂页面级，关闭时**不得**中止流或清空状态：运行在后台继续，重开续看实时进度 / 结果，仅「重新运行」时清空；工具栏入口以 `ActionButton.spin`（运行中图标转 `mdi-loading` 并旋转，复用 `@mdi/font` 的 `.mdi-spin`）展示运行中指示，且必须保持可点击——**禁止**用 `VBtn` 的 `loading` 表达运行中：它会落 `.v-btn--loading{pointer-events:none}` 吞掉点击（曾导致后台运行期间无法重开对话框）。先例：`pages/[language]/ai/knowledge.vue`（检索试运行）。
- **代码编辑器**：全部 CodeMirror 编辑器共享 `.zk-editor` 外壳（主题感知 hairline 描边、`--zk-radius-sm`、统一内边距 / 最小高度 / 字号），校验失败态加 `.is-error` 修饰类；语法配色继续复用 `utils/editorHighlight.ts`。编辑器级操作按钮（全屏 / 速查等）统一置于编辑区块顶部操作区右侧，全屏按钮固定最右：进入用 `mdi-fullscreen`、退出用 `mdi-fullscreen-exit`，全屏态 `tonal`、常态 `text`，点击同一按钮切换并在切换后重测分行；全屏属查看类动作，只读 / 禁用态仍可进入，仅写入类操作（如数据源速查）随 `disabled` 禁用。先例：`components/forms/RegoEditor.vue`、`components/forms/MarkdownEditor.vue`。
- **Markdown 内容字段**：正文为 Markdown 的输入字段（知识文档正文、技能正文等）一律走 `web/src/components/forms/MarkdownEditor.vue`，禁止再用裸 `v-textarea`。组件同一外壳内提供「明文 / 预览」两个独立开关（底部工具行右侧，非三选一）：仅开明文 = 只显示 Markdown 原文（默认），仅开预览 = 只显示渲染结果，两者同时开启 = 左右分屏同时展示，`mandatory` 保证至少保留一个；原文为 CodeMirror + Markdown 语法高亮，预览复用 `AssistantMarkdown`（全站唯一 Markdown 渲染入口，含 mermaid / 代码高亮 / 外链安全规则），因此不得另建第二套渲染器；分屏时两半之间以 hairline 竖直分隔，两侧正文区同高，固定取 `--zk-editor-body-height`(240px)，超出时在各自区内纵向滚动，勿按场景自写高度；仅明文视图不挂载预览（避免对不可见内容做整篇渲染），开启预览时按需挂载；标题行操作区最右另有全屏编辑按钮（与 `RegoEditor.vue` 同约定），全屏时经 `Teleport` 到 `body` 铺满视口（避开对话框 `transform` 祖先），退出后重新测量分行。只读正文（导入原件解析结果）用 `readonly`，整字段不可改时用 `disabled`，两者都保留预览能力。编辑态内置轻量语法校验（代码围栏未闭合、链接缺少右括号 / 空地址、标题与列表标记缺空格），诊断范围由 `.zk-markdown-editor` 下的 `cm-lintRange-{error,warning}` 叠加语义底色、CodeMirror 自带波浪下划线保留；首处问题以「行号 + 原因」展示在外壳底部工具行左侧，完整文案走 `title` 悬浮提示，禁止在正文内自绘装饰层。
- **输入光标随主题联动**：凡可输入的光标（原生 caret）颜色必须与全局主题联动——**深色近白、浅色近黑**，即与正文同色（`rgb(var(--v-theme-on-surface))`），不得固定为黑白字面量。原生 Vuetify 输入族（`VTextField` / `VTextarea` / `VSelect` 等）由 `VField.css` 的 `color: rgb(var(--v-theme-on-surface))` 自动带动，无需处理，**禁止**覆盖其 `caret-color`（Vuetify 自身为 `VSelect` / `VCombobox` / `VAutocomplete` / `VOtpInput` 置 `caret-color: transparent` 以隐藏 caret、改由选中项呈现，覆盖会破坏该行为）。自定义输入组件（以 CodeMirror 编辑器为代表）必须显式遵守：CodeMirror 的 baseTheme 只在 `&light` / `&dark` 两个分支里给 caret 定色，而分支由 `EditorView.darkTheme` facet 决定——本项目只切换语法高亮、**从不设置该 facet**，于是深色主题下仍走 `&light` 分支、caret 落为黑色，在深色编辑区中不可见（即先例缺陷）。统一修正在 `styles/main.scss` 的 CodeMirror 段：用 `.cm-editor.cm-focused .cm-content { caret-color: rgb(var(--v-theme-on-surface)) }`（0-3-0，压过 baseTheme 的 0-2-0，因其样式在挂载时才注入、晚于本文件、同权重会反压）覆盖，**不加 `!important`**（`drawSelection()` 启用后 baseTheme 会以 `caret-color: transparent !important` 隐藏原生 caret，不可与之争夺）；用法处与各编辑器 `<style scoped>` **禁止**再自写 `caret-color` / `caretColor`。若将来某编辑器启用 `drawSelection()`（改为绘制光标），必须同时把绘制光标纳入同一变量——`.cm-editor.cm-focused .cm-cursor, .cm-editor.cm-focused .cm-dropCursor { border-left-color: rgb(var(--v-theme-on-surface)); }`（0-3-0，压过 baseTheme 的 `&dark .cm-cursor` 0-2-0），否则该编辑器会退回黑白 caret。先例：`components/forms/JsonEditor.vue`（用户选中的 `.json-editor-host > .cm-content`）。
- **图标按钮**：统一 `variant="text"`（`density="compact"` 由 `VBtn` 默认值提供）+ `:title`（tooltip）+ `:aria-label`。
- **图标语义**：操作类用 `mdi-*` 动词图标（`mdi-close` / `mdi-refresh` / `mdi-delete`）；状态类用「语义色 + 图标」组合（`color="success|warning|error|info"`）；禁止装饰性图标，禁止只靠颜色表达状态。
- **密码类字段的明文 / 掩码切换图标**：`append-inner-icon` 表达字段**当前状态**，而非点击后将执行的动作——已明文展示时为 `mdi-eye`，处于掩码态时为 `mdi-eye-off`。`type` 与图标必须由同一开关驱动、方向一致：`:type="revealed ? 'text' : 'password'"` 配 `:append-inner-icon="revealed ? 'mdi-eye' : 'mdi-eye-off'"`，`@click:append-inner` 翻转该开关。禁止反向映射（掩码态显示睁眼图标）。先例：`components/forms/PasswordField.vue`、`components/admin/LlmModelPanel.vue`、`pages/[language]/datasource.vue`、`pages/[language]/notification/channel.vue`。
- **组件自有内边距不外覆**：自带内边距的组件（`v-btn` / `v-chip` / `v-list-item` / `v-text-field` 等）其内部间距由组件自身拥有，用法处**禁止**用间距工具类覆盖其**纵向**内边距（`pa-*` / `py-*` / `pt-*` / `pb-*`）。理由：Vuetify 间距工具类带 `!important`、会覆盖组件内边距，而组件高度是固定值（`VBtn` 的 `--v-btn-height` 在 `density="compact"` 下仅 24px）——上下各吃 8px 后内容区只剩 8px，装不下 20px 的行盒，文字会**溢出到背景之外**（登录页曾出现「登录」二字突破按钮底色 4px）。仅**横向**覆盖（`px-*`）不触发该问题，可用于有意收紧横向留白。按钮行的间距一律由容器提供：`v-card-actions` 自带 `padding: 8px` + `gap: 8px`，其余场景用容器的 `ga-*`。
- **无背景 / 无边框的独立控件必须自带对称内边距**：一个控件若视觉上既无底色也无描边、且**作为独立可点击单元与相邻控件并排**（工具条里的下拉、裸输入框等），其左右内边距必须由**控件自身**声明为对称值——不得为 0，也不得把内侧留白推给外层容器的 `padding` 或兄弟元素的 `margin` 拼凑。理由：`plain` / `underlined` 是 Vuetify 为「与周围文字齐平的内联输入」设计的变体，**有意把取值区内边距归零**（`.v-field--variant-plain.v-field { --v-field-padding-start/end: 0 }`）；而全站输入族默认登记为 `outlined`（见「输入族变体基线」）自带 16px 对称内边距，所以某一处显式改用 plain 就会成为全站唯一「内容贴死边界」的例外，无从判断留白是设计还是遗漏。再叠加 `v-select` 恒带 `append-inner` 插槽而命中的 `.v-field--appended` 6px **单侧**右内边距，还会进一步退化成左右不对称。正确做法是在该控件的 `.v-field` 上补回对称内边距（直接写 `padding-inline`，或用 `--v-field-padding-start/end` 自定义属性并抹平 append 的单侧偏移），使「外边界 → 内容 → 内部图标 → 外边界」各段间距相等。判断标准是**该控件是否作为独立单元与相邻控件并排**：是则必须自带对称内边距；反之，控件本身是**内联输入**（嵌在表格单元格 / 表单正文里、须与相邻文字左边缘齐平，如 `components/table/AdaptiveCell.vue`）时保留 0 内边距才是正确目标，不受此约束。同一容器内若有多个控件需要文字左边缘对齐（如对话框底部的正文输入框与工具条控件），对齐点 = **容器内边距 + 控件自身内边距**之和，两处必须一并核算，不能只算其中一层。先例：`components/ai/AssistantDrawer.vue` 的档位 / 审核阀门两个下拉（补 4px 对称内边距）与正文输入框（补 12px 对称内边距，与「容器 8px + 控件 4px」的控件文字左边缘对齐）。
- **列表 / 下拉选项的内聚与字号**：`v-list-item` 用作下拉选项或列表项时，图标与其相邻文本之间的间距**两侧**都由全局样式统一收紧到 8px（`--zk-space-2`）——Vuetify 在两者之间插入一个 32px 的 `.v-list-item__spacer`（前置侧为「图标 → 文本」，尾部侧为「文本 → 图标」），会把图标与文本拉散成两个互不相干的块；说明性副文本（`v-list-item-subtitle`）统一降一档为 caption（`--zk-font-size-caption`，12px）——Vuetify 默认 0.875rem（14px）与正文本同号、只靠颜色降级，在密集的选项里偏重。两处覆盖均登记在 `styles/main.scss`，用法处**禁止**就地覆写：下拉菜单挂在 `document.body` 下的 `.v-overlay-container` 里、**不在** `.v-application` 内，故不能带 `.v-application` 前缀，改用「四个类」（0-4-0）压过 Vuetify 自身的 `.v-list-item__{prepend,append} > .v-icon ~ .v-list-item__spacer`（0-3-0），才能同时命中应用内与浮层内并与注入顺序无关。先例：`components/ai/AssistantDrawer.vue` 的审核阀门下拉选项（图标 → 标题 32px → 8px）。
- **列表页骨架**：**搜索驱动列表页**（后端提供 `SearchIn` / `SearchOut` / `SearchOptionsOut` 搜索契约）必须使用 `web/src/components/table/SearchTable.vue`，禁止手写骨架；**非搜索驱动列表页**（客户端数据 / 看板类，后端无搜索契约，如 `pages/[language]/etl/{task,queue,scheduler}.vue`）**不得**强行套用 `SearchTable`，但必须逐项复用其视觉骨架：扁平卡片（`<v-card flat>`）内的「筛选行（`px-4 pt-3 pb-2` + `.zk-filter-form`）→ 操作按钮行（`px-4 pb-2 d-flex justify-end align-center ga-2` + `action-button-group` + `column-visibility-menu`）→ `<v-container fluid class="px-4 pt-0 pb-4">` 包裹的表格」；单元格一律用 `adaptive-cell`，列显示配置一律用 `useColumnVisibility`。理由：骨架一致是硬要求、组件复用是手段——当组件契约不匹配时，以「逐项复用骨架类」达成同一视觉结果，禁止各自发明间距与结构。
- **全屏居中页骨架**：认证页（登录 / 忘记密码 / 重置密码 / 重置成功）、错误页（401 / 404）与功能不可用说明页（`pages/[language]/feature-unavailable.vue`）必须走 `web/src/layouts/CentralLayout.vue`，不得自建居中容器；尺寸与滚动由骨架的 `.zk-centered-layout` / `.zk-centered-surface` 外壳类统一承载——宽度固定 `--zk-centered-surface-width`(420px)、不随视口放大，高度上限取 `v-main` 的可视高度、超出由表面内部滚动（页面整体不滚动）。页面只提供内容，**禁止**在页面内声明居中、宽度或高度（曾出现登录页卡片内联 `style="width: min(420px, 100%)"` 与骨架内联 `height="100%"`，前者随分辨率被撑到 868px）。
- **步骤条**：需要分步呈现的流程（如 AI 助手问卷面板的多题作答、工具调用审批的逐项确认）一律走 `VStepper`（`v-stepper` + `v-stepper-header` / `v-stepper-item` + `v-stepper-window` / `v-stepper-window-item`），不另手写步骤指示器；嵌在紧凑面板内时挂 `.zk-compact-stepper`，其尺度收敛登记在 `styles/main.scss` 的同一区块——**头部并入面板标题行**（与标题同处一个 flex 行内，占满标题右侧剩余宽度、徽标右对齐、徽标间隔 8px，步骤项内边距归零，高度由标题行的内边距提供；步骤条因此不独占一条横向带），另含头部阴影清零、步骤间连线收成 8px 定宽短段（Vuetify 默认 `flex: 1 1 100%` + 负外边距，会吃掉徽标的自适应左边距并压到徽标上）、徽标右外边距归零且 hover / focus 叠色层随徽标成圆（无 `title` 时该 8px 间隔会变成按钮盒内的死区，叠色层铺满按钮盒就会向右多出一截、看着不对称）、窗口承载滚动与高度上限（审批面板例外：挂 `.zk-compact-stepper--inner-scroll`，滚动落在参数 / 结果 JSON 块自身，窗口不再整体滚动）、步骤徽标不着底色（当前项改用主色文字、完成项显对勾，点击切换不出现整块跳色），用法处**禁止**再就地覆写。步骤条**必须可点击**（`v-stepper` 上传 `editable`，否则步骤按钮渲染为 `disabled`、点击不会选中）——「上一题 / 下一题」按钮只是同一动作的另一入口；徽标内容由 `#icon` 插槽自绘并只呈序号（`editable` 会把当前题徽标换成编辑铅笔，序号比「可编辑」更该被看到）。步骤条仅在步骤多于一项时呈现，单项隐藏（无进度可言）。先例：`components/ai/AskUserQuestionnaire.vue`、`components/ai/AssistantApprovalSteps.vue`。

正例：

```vue
<app-dialog v-model="visible" size="md" :title="$t('...')" icon="mdi-pencil" persistent>
  <v-text-field v-model="name" :label="$t('...')" />
  <template #footer>
    <v-btn variant="text" @click="visible = false">{{ $t("action.cancel") }}</v-btn>
    <v-btn color="primary" @click="submit">{{ $t("action.confirm") }}</v-btn>
  </template>
</app-dialog>
```

反例：

```vue
<!-- 反例：自建对话框骨架（标题栏 / 宽度 / 圆角都会与全站分裂） -->
<v-dialog v-model="visible" max-width="760" persistent>
  <v-card>
    <v-card-title class="text-h6 font-weight-600">编辑</v-card-title>
    <v-card-text>...</v-card-text>
  </v-card>
</v-dialog>

<!-- 反例：重复声明 VTextField 已由 defaults 覆盖的属性 -->
<v-text-field v-model="name" variant="outlined" density="compact" />
```

**组件默认值清单**（`plugins/vuetify.ts` 的 `defaults` 对象逐项对应；用法处禁止重复声明）：

| 组件 | 默认值 | 约定 |
|------|--------|------|
| `VField` / `VTextField` / `VSelect` / `VAutocomplete` / `VCombobox` / `VTextarea` / `VNumberInput` / `VFileInput` | `variant: "outlined"`, `density: "compact"` | 输入族统一 outlined + compact |
| `VSwitch` | `size: "small"`, `inset: true` | 开关统一 small 内嵌轨道形态 |
| `VDataTable` / `VDataTableServer` | `density: "compact"` | 表格恒为紧凑 |
| `VDialog` | `rounded: "lg"`, `maxWidth: "var(--zk-dialog-width-md)"` | 12px 圆角，默认 `md` 宽度档 |
| `VCard` | `elevation: 0` | 扁平 + hairline 描边的表面 |
| `VToolbar` | `density: "compact"`, `elevation: 0` | 工具栏紧凑且扁平 |
| `VAppBar` | `elevation: 0` | AppBar 扁平（分隔线来自 `.zk-hairline-bottom`） |
| `VBtn` | `density: "compact"` | 按钮紧凑（`variant="text"` 由用法处声明，用于图标 / 三级操作） |
| `VChip` | `size: "small"`, `variant: "tonal"` | chip 共享同一 small tonal 基线 |
| `VList` / `VListItem` | `density: "compact"` | 导航列表紧凑 |
| `VExpansionPanels` | `variant: "accordion"`, `density: "compact"` | 展开面板恒为手风琴式且紧凑 |
| `VTabs` | `density: "compact"` | 标签栏紧凑 |
| `VAlert` | `variant: "tonal"`, `density: "compact"` | 提示为紧凑 tonal，禁止高饱和色块 |
| `VSnackbar` | `location: "top"` | 提示条出现在顶部 |
| `VMenu` | `rounded: "md"` | 菜单与对话框同族圆角（6px） |

附加规则：

- **卡片**：1px 半透明描边由 `main.scss` 全局提供，禁止再叠加第二条边框或阴影。
- **按钮**：图标 / 三级操作默认 `variant="text"`；主操作使用 outlined 或填充（默认）；导航中选中 / 激活的列表项用 `tonal`。
- **语义色**：`success` / `warning` / `error` / `info` 只表达**状态**（通知、状态标记、校验、健康度）；`primary` 表达**动作与选中**（按钮、导航激活项、链接、focus、行 hover）。禁止用语义色装饰非状态 UI，也禁止用语义色表达主操作。
- **文本色**：默认继承（`on-background` / `on-surface`），次级 / 说明用 `on-surface-variant`；禁止自选灰度。

**`AppDialog` 骨架**：`web/src/components/base/AppDialog.vue` = `VDialog` + `VCard` + 紧凑工具栏（图标 + 标题 + 关闭图标按钮）+ `VDivider` + 可滚动 `VCardText` + 可选右对齐页脚；组件自身无 `<style>` 块。

| Prop | 类型 | 默认 | 含义 |
|------|------|------|------|
| `modelValue` | `boolean` | —（必填） | 打开状态（`v-model`） |
| `title` | `string` | —（必填） | 工具栏标题 |
| `subtitle` | `string` | `undefined` | 可选次级标题，紧随主标题（`.zk-section-subtitle`：同字号、降字重、`on-surface-variant`）。承载「视图名 + 实体名」这类双标题场景，实体名过长时随标题行整体截断，主标题始终完整可读 |
| `icon` | `string` | `undefined` | 可选前置 `mdi-*` 图标 |
| `size` | `"sm" \| "md" \| "lg" \| "xl"` | `"md"` | 宽度档 |
| `persistent` | `boolean` | `false` | 点击遮罩 / ESC 不关闭 |
| `scrollable` | `boolean` | `true` | 主体独立滚动，标题栏与页脚固定 |
| `loading` | `boolean` | `false` | 标题旁的进度指示 |

插槽：`default`（主体）、`footer`（操作行）、`header-append`（工具栏追加内容，位于 loading 指示器之前）。

宽度档——组件把 `size` 映射为同名令牌，页面永不书写 `max-width`：

| `size` | 令牌 | 值 |
|--------|------|-----|
| `sm` | `--zk-dialog-width-sm` | 560px |
| `md` | `--zk-dialog-width-md` | 760px |
| `lg` | `--zk-dialog-width-lg` | 900px |
| `xl` | `--zk-dialog-width-xl` | 1100px |

**发起后端请求的按钮统一走 `v-request`**：主流程按钮（登录、各 CRUD 提交、批量动作、确认框确认）一律 `v-request="handler"`，禁止裸 `@click`；纯导航 / 纯前端按钮不得加。指令接管点击：pending 期禁用并注入 `.zk-request-loading` spinner，`finally` 复位；只管理 DOM 层、不吞 handler 错误；handler 必须返回 Promise（同步逻辑包 `async`）。全局注册于 `plugins/index.ts`，实现见 `web/src/directives/request.ts`。

**一对多编辑统一复用 `InnerEditTable`**：用户权限组、通知目标、用户组策略等一对多编辑一律用 `web/src/components/table/InnerEditTable.vue`，禁止另建同类组件。候选条目由组件统一渲染为紧凑 `v-list-item`，经 `item-title` / `item-subtitle` / `item-append` 插槽定制；已选判定用 `isBound`（按键 `itemKeyField`）；字段名定制走 `itemKeyField` / `subtitleField` prop，不得改组件骨架或回退大卡片。

### 视觉层级与文本规范（Visual Hierarchy & Text）

**优先用视觉结构表达功能划分，而不是写说明文本**：区块靠「标题 + 分隔/边界 + 间距」表达层级，提示收敛为 icon / tooltip。

文本颜色只允许取自 Vuetify 主题 tokens（`web/src/plugins/vuetify.ts` 的 `--v-theme-*` 或对应 `text-*` 工具类）。按文字角色选用「字号 + 颜色」组合，确保 light/dark 双主题下对比达标。

| 角色 Role | 字号 Size | 颜色 Color | 字重 | 典型场景 |
|-----------|-----------|-----------|------|----------|
| 页面/区块标题 | `text-h6`（16px） | `text-on-surface` | 600 | 卡片/区块标题、对话框标题 |
| 正文 / 单元格 / 字段值 | 默认 body（0.875rem/14px） | `text-on-surface` | 400 | 表格单元格、长文本 |
| 表单字段标签（field label） | 默认 body（0.875rem/14px） | `text-on-surface` | 500 | `v-text-field` / `v-select` 的 label |
| 表头 / 列名 | `text-caption`（0.75rem） | `text-on-surface-variant` | 600 + uppercase | `.v-data-table th` |
| 次要说明 / 表单提示（hint/描述） | `text-caption`（0.75rem） | `text-on-surface-variant` | 400 | 表单 helper 提示、区块说明（如 SchemaEditor 提示） |
| 弱化辅助文本（时间戳、副标题） | `text-caption`（0.75rem） | `text-on-surface-variant` | 400 | 时间戳、元信息、面包屑 |
| 禁用态 | 继承 | Vuetify disabled 态 | — | disabled 控件文字 |

上表中的字号档位同时以令牌形式导出（`--zk-font-size-caption|body|subtitle|title|heading`）。**各主题色值（含 hex）与对比度数字见本文件「色板与主题（Color Palette）」一节。**

硬性规则：

- 主文本一律 `text-on-surface`，次要/说明/提示一律 `text-on-surface-variant`；不要自定义灰度或任意 hex。
- 说明/提示类文本**必须**缩到 `text-caption`（12px）以与字段名（14px）形成字号区分，而不是和字段名同样字号。
- 不要用 `text-medium-emphasis` / `text-low-emphasis` 表达「次要」——统一改用 `text-on-surface-variant`（固定透明度弱化会与背景色叠算，对比不足）。
- 表头一律 caption + `text-on-surface-variant` + 600 + uppercase，由 `styles/main.scss` 的 `.v-data-table th` 统一实现，页面不得重复声明。
- 表格行操作列（`__actions`）专用于摆放行级按钮，其表头与单元格一律水平居中并固定置尾；该约定由 `utils/column.ts` 的 `useColumnVisibility` 统一覆写，页面与表格不得就地声明该列的 `align` / `fixed`。
- **列表启用态一律用行内开关**：列表页 / 面板的行级「启用 / 禁用 / 启停」不使用操作列（`__actions`）菜单，一律在对应状态列内用 `EnabledSwitchCell`（`components/table/EnabledSwitchCell.vue`）就地开关——即时调用后端接口、乐观更新、按行 pending、失败回退；布尔字段用 `v-model` 绑定行字段，非布尔状态（枚举 / 布尔派生）用 `:model-value` + `@update:model-value` 映射。批量启停仍保留在批量操作中。

**icon**：可点击项一律带图标；状态一律「图标 + 语义色」，不只靠颜色。语义色（`success` / `warning` / `error` / `info`）只表达状态，`primary` 只表达动作与选中。

**边界包含关系**：用「外层容器 → 内层分组 → 条目」的描边 / 表面层级表达归属；分隔统一用 hairline（`on-surface` 半透明，令牌 `--zk-hairline-width` / `--zk-hairline-opacity`，亮色主题强化值 `--zk-hairline-opacity-strong`），不叠加双重边框或阴影。

**间距分组**：用 4px 栅格间距表达分组——组间 ≥ `--zk-space-4`（16px），组内 ≤ `--zk-space-2`（8px），不用空行文字解释。

**说明文本收敛**：字段 / 按钮用途优先靠 `label` + `icon` + `:title` tooltip 传达；`hint` 只保留**不可推断**的约束（格式、范围、副作用），且必须用 `.zk-hint`（或 `text-caption` + `text-on-surface-variant`）；**禁止**用段落解释界面如何操作（如「请先点击右侧按钮…」）。

反例（用说明文本解释界面操作）：

```vue
<!-- 反例：这段文字解释的是「怎么点」，用户看一眼界面就知道 -->
<span class="text-caption text-on-surface-variant">
  请先点击右上角「新建」按钮创建知识库，创建后即可在左侧列表选择
</span>
```

正例（靠标题 + 分隔 + 间距分层，hint 只留不可推断的约束）：

```vue
<section class="d-flex flex-column ga-4">
  <h2 class="zk-section-title">{{ $t("...") }}</h2>
  <div class="zk-hairline-top pt-4 d-flex flex-column ga-2">
    <v-text-field v-model="cron" :label="$t('...')" :hint="$t('...')" persistent-hint />
  </div>
</section>
```

### 就地样式禁令（No In-place Styling）

五条禁令：

1. **禁止**内联 `style="..."` / `:style="..."`：改用类；仅当值必须运行时计算（数据驱动的尺寸/偏移/图表尺寸）时允许，且必须附理由注释。
   - 反例：`<div style="max-width: 1440px">` → 正例：`<v-container class="zk-page-container">`
2. **禁止**硬编码颜色：hex、`rgb()/rgba()` 字面量、`grey-*`、未在主题注册的色名。
   - 反例：`color: #4B5563;` → 正例：`class="text-on-surface-variant"` 或 `color: rgb(var(--v-theme-on-surface-variant));`
3. **禁止**令牌外数值：字号、圆角、阴影、控件高度、对话框宽度一律取令牌。
   - 反例：`font-size: 13px; border-radius: 8px;` → 正例：`font-size: var(--zk-font-size-body); border-radius: var(--zk-radius-md);`
4. **禁止**在 `.vue` 中声明 `border` / `background-color` / `color` / `font-size` / `border-radius` / `box-shadow`：一律改用主题类、`.zk-*` 外壳类或全局规则。
   - 反例：`<style scoped>.box { border: 1px solid …; background-color: …; }</style>` → 正例：容器直接引用 `.zk-panel`，scoped 块只保留布局。
5. **禁止**重复声明 `plugins/vuetify.ts` 的 `defaults` 已覆盖的属性（`variant` / `density` / `rounded` / `elevation` / `maxWidth` / `size` / `inset`）；完整属性清单以「组件与变体约定 · 组件默认值清单」为准。
   - 反例：`<v-select v-model="x" variant="outlined" density="compact" />` → 正例：`<v-select v-model="x" />`

`<style scoped>` 目标：每个 `.vue` 的样式块**只保留布局定位**（flex / grid / 尺寸约束 / 溢出 / 定位），其余一律抽到外壳层（`styles/main.scss`）或改用主题类。兜底校验：`web/stylelint.config.js` 的 `declaration-property-value-disallowed-list` 拦截 hex 与 `rgb(数字…)` 字面量（`src/styles/**` 令牌层与外壳层豁免）。

**已注册例外**（仅限以下情形，其余一律违规）：

- **7 处内联 style，全部为运行期计算值，且各自带理由注释**：
  - `RegoEditor.vue` / `GoTemplateEditor.vue` / `TagExprEditor.vue` —— `:style="{ minHeight }"` 绑定公开的 `minHeight` prop
  - `LineChart.vue` / `PieChart.vue` / `BarsChart.vue` —— `:style="{ height, width }"` 绑定公开 props
  - `ScrollToBottomFab.vue` —— 动态 `positionStyle`（位置依赖运行期几何）
- **编辑器语法配色** —— `utils/editorHighlight.ts` 是唯一的语法色字面量来源，全部 CodeMirror 编辑器消费它
- **样式层字面量** —— `web/src/styles/**/*.{css,scss}` 对 Stylelint 颜色规则豁免（令牌层正是定义令牌的地方）；`main.scss` 中每一处颜色声明都走主题变量（`rgb(var(--v-theme-*))`），包括 `html` 的 overscroll 底色：Vuetify 在 `:root` 上发布当前主题变量，`html` 可解析，overscroll 颜色随双主题同步

### 布局约定（Layout）

- **导航抽屉**（`MainLayout.vue`）：常驻抽屉 `width="220"`，开启 `rail` + `expand-on-hover`——收起为 56px rail，悬停展开。抽屉底部 `#append` 槽提供固定按钮（纯图标 `v-btn`、`variant="text"`、居中），让用户常驻展开（`expand-on-hover` 由「未固定」且「悬停展开已交还」共同决定，见下），偏好持久化到 `localStorage`（`localManager.getNavPinned` / `setNavPinned`）。父级项作为分组标题（`color="primary"`）并遵循**手风琴行为**（`v-list-group` + `open-strategy="single"` + `v-model:opened`）：默认只展开当前路由所在分组，点击其他分组会收起上一个，点击已展开分组则收起它；展开状态用 Vuetify chevron 指示，展开 / 收起的高度过渡加速到 `0.15s`（见 `MainLayout.vue` 的 `:deep(.v-list-group__items)` 覆写）。rail 模式下全部分组收起，以免在 56px 宽度内溢出；悬停 rail 时重新展开当前路由所在分组。子项缩进 16px（`margin-left: 16px`，`variant="tonal"`）。抽屉与内容区之间由 `.zk-hairline-right` 外壳类分隔。**功能开关只标注可用性、不参与过滤**：功能关闭的子项仍展示，以不可用态呈现（标题 `text-on-surface-variant` 置灰 + 锁定标识，见「功能开关与不可用态 · 导航标识徽章」），点击进入页面后由路由守卫统一引导到「功能不可用」说明页。一级分组标题行同样带标识：全部子项不可用时跟随展示锁定标识，全部子项受命名空间影响时跟随展示命名空间标识（两者同时命中时锁定优先）。**rail 收起态仅展示图标**：由 `main.scss` 统一覆写（同一处选集覆盖「不悬停展开」与「悬停展开但未悬停」两个分支），隐藏 `.v-list-item__content` 与整块 `.v-list-item__append`，只保留 `#prepend` 图标——Vuetify 原生 rail 只把 `.v-list-item__content` 的 `min-width` 归零，56px 内标题仍被挤成 0 宽并露出半个首字，同时 `#append` 的标识与展开箭头继续占宽溢出裁切；抽屉展开（悬停展开中 / pin 固定）时规则不命中，标题、标识与展开箭头自动恢复。页面与组件不得就此另行就地覆写。**取消固定需与 hover 展开交接**：Vuetify 在 `rail && expandOnHover` 时把布局让位量恒定为 56px，而元素渲染宽度在悬停时仍是 220px；点击 pin 取消固定的瞬间指针本就悬停在抽屉内（`isHovering` 只由 mouseenter/mouseleave 驱动、与 `expandOnHover` 无关），于是布局已收窄 56px 而抽屉仍渲染 220px，会盖住刚展开的主体。故 `MainLayout.vue` 在「取消固定且指针仍在抽屉内」时先暂停 `expand-on-hover`（`hoverExpandEnabled`），让抽屉与布局同步收窄，待指针离开抽屉后再交还；不得退化为在抽屉内改回 `rail` 之外的其他收窄方式。
- **内容区**：`<v-main class="d-flex mb-6">` 包裹 `<v-container class="px-4 px-md-6 pt-0 pb-4 pb-md-6 zk-page-container">`。`.zk-page-container` 把内容上限压在 **`--zk-container-max-width`（1440px）** 并水平居中；页面内容放在 `rounded="lg"` 的 `v-sheet` 中（12px，与全局 `.v-card` 圆角对齐，避免圆角卡片嵌在直角容器里留下月牙形空底）。**容器显式 `pt-0`**：Vuetify `.v-container` 默认带 16px 全向内边距，不清零顶部会让面包屑上距被顶到 24px、下距仅 8px 而显得偏低；清零后该顶隙完全由容器首个元素（面包屑 `.zk-breadcrumbs`）的上下对称 `padding-block` 承担，使面包屑垂直居中于 AppBar 与内容表面之间、带高与内容区左侧的对齐留白同值 40px（容器 `px-md-6` 24px + 卡片 `px-4` 16px，即行高 16px + `padding-block` 各 12px）、不再单独占据一整行——页面不得给面包屑追加 `mb-*`（会重新把内容表面推下去一行）。页面不得自行内联 `max-width`，也不得改动容器内边距。认证页、错误页与功能不可用说明页改用 `CentralLayout`（见「组件与变体约定 · 全屏居中页骨架」）。
- **AppBar**（`AppHeader.vue`）：扁平（`elevation="0"`），与页面内容之间由 `.zk-hairline-bottom` 外壳类分隔。工具栏按钮用 `variant="text"`。**两侧按钮簇由容器统一间距**：`.v-toolbar__prepend` 与 `.v-toolbar__append` 内的按钮间隔统一为 `--zk-space-3`（12px），由 `main.scss` 对 `.v-application .v-app-bar` 下的这两个容器声明 `gap` 实现——Vuetify 的这两个簇虽是 flex 容器但未设 `gap`，簇内按钮之间没有任何默认间距（默认外边距只作用于 `.v-toolbar__content > .v-btn:first-child / :last-child`），此前靠各按钮零散的 `mr-2` 撑开，导致右侧末尾按钮缺外边距、左侧 logo 与命名空间切换按钮完全贴合。故顶栏按钮一律不得自行声明 `mr-*` / `ml-*` 来调间距，新增控件直接受益于容器 `gap`。
- **数据表页脚**：每页数目下拉（`.v-data-table-footer__items-per-page .v-select`）比常规紧凑控件再压一档，高度统一为 `--zk-control-height-dense`（32px）——由 `main.scss` 覆写 `--v-input-control-height`，SearchTable / 原生 `v-data-table` / InnerEditTable 共用；页面与组件不得就地调整该控件高度。
- **全局 focus**：`:focus-visible` 渲染 2px `primary` 描边 + 2px offset（`main.scss`），禁止抑制或替换。
- **表单 / 列表页组装顺序**：每个列表页依次由 `TabView`（标签栏）→ 筛选行 → `ActionButtonGroup` → `SearchTable` 组成：
  - 标签栏独占页面 sheet 顶部一行。
  - 单条件简单筛选（单个 `v-select` / 日期字段）置于同一行左侧；多条件复杂筛选（表达式 / 条件 chip，如 `ExpressionField`）独占一行。
  - 操作按钮组（`ActionButtonGroup`）位于筛选行右侧，默认纯图标：`variant="text"` 使描边与 sheet / card 表面一致，按钮名经 `:title` tooltip 悬停展示。`collapsible` 且收起时，常驻的刷新 / 新建按钮保持可见，次要按钮收到 chevron 之后。

### 功能开关与不可用态（Feature Flags & Unavailable State）

**核心原则：功能未开通时，相关入口不隐藏，而是「展示但不可用」，并把用户引导到开通入口。**

- **单一数据来源**：功能可用性与「不可用原因」一律由后端 `features` 接口（`internal/manager/features` → `POST /api/v1/features/list`）下发，原因按请求语言本地化。前端只保留功能键常量（`stores/features.ts` 的 `FEATURE_*`）用于引用，**禁止**在前端自行推断可用性或自造原因文案。`stores/features.ts` 是前端唯一访问点，按语言缓存（`loadedLang`），语言切换自动重载，拉取失败按「全部关闭」处理且不标记已加载（下次导航自动重试）。
- **侧边栏入口不隐藏**：`navigation.ts` 上声明了 `feature` 的导航分组 / 子项，在功能关闭时**禁止**从侧边栏过滤掉，必须以不可用态展示：
  - 标题用次级文本色（`text-on-surface-variant`）置灰；
  - 追加**锁定标识**（见下条「导航标识徽章」）；
  - 子项的功能键为「子项自身 `feature` 优先，否则继承所属分组」，见 `MainLayout.vue` 的 `childFeature`。
- **导航标识徽章**（`components/base/NavBadge.vue`）：侧边栏的「锁定」与「命名空间」两类标识是**同一组件族的两个变体**，父级与子级共用同一组件、同一套解析规则，禁止就地各写一套图标 + tooltip：
  - 标识种类由 `navigation.ts` 的 `navBadgeOf(unavailable, namespaceScoped)` 统一解析：**锁定优先于命名空间**——页面既然已经不可用，再说明它是否随命名空间变化已无意义，故两者同时命中时只返回 `lock`；父级与子级共用该函数，保证同类标识语义处处一致。
  - `lock`：`mdi-lock-outline` + `color="warning"`（不可用是**状态**，用语义色）；tooltip 取 `navbar.featureUnavailable`。
  - `namespace`：`mdi-alpha-n-box-outline` + `color="primary"`（命名空间是**归属**而非状态，禁止用语义色表达）；tooltip 取 `navbar.namespaceScoped`。
  - **tooltip 文案只表达「标识是什么」且必须单行**：受左侧可用宽度预算（约 6~7 个汉字）限制，文案须短（`navbar.featureUnavailable` / `navbar.namespaceScoped` 两个键，中英同步精简），字数超出时**优先精简文案**而非放宽宽度。**禁止**把后端下发的具体不可用原因塞进 tooltip：技术细节长且随实现变化，必然折行或被翻到右侧（该原因只由说明页承载）。
  - **子级标识**：命中条件由子项自身决定（见上条的功能键继承规则）。
  - **父级标识**：**仅在全部子项命中同一语义时**才跟随展示——部分子项不可用、或部分子项与命名空间无关时，父级标识会以偏概全，故不展示；判定基于该分组当前可见（已按权限过滤）的子项，即用户实际看到的子项。
  - 无标识时**不提供** `#append` 插槽（用 `v-if` 控制），避免空容器参与布局。
  - **tooltip 一律朝左展开，且必须用 `max-width` 属性限制宽度**：这不是装饰而是行为约束——Vuetify 的位置策略（`connectedLocationStrategy`）在首选方向放不下时会**自动翻到对侧**，而标识紧贴抽屉右侧、左侧可用空间只有约 133px，宽度超过该值的 tooltip 会被翻到右侧，造成同类标识方向不一致。该宽度只能用 Vuetify 的 `max-width` 属性（`makeDimensionProps` 会把它作为**行内样式**写到 `.v-overlay__content`），**不能**用 CSS 类覆盖；新增/改动这类 tooltip 文案时禁止移除该限制。
  - 父级标识复用分组标题行的 `#append` 插槽，而该插槽会接管 Vuetify 默认注入的展开箭头，故必须在同一插槽内保留一个**不指定 `icon`** 的 `v-icon` 以继承父级 `VListGroup` 经 defaults 注入的 `appendIcon`：否则展开箭头消失，或被迫重复声明箭头图标及其展开态映射（先例见 `MainLayout.vue` 的父级 `#append`）。
- **唯一例外**：没有对应页面、无法跳转的非导航入口（如登录页「忘记密码」链接）仍按功能开关隐藏——展示置灰却无处可去没有意义。
- **页面级引导统一走路由守卫 + 独立说明页**，禁止各功能页各自处理不可用态：
  - 路由 → 功能的映射由 `navigation.ts` 的 `featureOfRoute` 推导（子项 `feature` 优先，否则继承分组；未登记在导航中的路由返回 `undefined`），只维护一份声明；
  - `router/index.ts` 的守卫对 `requiresAuth` 路由校验：命中关闭功能时先 `load(true)` **强制刷新一次**功能开关再重定向到 `Feature Unavailable`。理由：前端没有「配置变更推送」通道，用户刚在开通页完成开通后若直接返回，缓存中的旧可用性会把自己再次拦回说明页；
  - 说明页 `pages/[language]/feature-unavailable.vue` 走 `CentralLayout` + `v-empty-state`（`mdi-lock-outline` + `color="warning"`），功能键经 query 传入。其文案与引导全部取自单一来源：功能展示名取导航分组标题，开通入口取分组 `activationRoute` 声明的路由名（标题与所需权限经 `navChildByRoute` 复用导航中既有子项），**禁止**在本页二次声明功能名或权限字面量；
  - 用户具备开通入口所需权限时展示「前往 {page} 开通」主按钮；不具备时改提示联系管理员，**禁止**引导其前往无权访问的页面；
  - 说明页是死胡同页面，必须同时提供「返回上一级」退出入口（`action.backPrevious`）：优先 `router.back()` 回到进入本页前的页面，直接用 URL 打开本页无历史可回退时退化为首页，避免按钮无响应或把用户带出应用。**为此守卫的该次重定向必须用 `router.push` 而非 `router.replace`**：守卫先于 `finalizeNavigation` 执行，被拦截的导航尚未写入历史，`replace` 顶掉的是用户当前所在页面的条目，说明页会变成「无上一级」的死胡同（用户点按钮或浏览器后退都只能越过该页）。
- **该约束只覆盖侧边栏导航入口**：首页快速开始选择器（`pages/[language]/index.vue`）仍按功能开关过滤链接卡片；其卡片本身不带可用性标识，点击后同样由路由守卫引导到说明页。
- **新增一项功能开关时必须同时**：后端在 `internal/manager/features` 注册，并在不可用原因可解释时下发 `reason`（依赖方实现 `Reasoned` 可选接口，见 `internal/ai/runtime.go`）；前端在 `navigation.ts` 的对应分组 / 子项上声明 `feature`，有页面级开通入口时声明 `activationRoute`。

### 图表（Charts）

所有 ECharts 实例必须由 `web/src/utils/chartTheme.ts` 的主题辅助函数驱动，以跟随当前亮 / 暗 Vuetify 主题。

- `useChartTheme()` —— 响应式，返回随亮暗切换同步的 `ComputedRef<ChartTheme>`；在组件内配合 `computed` 生成图表 option。
- `withChartTheme(option, theme)` —— 纯合并：把主题默认值并入 ECharts option。页面显式值优先（深合并，数组替换）；`grid` 刻意留给页面默认值。

```ts
const chartTheme = useChartTheme();
const option = computed(() =>
  withChartTheme({ /* 页面专属 series、xAxis、yAxis 等 */ }, chartTheme.value),
);
```

**调色板**：系列色按固定顺序构建 —— `primary → secondary → success → warning → error → info`（去重），再用派生的主色明暗补足到 8 色（向白混 70% / 45%、向黑混 20% / 35%）。系列色一律顺序取自 `ChartTheme.palette`，禁止硬编码系列颜色。

**文本、网格与 tooltip**：

| 元素 | 颜色来源 |
|------|----------|
| 文本（标题、轴标签、tooltip 文本） | `textColor` = `on-surface`（回退 `on-background`） |
| 图例 / 弱化标签 | `subTextColor` = `on-surface-variant` |
| 网格 / 轴线 / 分割线 | `gridColor` = 文本色 12% 透明度（`rgba(..., 0.12)`） |
| tooltip 背景 | `tooltipBg` = `surface` |
| tooltip 描边 | `tooltipBorder` = `surface-variant` |

### 补充禁令（Additional Prohibitions）

以下为跨章节**收口禁令**：同一主题若已在专职章节详述，此处只保留速查条目并注明出处，细节以专职章节为准。

- **不要使用重于 `$zk-shadow-md` 的阴影**，也不要在同一元素叠加多层阴影（详见「圆角、阴影与扁平化」）。
- **不要用非语义色表达状态**（如自造绿色表示成功）；语义色同样不得用于纯装饰（详见「组件与变体约定 · 语义色」与「视觉层级与文本规范」）。
- **不要用比例字体渲染技术性取值**（ID / 哈希 / hex / 字节数一律 `.font-mono`，详见「字号与排版」）。
- **不要抑制或替换 `:focus-visible`** 的全局 primary 描边（详见「布局约定 · 全局 focus」）。
- **不要发明未定义的颜色名**（如 `third` / `tertiary` / `accent2`），也不要在组件内重述主题色（本地 `const primary = "#2563EB"` 或重复书写 `border: 1px solid ...` 字符串）（详见「就地样式禁令」）。
- **不要新增第二套语法高亮配色**，也不要在组件内新建 `.zk-*` 类——新外壳统一注册在 `main.scss`（详见「样式体系分层」）。
- **不要手搭对话框、不要复制编辑器外壳、不要用散文解释界面**：三者正确做法分别见「组件与变体约定 · 对话框 / 代码编辑器」与「视觉层级与文本规范 · 说明文本收敛」。

### 单一信任源（Single Source of Truth）

- **本文件（`AGENTS.md`）是前端设计系统的唯一信任源**：视觉令牌、色板、组件视觉基线与开发禁令全部收敛于此，不存在第二份设计文档（原 `web/src/styles/DESIGN.md` 已并入本文件并删除）。
- 「样式体系分层（Style Layering）」一节所列的源文件均为规范的**实现载体**，规范只在必要处指向它们（该节表格是载体清单的唯一权威处，此处不重复枚举）；`.zk-*` 类的完整清单以 `styles/main.scss` 为准（本文件只列职责摘要与约定），组件默认值清单与 `plugins/vuetify.ts` 的 `defaults` 对象同步维护。
- 本文件的约定与实现**不得相互矛盾**：发现不一致时，先判断哪一方是错的，再同时修正实现与本文件，禁止只改一边，也禁止为此新增第二份文档。

### Code Style

- 2-space 缩进（`.editorconfig` 强制）；UTF-8；EOF 须有换行；去除行尾空白
- ESLint v9 flat config；允许 `&&` 短路与三元表达式
- 组件经 `unplugin-vue-components` 自动注册，禁止手动 import `src/components/` 下的组件
- 提交前必跑项：`scripts/web.sh lint`（ESLint）+ `scripts/web.sh lint:style`（Stylelint，覆盖 `src/**/*.{css,scss,vue}`）+ `scripts/web.sh type-check`（vue-tsc）**三者全部通过**

### SFC Structure

```vue
<template>
  <!-- HTML 结构 -->
</template>

<route lang="yaml">
name: Page Name
meta:
  title: i18n.key
  requiresAuth: true
</route>

<script setup lang="ts">
// 逻辑
</script>

<style scoped>
/* 仅布局定位 */
</style>
```

样式块职责约定：

- `<style scoped>` 必须位于 `<script setup>` 之后，且**仅**包含布局定位声明（flex / grid / 尺寸约束 / 溢出 / 定位）
- 需要视觉样式（颜色、字号、圆角、边框、阴影）时，优先引用 `.zk-*` 外壳类或主题工具类，而不是在 scoped 块内声明
- 没有布局需求时不写 `<style scoped>` 块

### Event Communication

跨组件通信用 `mitt` 事件总线（如用户状态刷新事件）。

---

## General Project Rules

### Directory Structure

```
├── api/              # Protobuf definitions
├── cmd/
│   ├── zki/          # 主程序入口 (main.go)
│   ├── template/     # 代码生成器 (生成 manager/views/search.go 等)
│   ├── commentlint/  # 注释检查器入口 (make lint 驱动 internal/lint/commentlint)
│   └── nic_probe/    # 网卡 XDP 支持探测工具
├── internal/
│   ├── benchmark/    # Benchmarking utilities
│   ├── capture/      # Packet capture engine
│   ├── cmd/          # CLI commands (Cobra)
│   ├── manager/      # HTTP API layer (Gin)，子包：middlewares（中间件）、router（路由注册）、views（视图 In/Out 与处理器）、response（统一响应与错误体系）
│   ├── lint/         # 代码检查 analyzer (commentlint，注释完备性)
│   ├── migrations/   # Database migrations
│   ├── models/       # Data models (core, api, network, tag)
│   ├── network/      # Network protocol processing
│   ├── pkg/          # Shared packages (auth, crypto, db, errors, logging, etc.)
│   ├── provider/     # DI provider definitions
│   ├── reporter/     # gRPC reporting service
│   ├── tagengine/    # OPA 标签引擎
│   └── worker/       # ETL 分布式任务处理 (pcap / tag_event handlers、flow processor)
├── web/              # Vue 3 frontend
└── Makefile          # Build automation
```

### Key Rules

1. 无死代码：及时清理未使用的 import、变量、函数
2. 一致性优先：遵循所改文件/模块既有模式
3. 注释语言：全项目统一使用中文（Go 与前端一致），禁止中英双语或纯英文；送入模型的提示词与工具描述同此（见「LLM 提示词与工具文案（中文约束）」）
4. 类型安全：TypeScript strict mode；Go 适当使用泛型
5. 错误处理：禁止忽略错误，始终包裹上下文
6. DI：新依赖一律走 `provider/` → `Inject` → `Invoke`
7. API：RESTful 资源命名 + POST，URL 模式 `{resource}/{id}/{action}`；集合化动作（delete / enable / disable / activate / deactivate）统一 `POST /{resource}/{action}`、标识符入 body（见「动作接口批量化（Batch Operations）」），`join-group` / `leave-group` / `password/reset` 等非集合化动作保留路径标识符
8. 测试文件与被测源码放在一起（`_test.go` / `_test.ts`）
9. 代码与注释中禁止 emoji，除非显式要求
10. 提交前运行 `scripts/web.sh lint`（web 前端 ESLint）、`scripts/web.sh lint:style`（Stylelint）与 `scripts/web.sh type-check`（vue-tsc），三者全部通过
11. 分层边界：`internal/pkg/llm` 只承载 AI 能力薄封装——OpenAI 兼容客户端、端点配置快照（`ModelConfig`）、embedding 探针与 Markdown 分块器，以及客户端入参契约所需的 `ModelKind` 取值与解析（模型档位 `Slot` 及其取值空间 `AllSlots` / 解析 helper 归 `internal/models/ai`，与该取值空间的定义处同包，并因被持久化模型 `agent_sessions.slot` 用作 GORM 字段类型）；LLM 行为参数注册表（`Specs` / `Params`）与内置工具元信息（`ToolSpecs`）归 `internal/ai/config`，pgvector 列校准（`ReconcileEmbeddingColumn` 等）与 embedding 兼容性判定（`VerifyEmbeddingCompat`）归 `internal/ai/llminfra`，业务数据清洗（`SanitizeBusinessData`）归 `agentkit`；业务枚举按取值空间归 `internal/models/ai`（`ToolSource` / `llmToolSource`、`Slot` / `llmSlot`、`AgentKind` / `AgentRunStatus` / `SessionStatus` / `MessageRole` / `MemoryKind` / `MemoryStatus`），禁止再上移进 `internal/pkg/llm`
12. 提交前运行 `make lint`（Go 注释完备性检查，见「Comments」），并保证 `gofmt`、`go build ./...`、`go vet ./...` 通过
13. 第三方依赖许可证门禁：新增 Go 运行时依赖或 Web 生产依赖前必须通过 `make license-check`；GPL/AGPL/LGPL/SSPL/非商业与未知许可证一律拒绝，例外只允许以「组件@版本 → 已核对 SPDX」登记在 `internal/lint/licensecheck`，并同步更新 `THIRD_PARTY_LICENSES.md`
14. 构建与脚本边界：Makefile 只负责项目文件构建（编译、生成码、静态检查、单元测试、构建依赖管理）；安装、服务启停、测试报告、发布等操作统一封装在 `scripts/` 中；`scripts/` 可以调用 `make`，但 `Makefile` 不得调用 `scripts/*.sh`；镜像构建的文件与脚本（后端 `core.Dockerfile`、前端 `web.Dockerfile`、构建环境镜像 `core.builder.Dockerfile`/`web.builder.Dockerfile`、多架构二进制编译 `build-binary.sh`、web 静态产物编译 `build-web.sh`、入口与本地构建脚本）统一放在 `deploys/build/`；前后端 Dockerfile 不编译源码，只装配构建环境镜像内预编译的产物（`build-binary.sh` 产出的 `dist/bin/zki-linux-<arch>`，`build-web.sh` 产出的 `web/dist` 与 `docs/.vitepress/dist`；后端版本元数据由 `build-binary.sh` 经 `make version-env` 生成、二进制由 `make build-image` 编译，不得在 Dockerfile 内重复实现），任何 Dockerfile 不得直接调用 `make build` / `make build-release`
