<template>
  <central-layout>
    <template #prepend>
      <app-header />
    </template>
    <template #body>
      <!-- 功能不可用说明页：由路由守卫在进入已关闭功能的页面时统一重定向至此。
           说明文案（不可用原因）与开通入口都取自单一数据来源：原因由后端按语言下发
           （features 接口的 reason），开通入口由侧边栏导航声明的 activationRoute 解析，
           本页不自行声明任何功能说明或权限字面量。 -->
      <v-empty-state
        icon="mdi-lock-outline"
        color="warning"
        :title="title"
        :text="reason || $t('page.featureUnavailable.hint')"
      >
        <template #actions>
          <v-btn v-if="activation" color="primary" variant="flat" :to="activationTo">
            {{ $t("page.featureUnavailable.activate", { page: $t(activation.title) }) }}
          </v-btn>
          <v-btn variant="text" @click="goBackPrevious">
            {{ $t("action.backPrevious") }}
          </v-btn>
          <v-btn variant="text" :to="{ name: 'Home', params: { language: appStore.language } }">
            {{ $t("action.backHome") }}
          </v-btn>
        </template>
      </v-empty-state>
      <!-- 功能有开通入口但当前用户无开通权限：不再引导其前往无权访问的页面，改为提示联系管理员 -->
      <p v-if="needsAdmin" class="text-body-medium on-surface-variant text-center mt-4">
        {{ $t("page.featureUnavailable.contactAdmin") }}
      </p>
    </template>
  </central-layout>
</template>

<route lang="yaml">
name: Feature Unavailable
meta:
  title: title.featureUnavailable
  requiresAuth: true
  requiresPerm: false
</route>

<script setup lang="ts" name="feature-unavailable">
import { featureGroupOf, navChildByRoute } from "@/navigation";
import router from "@/router";
import { useAppStore } from "@/stores/app";
import { useFeaturesStore } from "@/stores/features";
import { useUserStore } from "@/stores/user";
import i18n from "@/utils/i18n";

const route = useRoute();
const appStore = useAppStore();
const userStore = useUserStore();
const featuresStore = useFeaturesStore();

// featureKey 功能键：由路由守卫重定向时写入 query；直接收藏访问该页时可能缺失
const featureKey = computed(() => String(route.query.feature ?? ""));

// group 功能展示名与开通入口取侧边栏导航的同一份声明（navigationGroups），
// 使侧边栏与说明页对同一功能只有一处标题/权限声明
const group = computed(() => (featureKey.value ? featureGroupOf(featureKey.value) : undefined));

// featureName 功能展示名：导航分组标题即为该功能的展示名
const featureName = computed(() => (group.value ? i18n.global.t(group.value.title) : ""));

// title 说明页标题：带功能名的针对性标题，功能键缺失时回退通用标题
const title = computed(() =>
  featureName.value
    ? i18n.global.t("page.featureUnavailable.title", { feature: featureName.value })
    : i18n.global.t("title.featureUnavailable"),
);

// reason 不可用原因：后端按请求语言下发（如「档位未绑定模型」），未提供时模板回退通用提示
const reason = computed(() => featuresStore.reasonOf(featureKey.value));

// activation 开通入口：功能声明的开通页面须在当前用户具备其权限点时才引导，
// 否则不展示入口（由 needsAdmin 分支提示联系管理员）
const activation = computed(() => {
  const routeName = group.value?.activationRoute;
  if (!routeName) {
    return undefined;
  }
  const child = navChildByRoute(routeName);
  if (!child || (child.perm && !userStore.can(child.perm.obj, child.perm.act))) {
    return undefined;
  }
  return child;
});

// needsAdmin 功能有页面级开通入口但当前用户无权开通：须由管理员开通
const needsAdmin = computed(() => !!group.value?.activationRoute && !activation.value);

// activationTo 开通入口的路由目标：合并共享的语言段参数与入口自身参数
const activationTo = computed(() => ({
  // 开通入口路由名由导航声明动态解析，类型不满足 RouteLocationRaw 的具名路由字面量联合
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  name: activation.value?.routeName as any,
  params: { language: appStore.language, ...activation.value?.params },
}));

// goBackPrevious 返回上一级：回到进入本页前所处的页面（守卫以 push 重定向至此，
// 故该历史记录即用户点击功能入口时所在的页面）。
// 直接用 URL 打开本页时无历史可回退，此时退化为回到首页，
// 避免按钮点击后无响应或把用户带出应用。
const goBackPrevious = () => {
  const state = window.history.state as { back?: string | null } | null;
  if (state?.back) {
    router.back();
    return;
  }
  router.push({ name: "Home", params: { language: appStore.language } });
};
</script>
