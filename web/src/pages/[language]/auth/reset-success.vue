<template>
  <central-layout>
    <template #body>
      <v-empty-state
        headline="✓"
        :title="$t('notice.passwordReset')"
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
name: Password Reset Success
meta:
  title: notice.passwordReset
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import router from "@/router";
import { useAppStore } from "@/stores/app";

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

// goSignIn 跳转登录页。带 force 标记跳过守卫的「已登录」拦截：重置密码全程无需登录，
// 但仍持有效会话的用户同样可走完该流程，此时守卫会把登录页换成「已登录」中转页，
// 使倒计时承诺的「前往登录页」落空；force 与 already-signed-in 的「仍要登录」同一约定。
const goSignIn = (): void => {
  router.replace({
    name: "Sign In",
    params: {
      language: appStore.language,
    },
    query: {
      force: "1",
    },
  });
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
      goSignIn();
    }
  }, PROCESS_STEP_INTERVAL_MS);
});

onUnmounted(stopCountdown);
</script>
