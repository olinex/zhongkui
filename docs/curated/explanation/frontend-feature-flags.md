# 前端功能开关与不可用态

> 状态：已校对
> 责任人：@olinex
> 依据：`internal/manager/features/features.go`、`internal/manager/middlewares/feature_registry.go`

zhongkui 的某些能力依赖运行期条件才能启用（如 AI 助手需要先注册 LLM 模型并绑定档位、密码重置依赖系统 SMTP）。这些能力由后端的**功能注册表**统一裁决可用性，前端据此决定入口呈现。本文解释这套开关从后端流到 UI 的完整链路，以及为什么「不可用」也要展示出来。

## 核心原则

**功能未开通时，相关入口不隐藏，而是「展示但不可用」，并把用户引导到开通入口。** 隐藏入口会让用户以为产品没有这项能力；展示置灰则明确传达「能力存在、当前未开通、点这里可以开通」。

## 数据来源：后端功能注册表

- 后端在 `internal/manager/features` 注册功能键，经 `GET /api/v1/features/list` 下发，每项含 `key` / `enabled` / `reason`（不可用原因，按请求语言本地化）。
- 前端接口封装在 `web/src/apis/features.ts`（`featuresListGet`），类型 `FeatureOut` 定义在 `web/src/apis/@types/features.ts`。
- `web/src/stores/features.ts` 是前端唯一访问点：
  - `FEATURE_AI`（`"ai"`）、`FEATURE_PASSWORD_RESET`（`"passwordReset"`）等**功能键常量**，与后端 `features.Key` 保持一致；前端只用常量引用，**不自行推断可用性、不自造原因文案**。
  - `enabled` / `reasons` 按语言缓存（`loadedLang`）：语言切换后自动重新拉取。
  - `load(force)`：正常情况下按语言去重；`force` 为 `true` 时强制刷新（用于用户刚完成开通后）。
  - `isEnable(key)`：未加载 / 未注册一律视为**关闭**。拉取失败时按「全部关闭」处理且不标记已加载，下次导航自动重试——后端始终是可用性的最终裁决方。
  - `reasonOf(key)`：取后端下发的不可用原因，无则返回空串由调用方回退通用文案。

## 侧边栏：按不可用态渲染

侧边栏配置是 `web/src/navigation.ts` 的 `navigationGroups`（侧边栏与首页快速开始选择器的单一事实来源）。分组（`NavGroup`）与子项（`NavChild`）都可声明 `feature` 键。

- **功能键解析规则**：`MainLayout.vue` 的 `childFeature(parent, child)` 遵循「**子项自身 `feature` 优先，否则继承所属分组**」。AI 分组在分组上声明 `feature: FEATURE_AI`，其下子项无需逐个声明。
- **子项呈现**：功能关闭的子项**不从侧边栏过滤掉**，而是以不可用态展示——标题用次级文本色（`text-on-surface-variant`）置灰，并追加锁定标识。点击进入页面后由路由守卫统一引导到「功能不可用」说明页（见下）。
- **tooltip 回退**：锁定标识的 tooltip 取 i18n 键 `navbar.featureUnavailable`。具体不可用原因**不塞进 tooltip**（受抽屉左侧宽度预算限制，长文案会折行）；原因只由说明页承载。

## 导航标识徽章

「锁定」与「命名空间」两类标识是同一组件族的两个变体，由共享组件 `web/src/components/base/NavBadge.vue` 渲染，禁止就地各写一套：

| 变体 | 图标 + 语义色 | tooltip 键 | 语义 |
|------|---------------|------------|------|
| `lock` | `mdi-lock-outline` + `color="warning"` | `navbar.featureUnavailable` | 功能不可用（是**状态**，用语义色） |
| `namespace` | `mdi-alpha-n-box-outline` + `color="primary"` | `navbar.namespaceScoped` | 页面受命名空间影响（是**归属**，不用语义色） |

标识种类由 `navigation.ts` 的 `navBadgeOf(unavailable, namespaceScoped)` 统一解析，遵循「**锁定优先于命名空间**」：页面既然已经不可用，再说明它是否随命名空间变化已无意义，两者同时命中时只返回 `lock`。

父级与子级共用同一规则，但**父级徽章仅在全部子项命中同一语义时才跟随展示**（`MainLayout.vue` 的 `groupBadge`，基于当前已按权限过滤、用户实际可见的子项判定）。部分子项不可用、或部分与命名空间无关时，父级标识会以偏概全，故不展示。

## 路由守卫与「功能不可用」页

页面级不可用态**统一由路由守卫处理**，各功能页不各自实现：

1. `router/index.ts` 的 `beforeEach` 对 `requiresAuth` 路由，经 `navigation.ts` 的 `featureOfRoute`（子项 `feature` 优先，否则继承分组；未登记在导航中的路由返回 `undefined`）推导其功能键。
2. 命中关闭功能时，先 `useFeaturesStore().load(true)` **强制刷新一次**再判断——用户可能刚在开通页完成开通，缓存中的旧可用性会把自己再次拦回说明页。
3. 仍关闭则以 `router.push` 重定向到 `Feature Unavailable`，并把功能键经 query（`feature`）传入。**必须用 `push` 而非 `replace`**：被拦截的导航尚未写入历史，`replace` 会顶掉用户当前页面，使说明页成为「无上一级」的死胡同。

说明页 `web/src/pages/[language]/feature-unavailable.vue` 走 `web/src/layouts/CentralLayout.vue` + `v-empty-state`（`mdi-lock-outline` + `color="warning"`），其文案与引导全部取自单一来源：

- 功能展示名取导航分组标题（`featureGroupOf`），标题用 `page.featureUnavailable.title`；
- 原因取 `featuresStore.reasonOf(featureKey)`，未提供时回退 `page.featureUnavailable.hint`；
- 开通入口取分组 `activationRoute` 声明的路由名，其标题与所需权限经 `navChildByRoute` 复用导航中既有子项（避免二次声明）；用户具备该权限点时展示主按钮（`page.featureUnavailable.activate`），不具备时改为提示联系管理员（`page.featureUnavailable.contactAdmin`），不引导其前往无权页面；
- 死胡同页必须提供退出入口「返回上一级」（`action.backPrevious`）：优先 `router.back()`，直接用 URL 打开本页无历史时退化为首页。

## 例外与边界

- **唯一隐藏例外**：没有对应页面、无法跳转的非导航入口仍按开关隐藏——如登录页「忘记密码」链接（受 `FEATURE_PASSWORD_RESET` 控制）。展示置灰却无处可去没有意义。
- **首页快速开始选择器**（`web/src/pages/[language]/index.vue`）仍按功能开关**过滤**链接卡片（卡片本身不带可用性标识）；点击后同样由路由守卫引导到说明页。
- 新增一项功能开关时，需同时在后端 `internal/manager/features` 注册、在前端 `navigation.ts` 对应分组 / 子项声明 `feature`，有页面级开通入口时声明 `activationRoute`。

设计意图可以概括为一句话：**功能不支持也应展示而非隐藏**——让用户看得见能力、知道原因、找得到开通入口，而不是面对一个「凭空消失」的功能。
