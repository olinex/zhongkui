# Design Specification

This document is the canonical design specification for the zhongkui web client
(a network traffic capture and analysis platform). Every token, color and
convention listed here is locked to the actual implementation. If this document
and the code ever disagree, the code wins — and this document must be updated.

Source of truth files:

- Theme colors and component defaults: `src/plugins/vuetify.ts`
- Design tokens (font / radius / spacing / shadow): `src/styles/tokens.scss`
- Global styles: `src/styles/main.scss`
- Vuetify SASS overrides: `src/styles/settings.scss`
- Chart theme composable: `src/utils/chartTheme.ts`
- Layout conventions: `src/layouts/MainLayout.vue`, `src/components/base/AppHeader.vue`

## 1. Design Principles

The product is a network security audit platform used by operators who inspect
traffic captures, flows and tag rules under pressure. The UI follows three
principles:

1. **Simplicity** — Do one thing per screen. Dense but predictable layouts;
   flat surfaces with thin borders instead of heavy shadows; no decorative
   gradients or ornaments.
2. **Ease of use** — Compact input controls, compact tables, and navigation
   that stays out of the way (rail drawer that expands on hover). Frequent
   actions are one click away and every state change is communicated in words,
   not just colors.
3. **Clear information delivery** — Technical values (IDs, hashes, hex, byte
   counts) are rendered in a monospace font with tabular numerals so columns
   and hashes align. Semantic colors always express state; never style.

## 2. Color Palette

**The single source of truth for colors is the Vuetify theme defined in
`src/plugins/vuetify.ts`.** Components and styles must consume colors through
the theme CSS variables (`--v-theme-*`), e.g. `rgb(var(--v-theme-primary))`.
SCSS design tokens (`src/styles/tokens.scss`) intentionally do **not** define
any color, so light/dark themes always stay in sync.

### Light theme

| Token | Hex | Usage |
|-------|-----|-------|
| `background` | `#F4F6F9` | Page background |
| `surface` | `#FFFFFF` | Cards, sheets, app bar, tooltip background |
| `surface-variant` | `#EDF0F5` | Tooltip border, subtle surfaces |
| `primary` | `#2563EB` | Primary actions, selection, active nav |
| `secondary` | `#64748B` | Secondary actions, neutral accents |
| `success` | `#16A34A` | Success state |
| `warning` | `#D97706` | Warning state |
| `error` | `#DC2626` | Error / destructive state |
| `info` | `#0891B2` | Informational state |
| `border` | `#E3E8EF` | Custom border color (see note below) |
| `outline` | `#C6D0DC` | Custom outline color (see note below) |
| `on-background` | `#1F2937` | Primary text on `background` |
| `on-surface` | `#1F2937` | Primary text on `surface` |
| `on-surface-variant` | `#4B5563` | Secondary text (captions, table headers) |
| `on-primary` | `#FFFFFF` | Text / icons on `primary` |
| `on-secondary` | `#FFFFFF` | Text / icons on `secondary` |
| `on-success` | `#FFFFFF` | Text / icons on `success` |
| `on-warning` | `#FFFFFF` | Text / icons on `warning` |
| `on-error` | `#FFFFFF` | Text / icons on `error` |
| `on-info` | `#FFFFFF` | Text / icons on `info` |

### Dark theme

| Token | Hex | Usage |
|-------|-----|-------|
| `background` | `#0E1116` | Page background |
| `surface` | `#151A21` | Cards, sheets, app bar, tooltip background |
| `surface-variant` | `#1D242E` | Tooltip border, subtle surfaces |
| `primary` | `#3B82F6` | Primary actions, selection, active nav |
| `secondary` | `#94A3B8` | Secondary actions, neutral accents |
| `success` | `#22C55E` | Success state |
| `warning` | `#FBBF24` | Warning state |
| `error` | `#EF4444` | Error / destructive state |
| `info` | `#22D3EE` | Informational state |
| `border` | `#262E3A` | Custom border color (see note below) |
| `outline` | `#37404F` | Custom outline color (see note below) |
| `on-background` | `#E5E7EB` | Primary text on `background` |
| `on-surface` | `#E6E8EC` | Primary text on `surface` |
| `on-surface-variant` | `#9CA3AF` | Secondary text (captions, table headers) |
| `on-primary` | `#FFFFFF` | Text / icons on `primary` |
| `on-secondary` | `#0F172A` | Text / icons on `secondary` |
| `on-success` | `#052E16` | Text / icons on `success` |
| `on-warning` | `#422006` | Text / icons on `warning` |
| `on-error` | `#FFFFFF` | Text / icons on `error` |
| `on-info` | `#083344` | Text / icons on `info` |

### Borders and outlines

The palette also registers two custom colors, `border` and `outline`. They are
kept for Vuetify `color="border"` / `color="outline"` usage (e.g. border fills
in inputs, chips and badges).

**Do not hard-code hairline borders with hex values.** For separators and card
borders the codebase uses a translucent `on-surface` tint that matches
Vuetify's own border-opacity semantics:

```scss
border: 1px solid rgb(var(--v-theme-on-surface), 0.12);
```

(Vuetify has no `--v-theme-border` CSS variable, hence the translucent
`on-surface` approach — see `src/styles/main.scss`.)

## 3. Typography

### Font stacks

Defined in `src/styles/tokens.scss` as `$zk-font-family` / `$zk-font-mono` and
exported as `--zk-font-family` / `--zk-font-mono`.

```scss
$zk-font-family: "Roboto", "PingFang SC", "Hiragino Sans GB",
  "Microsoft YaHei", "Noto Sans SC", system-ui, -apple-system, "Segoe UI",
  sans-serif;

$zk-font-mono: ui-monospace, "SF Mono", sfmono-regular, menlo, consolas,
  "Liberation Mono", monospace;
```

The same family is wired into Vuetify via `$body-font-family` and
`$heading-font-family` in `src/styles/settings.scss`, so all Vuetify components
inherit it automatically.

### Sizes, weights and line heights

There is no separate type scale token set; the app relies on Vuetify's defaults
plus a few global rules:

| Usage | Value | Where |
|-------|-------|-------|
| Base body | Vuetify body default (0.875rem / 14px) | `settings.scss` (`$border-radius-root` applies to component radii only) |
| Data table header | `0.75rem`, weight `600`, uppercase, `letter-spacing: 0.05em`, color `rgb(var(--v-theme-on-surface-variant))` | `main.scss` `.v-data-table th` |
| Technical values | `.font-mono` utility (mono stack + `tabular-nums`) | `main.scss` |

Line heights follow Vuetify component defaults; do not invent a separate scale.

### `.font-mono` utility

Applied to technical values so digits stay aligned and readable:

- IDs (union IDs, rule IDs, task IDs)
- Hashes
- Hexadecimal values
- Byte / packet counts

```vue
<span class="font-mono">{{ packet.hex }}</span>
```

The body itself also sets `font-variant-numeric: tabular-nums`, so numerals
align everywhere by default.

## 4. Spacing

4px base scale, defined in `src/styles/tokens.scss` (`$zk-space-*` +
`--zk-space-*`):

| Token | Value |
|-------|-------|
| `$zk-space-1` | 4px |
| `$zk-space-2` | 8px |
| `$zk-space-3` | 12px |
| `$zk-space-4` | 16px |
| `$zk-space-5` | 24px |
| `$zk-space-6` | 32px |

Vuetify density utilities (e.g. `pa-4 pa-md-6`) map to the same 4px rhythm
(`pa-4` = 16px, `pa-md-6` = 24px). Use the scale for any custom margin or
padding; never free-form pixel values.

## 5. Border Radius

Defined in `src/styles/tokens.scss`:

| Token | Value | Typical use |
|-------|-------|-------------|
| `$zk-radius-sm` | 4px | Small elements, scrollbar thumb, chips |
| `$zk-radius-md` | 6px | Input controls, buttons (also Vuetify `$border-radius-root`) |
| `$zk-radius-lg` | 12px | Cards, dialogs |

- Vuetify's root radius is overridden to **6px** in `settings.scss`
  (`$border-radius-root: 6px`), so every input and button inherits it.
- Cards render at **12px** via the global `.v-card` rule in `main.scss`.
- Dialogs use `rounded="lg"` (12px) via the Vuetify default in
  `src/plugins/vuetify.ts` (`VDialog: { rounded: "lg" }`).

## 6. Elevation & Shadows

**Flatness is the default.** Cards and the app bar use `elevation="0"` and a
1px hairline border instead of heavy shadows:

```scss
.v-card {
  border: 1px solid rgb(var(--v-theme-on-surface), 0.12);
  border-radius: 12px;
}
```

`VCard: { elevation: 0 }` is set as a Vuetify default in `src/plugins/vuetify.ts`,
and `AppHeader` uses `<v-app-bar elevation="0">`.

Two shadow tokens exist for the rare elevated layer (menus, popovers, drag
overlays), tuned for the dark theme:

| Token | Value |
|-------|-------|
| `$zk-shadow-sm` | `0 1px 2px rgb(0 0 0 / 20%), 0 1px 3px rgb(0 0 0 / 30%)` |
| `$zk-shadow-md` | `0 2px 4px rgb(0 0 0 / 24%), 0 4px 12px rgb(0 0 0 / 32%)` |

Prefer `$zk-shadow-sm` over `$zk-shadow-md`; never stack multiple shadows on
one element.

## 7. Component Conventions

Defaults are centralized in `src/plugins/vuetify.ts` — components must not
re-declare these per usage:

| Component | Default | Convention |
|-----------|---------|------------|
| `VTextField`, `VSelect`, `VAutocomplete`, `VCombobox`, `VTextarea` | `variant="outlined"`, `density="compact"` | All input controls are outlined and compact |
| `VDataTable`, `VDataTableServer` | `density="compact"` | Tables are always compact |
| `VDialog` | `rounded="lg"` | Dialogs use large (12px) rounded corners |
| `VCard` | `elevation: 0` | Cards are flat, bordered surfaces |

Additional rules:

- **Cards** carry the global 1px translucent border from `main.scss`; never add
  a second border or elevation.
- **Buttons** — default to `variant="text"` for icon / tertiary actions and
  `variant="outlined"` or filled (default) for primary actions; `tonal` for
  selected/active list items in navigation.
- **Semantic color usage** — `success`, `warning`, `error`, `info` express
  *state* only (notifications, status badges, validation, health). `primary`
  expresses *action and selection* (buttons, active nav items, links, focus,
  row hover). Never use `error`/`warning`/`success`/`info` to decorate
  non-stateful UI, and never use a semantic color for a primary action.
- **Text colors** — default text is inherited (`on-background` / `on-surface`);
  use `on-surface-variant` for secondary/caption text. Never pick arbitrary
  grays.

## 8. Layout

- **Single navigation drawer** (`MainLayout.vue`): a permanent drawer at
  `width="220"` with `rail` mode enabled and `expand-on-hover`; it collapses to
  a rail and expands on hover. A pin toggle at the bottom of the drawer
  (`#append` slot, icon-only `v-btn` with `variant="text"`, centered) lets
  users keep it permanently expanded
  (`expand-on-hover="!pinned"`), with the preference persisted to
  `localStorage` (`localManager.getNavPinned` / `setNavPinned`). Parent items
  act as group headings (`color="primary"`) and follow **accordion behavior**
  (via `v-list-group` with `open-strategy="single"` + `v-model:opened`):
  only the group of the current route is expanded by default, clicking another
  group collapses the previous one, and clicking the expanded group collapses
  it; a Vuetify chevron indicates the expand state, and the expand/collapse
  height transition is accelerated to `0.15s` (see the `:deep(.v-list-group__items)`
  override in `MainLayout.vue`). In rail mode all groups collapse to avoid
  overflow in the 56px rail, and hovering the rail re-expands the group of the
  current route. Child
  items are indented 16px (`margin-left: 16px`, `variant="tonal"`).
  The drawer is separated from content by a hairline right border
  (`border-right: 1px solid rgb(var(--v-theme-on-surface), 0.12)`).
- **Content area**: `<v-main class="d-flex mb-6">` wraps a `<v-container>`
  capped at **`max-width: 1440px`** with responsive padding `pa-4 pa-md-6`;
  page content sits in a `v-sheet` with `rounded="lg"` (12px, aligned with the
  global `.v-card` radius so a rounded card inside a right-angled container does
  not leave crescent-shaped background gaps at the corners). Use this container
  in pages; do not exceed the 1440px cap.
- **App bar** (`AppHeader.vue`): `elevation="0"` with a hairline bottom border
  (`border-bottom: 1px solid rgb(var(--v-theme-on-surface), 0.12)`) separating
  it from page content. Toolbar buttons use `variant="text"` (or `outlined`
  for the prominent sign-up action).
- **Global focus**: `:focus-visible` renders a 2px `primary` outline with 2px
  offset (`main.scss`); never suppress or replace it.

## 9. Charts

All ECharts instances must be driven by the chart theme helpers in
`src/utils/chartTheme.ts` so they follow the active light/dark Vuetify theme.

### Usage

- `useChartTheme()` — reactive; returns a `ComputedRef<ChartTheme>` that stays
  in sync when the user toggles light/dark. Use inside a component with
  `computed`-driven chart options.
- `withChartTheme(option, theme)` — pure merge of theme defaults into an
  ECharts option. Page-level explicit values always win (deep merge; arrays are
  replaced). `grid` is intentionally left to page defaults.

```ts
const chartTheme = useChartTheme();
const option = computed(() =>
  withChartTheme({ /* page-specific series, xAxis, yAxis, ... */ }, chartTheme.value),
);
```

### Palette

- Series colors are built in a fixed order:
  `primary → secondary → success → warning → error → info`
  (duplicates removed), then padded with derived primary shades (mixed toward
  white 70% / 45% and black 20% / 35%) up to 8 colors.
- Assign series colors sequentially from `ChartTheme.palette`; never hard-code
  a series color.

### Text, grid and tooltip

| Element | Color source |
|---------|--------------|
| Text (titles, axis labels, tooltip text) | `textColor` = `on-surface` (fallback `on-background`) |
| Legend / dimmed labels | `subTextColor` = `on-surface-variant` |
| Grid / axis / split lines | `gridColor` = text color at 12% opacity (`rgba(..., 0.12)`) |
| Tooltip background | `tooltipBg` = `surface` |
| Tooltip border | `tooltipBorder` = `surface-variant` |

## 10. Usage Rules & Anti-patterns

### Do

- **Consume colors only through theme variables**: `rgb(var(--v-theme-xxx))`
  (e.g. `rgb(var(--v-theme-primary))`). This keeps light/dark themes in sync.
- Use translucent `on-surface` at 0.12 alpha for hairline separators, card
  borders, drawer and app-bar borders.
- Use `$zk-*` / `--zk-*` tokens for fonts, radii, spacing and shadows.
- Use `.font-mono` for all technical values (IDs, hashes, hex, bytes).
- Use `density="compact"` tables and `outlined` + `compact` inputs.
- Keep elevation at 0 for cards and app bars; prefer hairline borders.
- Let semantic colors express state and `primary` express action/selection.
- Build charts with `useChartTheme()` / `withChartTheme()`.

### Don't

- **Do not hard-code gray shades** (`grey-*` or `#4B5563`-style literals).
  Text grays come from `on-surface` / `on-surface-variant`; borders come from
  the translucent `on-surface` tint.
- **Do not hard-code any hex color** in components or SCSS. Even if it matches
  the palette today, it breaks theme switching tomorrow.
- **Do not invent undefined color names** (e.g. `third`, `tertiary`, `accent2`).
  Only the tokens listed in section 2 exist.
- **Do not re-declare theme colors in components** (a local `const primary =
  "#2563EB"` or a repeated `border: 1px solid ...` string). Reuse the global
  rules or reference the theme variable.
- **Do not re-declare component defaults** that already exist in
  `plugins/vuetify.ts` (outlined/compact inputs, compact tables, `rounded="lg"`
  dialogs, `elevation: 0` cards).
- **Do not add free-form spacing** outside the 4px scale, or new radius values
  beyond 4 / 6 / 12px.
- **Do not use shadows heavier than `$zk-shadow-md`**, and do not stack shadows.
- **Do not style state with non-semantic colors** (e.g. a custom green for
  success) or use semantic colors for pure decoration.
- **Do not render technical values in proportional fonts.**
- **Do not suppress `:focus-visible`** or remove the global primary outline.
