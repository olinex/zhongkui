<template></template>

<route lang="yaml">
name: "404 Wide Match"
meta:
  requiresAuth: false
  requiresPerm: false
</route>

<script setup lang="ts">
import router from "@/router";
import { useAppStore } from "@/stores/app";
import consola from "consola";

onBeforeMount(() => {
  const route = useRoute();
  const appStore = useAppStore();
  consola.warn(route.path);
  router.replace({
    name: "404",
    params: {
      language: appStore.language,
    },
    query: {
      redirect: route.fullPath,
    },
  });
});
</script>
