<template>
  <v-app-bar elevation="0" class="app-header">
    <template #prepend>
      <v-btn variant="text" @click="gotoHome">
        <v-img src="@/assets/logo.png" size="xs" cover absolute round />
      </v-btn>
      <v-menu transition="scale-transition">
        <template #activator="{ props }">
          <v-btn
            v-show="userStore.valid"
            v-bind="props"
            variant="text"
            append-icon="mdi-chevron-down"
            @click="refreshAvailableNamespace"
          >
            {{ appStore.namespace }}
          </v-btn>
        </template>
        <v-card class="mx-auto">
          <v-list density="compact">
            <v-list-item
              v-for="(namespace, index) in avaliableNamespaces"
              :key="index"
              :disabled="namespace == appStore.namespace"
              @click="appStore.asNamespace(namespace as string)"
            >
              <v-list-item-title>{{ namespace }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </template>
    <template #append>
      <v-menu transition="scale-transition">
        <template #activator="{ props }">
          <v-btn v-bind="props" variant="text" prepend-icon="mdi-translate">
            {{ $t("action.language") }}
          </v-btn>
        </template>
        <v-card class="mx-auto">
          <v-list density="compact">
            <v-list-item
              v-for="item in SUPPORT_LANGUAGES"
              v-show="item.value !== appStore.language"
              :key="item.value"
              @click="asLanguagePath($route, item.value)"
            >
              <v-list-item-title>{{ item.title }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
      <v-btn
        v-model="appStore.theme"
        class="mr-2"
        variant="text"
        :icon="appStore.theme === DARK_THEME ? 'mdi-weather-night' : 'mdi-weather-sunny'"
        @click="appStore.toggleTheme"
      />
      <v-btn
        v-show="!userStore.valid"
        class="mr-2"
        variant="outlined"
        color="primary"
        @click="gotoSignUp"
      >
        {{ $t("action.signUp") }}
      </v-btn>
      <v-btn
        v-show="!userStore.valid && !loginCurrent"
        class="mr-2"
        variant="text"
        @click="gotoSignIn"
      >
        {{ $t("action.signIn") }}
      </v-btn>
      <v-menu>
        <template #activator="{ props }">
          <v-btn
            v-show="userStore.valid"
            icon="mdi-dots-vertical"
            variant="text"
            v-bind="props"
          ></v-btn>
        </template>
        <v-list :lines="false" density="compact" nav>
          <v-list-item v-show="!profileCurrent" @click="gotoProfile">
            <template #prepend>
              <v-icon icon="mdi-account" />
            </template>
            <v-list-item-title>{{ $t("action.profile") }}</v-list-item-title>
          </v-list-item>
          <v-list-item @click="signOut">
            <template #prepend>
              <v-icon icon="mdi-logout" />
            </template>
            <v-list-item-title>{{ $t("action.signOut") }}</v-list-item-title>
          </v-list-item>
        </v-list>
      </v-menu>
    </template>
  </v-app-bar>
</template>

<script setup lang="ts" name="app-header">
import { DARK_THEME } from "@/stores/local";
import { useAppStore } from "@/stores/app";
import { useConfigSyncStore } from "@/stores/configSync";
import router from "@/router";
import { useUserStore } from "@/stores/user";
import { SUPPORT_LANGUAGES } from "@/utils/options";
import i18n from "@/utils/i18n";
import { asLanguagePath } from "@/router";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();

const avaliableNamespaces = ref([] as string[]);

const loginCurrent = computed(() => {
  return route.name === "Sign In";
});

const profileCurrent = computed(() => {
  return route.name === "Profile";
});

const refreshAvailableNamespace = async () => {
  avaliableNamespaces.value = await userStore.getAvailableNamespaces();
};

const gotoHome = () => {
  // route.params type does not satisfy RouteLocationRaw.params
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: "Home", params: route.params as any });
};

const gotoProfile = () => {
  // route.params type does not satisfy RouteLocationRaw.params
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: "Profile", params: route.params as any });
};

const gotoSignUp = () => {
  // route.params type does not satisfy RouteLocationRaw.params
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  router.push({ name: "Sign Up", params: route.params as any });
};

const gotoSignIn = () => {
  // 显式携带当前语言参数，避免透传 route.params 中与目标路由无关的
  // 多余参数（如流量页的 type）被 vue-router 丢弃并产生告警
  router.push({ name: "Sign In", params: { language: appStore.language } });
};

const signOut = () => {
  try {
    userStore.signOut();
    useConfigSyncStore().reset();
  } finally {
    // 无论状态清理是否异常，退出后都必须离开当前页面
    gotoSignIn();
  }
  appStore.notify({
    text: i18n.global.t("notice.goodBye"),
    timeout: 3000,
    color: "success",
  });
};
</script>

<style scoped>
/* Hairline bottom border separating the app bar from page content. */
.app-header {
  border-bottom: 1px solid rgb(var(--v-theme-on-surface), 0.12);
}
</style>
