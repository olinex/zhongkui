# 前端设计体系分层

> 分类：解释 · 角色：研发 · 适用场景：进行前端样式与组件开发时阅读。

`web/` 是 Vue 3 + Vuetify 3 + TypeScript 管理端。它的视觉表现不是各页面各写一套样式拼出来的，而是被组织成自上而下、**只能单向依赖**的四层结构。理解每层各自负责什么，以及「要改某处视觉时该动哪一层」，是改动任何前端界面的前提。

本文只解释这套结构为什么这样切分、依赖方向如何约束；具体规则、色板数值与禁令清单以 `AGENTS.md` 的「Vue Frontend Conventions」为唯一信任源，本文不重复。

## 四层结构

| 层 | 位置 | 职责 | 允许内容 |
|----|------|------|----------|
| 1 令牌层 | `web/src/styles/tokens.scss` | **唯一数值来源** | `$zk-*`（SASS 变量）与 `--zk-*`（`:root` 上的 CSS 自定义属性）双导出：字体族、字号档位、字重、控件/工具栏/表格高度、4px 间距栅格、圆角四档、hairline 描边、对话框与容器宽度档、内容尺寸约束、z-index、过渡时长、阴影 |
| 2 主题层 | `web/src/plugins/vuetify.ts` | **唯一颜色来源** + 组件视觉默认值 | `theme.colors`（由 Vuetify 发布为 `--v-theme-*`）与 `defaults`（各组件的 `variant` / `density` / `rounded` / `elevation` / `maxWidth`） |
| 3 外壳层 | `web/src/styles/main.scss` | 跨组件复用的外观 | `.zk-*` 复用类（面板、排版、编辑器外壳、请求态、行交互态等），以及 Vuetify / CodeMirror 内部类覆盖 |
| 4 组件层 | 各 `*.vue` 的 `<style scoped>` | **仅布局定位** | flex / grid、尺寸约束、溢出、定位 |

`web/src/styles/settings.scss` 是令牌层到 Vuetify 的桥梁：它被 `vite-plugin-vuetify` 注入每次 SCSS 编译，覆盖 Vuetify 的 SASS 变量（如 `$border-radius-root`、`$body-font-family`），因此只能包含 `@use` 与变量覆盖，不含普通 CSS 规则。

## 依赖方向

```
        ┌───────────────────────────────────────────┐
 4 组件层 │  *.vue  <style scoped>   仅布局定位         │
        └───────────────┬───────────────────────────┘
                        │ 引用
        ┌───────────────▼───────────────────────────┐
 3 外壳层 │  styles/main.scss   .zk-* 复用外观 + 覆盖   │
        └───────────────┬───────────────────────────┘
                        │ 引用
        ┌───────────────▼───────────────────────────┐
 2 主题层 │  plugins/vuetify.ts   颜色(theme.colors)    │
        │                       + 组件 defaults       │
        └───────────────┬───────────────────────────┘
                        │ 引用
        ┌───────────────▼───────────────────────────┐
 1 令牌层 │  styles/tokens.scss   $zk-* / --zk-* 数值   │
        └───────────────────────────────────────────┘
```

上层只能引用下层：组件层可引用外壳类、主题工具类与令牌；外壳层引用令牌与主题变量；主题层的默认值也引用令牌（如 `VDialog` 的 `maxWidth: "var(--zk-dialog-width-md)"`）。反向依赖（在令牌层写具体组件外观、在外壳层写页面内联布局）都是越层。

## 为什么要这样分层

- **单一数值来源**：字号、间距、圆角、边框、高度都只在 `tokens.scss` 声明一次。改一处，全站生效；不会出现「这个页面 36px 行高、那个页面 34px」的漂移。
- **颜色与主题同步**：`tokens.scss` 刻意不定义任何颜色，颜色的唯一来源是 Vuetify 主题。亮 / 暗两套主题因此不可能不同步，消费端统一用 `rgb(var(--v-theme-*))`。
- **避免就地样式**：同类组件一旦各自在 `<style scoped>` 里写视觉，就会分裂。把复用外观收敛到 `main.scss` 的 `.zk-*` 类，观感才一致；Stylelint 的字面量拦截作为兜底。
- **变体统一**：一个功能族共享一套骨架与基线，需要变体时成对定义（如 `AppDialog.vue` 的 `size` 四档），而不是每个用法自造一套。

## 决定该动哪一层

| 想改的东西 | 落点 |
|------------|------|
| 数值（字号 / 间距 / 圆角 / 边框 / 高度 / 宽度档） | 令牌层 `web/src/styles/tokens.scss`，先登记令牌再消费 |
| 颜色（含语义色与文字色） | 主题层 `web/src/plugins/vuetify.ts` 的 `theme.colors` |
| 组件的默认 `variant` / `density` / `rounded` / `elevation` | 主题层 `plugins/vuetify.ts` 的 `defaults` |
| 多个组件复用的外观块（面板 / 区块标题 / 编辑器外壳 / 提示文本） | 外壳层 `web/src/styles/main.scss` 的 `.zk-*` |
| 单个组件内部的排布（flex / grid / 溢出 / 定位） | 组件层 `*.vue` 的 `<style scoped>` |
| 代码编辑器语法高亮配色 | `web/src/utils/editorHighlight.ts`（唯一语法色来源） |
| 图表主题（系列色 / 文本 / 网格 / tooltip） | `web/src/utils/chartTheme.ts`（`useChartTheme` / `withChartTheme`） |
| 布局骨架 | `web/src/layouts/MainLayout.vue`、`web/src/layouts/CentralLayout.vue`、`web/src/components/base/AppHeader.vue` |
| 对话框 / 展开面板标题骨架 | `web/src/components/base/AppDialog.vue`、`web/src/components/base/ExpansionPanelTitle.vue` |

判断的一般方法是：**先问「这是一个可以在别处复用的取值或外观，还是只属于这个组件的位置关系」**——前者下沉到令牌 / 主题 / 外壳层，后者留在组件层。越靠下的层，改动的影响半径越大，越要谨慎。
