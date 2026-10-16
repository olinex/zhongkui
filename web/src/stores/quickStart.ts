// stores/quickStart.ts
// 快速开始清单 store：用户自定义的侧边栏导航链接分组，持久化到 localStorage
// （按浏览器维度，遵循 localManager）。

import { defineStore } from "pinia";

import { navigationGroups } from "@/navigation";
import type { NavChild } from "@/navigation";
import { useConfigSyncStore } from "./configSync";
import { localManager } from "./local";

// QuickStartLink 快速开始中的单个导航链接。
export interface QuickStartLink {
  title: string;
  icon: string;
  routeName: string;
  params?: Record<string, string>;
}

// QuickStartGroup 快速开始中的一组导航链接。
export interface QuickStartGroup {
  id: string;
  title: string;
  links: QuickStartLink[];
  // builtin 内置分组（默认分组）标记：不可删除
  builtin?: boolean;
}

// isBuiltinGroup 判断是否为内置分组；兼容旧数据（内置分组 id 恒为 "default"）
export const isBuiltinGroup = (group: Pick<QuickStartGroup, "id" | "builtin">): boolean => {
  return group.builtin === true || group.id === "default";
};

// linkKey 为导航链接构造稳定标识（路由名 + 参数），用于去重与作为 Vue 列表 key。
export const linkKey = (link: Pick<QuickStartLink, "routeName" | "params">): string => {
  return `${link.routeName}:${JSON.stringify(link.params ?? {})}`;
};

// seedDefaultGroups 初始化默认分组：包含全部侧边栏导航入口
const seedDefaultGroups = (): QuickStartGroup[] => {
  return [
    {
      id: "default",
      title: "home.quickStart.defaultGroup",
      builtin: true,
      links: navigationGroups.flatMap((group) => group.childrens),
    },
  ];
};

// legacyNavRemap 旧导航键 → 新导航键的迁移映射。
// 标签规则/标签事件规则经「管理 → 流量审计 → 标签事件」两次迁移，旧键已从语言包移除，
// 持久化的快速开始配置需就地改写，避免渲染裸键；幂等可重复应用。
const legacyNavRemap: Record<string, string | undefined> = {
  "navbar.admin.tagRule": "navbar.tagEvent.tagRule",
  "navbar.admin.tagEventRule": "navbar.tagEvent.tagEventRule",
  "navbar.traffic.tagRule": "navbar.tagEvent.tagRule",
  "navbar.traffic.tagEventRule": "navbar.tagEvent.tagEventRule",
};

// normalizeStoredGroups 迁移旧版本持久化配置：改写已迁移的导航键与路由名（幂等）。
const normalizeStoredGroups = (groups: QuickStartGroup[]): QuickStartGroup[] => {
  return groups.map((group) => ({
    ...group,
    links: group.links.map((link) => {
      const title = legacyNavRemap[link.title] ?? link.title;
      const routeName =
        link.routeName === "Admin Tag Rule List" || link.routeName === "Traffic Tag Rule List"
          ? "Tag Rule List"
          : link.routeName === "Admin Tag Event Rule List" ||
              link.routeName === "Traffic Tag Event Rule List"
            ? "Tag Event Rule List"
            : link.routeName;
      return { ...link, title, routeName };
    }),
  }));
};

// parseStoredGroups 解析本地存储的分组配置，损坏数据时回退默认分组；
// 解析后经 normalizeStoredGroups 迁移旧键，保证渲染与导航一致。
const parseStoredGroups = (raw: string | null): QuickStartGroup[] | null => {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) {
      return null;
    }
    return normalizeStoredGroups(parsed as QuickStartGroup[]);
  } catch {
    return null;
  }
};

// useQuickStartStore 快速开始清单 store：管理分组及其下导航链接的增删改与持久化。
export const useQuickStartStore = defineStore("quickStart", {
  state: () => ({
    groups: parseStoredGroups(localManager.getQuickStartGroups()) ?? seedDefaultGroups(),
  }),
  actions: {
    // reload 从本地存储重新加载分组配置；供前端配置同步应用远端配置后刷新
    reload() {
      this.groups = parseStoredGroups(localManager.getQuickStartGroups()) ?? seedDefaultGroups();
    },
    persist() {
      localManager.setQuickStartGroups(JSON.stringify(this.groups));
      useConfigSyncStore().notifyLocalChange();
    },
    addGroup(title: string): string {
      const id = crypto.randomUUID();
      this.groups.push({ id, title, links: [] });
      this.persist();
      return id;
    },
    renameGroup(id: string, title: string) {
      const group = this.groups.find((g) => g.id === id);
      if (group) {
        group.title = title;
        this.persist();
      }
    },
    removeGroup(id: string) {
      const group = this.groups.find((g) => g.id === id);
      // 内置（默认）分组不可删除
      if (!group || isBuiltinGroup(group)) {
        return;
      }
      this.groups = this.groups.filter((g) => g.id !== id);
      this.persist();
    },
    addLink(groupID: string, link: NavChild) {
      const group = this.groups.find((g) => g.id === groupID);
      if (!group) {
        return;
      }
      if (group.links.some((existing) => linkKey(existing) === linkKey(link))) {
        return;
      }
      group.links.push({ ...link });
      this.persist();
    },
    removeLink(groupID: string, index: number) {
      const group = this.groups.find((g) => g.id === groupID);
      if (group) {
        group.links.splice(index, 1);
        this.persist();
      }
    },
    // moveLink 将链接从源分组移动/排序到目标分组指定位置
    moveLink(fromGroupID: string, fromIndex: number, toGroupID: string, toIndex: number) {
      const fromGroup = this.groups.find((g) => g.id === fromGroupID);
      const toGroup = this.groups.find((g) => g.id === toGroupID);
      if (!fromGroup || !toGroup) {
        return;
      }
      const [link] = fromGroup.links.splice(fromIndex, 1);
      if (!link) {
        return;
      }
      // 同分组内向后移动时，源元素移除后目标索引需前移一位
      if (fromGroupID === toGroupID && fromIndex < toIndex) {
        toIndex -= 1;
      }
      toGroup.links.splice(Math.min(toIndex, toGroup.links.length), 0, link);
      this.persist();
    },
  },
});
