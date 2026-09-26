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
import { useConfigSyncStore } from "@/stores/configSync";
import { useEnumsStore } from "@/stores/enums";
import { useFeaturesStore } from "@/stores/features";
import { useUserStore } from "@/stores/user";
import i18n from "@/utils/i18n";

import { featureOfRoute } from "@/navigation";
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
  } else if (to.meta.requiresAuth && !(await userStore.ensureValid())) {
    // ensureValid 会在令牌过期但本地仍有会话时先尝试刷新恢复（滑动过期），
    // 刷新失败才视为会话失效，跳转 401 页面
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
  // 登录态有效时初始化前端配置同步：拉取服务端配置并开始监听本地变更
  if (to.meta.requiresAuth && userStore.exists) {
    await useConfigSyncStore().init();
    // 预取功能开关：导航侧边栏按开关标注入口可用性，须在页面渲染前加载完成
    await useFeaturesStore().load();
    // 功能不可用拦截：所属功能已关闭的路由不渲染其页面，统一重定向到「功能不可用」说明页，
    // 由该页展示后端下发的不可用原因并引导前往开通入口（避免各功能页各自处理不可用态）。
    // 命中时先强制刷新一次功能开关，使刚在开通入口完成开通的用户下一次导航即可通过。
    const featureKey = featureOfRoute(String(to.name));
    if (featureKey && !useFeaturesStore().isEnable(featureKey)) {
      await useFeaturesStore().load(true);
      if (!useFeaturesStore().isEnable(featureKey)) {
        consola.warn(`feature ${featureKey} is disabled, redirect to feature-unavailable`);
        // 此处必须用 push 而非 replace：被拦截的导航还没写入历史（守卫先于 finalizeNavigation），
        // 若以 replace 重定向，被顶掉的是用户当前所在页面的历史条目，说明页随即成为"无上一级"的
        // 死胡同——用户点「返回上一级」或浏览器后退只能越过该页落到更早的页面。
        // push 会保留用户所在页面作为上一条历史，两种后退方式都能正确回到它。
        await router.push({
          name: "Feature Unavailable",
          params: {
            language: appStore.language,
          },
          query: {
            ...to.query,
            feature: featureKey,
          },
        });
      }
    }
  }
  if (lang !== i18n.global.locale.value) {
    consola.info(`change language from ${i18n.global.locale.value} to ${lang}`);
    appStore.asLanguage(lang);
  }
  // 触发当前语言全量枚举预取与缓存（按语言在途去重，已加载同语言时内部跳过）；
  // 枚举值空间仅后端进程重启才可能更新，故永久缓存；首次导航即覆盖应用启动时的初始语言。
  await useEnumsStore().prefetch(lang);
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
