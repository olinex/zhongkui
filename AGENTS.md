# Trae Project Rules - zhongkui

## Project Overview

**Module**: `zhongkui.com/zhongkui` (Go 1.26.3)
**Type**: 网络流量抓包与数据分析平台
**Stack**: Go (Gin + GORM + gRPC + Cobra + Viper + OpenTelemetry) + Vue 3 + Vuetify 3

---

## Go Backend Conventions

### Import Organization

Always use three-block grouping, separated by blank lines, sorted alphabetically within each block:

```go
import (
    // Block 1: Standard library
    "context"
    "fmt"
    "net/http"

    // Block 2: Third-party libraries
    "github.com/gin-gonic/gin"
    "github.com/samber/do/v2"
    "go.uber.org/zap"

    // Block 3: Internal project packages
    "zhongkui.com/zhongkui/internal/models/core"
    "zhongkui.com/zhongkui/internal/pkg/errors"
    "zhongkui.com/zhongkui/internal/pkg/logging"
)
```

### Naming Conventions

| Category | Style | Example |
|----------|-------|---------|
| Exported types/functions | PascalCase | `JwtAuthenticator`, `GetUser` |
| Unexported functions | camelCase | `filePath`, `newViperProvider` |
| Constructor functions | `NewXxx` | `NewJwtAuthenticator`, `NewProductionLogger` |
| Getter functions | `GetXxx` | `GetUser`, `GetAuth` |
| Receiver variables | Single lowercase letter of type | `(j *JwtAuthenticator)`, `(u *User)` |
| Package-level constants | UPPER_SNAKE_CASE | `ANONYMOUS_USERNAME`, `AUTH_CTX_NAME` |
| Package-level error sentinels | `Err` prefix | `ErrUserNotExist`, `ErrJwtBadToken` |
| Package-level zero-value variables | camelCase | `anonymous` |

### Error Handling

- Use project's own `errors` package (`zhongkui.com/zhongkui/internal/pkg/errors`) which wraps `github.com/pkg/errors`
- `errors.New(msg)` / `errors.Newf(format, args...)` — create new errors
- `errors.WithStack(err)` — wrap third-party/stdlib errors with stack trace (most common pattern)
- `errors.Is(err, target)` — error chain comparison
- Always use `if-else` cascading style for sequential error handling:

```go
if cryptoKey, err := j.cfg.ExtractCryptoKey(j.kd); err != nil {
    return "", CreateAccidentError(err)
} else if tokenStr, err := token.SignedString(cryptoKey); err != nil {
    return "", ErrJwtBadToken
} else {
    return j.appendPrefix(tokenStr), nil
}
```

- Error is always the last return value

### Comments

Bilingual Chinese-English comments are standard. Every exported type, function, constant, and struct field must have a comment:

```go
// UserClaims 自定义 JWT 声明结构
// 继承 jwt.RegisteredClaims 标准声明，并添加用户相关字段
type UserClaims struct {
    jwt.RegisteredClaims
    UnionID serializer.HexUUID `json:"unionID"`
}
```

File-level `// Package xxx` doc comments are required and describe package responsibilities.

### Struct Organization

- Use struct embedding for composition (e.g., `BaseMeta` → `TimestampMeta` → `CaptureMeta` → `AsPacket`)
- Group fields logically with blank lines between groups
- All struct fields must have GORM/JSON/mapstructure tags as appropriate

### Dependency Injection

Use `samber/do/v2` with a fixed three-layer provider pattern:

```go
// Layer 1: Private provider factory
func newViperProvider() do.Provider[*viper.Viper] { ... }

// Layer 2: Injection function
func InjectViper(i do.Injector) { do.Provide(i, newViperProvider()) }

// Layer 3: Invocation with error
func InvokeViper(i do.Injector) (*viper.Viper, error) { return do.Invoke[*viper.Viper](i) }

// Layer 3b: Invocation with panic (only for startup-critical deps)
func MustInvokeViper(i do.Injector) *viper.Viper { return do.MustInvoke[*viper.Viper](i) }
```

All providers live under `internal/provider/` and are injected centrally in `root.go`'s `PersistentPreRunE`.

### Configuration

- Use `mapstructure` tags with Viper
- Use `default` struct tag for default values (`github.com/creasty/defaults`)
- Use `comment` struct tag for config descriptions — the `comment` tag value is rendered by the `config` command (via `DescribeConfig`) and MUST be written in Chinese (中文) as the source of truth
- Optional English translation: add `config.<path>` entries in `internal/pkg/translate/locales/en.toml` (e.g. `[config.i18n.rootPath]`); `describe config` renders via `I18n.Localize` with the Chinese `comment` as fallback
- Type-safe extraction via `config.ExtractConfig[T]()`
- Config prefix constants: `PREFIX = "capture"`

### CLI (Cobra)

- 所有命令的 `Short`/`Long` 帮助描述、flag 的 usage 描述以及交互式提示（`prompt` 标签、pterm 交互输入提示）必须使用中文书写，作为默认语言文案
- 命令 `Use` 字段保持命令名/语法不变，不参与汉化
- CLI 文案统一接入 go-i18n（`internal/pkg/translate`），翻译键布局：
  - `cmd.<path>.short` / `cmd.<path>.long` — 命令帮助描述（如 `cmd.run.web.short`）
  - `flag.<path>.<name>` — flag usage（如 `flag.create.tls.ttl`）
  - `cli.<topic>` — 交互式提示与确认文案（如 `cli.confirm.enter`）
  - 英文翻译维护在 `internal/pkg/translate/locales/en.toml`；未命中翻译时回退到命令定义中的中文原文
- `prompt:"..."` 标签的交互提示由 `internal/pkg/cliinput` 渲染时按 `cli.input.prompt.<StructName>.<FieldName>` key 翻译（如 `cli.input.prompt.TLSInputs.Name`），中文标签为回退文案；`cliinput` 通过 `SetTranslator` 注入翻译实现（`internal/cmd/i18n.go` 的 `init()` 注入 `cliT`），cliinput 内部固定文案 key 为 `cli.input.prompt.confirm-suffix` / `cli.input.prompt.comma-separated-suffix` / `cli.input.mismatch`（前缀常量 `CLI_I18N_PREFIX`）
- 语言切换：环境变量 `ZKI_LANG`（如 `en` / `zh-Hans`），默认 `zh-Hans`；取值统一在 `internal/pkg/base/environments.go` 的 `GlobalEnv.Lang`（`GLOBAL_PREFIX` 为 `zki` + `envconfig:"LANG"`），CLI 通过 `base.Env.Lang` 读取
- 翻译入口：`internal/cmd/i18n.go` 的 `cliLang()` / `getCliI18n()` / `cliT()`，命令树本地化在 `Execute()` 中的 `localizeCmdTree()`
- 覆盖 cobra 内置英文命令：`internal/cmd/help.go`（中文 help 命令）、`internal/cmd/completion.go`（中文 completion 命令），根命令 `init()` 中通过 `RootCmd.SetHelpCommand(helpCmd)` 注册
- `--help` 不会触发 `PersistentPreRunE`，因此语言切换逻辑必须放在 `Execute()` 入口的 `localizeCmdTree()` 中

### Protobuf

- `proto3` syntax, files in `api/`
- Package naming: `{service}.v1`
- Enum values: UPPER_CASE (`CREATED`, `MODIFIED`, `DELETED`)
- Comments: `///` triple-slash
- Generated code goes to `internal/pkg/protobuf/`

### General Go Rules

- `context.Context` is always the first parameter
- Function types use aliases for behavior contracts: `type LiveFunc = func(ctx context.Context, task *Task) error`
- Single-method interfaces defined at the consumer side
- Use `errgroup` for goroutine lifecycle management
- Custom serializer types implement `driver.Valuer` + `sql.Scanner`

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
| Type definition files | `@types/` prefix dir | `@types/admin.ts`, `@types/base.ts` |

### Script Setup

Always use `<script setup lang="ts">` with component `name` attribute:

```vue
<script setup lang="ts">
const searchTableRef = useTemplateRef("searchTableRef");
</script>
```

Vue APIs (`ref`, `computed`, `watch`, `onMounted`, etc.) are auto-imported — never manually import them.

### Import Organization

Order imports in blocks with comment labels:

```ts
// Third-party
import consola from "consola";

// Composables
// (auto-imported Vue APIs — no imports needed)

// Plugins / Utilities
import { registerPlugins } from "@/plugins";

// Components
import App from "./App.vue";

// Types
import type { App } from "vue";
```

Always use `import type` for type-only imports.

### API Layer

- Base HTTP client via `axios.create()` in `apis/base.ts`
- API function naming: `{resource}{Operation}` — e.g., `userListGet`, `groupCreate`, `capturerDelete`
- Type naming: `{Entity}{Action}In` / `{Entity}{Action}Out` — e.g., `SearchIn`, `UserDetailOut`
- All API calls use POST method (Go backend convention)
- URL pattern: `/{resource}/{identifier}/{action}` — e.g., `/admin/user/{unionID}/get`
- Unified response handling via `getResponse()` callback pattern with `success`/`failed`/`noResponse`/`noRequest`
- JWT passed via custom `X-JWT-Header` header

### i18n

- YAML files in `src/locales/`: `en.yaml`, `zhHans.yaml`
- Dot-separated key hierarchy: `model.user.field.username`, `action.signIn`
- Template: `{{ $t("key.path") }}`
- Script: `i18n.global.t("key.path")`
- Language switching via `[language]` dynamic route segment

### Pinia Stores

Use Options Store style (`state` + `actions`):

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

- `vee-validate` + `yup` with validation schemas in `@/schemas/`
- `useForm` + `defineField` pattern with `vuetifyCfg` adapter
- Use `handleSubmit` to wrap submission logic

### Code Style

- **2-space indentation** (`.editorconfig` enforced)
- UTF-8 encoding
- Trailing newline required at EOF
- Trim trailing whitespace
- ESLint v9 flat config
- Short-circuit (`&&`) and ternary expressions allowed
- Component auto-registration via `unplugin-vue-components` — do not manually import components from `src/components/`

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

Use `mitt` event bus for cross-component communication (e.g., user state refresh events).

---

## General Project Rules

### Directory Structure

```
├── api/              # Protobuf definitions
├── cmd/              # Application entry points
├── internal/
│   ├── benchmark/    # Benchmarking utilities
│   ├── capture/      # Packet capture engine
│   ├── cmd/          # CLI commands (Cobra)
│   ├── manager/      # HTTP API layer (Gin)
│   ├── migrations/   # Database migrations
│   ├── models/       # Data models (core, network, tag)
│   ├── network/      # Network protocol processing
│   ├── pkg/          # Shared packages (auth, crypto, db, errors, logging, etc.)
│   ├── provider/     # DI provider definitions
│   ├── reporter/     # gRPC reporting service
│   └── tasks/        # ETL task pipeline
├── web/              # Vue 3 frontend
└── Makefile          # Build automation
```

### Key Rules

1. **No dead code**: Remove unused imports, variables, and functions
2. **Consistency first**: Follow existing patterns in the file/module you're editing
3. **Chinese comments**: Chinese only for Go code; English only for frontend
4. **Type safety**: Use TypeScript strict mode; use Go generics where applicable
5. **Error handling**: Never ignore errors; always wrap with context
6. **DI pattern**: All new dependencies go through the `provider/` → `Inject` → `Invoke` pattern
7. **API convention**: RESTful resource naming with POST method, `{resource}/{id}/{action}` URL pattern
8. **Test files**: Co-locate `_test.go` / `_test.ts` files next to source files
9. **No emoji in code or comments** unless explicitly requested
10. **Run `make lint` or check ESLint output before committing changes**
