# Trae Project Rules - zhongkui

## Project Overview

- **Module**: `zhongkui.com/zhongkui` (Go 1.26.3)
- **Type**: 网络流量抓包与数据分析平台
- **Stack**: Go (Gin + GORM + gRPC + Cobra + Viper + OpenTelemetry) + Vue 3 + Vuetify 3

## Development Status 开发阶段

封闭首版本开发阶段（closed first-version development）：数据库 schema、项目架构等可自由重构，无需考虑向后兼容与既有数据迁移。

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
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
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

### Error Handling

- Use project `errors` package（`internal/pkg/errors`，包裹 `github.com/pkg/errors`）：
  - `errors.New(msg)` / `errors.Newf(format, args...)` — 创建错误
  - `errors.WithStack(err)` — 包裹第三方/stdlib 错误（最常见）
  - `errors.Is(err, target)` — 错误链比较
- Sequential error checks use `if-else` cascading style; error 始终是最后一个返回值。

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
- 例外：**局部匿名扫描容器**（`var rows []struct { network.Xxx; utils.BaseTotalOut }`）仅作 GORM 扫描用途，不属于 In/Out 结构体，允许内嵌模型，沿用 notification 模块既有模式。

### 长期记忆（Long-Term Memory）

- **隔离维度固定为「命名空间 + Agent 类型 + 来源会话」**：`agent_memories` 的记忆只在来源会话内可见，跨越会话互不检索。唯一索引 `uq_agent_memories_scope_hash` 与检索索引 `idx_agent_memories_namespace_kind` 均须包含 `session_id`；存储层的精确去重、语义合并与向量/关键词两路检索的 Where 一律带 `session_id`，禁止只按 namespace + agent_kind 过滤。
- **无会话运行不生成记忆**：`session_id = 0`（无会话运行）时 `MemoryStore.Write` 拒绝写入、`Retrieve` 直接返回空、Agent 不注入记忆运行时（`memoryToolContext` 原样返回 ctx）也不检索；避免落进任何会话都检索不到的孤儿桶。
- **管理端不提供新增/编辑**：记忆由 Agent 运行期经 `memory_write` 工具生成，管理接口仅保留列表 / 详情 / 删除 / 启停，`AgentMemoryCreateIn` / `AgentMemoryUpdateIn` 及其路由、RBAC 权限点与前端入口一律不得存在。
- **检索去重复用知识库文本 n-gram（4-gram Jaccard、阈值 `memory.dedupeThreshold`）**；写入去重是 sha256 精确去重 + pgvector 余弦语义合并，两者语义不同，禁止混用。

### 枚举统一注册表（enum 注册表 / search-generate 规范）

- **单一数据来源**：所有枚举值统一注册进 `internal/pkg/enum` 注册表（`enum.Register(enum.Type{Name, Entries})` 或 `enum.RegisterSource(name, entriesSource)`，条目 `enum.Entry{Key, Value}`），每个枚举类（类名如 `channelType`、`httpMethod`）有且仅有一个注册点，类内 key 互斥唯一、value 互斥唯一。数据库内存储该字段枚举值时一律存 **value**（不可存 key）。
- **声明与注册一律在取值空间的归属包**：每个枚举类的唯一注册点落在**取值空间的归属包**——即该取值空间的定义处，声明形态两类：
  - 取值空间固定的枚举：取值空间就地声明在该包内（类型化取值常量 + 取值序列），并在包内 `init()` 经 `enum.EntriesOf` + `enum.Register` 在注册时快照条目；若包内已有该取值空间的常量/清单（如 `base.SupportLanguages`、`base.EffectAllow`、`core.TemplateSource*`、`tag.EventStatus*`、`vtab.redisKind*`、`llm.AllSlots`），则**直接复用**为条目来源，不重复声明常量。当前归属：`language` / `policyEffect` → `internal/pkg/base`；`httpMethod` / `httpVersion` → `internal/models/network`；`notificationSource` / `notificationRecordStatus` → `internal/models/core`；`tagAlertStatus` → `internal/models/tag`；`datasourceStruct` → `internal/pkg/sqlitevtab/vtab`；`llmModelKind` / `llmSlot` / `llmToolSource` / `memoryKind` / `memoryStatus` → `internal/pkg/llm`；`mcpTransport` → `internal/ai/tools/mcp`；`evalStage` / `evalStageStatus` / `diagnosisCategory` → `internal/ai`。**禁止**任何集中的纯声明包（如曾经的 `internal/enums`）：类必须与它的取值空间同包，跨层共用类由消费方 import 归属包引用其常量。
  - 业务注册表来源（channelType / notificationEvent / datasourceType / tagRuleModel / tagEventDimension）：值空间随业务注册表在代码内固定，由**所属业务包**（message / notify / vtab / tagengine）经 `enum.RegisterSource(类名, func() []enum.Entry)` 声明条目来源——条目不在注册时快照，而在每次读取（`Get` / `List`）时从业务注册表实时派生，避免包内多 `init()` 的文件顺序影响条目完整性；与 `/channel/types`、`/template/events`、`/datasource/types`、tagengine 注册表共用单一来源。视图层不再组装任何枚举类。
- **归属判据是取值空间的拥有者，而非存储位置**：*被模型列存储 ≠ 归 model*。取值空间仅作为某个模型列的值域存在时，归该模型包（如 `knowledgeDocStatus` → `internal/models/ai`、`notificationRecordStatus` → `internal/models/core`、`tagAlertStatus` → `internal/models/tag`）；若该词汇由某个非 model 子系统定义或使用（如 `documentFormat` 由 `internal/pkg/docconv` 的 `Format` 定义、`mcpTransport` 由 `internal/ai/tools/mcp` 的传输实现使用），则归该子系统所属包，**不得**因「存在同名模型列」而迁入 model——后者会迫使 `internal/pkg` 反向依赖 `models`，或就地重复声明常量造成定义漂移。编排层（`internal/worker`、`internal/manager`）不设枚举注册点：记录的生命周期状态是持久化行的值域，归其模型包，驱动状态迁移的 handler/view 只是消费方。
- **字段与枚举类绑定**：`search.toml` `[[enums]]` 仅作「结构体类型 + 字段 JSON 名 → 枚举类名」绑定引用（`type` / `field` / `set`），`search-generate` 生成进视图层 `enum_bindings.go` 的 `enumBindings`（绑定视图层结构体字段名，属视图层关注点），meta.go 反射输出 `FieldMeta.EnumType`；运行时字段（如 `NotificationTemplateCreateIn.event`）经 `views.AddEnumBinding(typeName, field, className)`（`enum_registry.go`）在模块 `init()` 补充。`set` 类名仅为引用，不携带可选项——可选项唯一来源是 enum 注册表。
- **i18n label 键**：统一前缀 `enum.<类名>.<key>`（如 `enum.notificationEvent.tag_event_record`、`enum.httpMethod.GET`），只允许经 **key** 翻译 label，禁止经 value 翻译；中英文 `zh-Hans.toml` / `en.toml` 结构一一对应，新增枚举条目（含注册表来源的新渠道/事件）必须同步补两语言键，未命中回退显示 key 本身。**禁止**任何 `model.*.option.*` 平铺或分组键。键完整性由 `internal/manager/views/enum_i18n_test.go` 的 `TestEnumClassesHaveBilingualLabels` 兜底：遍历 `enum.List()` 断言每个类每个条目的 label 键在每种支持语言下均可解析（解析结果不等于 key），新增枚举类/条目必须同步补两语言翻译文件。
- **统一枚举接口**：`POST /admin/enum/get`（按 `{type, lang}` 返回单类条目）与 `POST /admin/enum/list`（按 `lang` 返回全量类条目），出参条目均为 `{key, value, label}`，label 跟随请求语言。前端按（枚举类, 语言）组合缓存（`stores/enums.ts`），仅后端进程重启才可能更新，页面间不重复请求。
- **前端消费契约**：类名常量集中在 `utils/enums.ts` 的 `EnumClass`；`enumFieldTypeOfMeta` 从 meta `bodyOutput` 取字段 `enumType`，`ensureEnum` / `enumEntriesOf` / `enumLabelOf`（value/key 双解析优先 value）/ `enumOptionsOf` 统一经枚举 store；表头/筛选（SearchTable.vue、AdaptiveCell.vue）与页面下拉一律按 `enumType` 定位枚举类，禁止手工维护选项字面量。通知模板 `t` 函数类感知：`t "<类名>" <key|value>` 优先按 value 匹配再按 key 匹配，经 `enum.<类名>.<key>` 渲染 label。
- **目标模型枚举（tagRuleModel）**：Key 为 `tagengine.ModelKey` 推导的短展示键（camelCase，如 `httpTransport`），Value 为完整类型键（数据库存储语义，驱动 `WHERE model = ?` 匹配 tag_summings）；`TagEventRecordHit.Model` 落库存枚举短键（`toEventHit` 收敛），通知模板 `t "tagRuleModel" .Model` 与展示经同一枚举类收敛；S3 明文查看接口的模型 `switch` 经 `tagengine.ModelKey` 幂等匹配（兼容完整类型键/枚举键两种入参）。禁止 snake_case 与完整类型键作为展示键双写。
- **前端禁止声明枚举字面量**：枚举的取值集合、默认值与展示文案**一律由后端接口返回**，前端不得在任何文件中书写枚举取值数组或取值常量——包括 `const SLOT_ORDER = ["flash", "standard", "advanced", "embedding"]`（取值集合）与 `const DEFAULT_SLOT = "standard"`（默认值）这类形式。理由：字面量一旦落在前端就会与后端注册表/参数配置漂移（曾出现助手页把默认档位写死 `standard`，运维把参数 `agent.defaultSlot` 改为 `advanced` 后该页仍按 `standard` 生成）。取值集合与默认值经接口下发（搜索类走 `enumType` / 参数类走 `options` + `enumType` / 状态类走 `defaultSlot` 等），选项文案经 `enum.<类名>.<key>` 渲染，需要固定展示顺序时以接口返回顺序为准，前端不再自行排序。
- **参数注册表的枚举取值**：参数注册表（`internal/pkg/llm` 的 `ParamSpec`）中取值空间属于某个枚举类的参数，必须同时声明 `Options`（封闭取值清单：可选项与写入校验的唯一来源，由枚举/业务注册表**派生**得出，禁止重复书写字面量）与 `EnumType`（枚举类名，仅供消费方按 `enum.<类名>.<key>` 渲染 label）；`param/list` 出参带 `enumType`，前端据此经枚举 store 渲染下拉项文案，未声明 `EnumType` 时字面量本身即文案（如 `retrieval.fusion`）。枚举类名与取值清单的一致性由 `internal/manager/views/llm/param_enum_test.go` 的守卫测试兜底。

### 字段校验标签（validator 自定义 tag）

- **单一登记点**：全部自定义校验规则的 tag → 校验函数登记表唯一维护在 `internal/pkg/validator` 的 `Funcs()`，`internal/provider/validator.go` 与各测试 setup（`views/base_test.go`、`views/*/testmain_test.go`、`views/*/setup_i18n_test.go`）一律经 `validator.Funcs()` 取用，禁止任何调用方重复抄写 tag 清单（漏抄会让该 tag 在对应场景下被判为未注册规则，表现为绑定失败而非编译错误）。
- **能力扩展范式**：新增规则一律「`TagXxx` 常量 + `IsValidXxx(string) bool` 谓词 + `ValidateXxx(fl validator.FieldLevel) bool` 适配器」，在 `Funcs()` 登记，并在中英文 locale（`internal/pkg/translate/locales/zh-Hans.toml` / `en.toml`）的 `[validator]` 段补 `validator.<tag>` 文案（缺失即 `MustLocalizeMessage` panic）。
- **通用字段级 tag（禁止就地实现同等能力）**：
  - `enum=<枚举类名>`：取值必须命中 `internal/pkg/enum` 注册表中该类的某个 **value**（按 value 严格匹配，不接受 key）；类名未注册属编程缺陷，直接 panic。
  - `httpUrl`：绝对 `http` / `https` 地址（scheme 与 host 均非空）；可选时配 `omitempty`。
  - `jsonObject`：所有取值均为字符串的 JSON 对象文本（字面量 `null`、数组、非字符串取值一律不合法）；可选时配 `omitempty`。
  - `trimmed`：字符串首尾无空白。后端**只判定、不静默裁剪**——裁剪会让前端提交原值与实际落库值不一致；由此前端对同名单行输入必须 trim，文本域（`v-textarea`）豁免（见「String Input Trimming 字符串输入去空白」）。
- **就地校验禁令**：视图层 In 结构体的字段合法性**一律由绑定标签表达**，禁止书写 `normalizeXxx` / `parseXxx` 这类仅做 trim、枚举白名单、URL 或 JSON 形态判断的私有方法；处理器只做「绑定 → 业务约束（唯一性、存在性等）→ 落库」。数据层解码（如密文列解出的 JSON 文本转 `map[string]string`）不属于校验，保留在处理器内。
- **条件必填**：同一 In 结构体存在多种用法时（如「带 id 时其余字段被忽略」），用 `required_if=<字段> <值>` + `omitempty` 表达条件必填，而非无条件的 `required`。

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

### String Input Trimming 字符串输入去空白

- 所有单行文本输入框（用户名、邮箱、昵称、名称、标题、Cron 表达式、搜索关键字等）在提交或作为查询条件前**必须去除首尾空白字符**（trim），避免用户误输入不可见的空白字符导致登录失败、查询无结果或创建出看似相同实则不同的记录。
- 强制豁免（禁止 trim）：
  - **密码字段**：密码可合法包含首尾空格，trim 会改变凭据本身
  - **多行文本域（`v-textarea`）**：内容中的空白具有语义（模板正文、表达式/代码、长描述）
  - **校验性输入**（如 `ConfirmDialog` 的双重确认随机串）：仅用于精确比对，不属于数据输入
- 推荐实现方式：优先在 `@/schemas/` 的 yup schema 上对单行字符串字段统一使用 `.transform((v) => v?.trim())` 做规范化，避免散落在各页面提交逻辑中；未走 schema 的搜索/筛选字符串输入（`AdaptiveField`、`ScrollSearchField` 等）同样应在使用前 trim。

### 样式体系分层（Style Layering）

本文件是前端设计系统的**唯一信任源**——视觉令牌、色板、组件视觉基线与开发禁令全部收敛于此，不存在第二份设计文档（原 `web/src/styles/DESIGN.md` 已并入本文件并删除）。下列源文件是规范的**实现载体**：

| 载体 | 职责 |
|------|------|
| `web/src/plugins/vuetify.ts` | 主题色（`theme.colors`）与组件默认值（`defaults`） |
| `web/src/styles/tokens.scss` | 设计令牌：字体 / 字号 / 字重 / 间距 / 圆角 / 边框 / 高度 / 宽度 / 阴影 |
| `web/src/styles/main.scss` | 全局样式与 `.zk-*` 复用外壳类 |
| `web/src/styles/settings.scss` | Vuetify SASS 覆写（`$border-radius-root` / `$body-font-family` 等） |
| `web/src/components/base/AppDialog.vue` | 对话框骨架 |
| `web/src/components/base/ExpansionPanelTitle.vue` | 展开面板标题区骨架 |
| `web/src/utils/editorHighlight.ts` | 编辑器语法高亮配色 |
| `web/src/utils/chartTheme.ts` | 图表主题合成 |
| `web/src/layouts/{MainLayout,CentralLayout}.vue`、`web/src/components/base/AppHeader.vue` | 布局骨架 |

样式按**四层**组织，上层只能引用下层，禁止越层就地造值：

| 层 | 位置 | 职责 | 允许内容 |
|----|------|------|----------|
| 1 令牌层 | `web/src/styles/tokens.scss` | **唯一数值来源** | `$zk-*`（SASS）+ `--zk-*`（CSS 变量）双导出：字体族、字号档位、字重、控件高度、工具栏高度、表格行高、4px 间距栅格、圆角四档、hairline 边框、对话框宽度四档、容器宽度、内容尺寸约束、编辑器外壳、面板表面透明度、z-index、过渡时长、阴影 |
| 2 主题层 | `web/src/plugins/vuetify.ts` | **唯一颜色来源** + 组件默认值 | `theme.colors`（导出为 `--v-theme-*`）+ `defaults`（组件 `variant` / `density` / `rounded` / `elevation` / `maxWidth`） |
| 3 外壳层 | `web/src/styles/main.scss` | 跨组件复用外观 | `.zk-*` 复用类 + Vuetify / CodeMirror 内部类覆盖 |
| 4 组件层 | `*.vue` 的 `<style scoped>` | **仅**布局定位 | flex / grid、尺寸约束、溢出、定位 |

要改视觉先改对应的唯一来源：

- **数值**（字号 / 间距 / 圆角 / 边框 / 阴影 / 高度 / 宽度档位）→ `web/src/styles/tokens.scss`，新增视觉数值必须先在此登记令牌
- **颜色**（含语义色与文字色）→ `plugins/vuetify.ts` 的 `theme.colors`，消费方式 `rgb(var(--v-theme-xxx))`
- **跨组件复用外观**（面板 / 编辑器外壳 / 区块标题 / 提示文本）→ `styles/main.scss` 的 `.zk-*` 类
- **编辑器语法高亮配色** → `utils/editorHighlight.ts`（`LIGHT_PALETTE` / `DARK_PALETTE` / `sqlHighlightFor`），禁止新增第二套配色

外壳层（`main.scss`）的 `.zk-*` 类按用途分组，下表为职责摘要，**完整清单以 `styles/main.scss` 为准**：

| 族 | 类 | 职责 |
|----|-----|------|
| 边框 | `.zk-hairline` / `-top` / `-right` / `-bottom` | 四边或单边 1px hairline 分隔 |
| 面板 | `.zk-panel`（+ `--subtle`）、`.zk-code-preview`、`.zk-interactive-card`、`.zk-tabs-embedded` | 表面容器（hairline + `radius-md`）；弱化面板用 `on-surface` 6% 底 + `radius-sm`（代码块、键值行、预览）；可点卡片 hover 转 `primary` 描边；嵌入标签页压平内层 `v-card` |
| 排版 | `.zk-section-title` / `.zk-section-subtitle` / `.zk-editor-title` / `.zk-hint` / `.zk-error-text` / `.zk-label-caps` / `.zk-mono-emphasis` / `.zk-metric` / `.zk-step-badge` | 区块标题、次级标题（主标题旁的同级弱化标题，如对话框标题后的实体名）、编辑器标题、提示、字段错误、分组标签、技术标识强调、指标值、步骤徽标 |
| 编辑器 | `.zk-editor`（+ `.is-error`）/ `.zk-editor-header` | CodeMirror 外壳（主题感知 hairline + `radius-sm`）与标题行 |
| 请求态 | `.zk-request-loading` / `.zk-request-spinner` | `v-request` 加载态：隐藏按钮原内容并注入 spinner 覆盖层 |
| 布局 | `.zk-page-container` / `.zk-centered-layout` / `.zk-centered-surface` / `.zk-filter-form` / `.zk-breadcrumbs` / `.zk-refresh-group` | 页面容器、居中骨架、筛选表单（压掉空提示区以保持单行）、面包屑、刷新分体按钮组 |
| 交互态 | `.zk-drag-over` / `.zk-row-hover` / `.zk-row-selected` | 拖拽悬停行、可点击行悬停（primary 6%）、常驻选中行（展开面板为「描边盒 + 盒内平铺条目」，盒由外层容器提供，见「展开面板族」） |
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
- 控件高度、工具栏高度、表格行高一律取令牌（`--zk-control-height-compact|default`、`--zk-toolbar-height-compact`、`--zk-table-row-height-compact`、`--zk-table-header-height-compact`）
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

- **默认值集中声明**：凡 `plugins/vuetify.ts` 的 `defaults` 已覆盖的属性（`variant` / `density` / `rounded` / `elevation` / `maxWidth` / `size`），用法处**禁止**重复声明。默认值全清单见本节末「组件默认值清单」，与 `plugins/vuetify.ts` 的 `defaults` 对象逐项对应。
- **一个功能族一个变体基线**：同一功能族的组件（全部对话框、全部 CodeMirror 编辑器、全部信息面板、全部 chip、全部日期时间字段）共享同一骨架与同一视觉基线；需要变体时必须**成对定义**（如 `size: "sm" | "md" | "lg" | "xl"`）并在样式层集中实现，禁止各组件内自写一套。
- **输入族变体基线**：所有输入类组件（`VTextField` / `VSelect` / `VAutocomplete` / `VCombobox` / `VTextarea` / `VNumberInput` / `VFileInput` / `VField`）必须**全部**在 `plugins/vuetify.ts` 的 `defaults` 中登记 `variant: "outlined"` + `density: "compact"`。理由：Vuetify 这些组件**自带的默认 variant 是 `filled`**，漏登记不会报错，只会静默退化为灰色填充，与同族 outlined 控件并排时明显分裂（`VNumberInput` / `VFileInput` 曾长期如此）。新增或升级 Vuetify 输入组件时必须同步补登记，并在本节末「组件默认值清单」中更新对应行——该清单与 `defaults` 对象必须逐项一致。
- **扩展面板单展开**：`VExpansionPanels` 已由 `plugins/vuetify.ts` 的 `defaults` 固定为 `variant: "accordion"`，语义是**同一时刻只展开一项**，用法处**禁止**声明 `multiple`。理由：`multiple` 与 `variant` 在 Vuetify 内是**正交**的两组属性（`variant` 只落 `--variant-accordion` 样式类，是否允许多开只由 `multiple` 决定），一旦声明就只保留手风琴外观、却突破其单开语义，出现多个面板同时展开；同一手风琴内的 `v-model` 也必须是**单值**（无展开项时为空串），不得声明为数组。先例：`components/admin/AgentParamPanel.vue`、`pages/[language]/ai/knowledge.vue`。
- **展开面板标题区**：全部 `v-expansion-panels` 的标题一律走 `web/src/components/base/ExpansionPanelTitle.vue`（图标头像 + `.zk-section-title` 标题 + `.zk-hint` 说明；右侧 `actions` 槽插在组件统一渲染的 chevron 之前），**禁止**在用法处自写 `v-expansion-panel-title` 结构或手写 chevron。状态类图标需要旋转动效（运行中 / 加载态）时走组件的 `spin` prop（复用 `@mdi/font` 的 `.mdi-spin` 动画），用法处不得另挂图标。先例：`pages/[language]/profile.vue`、`components/admin/AgentParamPanel.vue`、`components/admin/RegoDryRunPanel.vue`、`components/admin/TagEventRuleDryRunPanel.vue`、`pages/[language]/ai/knowledge.vue`、`components/forms/DatasourceGuidePanel.vue`。
- **展开面板族 = 描边盒 + 盒内平铺条目**：全站 `v-expansion-panels` 一律呈现「一个 hairline 描边盒，盒内以面板间 1px 分隔线分条」，条目自身无边框、无悬浮阴影。描边盒**只能由外层容器提供，且恰好一层**（`.zk-panel` 包裹 div、`TabView` 卡片、`.zk-editor` 外壳三种皆可，按所在页面结构择一）；`v-expansion-panels` 自身**禁止**再带 `.zk-panel` / `.zk-editor` / `.zk-hairline` 等描边类（曾出现面板组自身带 `.zk-panel`、外层参考栏也带 `.zk-panel` 的双层框，以及「有阴影 / 无边框平铺 / 有边框」三种观感并存）。`.v-expansion-panel__shadow` 抬升阴影已由 `styles/main.scss` 全局 `display: none`，用法处不得覆写或重新引入。其余外观（标题 `16px 24px` / 48px、展开态 64px；内容 `8px 24px 16px` 内边距；手风琴 variant 下展开项 `margin-top: 0`）保持 Vuetify 默认，禁止在用法处覆写内边距 / 底色 / hover 遮罩。先例：`pages/[language]/profile.vue`（包裹 div 提供盒）、`components/admin/AgentParamPanel.vue`（TabView 卡片提供盒）、`components/admin/{Rego,TagEventRule}DryRunPanel.vue`（`.zk-editor` 外壳提供盒）、`components/forms/DatasourceGuidePanel.vue`（外层参考栏提供盒，根节点已去掉自身描边）。
- **对话框**：一律走 `web/src/components/base/AppDialog.vue` 骨架（紧凑工具栏「图标 + 标题 + 关闭图标按钮」+ `VDivider` + 可滚动主体 + 可选右对齐页脚）；宽度只取 `size` 四档（映射 `--zk-dialog-width-sm|md|lg|xl`），禁止自由 px。例外：全屏 + 底部过渡形态的 `NetworkStackDialog.vue` 保留原生 `VDialog`。
- **代码编辑器**：全部 CodeMirror 编辑器共享 `.zk-editor` 外壳（主题感知 hairline 描边、`--zk-radius-sm`、统一内边距 / 最小高度 / 字号），校验失败态加 `.is-error` 修饰类；语法配色继续复用 `utils/editorHighlight.ts`。
- **图标按钮**：统一 `variant="text"`（`density="compact"` 由 `VBtn` 默认值提供）+ `:title`（tooltip）+ `:aria-label`。
- **图标语义**：操作类用 `mdi-*` 动词图标（`mdi-close` / `mdi-refresh` / `mdi-delete`）；状态类用「语义色 + 图标」组合（`color="success|warning|error|info"`）；禁止装饰性图标，禁止只靠颜色表达状态。
- **密码类字段的明文 / 掩码切换图标**：`append-inner-icon` 表达字段**当前状态**，而非点击后将执行的动作——已明文展示时为 `mdi-eye`，处于掩码态时为 `mdi-eye-off`。`type` 与图标必须由同一开关驱动、方向一致：`:type="revealed ? 'text' : 'password'"` 配 `:append-inner-icon="revealed ? 'mdi-eye' : 'mdi-eye-off'"`，`@click:append-inner` 翻转该开关。禁止反向映射（掩码态显示睁眼图标）。先例：`components/forms/PasswordField.vue`、`components/admin/LlmModelPanel.vue`、`pages/[language]/datasource.vue`、`pages/[language]/notification/channel.vue`。
- **组件自有内边距不外覆**：自带内边距的组件（`v-btn` / `v-chip` / `v-list-item` / `v-text-field` 等）其内部间距由组件自身拥有，用法处**禁止**用间距工具类覆盖其**纵向**内边距（`pa-*` / `py-*` / `pt-*` / `pb-*`）。理由：Vuetify 间距工具类带 `!important`、会覆盖组件内边距，而组件高度是固定值（`VBtn` 的 `--v-btn-height` 在 `density="compact"` 下仅 24px）——上下各吃 8px 后内容区只剩 8px，装不下 20px 的行盒，文字会**溢出到背景之外**（登录页曾出现「登录」二字突破按钮底色 4px）。仅**横向**覆盖（`px-*`）不触发该问题，可用于有意收紧横向留白。按钮行的间距一律由容器提供：`v-card-actions` 自带 `padding: 8px` + `gap: 8px`，其余场景用容器的 `ga-*`。
- **列表页骨架**：**搜索驱动列表页**（后端提供 `SearchIn` / `SearchOut` / `SearchOptionsOut` 搜索契约）必须使用 `web/src/components/table/SearchTable.vue`，禁止手写骨架；**非搜索驱动列表页**（客户端数据 / 看板类，后端无搜索契约，如 `pages/[language]/etl/{task,queue,scheduler}.vue`）**不得**强行套用 `SearchTable`，但必须逐项复用其视觉骨架：扁平卡片（`<v-card flat>`）内的「筛选行（`px-4 pt-3 pb-2` + `.zk-filter-form`）→ 操作按钮行（`px-4 pb-2 d-flex justify-end align-center ga-2` + `action-button-group` + `column-visibility-menu`）→ `<v-container fluid class="px-4 pt-0 pb-4">` 包裹的表格」；单元格一律用 `adaptive-cell`，列显示配置一律用 `useColumnVisibility`。理由：骨架一致是硬要求、组件复用是手段——当组件契约不匹配时，以「逐项复用骨架类」达成同一视觉结果，禁止各自发明间距与结构。
- **全屏居中页骨架**：认证页（登录 / 忘记密码 / 重置密码 / 重置成功）、错误页（401 / 404）与功能不可用说明页（`pages/[language]/feature-unavailable.vue`）必须走 `web/src/layouts/CentralLayout.vue`，不得自建居中容器；尺寸与滚动由骨架的 `.zk-centered-layout` / `.zk-centered-surface` 外壳类统一承载——宽度固定 `--zk-centered-surface-width`(420px)、不随视口放大，高度上限取 `v-main` 的可视高度、超出由表面内部滚动（页面整体不滚动）。页面只提供内容，**禁止**在页面内声明居中、宽度或高度（曾出现登录页卡片内联 `style="width: min(420px, 100%)"` 与骨架内联 `height="100%"`，前者随分辨率被撑到 868px）。

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

### 视觉层级与文本规范（Visual Hierarchy & Text）

**优先用视觉结构表达功能划分，而不是写说明文本**：区块靠「标题 + 分隔/边界 + 间距」表达层级，提示收敛为 icon / tooltip。

文本颜色只允许取自 Vuetify 主题 tokens（`web/src/plugins/vuetify.ts` 的 `--v-theme-*` 或对应 `text-*` 工具类），**禁止** `text-medium-emphasis` 等固定透明度弱化——其背景色叠算对比不足。按文字角色选用「字号 + 颜色」组合，确保 light/dark 双主题下对比达标。

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
- 不要用 `text-medium-emphasis` / `text-low-emphasis` 表达「次要」——统一改用 `text-on-surface-variant`。
- 表头一律 caption + `text-on-surface-variant` + 600 + uppercase，由 `styles/main.scss` 的 `.v-data-table th` 统一实现，页面不得重复声明。

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
5. **禁止**重复声明 `plugins/vuetify.ts` 的 `defaults` 已覆盖的属性（`variant` / `density` / `rounded` / `elevation`）。
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

- **导航抽屉**（`MainLayout.vue`）：常驻抽屉 `width="220"`，开启 `rail` + `expand-on-hover`——收起为 56px rail，悬停展开。抽屉底部 `#append` 槽提供固定按钮（纯图标 `v-btn`、`variant="text"`、居中），让用户常驻展开（`expand-on-hover="!pinned"`），偏好持久化到 `localStorage`（`localManager.getNavPinned` / `setNavPinned`）。父级项作为分组标题（`color="primary"`）并遵循**手风琴行为**（`v-list-group` + `open-strategy="single"` + `v-model:opened`）：默认只展开当前路由所在分组，点击其他分组会收起上一个，点击已展开分组则收起它；展开状态用 Vuetify chevron 指示，展开 / 收起的高度过渡加速到 `0.15s`（见 `MainLayout.vue` 的 `:deep(.v-list-group__items)` 覆写）。rail 模式下全部分组收起，以免在 56px 宽度内溢出；悬停 rail 时重新展开当前路由所在分组。子项缩进 16px（`margin-left: 16px`，`variant="tonal"`）。抽屉与内容区之间由 `.zk-hairline-right` 外壳类分隔。**功能开关只标注可用性、不参与过滤**：功能关闭的子项仍展示，以不可用态呈现（标题 `text-on-surface-variant` 置灰 + 锁定标识，见「功能开关与不可用态 · 导航标识徽章」），点击进入页面后由路由守卫统一引导到「功能不可用」说明页。一级分组标题行同样带标识：全部子项不可用时跟随展示锁定标识，全部子项受命名空间影响时跟随展示命名空间标识（两者同时命中时锁定优先）。
- **内容区**：`<v-main class="d-flex mb-6">` 包裹 `<v-container class="pa-4 pa-md-6 zk-page-container">`。`.zk-page-container` 把内容上限压在 **`--zk-container-max-width`（1440px）** 并水平居中；页面内容放在 `rounded="lg"` 的 `v-sheet` 中（12px，与全局 `.v-card` 圆角对齐，避免圆角卡片嵌在直角容器里留下月牙形空底）。页面不得自行内联 `max-width`，也不得改动容器内边距。认证页、错误页与功能不可用说明页改用 `CentralLayout`（见「组件与变体约定 · 全屏居中页骨架」）。
- **AppBar**（`AppHeader.vue`）：扁平（`elevation="0"`），与页面内容之间由 `.zk-hairline-bottom` 外壳类分隔。工具栏按钮用 `variant="text"`。
- **全局 focus**：`:focus-visible` 渲染 2px `primary` 描边 + 2px offset（`main.scss`），禁止抑制或替换。
- **表单 / 列表页组装顺序**：每个列表页依次由 `TabView`（标签栏）→ 筛选行 → `ActionButtonGroup` → `SearchTable` 组成：
  - 标签栏独占页面 sheet 顶部一行。
  - 单条件简单筛选（单个 `v-select` / 日期字段）置于同一行左侧；多条件复杂筛选（表达式 / 条件 chip，如 `ExpressionField`）独占一行。
  - 操作按钮组（`ActionButtonGroup`）位于筛选行右侧，默认纯图标：`variant="text"` 使描边与 sheet / card 表面一致，按钮名经 `:title` tooltip 悬停展示。`collapsible` 且收起时，常驻的刷新 / 新建按钮保持可见，次要按钮收到 chevron 之后。

### 功能开关与不可用态（Feature Flags & Unavailable State）

**核心原则：功能未开通时，相关入口不隐藏，而是「展示但不可用」，并把用户引导到开通入口。**

- **单一数据来源**：功能可用性与「不可用原因」一律由后端 `features` 接口（`internal/manager/features` → `GET /api/v1/features`）下发，原因按请求语言本地化。前端只保留功能键常量（`stores/features.ts` 的 `FEATURE_*`）用于引用，**禁止**在前端自行推断可用性或自造原因文案。`stores/features.ts` 是前端唯一访问点，按语言缓存（`loadedLang`），语言切换自动重载，拉取失败按「全部关闭」处理且不标记已加载（下次导航自动重试）。
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

以下是上文各节未覆盖的独立条款：

- **不要使用重于 `$zk-shadow-md` 的阴影**，也不要在同一元素叠加多层阴影。
- **不要用非语义色表达状态**（如自造绿色表示成功）；语义色同样不得用于纯装饰。
- **不要用比例字体渲染技术性取值**（ID / 哈希 / hex / 字节数一律 `.font-mono`）。
- **不要抑制或替换 `:focus-visible`** 的全局 primary 描边。
- **不要发明未定义的颜色名**（如 `third` / `tertiary` / `accent2`），也不要在组件内重述主题色（本地 `const primary = "#2563EB"` 或重复书写 `border: 1px solid ...` 字符串）。
- **不要新增第二套语法高亮配色**，也不要在组件内新建 `.zk-*` 类——新外壳统一注册在 `main.scss`。
- **不要手搭对话框、不要复制编辑器外壳、不要用散文解释界面**：三者正确做法分别见「组件与变体约定 · 对话框 / 代码编辑器」与「视觉层级与文本规范 · 说明文本收敛」。

### 单一信任源（Single Source of Truth）

- **本文件（`AGENTS.md`）是前端设计系统的唯一信任源**：视觉令牌、色板、组件视觉基线与开发禁令全部收敛于此，不存在第二份设计文档（原 `web/src/styles/DESIGN.md` 已并入本文件并删除）。
- 上列源文件（`plugins/vuetify.ts` / `styles/tokens.scss` / `styles/main.scss` / `styles/settings.scss` / `components/base/AppDialog.vue` / `utils/editorHighlight.ts` / `utils/chartTheme.ts`）是规范的**实现载体**，规范只在必要处指向它们；`.zk-*` 类的完整清单以 `styles/main.scss` 为准（本文件只列职责摘要与约定），组件默认值清单与 `plugins/vuetify.ts` 的 `defaults` 对象同步维护。
- 本文件的约定与实现**不得相互矛盾**：发现不一致时，先判断哪一方是错的，再同时修正实现与本文件，禁止只改一边，也禁止为此新增第二份文档。

### Code Style

- 2-space 缩进（`.editorconfig` 强制）；UTF-8；EOF 须有换行；去除行尾空白
- ESLint v9 flat config；允许 `&&` 短路与三元表达式
- 组件经 `unplugin-vue-components` 自动注册，禁止手动 import `src/components/` 下的组件
- 提交前必跑项：`pnpm lint`（ESLint）+ `pnpm lint:style`（Stylelint，覆盖 `src/**/*.{css,scss,vue}`）+ `pnpm type-check`（vue-tsc）**三者全部通过**

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
│   └── nic_probe/    # 网卡 XDP 支持探测工具
├── internal/
│   ├── benchmark/    # Benchmarking utilities
│   ├── capture/      # Packet capture engine
│   ├── cmd/          # CLI commands (Cobra)
│   ├── manager/      # HTTP API layer (Gin)，子包：middlewares（中间件）、router（路由注册）、views（视图 In/Out 与处理器）、response（统一响应与错误体系）
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
3. 注释语言：全项目统一使用中文（Go 与前端一致），禁止中英双语或纯英文
4. 类型安全：TypeScript strict mode；Go 适当使用泛型
5. 错误处理：禁止忽略错误，始终包裹上下文
6. DI：新依赖一律走 `provider/` → `Inject` → `Invoke`
7. API：RESTful 资源命名 + POST，URL 模式 `{resource}/{id}/{action}`
8. 测试文件与被测源码放在一起（`_test.go` / `_test.ts`）
9. 代码与注释中禁止 emoji，除非显式要求
10. 提交前运行 `pnpm lint`（web 前端 ESLint）、`pnpm lint:style`（Stylelint）与 `pnpm type-check`（vue-tsc），三者全部通过
