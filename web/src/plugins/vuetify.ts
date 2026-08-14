/**
 * plugins/vuetify.ts
 *
 * Framework documentation: https://vuetifyjs.com`
 */

// Styles
import "@mdi/font/css/materialdesignicons.css";
import "vuetify/styles";

import { configure } from "vee-validate";
import { useI18n } from "vue-i18n";
// Composables
import { createVuetify, type LocaleOptions } from "vuetify";
import { createVueI18nAdapter } from "vuetify/locale/adapters/vue-i18n";

import { localManager } from "@/stores/local";
import i18n from "@/utils/i18n";

configure({
  validateOnInput: true,
  validateOnBlur: true,
});

// https://vuetifyjs.com/en/introduction/why-vuetify/#feature-guides
export default createVuetify({
  theme: {
    defaultTheme: localManager.getTheme(),
    themes: {
      light: {
        colors: {
          background: "#F4F6F9",
          surface: "#FFFFFF",
          "surface-variant": "#EDF0F5",
          primary: "#2563EB",
          secondary: "#64748B",
          success: "#16A34A",
          warning: "#D97706",
          error: "#DC2626",
          info: "#0891B2",
          border: "#E3E8EF",
          outline: "#C6D0DC",
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
  defaults: {
    VTextField: { variant: "outlined", density: "compact" },
    VSelect: { variant: "outlined", density: "compact" },
    VAutocomplete: { variant: "outlined", density: "compact" },
    VCombobox: { variant: "outlined", density: "compact" },
    VTextarea: { variant: "outlined", density: "compact" },
    VDataTable: { density: "compact" },
    VDataTableServer: { density: "compact" },
    VDialog: { rounded: "lg" },
    VCard: { elevation: 0 },
  },
  locale: {
    current: i18n.global.locale.value,
    adapter: createVueI18nAdapter({ i18n, useI18n }),
    t: i18n.global.t,
  } as LocaleOptions,
});
