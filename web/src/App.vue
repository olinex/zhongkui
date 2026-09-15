<template>
  <v-app :theme="appStore.theme">
    <router-view :key="routeKey" />
    <!-- 全局悬浮「直达底部」按钮：列表内容过高时出现，可拖动/临时关闭 -->
    <scroll-to-bottom-fab />
    <v-snackbar-queue v-model="appStore.messages" />
  </v-app>
</template>

<script setup lang="ts" name="App">
import { useAppStore } from "./stores/app";
import { useEnumsStore } from "./stores/enums";
import { useUserStore } from "./stores/user";
import { mitter } from "@/utils/mitter";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();
const routeKey = computed(() => route.params.language);

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
