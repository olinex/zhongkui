/**
 * plugins/index.ts
 *
 * 由 `./src/main.ts` 自动引入
 */

import i18n from "@/utils/i18n";

// 指令
import { requestDirective } from "@/directives/request";

import router from "../router";
// 插件
import pinia from "../stores";
import vuetify from "./vuetify";

// 类型
import type { App } from "vue";

// registerPlugins 注册应用级插件（i18n、pinia、vuetify、router）与全局指令。
export function registerPlugins(app: App) {
  app.use(i18n).use(pinia).use(vuetify).use(router);
  // v-request：为发起后端请求的主流程按钮统一提供"防重复点击 + 加载中动画"
  app.directive("request", requestDirective);
}
