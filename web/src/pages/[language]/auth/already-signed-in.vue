<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <!-- 已登录中转页：由路由守卫在已登录用户进入登录页时重定向至此。
           直接跳转主页会让用户以为出现了故障，故在此说明已登录状态，并给出
           返回上一级 / 返回首页 / 仍要登录 三个出口，由用户自行选择。 -->
      <v-empty-state
        icon="mdi-account-check-outline"
        color="success"
        :title="$t('summary.alreadySignedIn')"
        :text="$t('summary.alreadySignedInHint')"
      >
        <template #actions>
          <v-btn variant="text" @click="goBackPrevious">{{ $t("action.backPrevious") }}</v-btn>
          <v-btn variant="text" :to="{ name: 'Home', params: { language: appStore.language } }">
            {{ $t("action.backHome") }}
          </v-btn>
          <v-btn color="primary" variant="flat" replace :to="signInTo">
            {{ $t("action.signInAnyway") }}
          </v-btn>
        </template>
      </v-empty-state>
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: Already Signed In
meta:
  title: title.alreadySignedIn
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import router from "@/router";
import { useAppStore } from "@/stores/app";

const route = useRoute();
const appStore = useAppStore();

// signInTo 仍要登录的目标：保留原 redirect（若有），并带 force 标记跳过守卫的已登录拦截，
// 否则会立即被守卫送回本页，形成循环
const signInTo = computed(() => ({
  name: "Sign In" as const,
  params: { language: appStore.language },
  query: {
    ...(route.query.redirect ? { redirect: String(route.query.redirect) } : {}),
    force: "1",
  },
}));

// goBackPrevious 返回上一级：回到进入登录页前所处的页面。守卫以 replace 重定向至此，
// 原历史条目的回退指针被保留，故 router.back() 即回到该页面；直接用 URL 打开本页时
// 无历史可回退，此时退化为回首页，避免按钮无响应或把用户带出应用。
const goBackPrevious = () => {
  const state = window.history.state as { back?: string | null } | null;
  if (state?.back) {
    router.back();
    return;
  }
  router.replace({ name: "Home", params: { language: appStore.language } });
};
</script>
