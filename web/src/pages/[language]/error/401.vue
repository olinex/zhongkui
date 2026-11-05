<template>
  <central-layout>
    <template #body>
      <v-empty-state
        headline="401"
        :title="$t('summary.notSignInYet')"
        :text="$t('summary.goingToSignIn')"
      >
        <template #actions>
          <v-progress-circular
            :model-value="process"
            :rotate="360"
            :size="100"
            :width="15"
            color="success"
          >
            {{ release }} {{ $t("unit.second") }}
          </v-progress-circular>
        </template>
      </v-empty-state>
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: "401"
meta:
  title: title.err401
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import router from "@/router";
import { useAppStore } from "@/stores/app";

// 类型
import type { RouteLocationRaw } from "vue-router";

const route = useRoute();
const appStore = useAppStore();
const SIGN_IN_TIMEOUT_MS = 3000;
const PROCESS_STEP_INTERVAL_MS = 1000;
// STEP_COUNT 倒计时总格数：把总时长按固定步长切成整格，末格正好归零
const STEP_COUNT = SIGN_IN_TIMEOUT_MS / PROCESS_STEP_INTERVAL_MS;
const process = ref(0);
const release = ref(STEP_COUNT);
let tickTimer: number | undefined;

// stopCountdown 停止倒计时计时器。
const stopCountdown = (): void => {
  if (tickTimer !== undefined) {
    window.clearInterval(tickTimer);
    tickTimer = undefined;
  }
};

// goSignIn 跳转登录页。带 force 标记跳过守卫的「已登录」拦截：本页即会话失效后的引导页，
// 而本地会话未必已清空（如令牌仅过期、刷新失败时守卫只跳转不清会话），此时守卫会把登录页
// 换成「已登录」中转页，使倒计时承诺的「前往登录页」落空；force 与 already-signed-in 的
// 「仍要登录」同一约定，用于确保引导的实际落地页就是登录页。
const goSignIn = async (): Promise<void> => {
  const target: RouteLocationRaw = {
    name: "Sign In" as const,
    params: {
      language: appStore.language,
    },
    query: {
      redirect: route.query.redirect || "",
      force: "1",
    },
  };
  try {
    await router.replace(target);
  } catch {
    // 导航被拒绝（守卫抛错 / 动态导入失败）时忽略，交由下方兜底处理
  }
  // 兜底整页跳转：导航被取消时 router.replace 既不抛错也不改变路由，页面会停在「0 秒」
  // 不再跳转（倒计时已停表，无从恢复）。故以当前路由是否仍停留在本页作为判据，
  // 命中即改用整页跳转，确保倒计时归零后必定离开本页。
  if (router.currentRoute.value.name === "401") {
    window.location.assign(router.resolve(target).href);
  }
};

onMounted(() => {
  // 单一定时器同时驱动进度与剩余秒数：每格递减一档，归零即停表并跳转。
  // 停表必须早于跳转——跳转要经过异步守卫，在途期间若计时器仍在走，剩余秒数会跌破零值
  // 显示为负数（原实现只在卸载时清 setTimeout，未清的 interval 会持续递减且永不停止）。
  tickTimer = window.setInterval(() => {
    const left = release.value - 1;
    release.value = left;
    process.value = ((STEP_COUNT - left) / STEP_COUNT) * 100;
    if (left <= 0) {
      stopCountdown();
      void goSignIn();
    }
  }, PROCESS_STEP_INTERVAL_MS);
});

onUnmounted(stopCountdown);
</script>
