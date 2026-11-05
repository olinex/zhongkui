// Plugins
import AutoImport from "unplugin-auto-import/vite";
import Components from "unplugin-vue-components/vite";
import Fonts from "unplugin-fonts/vite";
import Layouts from "vite-plugin-vue-layouts";
import Vue from "@vitejs/plugin-vue";
import VueRouter from "unplugin-vue-router/vite";
import Vuetify, { transformAssetUrls } from "vite-plugin-vuetify";
import vueI18n from "@intlify/unplugin-vue-i18n/vite";

// Utilities
import path from "path";
import { defineConfig, loadEnv } from "vite";
import { fileURLToPath, URL } from "node:url";
import { nodePolyfills } from "vite-plugin-node-polyfills";

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  return {
    plugins: [
      vueI18n({
        runtimeOnly: false,
        compositionOnly: true,
        defaultSFCLang: "yaml",
        include: path.resolve(__dirname, "./src/locales/**"),
      }),
      nodePolyfills({
        globals: { Buffer: true },
      }),
      VueRouter({
        dts: "src/typed-router.d.ts",
      }),
      Layouts(),
      AutoImport({
        imports: [
          "vue",
          {
            "vue-router/auto": ["useRoute", "useRouter"],
          },
        ],
        dts: "src/auto-imports.d.ts",
        eslintrc: {
          enabled: true,
        },
        vueTemplate: true,
      }),
      Components({
        dirs: ["src/components/", "src/layouts/"],
        dts: "src/components.d.ts",
      }),
      Vue({
        template: { transformAssetUrls },
      }),
      // https://github.com/vuetifyjs/vuetify-loader/tree/master/packages/vite-plugin#readme
      Vuetify({
        autoImport: true,
        styles: {
          configFile: "src/styles/settings.scss",
        },
      }),
      Fonts({
        google: {
          families: [
            {
              name: "Roboto",
              styles: "wght@100;300;400;500;700;900",
            },
          ],
        },
      }),
    ],
    define: {
      "process.env": {
        ZHONGKUI_BASE_URL: loadEnv(mode, process.cwd()).VITE_ZHONGKUI_BASE_URL,
      },
    },
    resolve: {
      alias: {
        "@": fileURLToPath(new URL("./src", import.meta.url)),
      },
      extensions: [".js", ".json", ".jsx", ".mjs", ".ts", ".tsx", ".vue"],
    },
    server: {
      port: 8000,
    },
    css: {
      preprocessorOptions: {
        sass: {
          api: "modern-compiler",
        },
        scss: {
          api: "modern-compiler",
        },
      },
    },
    // vite-plugin-vuetify's autoImport generates `vuetify/components/*` imports
    // on-the-fly during .vue transform, so Vite's dep optimizer cannot discover
    // them at startup. Without excluding vuetify, the first page load triggers
    // "new dependencies optimized -> reloading", aborting in-flight requests
    // (ERR_ABORTED) and breaking the dynamic import of lazy routes.
    // This mirrors the official `create vuetify` Vite template.
    optimizeDeps: {
      exclude: ["vuetify"],
    },
  };
});
