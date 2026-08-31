/**
 * plugins/index.ts
 *
 * Automatically included in `./src/main.ts`
 */

import i18n from "@/utils/i18n";

// Directives
import { requestDirective } from "@/directives/request";

import router from "../router";
// Plugins
import pinia from "../stores";
import vuetify from "./vuetify";

// Types
import type { App } from "vue";

export function registerPlugins(app: App) {
  app.use(i18n).use(pinia).use(vuetify).use(router);
  // v-request：为发起后端请求的主流程按钮统一提供"防重复点击 + 加载中动画"
  app.directive("request", requestDirective);
}
