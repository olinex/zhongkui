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
import { buildInfoEnv, defaultBuildInfo } from "./build.config";

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  // loadEnv 同时读取 .env* 与 process.env，环境变量优先于 build.config.ts 的默认值
  const env = loadEnv(mode, process.cwd(), "VITE_");
  const buildInfo = {
    tag: env[buildInfoEnv.tag]?.trim() || defaultBuildInfo.tag,
    commit: env[buildInfoEnv.commit]?.trim() || defaultBuildInfo.commit,
  };

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
        ZHONGKUI_BASE_URL: env.VITE_ZHONGKUI_BASE_URL,
      },
      // 构建标识（tag / commit）编译期固化进产物，展示见 components/base/BuildStamp.vue
      __BUILD_INFO__: JSON.stringify(buildInfo),
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
