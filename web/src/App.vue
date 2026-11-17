<template>
  <v-app :theme="appStore.theme">
    <router-view :key="routeKey" />
    <!-- 全局悬浮「直达底部」按钮：列表内容过高时出现，可拖动/临时关闭 -->
    <scroll-to-bottom-fab />
    <!-- 全局 AI 助手抽屉：登录后挂载在 v-app 内，跨路由常驻，会话不随导航丢失；
         未登录（登出或会话失效）时不挂载，避免抽屉残留在登录页等公开页面 -->
    <assistant-drawer v-if="userStore.valid" v-model="assistantStore.opened" />
    <v-snackbar-queue v-model="appStore.messages" />
    <!-- 构建标识：贴靠视口右下角，供线上排障时核对代码版本 -->
    <build-stamp />
  </v-app>
</template>

<script setup lang="ts" name="App">
import { useAppStore } from "./stores/app";
import { useAssistantStore } from "./stores/assistant";
import { useEnumsStore } from "./stores/enums";
import { useUserStore } from "./stores/user";
import { mitter } from "@/utils/mitter";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();
const assistantStore = useAssistantStore();
const routeKey = computed(() => route.params.language);

// 登录态失效（登出或会话清空）时同步复位抽屉开关：此时抽屉随即被卸载，
// 若不复位，重新登录后会被上一次的开启态直接弹出
watch(
  () => userStore.valid,
  (valid) => {
    if (!valid) {
      assistantStore.close();
    }
  },
);

onBeforeMount(() => {
  userStore.refreshEventOn(mitter);
});

onMounted(() => {
  // 应用启动时对初始语言全量预取枚举条目并缓存（已加载同语言时内部跳过）；
  // 枚举值空间仅后端进程重启才可能更新，按语言永久缓存，语言切换后惰性获取。
  useEnumsStore().prefetch(appStore.language);
});

onUnmounted(() => {
  userStore.refreshEventOff(mitter);
});
</script>
