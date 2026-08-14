<template>
  <v-layout>
    <app-header />
    <v-navigation-drawer
      v-model:rail="rail"
      :expand-on-hover="!pinned"
      permanent
      width="220"
      class="nav-drawer"
    >
      <!-- 固定展开切换：点击后侧边栏保持展开，不再随鼠标悬停收起；偏好持久化到本地 -->
      <template #append>
        <div class="d-flex justify-center py-2">
          <v-btn
            :icon="pinned ? 'mdi-pin' : 'mdi-pin-outline'"
            :color="pinned ? 'primary' : undefined"
            :title="$t(pinned ? 'navbar.unpin' : 'navbar.pin')"
            :aria-label="$t(pinned ? 'navbar.unpin' : 'navbar.pin')"
            variant="text"
            @click="togglePinned"
          />
        </div>
      </template>
      <v-list :opened="openedNav" nav open-strategy="single" @update:opened="onUpdateOpened">
        <!--
          v-list-group 自带展开/收起高度过渡动画；
          open-strategy="single" 保证手风琴互斥（同一时间仅一个分组展开）；
          opened 记录展开分组，路由变化时由 watch 自动展开当前页面所属分组
        -->
        <template v-for="(parent, index) in navigation" :key="parent.value">
          <v-list-group :value="parent.value" color="primary">
            <template #activator="{ props: groupProps }">
              <v-list-item
                v-bind="groupProps"
                :title="$t(parent.title)"
                :active="mainNav === parent.value"
                color="primary"
                nav
              >
                <template #prepend>
                  <v-icon>{{ parent.icon }}</v-icon>
                </template>
              </v-list-item>
            </template>
            <v-list-item
              v-for="(child, cindex) in parent.childrens"
              :key="`${index}-${cindex}`"
              :title="$t(child.title)"
              :active="route.name === child.routeName"
              color="primary"
              variant="tonal"
              class="nav-child"
              nav
              :to="{ name: child.routeName, params: route.params }"
            />
          </v-list-group>
        </template>
      </v-list>
    </v-navigation-drawer>
    <v-main class="d-flex mb-6">
      <v-container class="pa-4 pa-md-6" style="max-width: 1440px">
        <slot name="prepend" />
        <!--
          v-sheet 圆角与全局 .v-card 的 12px 圆角对齐（见 main.scss），
          避免直角容器内嵌圆角卡片时四角出现月牙形色差空缺
        -->
        <v-sheet rounded="lg">
          <slot name="body" />
        </v-sheet>
        <slot name="append" />
      </v-container>
    </v-main>
  </v-layout>
</template>

<script setup lang="ts" name="main-layout">
import { localManager } from "@/stores/local";

const route = useRoute();

const props = defineProps({
  currentNav: {
    type: String,
    required: true,
  },
});

// 固定展开状态：固定时禁用 hover 展开并保持展开，初始值从本地偏好恢复。
const pinned = ref(localManager.getNavPinned());
// Whether the navigation drawer is collapsed into a rail (expands on hover).
const rail = ref(!pinned.value);

// 切换固定展开：固定时展开抽屉，取消固定时恢复 rail 收起模式，并持久化偏好
const togglePinned = () => {
  pinned.value = !pinned.value;
  localManager.setNavPinned(pinned.value);
  rail.value = !pinned.value;
};

const mainNav = ref(props.currentNav);
// 手风琴展开分组：open-strategy="single" 保证互斥，默认仅当前页面所属一级菜单展开
const openedNav = ref<(string | number)[]>([props.currentNav]);

// 拦截展开状态更新：rail 收起模式下保持分组全部收起。
// Vuetify 的 handleActiveLink 会在 active 子项挂载时自动展开父分组
// （v-list-group__items 在 56px 窄栏中溢出），此处统一在事件层拦截复位
const onUpdateOpened = (value: (string | number)[]) => {
  openedNav.value = rail.value ? [] : value;
};

// rail 收起模式下收起所有分组，避免子项在 56px 窄栏中溢出；
// 抽屉展开（hover / pin 固定）时恢复当前页面所属分组
watch(
  rail,
  (isRail) => {
    if (isRail) {
      openedNav.value = [];
    } else if (openedNav.value.length === 0) {
      openedNav.value = [mainNav.value];
    }
  },
  { immediate: true },
);

const navigation = ref([
  {
    title: "navbar.etl.self",
    icon: "mdi-timetable",
    value: "etl",
    childrens: [
      {
        title: "navbar.etl.server",
        routeName: "ETL Server List",
      },
      {
        title: "navbar.etl.task",
        routeName: "ETL Task List",
      },
    ],
  },
  {
    title: "navbar.admin.self",
    icon: "mdi-wrench",
    value: "admin",
    childrens: [
      {
        title: "navbar.admin.user",
        routeName: "Admin User List",
      },
      {
        title: "navbar.admin.group",
        routeName: "Admin Group List",
      },
      {
        title: "navbar.admin.namespace",
        routeName: "Admin Namespace List",
      },
      {
        title: "navbar.admin.capturer",
        routeName: "Admin Capturer List",
      },
      {
        title: "navbar.admin.tagRule",
        routeName: "Admin Tag Rule List",
      },
      {
        title: "navbar.admin.tagEventRule",
        routeName: "Admin Tag Event Rule List",
      },
    ],
  },
]);

// Keep the active group heading in sync with the current route so the parent
// highlight stays consistent after navigating through a child item.
// 路由变化时同时展开对应一级菜单，保证"当前子页面对应的一级导航栏打开"
watch(
  () => route.name,
  (name) => {
    const parent = navigation.value.find((group) =>
      group.childrens.some((child) => child.routeName === String(name)),
    );
    if (parent) {
      mainNav.value = parent.value;
      // rail 收起模式下保持分组收起（避免子项在 56px 窄栏溢出），
      // 仅展开模式（pin 固定 / hover 展开）下自动展开当前页面所属分组
      if (!rail.value) {
        openedNav.value = [parent.value];
      }
    }
  },
  { immediate: true },
);
</script>

<style scoped>
/* Hairline right border separating the drawer from page content. */
.nav-drawer {
  border-right: 1px solid rgb(var(--v-theme-on-surface), 0.12);
}

/* Indent child items under their parent group heading. */
.nav-child {
  margin-left: 16px;
}

/* 加速 v-list-group 展开/收起高度动画（Vuetify 默认 0.3s → 0.15s） */
/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部 BEM 类名 */
:deep(.v-list-group__items) {
  transition-duration: 0.15s !important;
}
</style>
