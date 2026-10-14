<template>
  <v-layout>
    <app-header />
    <v-navigation-drawer
      v-model:rail="rail"
      :expand-on-hover="expandOnHover"
      permanent
      width="220"
      class="zk-hairline-right"
      @mouseenter="drawerHovered = true"
      @mouseleave="onDrawerLeave"
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
        <template v-for="(parent, index) in visibleNavigationGroups" :key="parent.value">
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
                <!--
                  父级标识跟随全部子项的标识语义：全部子项功能不可用 → 锁定标识，全部子项受
                  命名空间影响 → 命名空间标识（两者同时命中时锁定优先，见 navBadgeOf）。
                  声明 #append 会接管 Vuetify 默认注入的展开箭头，故此处以不指定 icon 的 v-icon
                  继承父级 VListGroup 经 defaults 注入的 appendIcon（即展开/收起箭头），
                  避免重复声明箭头图标及其展开态映射
                -->
                <template #append>
                  <nav-badge :kind="groupBadge(parent)" />
                  <v-icon />
                </template>
              </v-list-item>
            </template>
            <v-list-item
              v-for="(child, cindex) in parent.childrens"
              :key="`${index}-${cindex}`"
              :title="$t(child.title)"
              :active="isChildActive(child)"
              color="primary"
              variant="tonal"
              class="nav-child"
              nav
              :to="childTo(child)"
            >
              <!-- 功能未开启的子项不隐藏，而以次级文本色置为不可用态；点击进入该页面后由
                   路由守卫统一引导到「功能不可用」说明页 -->
              <template v-if="isChildUnavailable(parent, child)" #title>
                <span class="on-surface-variant">{{ $t(child.title) }}</span>
              </template>
              <!-- 子项标识：功能不可用（锁定）优先于受命名空间影响，两者同时命中时只展示锁定 -->
              <template v-if="childBadge(parent, child)" #append>
                <nav-badge :kind="childBadge(parent, child)" />
              </template>
            </v-list-item>
          </v-list-group>
        </template>
      </v-list>
    </v-navigation-drawer>
    <v-main class="d-flex mb-6">
      <v-container class="px-4 px-md-6 pt-0 pb-4 pb-md-6 zk-page-container">
        <slot name="prepend" />
        <!--
          页面标题以面包屑形式展示：首页可点击返回，导航树匹配到的祖先层级
          （如流量审计/管理）暂为纯文本，未来多级路径加深时自动跟随导航树增深。
          容器显式 pt-0：Vuetify .v-container 默认带 16px 全向内边距，不清零顶部会使
          面包屑上距被顶到 24px、下距仅 8px 而显得偏低；清零后顶隙完全由面包屑自身的
          上下对称内边距承担，使其垂直居中于 AppBar 与内容表面之间、不再单独占据一行。
        -->
        <breadcrumbs />
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
import { filterNavigationByPerm, navBadgeOf, navigationGroups } from "@/navigation";
import type { NavBadgeKind, NavChild, NavGroup } from "@/navigation";
import { useConfigSyncStore } from "@/stores/configSync";
import { useFeaturesStore } from "@/stores/features";
import { localManager } from "@/stores/local";
import { useUserStore } from "@/stores/user";

const route = useRoute();
const featuresStore = useFeaturesStore();

// 按权限点过滤导航组：子项需满足其声明的权限点，分组内子项全部隐藏时该分组一并隐藏。
// 功能开关不参与过滤：功能关闭的入口仍展示在侧边栏，仅置为不可用态（见 isChildUnavailable）。
const visibleNavigationGroups = computed(() => {
  const userStore = useUserStore();
  return filterNavigationByPerm(navigationGroups, (perm) => userStore.can(perm.obj, perm.act));
});

// childFeature 子项自身声明的 feature 优先，否则继承所属分组（如 AI 分组下的子项）
const childFeature = (parent: NavGroup, child: NavChild): string | undefined =>
  child.feature ?? parent.feature;

// isChildUnavailable 功能关闭的入口不隐藏，而是置为不可用态；点击后由路由守卫
// 统一引导到「功能不可用」说明页
const isChildUnavailable = (parent: NavGroup, child: NavChild): boolean => {
  const key = childFeature(parent, child);
  return !!key && !featuresStore.isEnable(key);
};

// childBadge 子项标识：功能不可用优先于受命名空间影响（规则见 navBadgeOf）
const childBadge = (parent: NavGroup, child: NavChild): NavBadgeKind | undefined =>
  navBadgeOf(isChildUnavailable(parent, child), !!child.namespaceScoped);

// groupBadge 父级标识：仅在**全部**子项命中同一语义时才跟随展示——部分子项不可用、或部分子项
// 与命名空间无关时，父级标识会以偏概全，故此时不展示。判定基于该分组当前可见（已按权限过滤）
// 的子项，即用户实际看到的子项；两类语义同时命中时锁定优先（与子项共用 navBadgeOf）
const groupBadge = (parent: NavGroup): NavBadgeKind | undefined =>
  navBadgeOf(
    parent.childrens.every((child) => isChildUnavailable(parent, child)),
    parent.childrens.every((child) => !!child.namespaceScoped),
  );

const props = defineProps({
  currentNav: {
    type: String,
    required: true,
  },
});

// 子项路由目标：合并共享的语言段参数与子项自身参数（如流量模型 type）。
// 不保留当前路由的其它参数（如 /traffic/:type 的 type），避免把不匹配的参数带入
// 目标路由触发 Vue Router "Discarded invalid param(s)" 告警。
const childTo = (child: NavChild) => {
  // 导航声明的路由名为宽泛字符串，类型不满足 RouteLocationRaw 的具名路由字面量联合

  return {
    name: child.routeName as any,
    params: { language: route.params.language, ...child.params },
  };
};

// 子项激活判断：路由名一致且参数一致（同名路由的不同参数子项互斥高亮）
const isChildActive = (child: NavChild): boolean => {
  if (route.name !== child.routeName) {
    return false;
  }
  return Object.entries(child.params ?? {}).every(([key, value]) => {
    return route.params[key] === value;
  });
};

// 固定展开状态：固定时禁用 hover 展开并保持展开，初始值从本地偏好恢复。
const pinned = ref(localManager.getNavPinned());
// 是否将导航抽屉收起为窄栏（悬停时展开）。
const rail = ref(!pinned.value);

// 指针是否位于抽屉内：由抽屉根元素的 mouseenter / mouseleave 维护，用于判断
// 「取消固定」的瞬间抽屉是否正被悬停撑开。
const drawerHovered = ref(false);

// 悬停展开是否已交还给抽屉。取消固定的瞬间指针仍悬停在抽屉内时，Vuetify 会按
// 「已悬停」把抽屉继续渲染成展开宽度（220px），而布局已按 rail 收窄到 56px——
// 于是抽屉覆盖在刚刚展开的主体内容之上。故此刻先暂停悬停展开让抽屉与布局一起
// 收窄，待指针离开抽屉后再交还（见 onDrawerLeave）。
const hoverExpandEnabled = ref(true);

// 仅在未固定、且悬停展开已交还时启用 expand-on-hover
const expandOnHover = computed(() => !pinned.value && hoverExpandEnabled.value);

// 切换固定展开：固定时展开抽屉，取消固定时恢复 rail 收起模式，并持久化偏好
const togglePinned = () => {
  pinned.value = !pinned.value;
  localManager.setNavPinned(pinned.value);
  rail.value = !pinned.value;
  // 取消固定且指针仍在抽屉内时，暂停悬停展开（固定时不生效，故无需区分）
  hoverExpandEnabled.value = pinned.value || !drawerHovered.value;
  // 页面配置变更：通知前端配置同步 store 防抖上传
  useConfigSyncStore().notifyLocalChange();
};

// 指针离开抽屉：交还悬停展开，之后抽屉按悬停正常展开
const onDrawerLeave = () => {
  drawerHovered.value = false;
  hoverExpandEnabled.value = true;
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

// 路由变化时同时展开对应一级菜单，保证"当前子页面对应的一级导航栏打开"
watch(
  () => route.name,
  (name) => {
    const parent = navigationGroups.find((group) =>
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
/* 将子项缩进到其父分组标题之下。 */
.nav-child {
  margin-left: var(--zk-space-4);
}

/* 加速 v-list-group 展开/收起高度动画（Vuetify 默认 0.3s → 0.15s） */
/* stylelint-disable-next-line selector-class-pattern -- Vuetify 内部 BEM 类名 */
:deep(.v-list-group__items) {
  transition-duration: var(--zk-transition-fast) !important;
}
</style>
