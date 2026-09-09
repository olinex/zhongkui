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
	// stdlib
	"context"
	"fmt"

	// third-party
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	"go.uber.org/zap"

	// internal
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

中英双语注释为标准；所有导出类型/函数/常量/结构体字段必须有注释，文件必须有 `// Package xxx` 文档注释。

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
// Layer 1: private provider factory
func newViperProvider() do.Provider[*viper.Viper] { ... }

// Layer 2: injection function
func InjectViper(i do.Injector) { do.Provide(i, newViperProvider()) }

// Layer 3: invocation with error
func InvokeViper(i do.Injector) (*viper.Viper, error) { ... }

// Layer 3b: panic invocation (仅用于启动关键依赖)
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

### Search.toml 枚举筛选配置（search-generate 规范）

- 枚举筛选字段（`enum = true`）的可选项必须以内联数组形式直接写在字段上：`options = [{title = "...", value = "..."}, ...]`；**禁止**使用 `[[<List>ListGet.filterByFields.options]]` 的独立表格列表写法。
- 每个 option 的 `title` 直接等于 `value`（即枚举值本身，如 `pending`、`email`），不得写成完整 i18n 键（如 `model.notificationRecord.type.status.pending`）。
- 展示名由前端按 `model.<modelName>.option.<title>` 定位 i18n 翻译（`web/src/utils/options.ts` 的 `filterOptionTitle`，SearchTable.vue 与流量审计页统一接入）；未配置翻译的枚举（如 HTTP 方法 GET、版本 HTTP/1.1）回退显示原始 title。
- `title`/`value` 顺序固定 `{title, value}`；新增枚举选项须同步补充 zhHans.yaml / en.yaml 的 `model.<modelName>.option.<value>` 键。

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
// Third-party
import consola from "consola";

// Plugins / Utilities
import { registerPlugins } from "@/plugins";

// Components
import App from "./App.vue";

// Types
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

### Code Style

- 2-space 缩进（`.editorconfig` 强制）；UTF-8；EOF 须有换行；去除行尾空白
- ESLint v9 flat config；允许 `&&` 短路与三元表达式
- 组件经 `unplugin-vue-components` 自动注册，禁止手动 import `src/components/` 下的组件

### SFC Structure

```vue
<template>
  <!-- HTML -->
</template>

<route lang="yaml">
name: Page Name
meta:
  title: i18n.key
  requiresAuth: true
</route>

<script setup lang="ts">
// Logic
</script>
```

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
3. 注释语言：Go 代码用中文，前端用英文
4. 类型安全：TypeScript strict mode；Go 适当使用泛型
5. 错误处理：禁止忽略错误，始终包裹上下文
6. DI：新依赖一律走 `provider/` → `Inject` → `Invoke`
7. API：RESTful 资源命名 + POST，URL 模式 `{resource}/{id}/{action}`
8. 测试文件与被测源码放在一起（`_test.go` / `_test.ts`）
9. 代码与注释中禁止 emoji，除非显式要求
10. 提交前运行 `pnpm lint`（web 前端 ESLint）或检查 ESLint 输出
