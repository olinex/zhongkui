<template>
  <v-breadcrumbs
    :items="crumbs"
    :divider="'/'"
    density="compact"
    class="px-0 mb-1 text-caption breadcrumbs"
  />
</template>

<script setup lang="ts" name="breadcrumbs">
import type { RouteLocationRaw } from "vue-router";
import { navigationGroups } from "@/navigation";
import type { NavChild, NavGroup } from "@/navigation";
import i18n from "@/utils/i18n";

interface BreadcrumbItem {
  title: string;
  to?: RouteLocationRaw;
  disabled?: boolean;
}

const route = useRoute();

// 子项参数匹配：导航项配置参数须与当前路由参数一致（同名路由不同参数子项互斥）
const matchesParams = (
  expected?: Record<string, string>,
  actual: Record<string, unknown> = {},
): boolean => {
  return Object.entries(expected ?? {}).every(([key, value]) => actual[key] === value);
};

// 递归遍历导航树，返回从根到当前路由匹配节点的标题键路径；
// 未来导航树加深层级（多级路径）时无需改动，面包屑自动随之增深
const findNavPath = (
  nodes: Array<NavGroup | NavChild>,
  routeName: string,
  params: Record<string, unknown>,
): string[] => {
  for (const node of nodes) {
    if ("routeName" in node && node.routeName === routeName && matchesParams(node.params, params)) {
      return [node.title];
    }
    if ("childrens" in node) {
      const sub = findNavPath(node.childrens, routeName, params);
      if (sub.length > 0) {
        return [node.title, ...sub];
      }
    }
  }
  return [];
};

// 面包屑路径：首页（可点击返回）+ 导航树匹配到的祖先层级（当前无落地页的纯文本）；
// 未命中导航树的页面（如个人资料）回退使用路由 meta.title 作为当前页标题
const crumbs = computed<BreadcrumbItem[]>(() => {
  const items: BreadcrumbItem[] = [
    {
      title: i18n.global.t("title.home"),
      to: { name: "Home", params: { ...route.params } },
    },
  ];
  const path = findNavPath(navigationGroups, String(route.name), route.params);
  if (path.length > 0) {
    path.forEach((titleKey) => {
      items.push({ title: i18n.global.t(titleKey) });
    });
  } else if (route.meta.title) {
    items.push({ title: i18n.global.t(String(route.meta.title)) });
  }
  // 当前页为最后一项，标记禁用态不可再点击
  items[items.length - 1].disabled = true;
  return items;
});
</script>

<style scoped>
/* 标题栏紧凑化：进一步收窄面包屑上下内边距，分隔符左右间距同步收紧；
   默认（不可点击的中间层级）弱化显示，与可点击项形成对比 */
.breadcrumbs {
  color: rgb(var(--v-theme-on-surface-variant));
  padding-top: 4px;
  padding-bottom: 4px;
}

/* 可点击项（如首页/上级页面）以主色高亮，用户一眼即可识别可跳转 */
/* stylelint-disable-next-line selector-class-pattern */
.breadcrumbs :deep(.v-breadcrumbs-item--link) {
  color: rgb(var(--v-theme-primary));
}

.breadcrumbs :deep(.v-breadcrumbs-divider) {
  padding-left: 6px;
  padding-right: 6px;
}
</style>
