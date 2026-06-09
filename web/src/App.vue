<template>
  <v-app :theme="appStore.theme">
    <router-view :key="routeKey" />
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
