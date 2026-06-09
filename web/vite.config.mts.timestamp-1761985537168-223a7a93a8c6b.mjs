// vite.config.mts
import AutoImport from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-auto-import@0.17.8_rollup@4.52.5/node_modules/unplugin-auto-import/dist/vite.js";
import Components from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-vue-components@0.27.5_@babel+parser@7.28.5_rollup@4.52.5_vue@3.5.22_typescript@5.6.3_/node_modules/unplugin-vue-components/dist/vite.js";
import Fonts from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-fonts@1.4.0_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8_/node_modules/unplugin-fonts/dist/vite.mjs";
import Layouts from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/vite-plugin-vue-layouts@0.11.0_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sa_7f784ff2d1a1fa7e56b11211af2c11a4/node_modules/vite-plugin-vue-layouts/dist/index.mjs";
import Vue from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/@vitejs+plugin-vue@5.2.4_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8__vue@3.5.22_typescript@5.6.3_/node_modules/@vitejs/plugin-vue/dist/index.mjs";
import VueRouter from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/unplugin-vue-router@0.10.9_rollup@4.52.5_vue-router@4.6.3_vue@3.5.22_typescript@5.6.3___vue@3.5.22_typescript@5.6.3_/node_modules/unplugin-vue-router/dist/vite.js";
import Vuetify, { transformAssetUrls } from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/vite-plugin-vuetify@2.1.2_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1._33d7dc8fecbc46bc5f37131fb10e756e/node_modules/vite-plugin-vuetify/dist/index.mjs";
import vueI18n from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/@intlify+unplugin-vue-i18n@6.0.8_@vue+compiler-dom@3.5.22_eslint@9.38.0_rollup@4.52.5_t_e5d385869af179f828cfc55076938007/node_modules/@intlify/unplugin-vue-i18n/lib/vite.mjs";
import path from "path";
import { defineConfig, loadEnv } from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8/node_modules/vite/dist/node/index.js";
import { fileURLToPath, URL } from "node:url";
import { nodePolyfills } from "file:///home/jcm/workspace/golang/zhongkui/web/node_modules/.pnpm/vite-plugin-node-polyfills@0.23.0_rollup@4.52.5_vite@5.4.21_@types+node@22.18.12_sass-embedded@1.93.2_sass@1.77.8_/node_modules/vite-plugin-node-polyfills/dist/index.js";
var __vite_injected_original_dirname = "/home/jcm/workspace/golang/zhongkui/web";
var __vite_injected_original_import_meta_url = "file:///home/jcm/workspace/golang/zhongkui/web/vite.config.mts";
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
    define: { "process.env": {
      ZHONGKUI_BASE_URL: loadEnv(mode, process.cwd()).VITE_ZHONGKUI_BASE_URL
    } },
    resolve: {
      alias: {
        "@": fileURLToPath(new URL("./src", __vite_injected_original_import_meta_url))
      },
      extensions: [".js", ".json", ".jsx", ".mjs", ".ts", ".tsx", ".vue"]
    },
    server: {
      port: 3e3
    },
    css: {
      preprocessorOptions: {
        sass: {
          api: "modern-compiler"
        }
      }
    }
  };
});
export {
  vite_config_default as default
};
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidml0ZS5jb25maWcubXRzIl0sCiAgInNvdXJjZXNDb250ZW50IjogWyJjb25zdCBfX3ZpdGVfaW5qZWN0ZWRfb3JpZ2luYWxfZGlybmFtZSA9IFwiL2hvbWUvamNtL3dvcmtzcGFjZS9nb2xhbmcvemhvbmdrdWkvd2ViXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ZpbGVuYW1lID0gXCIvaG9tZS9qY20vd29ya3NwYWNlL2dvbGFuZy96aG9uZ2t1aS93ZWIvdml0ZS5jb25maWcubXRzXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ltcG9ydF9tZXRhX3VybCA9IFwiZmlsZTovLy9ob21lL2pjbS93b3Jrc3BhY2UvZ29sYW5nL3pob25na3VpL3dlYi92aXRlLmNvbmZpZy5tdHNcIjsvLyBQbHVnaW5zXG5pbXBvcnQgQXV0b0ltcG9ydCBmcm9tIFwidW5wbHVnaW4tYXV0by1pbXBvcnQvdml0ZVwiO1xuaW1wb3J0IENvbXBvbmVudHMgZnJvbSBcInVucGx1Z2luLXZ1ZS1jb21wb25lbnRzL3ZpdGVcIjtcbmltcG9ydCBGb250cyBmcm9tIFwidW5wbHVnaW4tZm9udHMvdml0ZVwiO1xuaW1wb3J0IExheW91dHMgZnJvbSBcInZpdGUtcGx1Z2luLXZ1ZS1sYXlvdXRzXCI7XG5pbXBvcnQgVnVlIGZyb20gXCJAdml0ZWpzL3BsdWdpbi12dWVcIjtcbmltcG9ydCBWdWVSb3V0ZXIgZnJvbSBcInVucGx1Z2luLXZ1ZS1yb3V0ZXIvdml0ZVwiO1xuaW1wb3J0IFZ1ZXRpZnksIHsgdHJhbnNmb3JtQXNzZXRVcmxzIH0gZnJvbSBcInZpdGUtcGx1Z2luLXZ1ZXRpZnlcIjtcbmltcG9ydCB2dWVJMThuIGZyb20gXCJAaW50bGlmeS91bnBsdWdpbi12dWUtaTE4bi92aXRlXCI7XG5cbi8vIFV0aWxpdGllc1xuaW1wb3J0IHBhdGggZnJvbSBcInBhdGhcIjtcbmltcG9ydCB7IGRlZmluZUNvbmZpZywgbG9hZEVudiB9IGZyb20gXCJ2aXRlXCI7XG5pbXBvcnQgeyBmaWxlVVJMVG9QYXRoLCBVUkwgfSBmcm9tIFwibm9kZTp1cmxcIjtcbmltcG9ydCB7IG5vZGVQb2x5ZmlsbHMgfSBmcm9tIFwidml0ZS1wbHVnaW4tbm9kZS1wb2x5ZmlsbHNcIjtcblxuLy8gaHR0cHM6Ly92aXRlanMuZGV2L2NvbmZpZy9cbmV4cG9ydCBkZWZhdWx0IGRlZmluZUNvbmZpZygoeyBtb2RlIH0pID0+IHtcbiAgcmV0dXJuIHtcbiAgICBwbHVnaW5zOiBbXG4gICAgICB2dWVJMThuKHtcbiAgICAgICAgcnVudGltZU9ubHk6IGZhbHNlLFxuICAgICAgICBjb21wb3NpdGlvbk9ubHk6IHRydWUsXG4gICAgICAgIGRlZmF1bHRTRkNMYW5nOiBcInlhbWxcIixcbiAgICAgICAgaW5jbHVkZTogcGF0aC5yZXNvbHZlKF9fZGlybmFtZSwgXCIuL3NyYy9sb2NhbGVzLyoqXCIpLFxuICAgICAgfSksXG4gICAgICBub2RlUG9seWZpbGxzKHtcbiAgICAgICAgZ2xvYmFsczogeyBCdWZmZXI6IHRydWUgfSxcbiAgICAgIH0pLFxuICAgICAgVnVlUm91dGVyKHtcbiAgICAgICAgZHRzOiBcInNyYy90eXBlZC1yb3V0ZXIuZC50c1wiLFxuICAgICAgfSksXG4gICAgICBMYXlvdXRzKCksXG4gICAgICBBdXRvSW1wb3J0KHtcbiAgICAgICAgaW1wb3J0czogW1xuICAgICAgICAgIFwidnVlXCIsXG4gICAgICAgICAge1xuICAgICAgICAgICAgXCJ2dWUtcm91dGVyL2F1dG9cIjogW1widXNlUm91dGVcIiwgXCJ1c2VSb3V0ZXJcIl0sXG4gICAgICAgICAgfSxcbiAgICAgICAgXSxcbiAgICAgICAgZHRzOiBcInNyYy9hdXRvLWltcG9ydHMuZC50c1wiLFxuICAgICAgICBlc2xpbnRyYzoge1xuICAgICAgICAgIGVuYWJsZWQ6IHRydWUsXG4gICAgICAgIH0sXG4gICAgICAgIHZ1ZVRlbXBsYXRlOiB0cnVlLFxuICAgICAgfSksXG4gICAgICBDb21wb25lbnRzKHtcbiAgICAgICAgZGlyczogW1wic3JjL2NvbXBvbmVudHMvXCIsIFwic3JjL2xheW91dHMvXCJdLFxuICAgICAgICBkdHM6IFwic3JjL2NvbXBvbmVudHMuZC50c1wiLFxuICAgICAgfSksXG4gICAgICBWdWUoe1xuICAgICAgICB0ZW1wbGF0ZTogeyB0cmFuc2Zvcm1Bc3NldFVybHMgfSxcbiAgICAgIH0pLFxuICAgICAgLy8gaHR0cHM6Ly9naXRodWIuY29tL3Z1ZXRpZnlqcy92dWV0aWZ5LWxvYWRlci90cmVlL21hc3Rlci9wYWNrYWdlcy92aXRlLXBsdWdpbiNyZWFkbWVcbiAgICAgIFZ1ZXRpZnkoe1xuICAgICAgICBhdXRvSW1wb3J0OiB0cnVlLFxuICAgICAgICBzdHlsZXM6IHtcbiAgICAgICAgICBjb25maWdGaWxlOiBcInNyYy9zdHlsZXMvc2V0dGluZ3Muc2Nzc1wiLFxuICAgICAgICB9LFxuICAgICAgfSksXG4gICAgICBGb250cyh7XG4gICAgICAgIGdvb2dsZToge1xuICAgICAgICAgIGZhbWlsaWVzOiBbXG4gICAgICAgICAgICB7XG4gICAgICAgICAgICAgIG5hbWU6IFwiUm9ib3RvXCIsXG4gICAgICAgICAgICAgIHN0eWxlczogXCJ3Z2h0QDEwMDszMDA7NDAwOzUwMDs3MDA7OTAwXCIsXG4gICAgICAgICAgICB9LFxuICAgICAgICAgIF0sXG4gICAgICAgIH0sXG4gICAgICB9KSxcbiAgICBdLFxuICAgIGRlZmluZTogeyBcInByb2Nlc3MuZW52XCI6IHtcbiAgICAgIFpIT05HS1VJX0JBU0VfVVJMOiBsb2FkRW52KG1vZGUsIHByb2Nlc3MuY3dkKCkpLlZJVEVfWkhPTkdLVUlfQkFTRV9VUkwsXG4gICAgfSB9LFxuICAgIHJlc29sdmU6IHtcbiAgICAgIGFsaWFzOiB7XG4gICAgICAgIFwiQFwiOiBmaWxlVVJMVG9QYXRoKG5ldyBVUkwoXCIuL3NyY1wiLCBpbXBvcnQubWV0YS51cmwpKSxcbiAgICAgIH0sXG4gICAgICBleHRlbnNpb25zOiBbXCIuanNcIiwgXCIuanNvblwiLCBcIi5qc3hcIiwgXCIubWpzXCIsIFwiLnRzXCIsIFwiLnRzeFwiLCBcIi52dWVcIl0sXG4gICAgfSxcbiAgICBzZXJ2ZXI6IHtcbiAgICAgIHBvcnQ6IDMwMDAsXG4gICAgfSxcbiAgICBjc3M6IHtcbiAgICAgIHByZXByb2Nlc3Nvck9wdGlvbnM6IHtcbiAgICAgICAgc2Fzczoge1xuICAgICAgICAgIGFwaTogXCJtb2Rlcm4tY29tcGlsZXJcIixcbiAgICAgICAgfSxcbiAgICAgIH0sXG4gICAgfSxcbiAgfTtcbn0pO1xuIl0sCiAgIm1hcHBpbmdzIjogIjtBQUNBLE9BQU8sZ0JBQWdCO0FBQ3ZCLE9BQU8sZ0JBQWdCO0FBQ3ZCLE9BQU8sV0FBVztBQUNsQixPQUFPLGFBQWE7QUFDcEIsT0FBTyxTQUFTO0FBQ2hCLE9BQU8sZUFBZTtBQUN0QixPQUFPLFdBQVcsMEJBQTBCO0FBQzVDLE9BQU8sYUFBYTtBQUdwQixPQUFPLFVBQVU7QUFDakIsU0FBUyxjQUFjLGVBQWU7QUFDdEMsU0FBUyxlQUFlLFdBQVc7QUFDbkMsU0FBUyxxQkFBcUI7QUFkOUIsSUFBTSxtQ0FBbUM7QUFBOEksSUFBTSwyQ0FBMkM7QUFpQnhPLElBQU8sc0JBQVEsYUFBYSxDQUFDLEVBQUUsS0FBSyxNQUFNO0FBQ3hDLFNBQU87QUFBQSxJQUNMLFNBQVM7QUFBQSxNQUNQLFFBQVE7QUFBQSxRQUNOLGFBQWE7QUFBQSxRQUNiLGlCQUFpQjtBQUFBLFFBQ2pCLGdCQUFnQjtBQUFBLFFBQ2hCLFNBQVMsS0FBSyxRQUFRLGtDQUFXLGtCQUFrQjtBQUFBLE1BQ3JELENBQUM7QUFBQSxNQUNELGNBQWM7QUFBQSxRQUNaLFNBQVMsRUFBRSxRQUFRLEtBQUs7QUFBQSxNQUMxQixDQUFDO0FBQUEsTUFDRCxVQUFVO0FBQUEsUUFDUixLQUFLO0FBQUEsTUFDUCxDQUFDO0FBQUEsTUFDRCxRQUFRO0FBQUEsTUFDUixXQUFXO0FBQUEsUUFDVCxTQUFTO0FBQUEsVUFDUDtBQUFBLFVBQ0E7QUFBQSxZQUNFLG1CQUFtQixDQUFDLFlBQVksV0FBVztBQUFBLFVBQzdDO0FBQUEsUUFDRjtBQUFBLFFBQ0EsS0FBSztBQUFBLFFBQ0wsVUFBVTtBQUFBLFVBQ1IsU0FBUztBQUFBLFFBQ1g7QUFBQSxRQUNBLGFBQWE7QUFBQSxNQUNmLENBQUM7QUFBQSxNQUNELFdBQVc7QUFBQSxRQUNULE1BQU0sQ0FBQyxtQkFBbUIsY0FBYztBQUFBLFFBQ3hDLEtBQUs7QUFBQSxNQUNQLENBQUM7QUFBQSxNQUNELElBQUk7QUFBQSxRQUNGLFVBQVUsRUFBRSxtQkFBbUI7QUFBQSxNQUNqQyxDQUFDO0FBQUE7QUFBQSxNQUVELFFBQVE7QUFBQSxRQUNOLFlBQVk7QUFBQSxRQUNaLFFBQVE7QUFBQSxVQUNOLFlBQVk7QUFBQSxRQUNkO0FBQUEsTUFDRixDQUFDO0FBQUEsTUFDRCxNQUFNO0FBQUEsUUFDSixRQUFRO0FBQUEsVUFDTixVQUFVO0FBQUEsWUFDUjtBQUFBLGNBQ0UsTUFBTTtBQUFBLGNBQ04sUUFBUTtBQUFBLFlBQ1Y7QUFBQSxVQUNGO0FBQUEsUUFDRjtBQUFBLE1BQ0YsQ0FBQztBQUFBLElBQ0g7QUFBQSxJQUNBLFFBQVEsRUFBRSxlQUFlO0FBQUEsTUFDdkIsbUJBQW1CLFFBQVEsTUFBTSxRQUFRLElBQUksQ0FBQyxFQUFFO0FBQUEsSUFDbEQsRUFBRTtBQUFBLElBQ0YsU0FBUztBQUFBLE1BQ1AsT0FBTztBQUFBLFFBQ0wsS0FBSyxjQUFjLElBQUksSUFBSSxTQUFTLHdDQUFlLENBQUM7QUFBQSxNQUN0RDtBQUFBLE1BQ0EsWUFBWSxDQUFDLE9BQU8sU0FBUyxRQUFRLFFBQVEsT0FBTyxRQUFRLE1BQU07QUFBQSxJQUNwRTtBQUFBLElBQ0EsUUFBUTtBQUFBLE1BQ04sTUFBTTtBQUFBLElBQ1I7QUFBQSxJQUNBLEtBQUs7QUFBQSxNQUNILHFCQUFxQjtBQUFBLFFBQ25CLE1BQU07QUFBQSxVQUNKLEtBQUs7QUFBQSxRQUNQO0FBQUEsTUFDRjtBQUFBLElBQ0Y7QUFBQSxFQUNGO0FBQ0YsQ0FBQzsiLAogICJuYW1lcyI6IFtdCn0K
