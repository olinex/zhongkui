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

const route = useRoute();
const appStore = useAppStore();
const SIGN_IN_TIMEOUT_MS = 3000;
const PROCESS_STEP_INTERVAL_MS = 1000;
const process = ref(0);
const release = ref(SIGN_IN_TIMEOUT_MS / PROCESS_STEP_INTERVAL_MS);
const timer = ref(null as NodeJS.Timeout | null);

onMounted(() => {
  setInterval(() => {
    process.value += (PROCESS_STEP_INTERVAL_MS / SIGN_IN_TIMEOUT_MS) * 100;
    release.value -= PROCESS_STEP_INTERVAL_MS / 1000;
  }, PROCESS_STEP_INTERVAL_MS);
  timer.value = setTimeout(
    () =>
      router.replace({
        name: "Sign In",
        params: {
          language: appStore.language,
        },
        query: {
          redirect: route.query.redirect || "",
        },
      }),
    SIGN_IN_TIMEOUT_MS
  );
});

onUnmounted(() => {
  if (timer.value) {
    clearTimeout(timer.value);
    timer.value = null;
  }
});
</script>
