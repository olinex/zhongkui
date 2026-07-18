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
- Type-safe extraction via `config.ExtractConfig[T]()`
- Config prefix constants: `PREFIX = "capture"`

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
3. **Bilingual comments**: Chinese primary with English secondary for Go code; English only for frontend
4. **Type safety**: Use TypeScript strict mode; use Go generics where applicable
5. **Error handling**: Never ignore errors; always wrap with context
6. **DI pattern**: All new dependencies go through the `provider/` → `Inject` → `Invoke` pattern
7. **API convention**: RESTful resource naming with POST method, `{resource}/{id}/{action}` URL pattern
8. **Test files**: Co-locate `_test.go` / `_test.ts` files next to source files
9. **No emoji in code or comments** unless explicitly requested
10. **Run `make lint` or check ESLint output before committing changes**
