/**
 * router/index.ts
 *
 * 为 `./src/pages/*.vue` 自动生成的路由。
 */

import consola from "consola";
import { setupLayouts } from "virtual:generated-layouts";
// 组合式函数
import { createRouter, createWebHistory } from "vue-router";
import { routes } from "vue-router/auto-routes";

import { useAppStore } from "@/stores/app";
import { useConfigSyncStore } from "@/stores/configSync";
import { useEnumsStore } from "@/stores/enums";
import { useFeaturesStore } from "@/stores/features";
import { useUserStore } from "@/stores/user";
import i18n from "@/utils/i18n";
import { mitter } from "@/utils/mitter";

import { AUTH_FAILED_EVENT } from "@/apis/base";
import { featureOfRoute } from "@/navigation";
import { SupportLanguage } from "@/types";
import { isEnumValue } from "@/utils/options";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: setupLayouts(routes),
});

// 针对 https://github.com/vitejs/vite/issues/11804 的变通处理
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

// 全局登录态捕获：已登录（userStore.exists）却收到后端 401（JWT 无效 / 过期 /
// 用户不存在，由 apis/base.ts 的响应拦截器广播），说明服务端会话已失效。
// 此时复位本地会话并跳转 401 页面——与守卫的失效出口同一落点，由该页引导回登录页，
// 并在重新登录后回到原路径。订阅必须在此模块注册：它先于应用挂载求值，
// 而路由守卫在首次导航时即可能发起请求，晚注册会漏掉启动阶段的 401。
mitter.on(AUTH_FAILED_EVENT, () => {
  const userStore = useUserStore();
  // 尚未加载出登录态（如启动时本地残留失效令牌）不打扰用户：
  // 该情形由 userStore.load 的失败回调自行复位，停留在公开页面即可
  if (!userStore.exists) {
    return;
  }
  const current = router.currentRoute.value;
  // 已在 401 / 登录页时不再跳转，避免与守卫的重定向互相覆盖
  if (current.name === "401" || current.name === "Sign In") {
    return;
  }
  consola.warn("auth failed while signed in, redirect to 401");
  userStore.signOut();
  router.replace({
    name: "401",
    params: {
      language: useAppStore().language,
    },
    query: {
      ...current.query,
      redirect: current.fullPath,
    },
  });
});

router.beforeEach(async (to, from, next) => {
  const userStore = useUserStore();
  // 受保护路由需等待用户状态初始化完成后再校验登录态，
  // 避免授权信息尚未加载完成就被误判为未登录而重定向到 401 页面；
  // 仅面向未登录用户的页面（guestOnly）同样需要先确定登录态，
  // 才能在已登录时判定是否跳过该页
  if ((to.meta.requiresAuth || to.meta.guestOnly) && !userStore.initial) {
    await userStore.init();
  }
  const appStore = useAppStore();
  const params = to.params as Record<string, unknown>;
  const lang = params.language as SupportLanguage;
  // 以下重定向分支一律以 next(location) 结束并立即返回：若沿用 router.replace 后继续执行、
  // 最终再下一次 next()，同一次导航会派生两条导航（新的 replace 与原来的 next），二者可能
  // 互相取消，使重定向偶发失效（失效会话进入 401 页后停在原地不再跳转即其一）。next(location)
  // 由 vue-router 原子地中止本次导航并开启目标导航，避免该竞态。
  if (to.path === "/") {
    return next({
      name: "Home",
      params: {
        language: appStore.language,
      },
      query: to.query,
    });
  } else if (!isEnumValue(SupportLanguage, lang)) {
    consola.warn("language not found, redirect to 404");
    return next({
      name: "404",
      params: {
        language: appStore.language,
      },
      query: {
        ...to.query,
        redirect: to.fullPath,
      },
    });
  } else if (to.meta.guestOnly && userStore.exists && to.query.force !== "1") {
    // 已登录用户访问仅面向未登录用户的页面（登录页）时，不静默跳转主页
    // （看起来像故障），而是先到「已登录」中转页说明状态，由用户选择
    // 返回上一级 / 返回首页 / 仍要登录。中转页选择「仍要登录」时携带 force=1
    // 重新进入登录页，此处据此放行，避免来回重定向形成循环。
    consola.info("already signed in, redirect to already-signed-in interlude");
    return next({
      name: "Already Signed In",
      params: {
        language: appStore.language,
      },
      query: {
        // 保留登录页原定的回跳目标，供「仍要登录」完成后继续前往
        ...(to.query.redirect ? { redirect: to.query.redirect } : {}),
      },
    });
  } else if (to.meta.requiresAuth && !(await userStore.ensureValid())) {
    // ensureValid 会在令牌过期但本地仍有会话时先尝试刷新恢复（滑动过期），
    // 刷新失败才视为会话失效，跳转 401 页面
    consola.warn("user not valid, redirect to 401");
    return next({
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

// 路由对象会被展开传入 `router.push`，因此其类型刻意保持宽松。
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
