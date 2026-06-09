<template>
  <v-app-bar color="grey-darken-3">
    <template #prepend>
      <v-btn @click="gotoHome">
        <v-img src="@/assets/logo.png" size="xs" cover absolute round />
      </v-btn>
      <v-menu transition="scale-transition">
        <template #activator="{ props }">
          <v-btn
            v-show="userStore.valid"
            v-bind="props"
            @click="refreshAvailableNamespace"
          >
            {{ appStore.namespace }}
          </v-btn>
        </template>
        <v-card class="mx-auto">
          <v-list density="compact">
            <v-list-item
              v-for="(namespace, index) in avaliableNamespaces"
              @click="appStore.asNamespace(namespace as string)"
              :disabled="namespace == appStore.namespace"
              :key="index"
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
          <v-btn v-bind="props">
            {{ $t("action.language") }}
          </v-btn>
        </template>
        <v-card class="mx-auto">
          <v-list density="compact">
            <v-list-item
              v-for="item in SUPPORT_LANGUAGES"
              v-show="item.value !== appStore.language"
              @click="asLanguagePath($route, item.value)"
              :key="item.value"
            >
              <v-list-item-title>{{ item.title }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
      <v-btn
        class="mr-2"
        v-model="appStore.theme"
        @click="appStore.toggleTheme"
        :icon="
          appStore.theme === DARK_THEME
            ? 'mdi-weather-night'
            : 'mdi-weather-sunny'
        "
      />
      <v-btn
        class="mr-2"
        v-show="!userStore.valid"
        @click="gotoSignUp"
        variant="outlined"
        >{{ $t("action.signUp") }}</v-btn
      >
      <v-btn
        class="mr-2"
        v-show="!userStore.valid && !loginCurrent"
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
  router.push({ name: "Home", params: route.params as any });
};

const gotoProfile = () => {
  router.push({ name: "Profile", params: route.params as any });
};

const gotoSignUp = () => {
  router.push({ name: "Sign Up", params: route.params as any });
};

const gotoSignIn = () => {
  router.push({ name: "Sign In", params: route.params as any });
};

const signOut = () => {
  userStore.signOut();
  gotoSignIn();
  appStore.notify({
    text: i18n.global.t("notice.goodBye"),
    timeout: 3000,
    color: "success",
  });
};
</script>
