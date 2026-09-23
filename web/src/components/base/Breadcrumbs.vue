<template>
  <v-breadcrumbs
    :items="crumbs"
    :divider="'/'"
    density="compact"
    class="px-0 mb-1 text-caption zk-breadcrumbs"
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
      // 仅携带语言参数，避免透传 route.params 中与目标路由无关的多余参数
      // （如流量页的 type）被 vue-router 丢弃并产生告警
      to: { name: "Home", params: { language: route.params.language } },
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
