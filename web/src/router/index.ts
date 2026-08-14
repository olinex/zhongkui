/**
 * router/index.ts
 *
 * Automatic routes for `./src/pages/*.vue`
 */

import consola from "consola";
import { setupLayouts } from "virtual:generated-layouts";
// Composables
import { createRouter, createWebHistory } from "vue-router";
import { routes } from "vue-router/auto-routes";

import { useAppStore } from "@/stores/app";
import { useUserStore } from "@/stores/user";
import i18n from "@/utils/i18n";

import { SupportLanguage } from "@/types";
import { isEnumValue } from "@/utils/options";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: setupLayouts(routes),
});

// Workaround for https://github.com/vitejs/vite/issues/11804
router.onError((err, to) => {
  if (err?.message?.includes?.("Failed to fetch dynamically imported module")) {
    if (!localStorage.getItem("vuetify:dynamic-reload")) {
      console.log("Reloading page to fix dynamic import error");
      localStorage.setItem("vuetify:dynamic-reload", "true");
      location.assign(to.fullPath);
    } else {
      console.error("Dynamic import error, reloading page did not fix it", err);
    }
  } else {
    console.error(err);
  }
});

router.beforeEach(async (to, from, next) => {
  const userStore = useUserStore();
  // 受保护路由需等待用户状态初始化完成后再校验登录态，
  // 避免授权信息尚未加载完成就被误判为未登录而重定向到 401 页面
  if (to.meta.requiresAuth && !userStore.initial) {
    await userStore.init();
  }
  const appStore = useAppStore();
  const params = to.params as Record<string, unknown>;
  let lang = params.language as SupportLanguage;
  if (to.path === "/") {
    lang = appStore.language;
    await router.replace({
      name: "Home",
      params: {
        language: appStore.language,
      },
      query: to.query,
    });
  } else if (!isEnumValue(SupportLanguage, lang)) {
    consola.warn("language not found, redirect to 404");
    lang = appStore.language;
    await router.replace({
      name: "404",
      params: {
        language: appStore.language,
      },
      query: {
        ...to.query,
        redirect: to.fullPath,
      },
    });
  } else if (to.meta.requiresAuth && !userStore.valid) {
    consola.warn("user not valid, redirect to 401");
    await router.replace({
      name: "401",
      params: {
        language: appStore.language,
      },
      query: {
        ...to.query,
        redirect: to.fullPath,
      },
    });
  }
  if (lang !== i18n.global.locale.value) {
    consola.info(`change language from ${i18n.global.locale.value} to ${lang}`);
    appStore.asLanguage(lang);
  }
  if (to.meta.title) {
    document.title = i18n.global.t(to.meta.title as string);
  }
  next();
});

router.isReady().then(() => {
  localStorage.removeItem("vuetify:dynamic-reload");
});

// The route object is spread into `router.push`, so its shape is intentionally loose.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const asLanguagePath = (route: any, language: SupportLanguage) => {
  router.push({
    ...route,
    params: {
      ...route.params,
      language,
    },
    query: {
      ...route.query,
    },
  });
};

export default router;
