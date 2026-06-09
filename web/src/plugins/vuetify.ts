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
          // 核心：更灰的背景色（推荐值）
          background: '#EFEFEF', // 中度浅灰（比 #F5F5F5 深，不刺眼）
          // 或 #E8E8E8（稍深）、#E0E0E0（更深，适合强质感需求）
          
          // 卡片/组件背景（比全局背景稍浅，形成层次）
          surface: '#F5F5F5', // 比 background 浅一级，避免单调
          
          // 文本色需同步加深，确保对比度
          'on-background': '#222222', // 深灰文本（比 #333 更深，适配深色背景）
          'on-surface': '#222222', // 卡片上的文本色
        },
      },
    },
  },
  locale: {
    current: i18n.global.locale.value,
    adapter: createVueI18nAdapter({ i18n, useI18n }),
    t: i18n.global.t,
  } as LocaleOptions,
});
