// vite.config.mts
import AutoImport from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-auto-import@0.17.8_rollup@4.52.5/node_modules/unplugin-auto-import/dist/vite.js";
import Components from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-vue-components@0.27.5_@babel+parser@7.28.5_rollup@4.52.5_vue@3.5.22_typescript@5.7.3_/node_modules/unplugin-vue-components/dist/vite.js";
import Fonts from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-fonts@1.4.0_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8_/node_modules/unplugin-fonts/dist/vite.mjs";
import Layouts from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/vite-plugin-vue-layouts@0.11.0_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sa_e6ce30eceaf44af266d409d4407d9d7d/node_modules/vite-plugin-vue-layouts/dist/index.mjs";
import Vue from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/@vitejs+plugin-vue@5.2.4_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8__vue@3.5.22_typescript@5.7.3_/node_modules/@vitejs/plugin-vue/dist/index.mjs";
import VueRouter from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-vue-router@0.10.9_rollup@4.52.5_vue-router@4.6.3_vue@3.5.22_typescript@5.7.3___vue@3.5.22_typescript@5.7.3_/node_modules/unplugin-vue-router/dist/vite.js";
import Vuetify, { transformAssetUrls } from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/vite-plugin-vuetify@2.1.2_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1._817c873b1ed709897bc838215866c654/node_modules/vite-plugin-vuetify/dist/index.mjs";
import vueI18n from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/@intlify+unplugin-vue-i18n@6.0.8_@vue+compiler-dom@3.5.22_eslint@9.38.0_rollup@4.52.5_t_1d9f506cd9225ed1989323e287db4c13/node_modules/@intlify/unplugin-vue-i18n/lib/vite.mjs";
import path from "path";
import { defineConfig, loadEnv } from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8/node_modules/vite/dist/node/index.js";
import { fileURLToPath, URL } from "node:url";
import { nodePolyfills } from "file:///Users/olin/Documents/workspace/golang/zhongkui/web/node_modules/.pnpm/vite-plugin-node-polyfills@0.23.0_rollup@4.52.5_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8_/node_modules/vite-plugin-node-polyfills/dist/index.js";
var __vite_injected_original_dirname = "/Users/olin/Documents/workspace/golang/zhongkui/web";
var __vite_injected_original_import_meta_url = "file:///Users/olin/Documents/workspace/golang/zhongkui/web/vite.config.mts";
var vite_config_default = defineConfig(({ mode }) => {
  return {
    plugins: [
      vueI18n({
        runtimeOnly: false,
        compositionOnly: true,
        defaultSFCLang: "yaml",
        include: path.resolve(__vite_injected_original_dirname, "./src/locales/**")
      }),
      nodePolyfills({
        globals: { Buffer: true }
      }),
      VueRouter({
        dts: "src/typed-router.d.ts"
      }),
      Layouts(),
      AutoImport({
        imports: [
          "vue",
          {
            "vue-router/auto": ["useRoute", "useRouter"]
          }
        ],
        dts: "src/auto-imports.d.ts",
        eslintrc: {
          enabled: true
        },
        vueTemplate: true
      }),
      Components({
        dirs: ["src/components/", "src/layouts/"],
        dts: "src/components.d.ts"
      }),
      Vue({
        template: { transformAssetUrls }
      }),
      // https://github.com/vuetifyjs/vuetify-loader/tree/master/packages/vite-plugin#readme
      Vuetify({
        autoImport: true,
        styles: {
          configFile: "src/styles/settings.scss"
        }
      }),
      Fonts({
        google: {
          families: [
            {
              name: "Roboto",
              styles: "wght@100;300;400;500;700;900"
            }
          ]
        }
      })
    ],
    define: {
      "process.env": {
        ZHONGKUI_BASE_URL: loadEnv(mode, process.cwd()).VITE_ZHONGKUI_BASE_URL
      }
    },
    resolve: {
      alias: {
        "@": fileURLToPath(new URL("./src", __vite_injected_original_import_meta_url))
      },
      extensions: [".js", ".json", ".jsx", ".mjs", ".ts", ".tsx", ".vue"]
    },
    server: {
      port: 8e3
    },
    css: {
      preprocessorOptions: {
        sass: {
          api: "modern-compiler"
        },
        scss: {
          api: "modern-compiler"
        }
      }
    },
    // vite-plugin-vuetify's autoImport generates `vuetify/components/*` imports
    // on-the-fly during .vue transform, so Vite's dep optimizer cannot discover
    // them at startup. Without excluding vuetify, the first page load triggers
    // "new dependencies optimized -> reloading", aborting in-flight requests
    // (ERR_ABORTED) and breaking the dynamic import of lazy routes.
    // This mirrors the official `create vuetify` Vite template.
    optimizeDeps: {
      exclude: ["vuetify"]
    }
  };
});
export {
  vite_config_default as default
};
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidml0ZS5jb25maWcubXRzIl0sCiAgInNvdXJjZXNDb250ZW50IjogWyJjb25zdCBfX3ZpdGVfaW5qZWN0ZWRfb3JpZ2luYWxfZGlybmFtZSA9IFwiL1VzZXJzL29saW4vRG9jdW1lbnRzL3dvcmtzcGFjZS9nb2xhbmcvemhvbmdrdWkvd2ViXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ZpbGVuYW1lID0gXCIvVXNlcnMvb2xpbi9Eb2N1bWVudHMvd29ya3NwYWNlL2dvbGFuZy96aG9uZ2t1aS93ZWIvdml0ZS5jb25maWcubXRzXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ltcG9ydF9tZXRhX3VybCA9IFwiZmlsZTovLy9Vc2Vycy9vbGluL0RvY3VtZW50cy93b3Jrc3BhY2UvZ29sYW5nL3pob25na3VpL3dlYi92aXRlLmNvbmZpZy5tdHNcIjsvLyBQbHVnaW5zXG5pbXBvcnQgQXV0b0ltcG9ydCBmcm9tIFwidW5wbHVnaW4tYXV0by1pbXBvcnQvdml0ZVwiO1xuaW1wb3J0IENvbXBvbmVudHMgZnJvbSBcInVucGx1Z2luLXZ1ZS1jb21wb25lbnRzL3ZpdGVcIjtcbmltcG9ydCBGb250cyBmcm9tIFwidW5wbHVnaW4tZm9udHMvdml0ZVwiO1xuaW1wb3J0IExheW91dHMgZnJvbSBcInZpdGUtcGx1Z2luLXZ1ZS1sYXlvdXRzXCI7XG5pbXBvcnQgVnVlIGZyb20gXCJAdml0ZWpzL3BsdWdpbi12dWVcIjtcbmltcG9ydCBWdWVSb3V0ZXIgZnJvbSBcInVucGx1Z2luLXZ1ZS1yb3V0ZXIvdml0ZVwiO1xuaW1wb3J0IFZ1ZXRpZnksIHsgdHJhbnNmb3JtQXNzZXRVcmxzIH0gZnJvbSBcInZpdGUtcGx1Z2luLXZ1ZXRpZnlcIjtcbmltcG9ydCB2dWVJMThuIGZyb20gXCJAaW50bGlmeS91bnBsdWdpbi12dWUtaTE4bi92aXRlXCI7XG5cbi8vIFV0aWxpdGllc1xuaW1wb3J0IHBhdGggZnJvbSBcInBhdGhcIjtcbmltcG9ydCB7IGRlZmluZUNvbmZpZywgbG9hZEVudiB9IGZyb20gXCJ2aXRlXCI7XG5pbXBvcnQgeyBmaWxlVVJMVG9QYXRoLCBVUkwgfSBmcm9tIFwibm9kZTp1cmxcIjtcbmltcG9ydCB7IG5vZGVQb2x5ZmlsbHMgfSBmcm9tIFwidml0ZS1wbHVnaW4tbm9kZS1wb2x5ZmlsbHNcIjtcblxuLy8gaHR0cHM6Ly92aXRlanMuZGV2L2NvbmZpZy9cbmV4cG9ydCBkZWZhdWx0IGRlZmluZUNvbmZpZygoeyBtb2RlIH0pID0+IHtcbiAgcmV0dXJuIHtcbiAgICBwbHVnaW5zOiBbXG4gICAgICB2dWVJMThuKHtcbiAgICAgICAgcnVudGltZU9ubHk6IGZhbHNlLFxuICAgICAgICBjb21wb3NpdGlvbk9ubHk6IHRydWUsXG4gICAgICAgIGRlZmF1bHRTRkNMYW5nOiBcInlhbWxcIixcbiAgICAgICAgaW5jbHVkZTogcGF0aC5yZXNvbHZlKF9fZGlybmFtZSwgXCIuL3NyYy9sb2NhbGVzLyoqXCIpLFxuICAgICAgfSksXG4gICAgICBub2RlUG9seWZpbGxzKHtcbiAgICAgICAgZ2xvYmFsczogeyBCdWZmZXI6IHRydWUgfSxcbiAgICAgIH0pLFxuICAgICAgVnVlUm91dGVyKHtcbiAgICAgICAgZHRzOiBcInNyYy90eXBlZC1yb3V0ZXIuZC50c1wiLFxuICAgICAgfSksXG4gICAgICBMYXlvdXRzKCksXG4gICAgICBBdXRvSW1wb3J0KHtcbiAgICAgICAgaW1wb3J0czogW1xuICAgICAgICAgIFwidnVlXCIsXG4gICAgICAgICAge1xuICAgICAgICAgICAgXCJ2dWUtcm91dGVyL2F1dG9cIjogW1widXNlUm91dGVcIiwgXCJ1c2VSb3V0ZXJcIl0sXG4gICAgICAgICAgfSxcbiAgICAgICAgXSxcbiAgICAgICAgZHRzOiBcInNyYy9hdXRvLWltcG9ydHMuZC50c1wiLFxuICAgICAgICBlc2xpbnRyYzoge1xuICAgICAgICAgIGVuYWJsZWQ6IHRydWUsXG4gICAgICAgIH0sXG4gICAgICAgIHZ1ZVRlbXBsYXRlOiB0cnVlLFxuICAgICAgfSksXG4gICAgICBDb21wb25lbnRzKHtcbiAgICAgICAgZGlyczogW1wic3JjL2NvbXBvbmVudHMvXCIsIFwic3JjL2xheW91dHMvXCJdLFxuICAgICAgICBkdHM6IFwic3JjL2NvbXBvbmVudHMuZC50c1wiLFxuICAgICAgfSksXG4gICAgICBWdWUoe1xuICAgICAgICB0ZW1wbGF0ZTogeyB0cmFuc2Zvcm1Bc3NldFVybHMgfSxcbiAgICAgIH0pLFxuICAgICAgLy8gaHR0cHM6Ly9naXRodWIuY29tL3Z1ZXRpZnlqcy92dWV0aWZ5LWxvYWRlci90cmVlL21hc3Rlci9wYWNrYWdlcy92aXRlLXBsdWdpbiNyZWFkbWVcbiAgICAgIFZ1ZXRpZnkoe1xuICAgICAgICBhdXRvSW1wb3J0OiB0cnVlLFxuICAgICAgICBzdHlsZXM6IHtcbiAgICAgICAgICBjb25maWdGaWxlOiBcInNyYy9zdHlsZXMvc2V0dGluZ3Muc2Nzc1wiLFxuICAgICAgICB9LFxuICAgICAgfSksXG4gICAgICBGb250cyh7XG4gICAgICAgIGdvb2dsZToge1xuICAgICAgICAgIGZhbWlsaWVzOiBbXG4gICAgICAgICAgICB7XG4gICAgICAgICAgICAgIG5hbWU6IFwiUm9ib3RvXCIsXG4gICAgICAgICAgICAgIHN0eWxlczogXCJ3Z2h0QDEwMDszMDA7NDAwOzUwMDs3MDA7OTAwXCIsXG4gICAgICAgICAgICB9LFxuICAgICAgICAgIF0sXG4gICAgICAgIH0sXG4gICAgICB9KSxcbiAgICBdLFxuICAgIGRlZmluZToge1xuICAgICAgXCJwcm9jZXNzLmVudlwiOiB7XG4gICAgICAgIFpIT05HS1VJX0JBU0VfVVJMOiBsb2FkRW52KG1vZGUsIHByb2Nlc3MuY3dkKCkpLlZJVEVfWkhPTkdLVUlfQkFTRV9VUkwsXG4gICAgICB9LFxuICAgIH0sXG4gICAgcmVzb2x2ZToge1xuICAgICAgYWxpYXM6IHtcbiAgICAgICAgXCJAXCI6IGZpbGVVUkxUb1BhdGgobmV3IFVSTChcIi4vc3JjXCIsIGltcG9ydC5tZXRhLnVybCkpLFxuICAgICAgfSxcbiAgICAgIGV4dGVuc2lvbnM6IFtcIi5qc1wiLCBcIi5qc29uXCIsIFwiLmpzeFwiLCBcIi5tanNcIiwgXCIudHNcIiwgXCIudHN4XCIsIFwiLnZ1ZVwiXSxcbiAgICB9LFxuICAgIHNlcnZlcjoge1xuICAgICAgcG9ydDogODAwMCxcbiAgICB9LFxuICAgIGNzczoge1xuICAgICAgcHJlcHJvY2Vzc29yT3B0aW9uczoge1xuICAgICAgICBzYXNzOiB7XG4gICAgICAgICAgYXBpOiBcIm1vZGVybi1jb21waWxlclwiLFxuICAgICAgICB9LFxuICAgICAgICBzY3NzOiB7XG4gICAgICAgICAgYXBpOiBcIm1vZGVybi1jb21waWxlclwiLFxuICAgICAgICB9LFxuICAgICAgfSxcbiAgICB9LFxuICAgIC8vIHZpdGUtcGx1Z2luLXZ1ZXRpZnkncyBhdXRvSW1wb3J0IGdlbmVyYXRlcyBgdnVldGlmeS9jb21wb25lbnRzLypgIGltcG9ydHNcbiAgICAvLyBvbi10aGUtZmx5IGR1cmluZyAudnVlIHRyYW5zZm9ybSwgc28gVml0ZSdzIGRlcCBvcHRpbWl6ZXIgY2Fubm90IGRpc2NvdmVyXG4gICAgLy8gdGhlbSBhdCBzdGFydHVwLiBXaXRob3V0IGV4Y2x1ZGluZyB2dWV0aWZ5LCB0aGUgZmlyc3QgcGFnZSBsb2FkIHRyaWdnZXJzXG4gICAgLy8gXCJuZXcgZGVwZW5kZW5jaWVzIG9wdGltaXplZCAtPiByZWxvYWRpbmdcIiwgYWJvcnRpbmcgaW4tZmxpZ2h0IHJlcXVlc3RzXG4gICAgLy8gKEVSUl9BQk9SVEVEKSBhbmQgYnJlYWtpbmcgdGhlIGR5bmFtaWMgaW1wb3J0IG9mIGxhenkgcm91dGVzLlxuICAgIC8vIFRoaXMgbWlycm9ycyB0aGUgb2ZmaWNpYWwgYGNyZWF0ZSB2dWV0aWZ5YCBWaXRlIHRlbXBsYXRlLlxuICAgIG9wdGltaXplRGVwczoge1xuICAgICAgZXhjbHVkZTogW1widnVldGlmeVwiXSxcbiAgICB9LFxuICB9O1xufSk7XG4iXSwKICAibWFwcGluZ3MiOiAi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
