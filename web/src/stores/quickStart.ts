// stores/quickStart.ts
// Quick start checklist store: user-defined groups of sidebar navigation
// links, persisted to localStorage (per browser, following localManager).

import { defineStore } from "pinia";

import { navigationGroups } from "@/navigation";
import type { NavChild } from "@/navigation";
import { useConfigSyncStore } from "./configSync";
import { localManager } from "./local";

export interface QuickStartLink {
  title: string;
  icon: string;
  routeName: string;
  params?: Record<string, string>;
}

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

// linkKey builds a stable identity for a navigation link (route name + params),
// used for de-duplication and as a Vue list key.
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

// parseStoredGroups 解析本地存储的分组配置，损坏数据时回退默认分组
const parseStoredGroups = (raw: string | null): QuickStartGroup[] | null => {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? (parsed as QuickStartGroup[]) : null;
  } catch {
    return null;
  }
};

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
    addLink(groupId: string, link: NavChild) {
      const group = this.groups.find((g) => g.id === groupId);
      if (!group) {
        return;
      }
      if (group.links.some((existing) => linkKey(existing) === linkKey(link))) {
        return;
      }
      group.links.push({ ...link });
      this.persist();
    },
    removeLink(groupId: string, index: number) {
      const group = this.groups.find((g) => g.id === groupId);
      if (group) {
        group.links.splice(index, 1);
        this.persist();
      }
    },
    // moveLink 将链接从源分组移动/排序到目标分组指定位置
    moveLink(fromGroupId: string, fromIndex: number, toGroupId: string, toIndex: number) {
      const fromGroup = this.groups.find((g) => g.id === fromGroupId);
      const toGroup = this.groups.find((g) => g.id === toGroupId);
      if (!fromGroup || !toGroup) {
        return;
      }
      const [link] = fromGroup.links.splice(fromIndex, 1);
      if (!link) {
        return;
      }
      // 同分组内向后移动时，源元素移除后目标索引需前移一位
      if (fromGroupId === toGroupId && fromIndex < toIndex) {
        toIndex -= 1;
      }
      toGroup.links.splice(Math.min(toIndex, toGroup.links.length), 0, link);
      this.persist();
    },
  },
});
