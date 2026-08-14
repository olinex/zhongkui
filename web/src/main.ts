/**
 * main.ts
 *
 * Bootstraps Vuetify and other plugins then mounts the App`
 */

// buffer module global inject
// see: vite.config.ts
import { Buffer } from "buffer";
// Composables
import { createApp } from "vue";

// Plugins
import { registerPlugins } from "@/plugins";

// Components
import App from "./App.vue";

// Styles
import "@/styles/main.scss";

window.Buffer = Buffer;

const app = createApp(App);

registerPlugins(app);

app.mount("#app");
