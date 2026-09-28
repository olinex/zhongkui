/**
 * main.ts
 *
 * 应用入口：引导初始化 Vuetify 等插件后挂载 App。
 */

// 全局注入 buffer 模块
// 详见 vite.config.ts
import { Buffer } from "buffer";
// 组合式函数
import { createApp } from "vue";

// 插件
import { registerPlugins } from "@/plugins";

// 组件
import App from "./App.vue";

// 样式
import "@/styles/main.scss";

window.Buffer = Buffer;

const app = createApp(App);

registerPlugins(app);

app.mount("#app");
