# 如何新增一个管理端页面

> 状态：已校对
> 责任人：@olinex
> 依据：`web/src/pages`、`internal/manager/views`

**适用角色**：研发、开源贡献者（含定制化前端开发）。普通使用者无需阅读；新增页面属于代码级定制开发。

本指南说明在 `web/`（Vue 3 + Vuetify 3 + TypeScript）中新增一个管理端页面时需要触碰的文件与次序。以现有列表页为模板（如 `web/src/pages/[language]/notification/group.vue`）最省事——它的路由、视图、API、store、校验、i18n 六处都已就位，照着补齐即可。

## 步骤

### 1. 新增页面组件

在 `web/src/pages/[language]/` 下按现有目录与命名约定新增页面（页面组件用小写连字符命名，如 `group.vue`；子目录按业务域分层，如 `admin/`、`notification/`）。`[language]` 是动态语言段，页面靠它承载 i18n 语言切换。

页面顶层用主布局包裹，`current-nav` 填所属一级分组：

```vue
<template>
  <main-layout current-nav="notification">
    <template #body>
      <!-- 页面内容 -->
    </template>
  </main-layout>
</template>
```

### 2. 声明路由元信息

在同一 `.vue` 文件顶部（`<script setup>` 之前）写 `<route lang="yaml">` 块，`unplugin-vue-router` 会据此生成路由：

```yaml
<route lang="yaml">
name: Notification Group List
meta:
  title: title.notification.group
  requiresAuth: true
  requiresPerm: true
</route>
```

- `name` 是路由名（Title Case），`navigation.ts` 与跳转都以它为准；
- `meta.title` 是 i18n 键（通常落在 `title.*`，如 `title.notification.group`）；
- `requiresAuth` 决定是否走登录校验；`requiresPerm` 决定是否做权限点校验。

### 3. 在 `navigation.ts` 登记菜单

在 `web/src/navigation.ts` 的 `navigationGroups` 中找到（或新建）所属分组，追加子项：

```ts
{
  title: "navbar.notification.group",   // i18n 键
  icon: "mdi-account-group-outline",
  routeName: "Notification Group List", // 与 <route> 的 name 一致
  namespaceScoped: true,                 // 受命名空间影响时置 true
  feature: FEATURE_AI,                   // 受功能开关控制时声明（缺省则继承分组）
  perm: { obj: "api:/api/v1/.../list", act: "POST" }, // 需要权限点时才声明
}
```

分组（`NavGroup`）可声明 `feature` 与 `activationRoute`；子项上声明 `feature` 会覆盖分组的。公告见 [`frontend-feature-flags`](../explanation/frontend-feature-flags.md)。

### 4. 新增 API 层封装

在 `web/src/apis/<resource>.ts` 按现有模式新增函数，类型放 `web/src/apis/@types/<resource>.ts`：

```ts
import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

export function notificationGroupListGet(data: SearchIn): AxiosPromise {
  return request.post("/notification/group/list", data, { headers: getJwtHeader() });
}
```

- `request` 是 `web/src/apis/base.ts` 里的 axios 实例，`baseURL` 已含 `/api/v1`；全部使用 POST，URL 模式为 `/{resource}/{identifier}/{action}`；
- 需要 JWT 的接口带上 `getJwtHeader()`（自定义 `X-Jwtoken` 头）；
- 文件上传直接以 `FormData` 作为 data：`base.ts` 的请求拦截器会在检测到 `FormData` 时清除 JSON `Content-Type`，交给浏览器补全 multipart 边界；
- 统一用 `getResponse` / `getSimpleResponse` 处理 `success` / `failed` / `noResponse` / `noRequest` 回调，避免各页面散落错误处理。

### 5. 组织 Pinia store（如需要）

页面级状态优先放组件内；跨页面共享的状态才建 store，写在 `web/src/stores/<name>.ts`，采用 Options Store 风格（`state` + `actions`），参考 `web/src/stores/app.ts`。功能开关等专用 store 已存在，直接复用即可。

### 6. 复用列表骨架与表单校验

- **搜索驱动列表页**（后端提供 `SearchIn` / `SearchOut` / `SearchOptionsOut` 契约）用 `web/src/components/table/SearchTable.vue`，传 `list-request` / `options-request` / `actions` / `batch-actions` / `create` / `project-fields` / `model-name`；列显示配置用 `useColumnVisibility`（`web/src/utils/column.ts`）+ `column-visibility-menu`。
- **非搜索驱动列表页**不强行套用 `SearchTable`，但须逐项复用其视觉骨架（扁平卡片内的筛选行 → 操作按钮行 → `v-container` 包裹的表格），单元格用 `adaptive-cell`（`web/src/components/table/AdaptiveCell.vue`）。
- **表单校验**：yup schema 放 `web/src/schemas/`，用 `useForm` + `defineField`，配 `web/src/utils/form.ts` 导出的 `vuetifyCfg` 适配器，提交逻辑用 `handleSubmit` 包裹；单行文本字段在 schema 上用 `.trim()` 规范化（密码与多行文本域除外）。

### 7. 同步补 i18n 文案

在 `web/src/locales/zhHans.yaml` 与 `web/src/locales/en.yaml` **同步**补齐所需键，中英结构必须一一对应。常见键族：菜单 `navbar.*`、页面标题 `title.*`、模型名与字段 `model.<entity>.{name,field,type}`（字段键与 API JSON 字段一致）、操作 `action.*`。模板用 <code v-pre>{{ $t("key.path") }}</code>，脚本用 `i18n.global.t("key.path")`。

### 8. 样式遵循四层架构

组件只负责布局定位（`<style scoped>` 仅 flex / grid / 溢出 / 定位）；颜色走主题工具类或 `rgb(var(--v-theme-*))`，数值走 `--zk-*` 令牌，跨组件复用的外观引用 `main.scss` 的 `.zk-*` 类。详见 [`frontend-design-system`](../explanation/frontend-design-system.md)；不要就地写字号 / 圆角 / 边框 / 阴影，也不要重复声明 `plugins/vuetify.ts` 已覆盖的 `defaults`。

### 9. 提交前校验

运行 `pnpm lint`（ESLint）、`pnpm lint:style`（Stylelint）、`pnpm type-check`（vue-tsc），三者必须全部通过。

## 检查清单

- [ ] 页面组件落在 `web/src/pages/[language]/`，命名与目录符合现有约定
- [ ] 页面用 `main-layout` 包裹，`current-nav` 指向所属一级分组
- [ ] `<route lang="yaml">` 已写 `name` / `meta.title` / `requiresAuth`（必要时 `requiresPerm`）
- [ ] `navigation.ts` 已登记子项，`routeName` 与路由 `name` 一致
- [ ] 受功能开关控制时已声明 `feature`（必要时分组 `activationRoute`）
- [ ] API 封装在 `web/src/apis/`，类型在 `web/src/apis/@types/`，无权限接口按需带 `getJwtHeader()`
- [ ] 列表页复用 `SearchTable`（或逐项复用其骨架）与 `useColumnVisibility`
- [ ] 表单校验复用 `web/src/schemas/` 的 yup schema 与 `vuetifyCfg`
- [ ] `zhHans.yaml` 与 `en.yaml` 已同步补键
- [ ] 样式遵循四层架构，`<style scoped>` 仅布局定位
- [ ] `pnpm lint` / `pnpm lint:style` / `pnpm type-check` 全部通过
