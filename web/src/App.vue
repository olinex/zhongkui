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
import { useUserStore } from "./stores/user";
import { mitter } from "@/utils/mitter";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();
const routeKey = computed(() => route.params.language);

onBeforeMount(() => {
  userStore.refreshEventOn(mitter);
});

onUnmounted(() => {
  userStore.refreshEventOff(mitter);
});
</script>
