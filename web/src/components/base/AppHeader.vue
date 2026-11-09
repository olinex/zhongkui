<template>
  <v-app-bar class="zk-hairline-bottom">
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
            <!-- 命名空间图标用 primary：与导航标识徽章的「命名空间」变体同色（归属用 primary，
                 非状态色），提示此处可切换命名空间 -->
            <template #prepend>
              <v-icon icon="mdi-alpha-n-box-outline" color="primary" />
            </template>
            {{ appStore.namespace }}
          </v-btn>
        </template>
        <v-card class="mx-auto">
          <v-list>
            <v-list-item
              v-for="(namespace, index) in avaliableNamespaces"
              :key="index"
              :disabled="namespace == appStore.namespace"
              @click="onNamespaceChange(namespace as string)"
            >
              <v-list-item-title>{{ namespace }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </template>
    <template #append>
      <v-btn
        variant="text"
        icon="mdi-book-open-page-variant-outline"
        :href="docsURL"
        target="_blank"
        rel="noopener"
        :title="$t('action.docs')"
        :aria-label="$t('action.docs')"
      />
      <v-btn
        v-show="userStore.valid"
        variant="text"
        icon="mdi-robot-outline"
        :title="$t('action.aiAssistant')"
        :aria-label="$t('action.aiAssistant')"
        @click="assistantStore.open()"
      />
      <v-menu transition="scale-transition">
        <template #activator="{ props }">
          <v-btn
            v-bind="props"
            variant="text"
            icon="mdi-translate"
            :title="$t('action.language')"
            :aria-label="$t('action.language')"
          />
        </template>
        <v-card class="mx-auto">
          <v-list>
            <v-list-item
              v-for="item in SUPPORT_LANGUAGES"
              v-show="item.value !== appStore.language"
              :key="item.value"
              @click="asLanguagePath($route, item.value)"
            >
              <v-list-item-title>{{ $t(item.titleKey) }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
      <v-btn
        v-model="appStore.theme"
        variant="text"
        :icon="appStore.theme === DARK_THEME ? 'mdi-weather-night' : 'mdi-weather-sunny'"
        :title="$t('action.toggleTheme')"
        :aria-label="$t('action.toggleTheme')"
        @click="appStore.toggleTheme"
      />
      <v-btn
        v-show="!userStore.valid && !loginCurrent"
        variant="outlined"
        color="primary"
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
            :title="$t('action.accountMenu')"
            :aria-label="$t('action.accountMenu')"
            v-bind="props"
          ></v-btn>
        </template>
        <v-list :lines="false" nav>
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
import { useAssistantStore } from "@/stores/assistant";
import { useConfigSyncStore } from "@/stores/configSync";
import router from "@/router";
import { useUserStore } from "@/stores/user";
import { SUPPORT_LANGUAGES } from "@/utils/options";
import i18n from "@/utils/i18n";
import { asLanguagePath } from "@/router";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();
const assistantStore = useAssistantStore();

const avaliableNamespaces = ref([] as string[]);

// 文档站默认与前端同源部署在 /docs/，可通过 VITE_DOCS_URL 覆盖
const docsURL = import.meta.env.VITE_DOCS_URL || "/docs/";

const loginCurrent = computed(() => {
  return route.name === "Sign In";
});

const profileCurrent = computed(() => {
  return route.name === "Profile";
});

const refreshAvailableNamespace = async () => {
  avaliableNamespaces.value = await userStore.getAvailableNamespaces();
};

// onNamespaceChange 切换命名空间：asNamespace 更新 store 与 localStorage，并通知配置同步。
// 已开启配置同步（syncEnabled）时必须先等待远端保存成功再重载页面：
// 否则 reload 会打断 500ms 防抖的异步 push，服务端仍存旧命名空间，
// reload 后 configSync 拉回旧配置覆盖本地 localStorage，导致跳回原命名空间。
const onNamespaceChange = async (namespace: string): Promise<void> => {
  appStore.asNamespace(namespace);
  const configSync = useConfigSyncStore();
  if (configSync.enabled) {
    await configSync.push();
  }
  window.location.reload();
};

const gotoHome = () => {
  // 显式携带当前语言参数，避免透传 route.params 中与目标路由无关的
  // 多余参数（如流量页的 type）被 vue-router 丢弃并产生告警
  router.push({ name: "Home", params: { language: route.params.language } });
};

const gotoProfile = () => {
  // 同上：仅携带语言参数，避免多余参数（如流量页的 type）触发丢弃告警
  router.push({ name: "Profile", params: { language: route.params.language } });
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
