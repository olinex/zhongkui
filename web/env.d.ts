/// <reference types="vite/client" />
/// <reference types="unplugin-vue-router/client" />
/// <reference types="vite-plugin-vue-layouts/client" />

/**
 * 构建标识：由 vite.config.mts 在编译期注入。
 * 默认值与环境变量映射见 build.config.ts。
 */
declare const __BUILD_INFO__: {
  tag: string;
  commit: string;
};
