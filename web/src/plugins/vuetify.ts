/**
 * plugins/vuetify.ts
 *
 * 框架文档：https://vuetifyjs.com`
 */

// 样式
import "@mdi/font/css/materialdesignicons.css";
import "vuetify/styles";

import { configure } from "vee-validate";
import { useI18n } from "vue-i18n";
// 组合式 API
import { createVuetify, type LocaleOptions } from "vuetify";
import { createVueI18nAdapter } from "vuetify/locale/adapters/vue-i18n";

import { localManager } from "@/stores/local";
import i18n from "@/utils/i18n";

configure({
  validateOnInput: true,
  validateOnBlur: true,
});

// 参见 https://vuetifyjs.com/en/introduction/why-vuetify/#feature-guides
export default createVuetify({
  theme: {
    defaultTheme: localManager.getTheme(),
    themes: {
      light: {
        colors: {
          background: "#E6EAF0",
          surface: "#FFFFFF",
          "surface-variant": "#E2E7EF",
          primary: "#2563EB",
          secondary: "#64748B",
          success: "#16A34A",
          warning: "#D97706",
          error: "#DC2626",
          info: "#0891B2",
          border: "#D5DCE7",
          outline: "#B4C0D2",
          "on-background": "#1F2937",
          "on-surface": "#1F2937",
          "on-surface-variant": "#4B5563",
          "on-primary": "#FFFFFF",
          "on-secondary": "#FFFFFF",
          "on-success": "#FFFFFF",
          "on-warning": "#FFFFFF",
          "on-error": "#FFFFFF",
          "on-info": "#FFFFFF",
        },
      },
      dark: {
        colors: {
          background: "#0E1116",
          surface: "#151A21",
          "surface-variant": "#1D242E",
          primary: "#3B82F6",
          secondary: "#94A3B8",
          success: "#22C55E",
          warning: "#FBBF24",
          error: "#EF4444",
          info: "#22D3EE",
          border: "#262E3A",
          outline: "#37404F",
          "on-background": "#E5E7EB",
          "on-surface": "#E6E8EC",
          "on-surface-variant": "#9CA3AF",
          "on-primary": "#FFFFFF",
          "on-secondary": "#0F172A",
          "on-success": "#052E16",
          "on-warning": "#422006",
          "on-error": "#FFFFFF",
          "on-info": "#083344",
        },
      },
    },
  },
  // 组件视觉默认值集中声明处。
  // 凡在此覆盖的属性（variant / density / rounded / elevation / maxWidth），
  // 用法处禁止重复声明（见 AGENTS.md「Vue Frontend Conventions / 组件与变体约定」）。
  // 宽度等数值统一引用 styles/tokens.scss 导出的 --zk-* 变量，避免二次硬编码。
  defaults: {
    // 输入族：全站控件统一 outlined + compact
    // 注意：Vuetify 各输入组件自带的默认 variant 是 filled，未在此登记的一律会退化为
    // filled 而与同族控件分裂（VNumberInput / VFileInput 曾因此与 VTextField 不一致）。
    VField: { variant: "outlined", density: "compact" },
    VTextField: { variant: "outlined", density: "compact" },
    VSelect: { variant: "outlined", density: "compact" },
    VAutocomplete: { variant: "outlined", density: "compact" },
    VCombobox: { variant: "outlined", density: "compact" },
    VTextarea: { variant: "outlined", density: "compact" },
    VNumberInput: { variant: "outlined", density: "compact" },
    VFileInput: { variant: "outlined", density: "compact" },
    // 数据表：全站紧凑密度
    VDataTable: { density: "compact" },
    VDataTableServer: { density: "compact" },
    // 对话框：大圆角 + md 档默认宽度（AppDialog 按 size 覆写）
    VDialog: { rounded: "lg", maxWidth: "var(--zk-dialog-width-md)" },
    // 卡片与应用栏：扁平化，靠 hairline 描边而非阴影分层
    VCard: { elevation: 0 },
    VToolbar: { density: "compact", elevation: 0 },
    VAppBar: { elevation: 0 },
    // 按钮：紧凑密度（图标/次级按钮在用法处使用 variant="text"）
    VBtn: { density: "compact" },
    // 标签：统一小尺寸 tonal 基线
    VChip: { size: "small", variant: "tonal" },
    // 导航列表：紧凑密度
    VList: { density: "compact" },
    VListItem: { density: "compact" },
    // 展开面板：全站手风琴 + 紧凑
    VExpansionPanels: { variant: "accordion", density: "compact" },
    // 标签页：紧凑密度
    VTabs: { density: "compact" },
    // 提示：紧凑 tonal，避免高饱和色块抢占注意力
    VAlert: { variant: "tonal", density: "compact" },
    VSnackbar: { location: "top" },
    // 菜单：与对话框统一的圆角
    VMenu: { rounded: "md" },
  },
  locale: {
    current: i18n.global.locale.value,
    adapter: createVueI18nAdapter({ i18n, useI18n }),
    t: i18n.global.t,
  } as LocaleOptions,
});
