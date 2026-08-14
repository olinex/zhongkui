/**
 * plugins/index.ts
 *
 * Automatically included in `./src/main.ts`
 */

import i18n from "@/utils/i18n";

import router from "../router";
// Plugins
import pinia from "../stores";
import vuetify from "./vuetify";

// Types
import type { App } from "vue";

export function registerPlugins(app: App) {
  app.use(i18n).use(pinia).use(vuetify).use(router);
}
