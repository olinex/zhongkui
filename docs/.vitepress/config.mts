import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitepress";
import type { DefaultTheme } from "vitepress";

// 文档站点直接以 docs/ 为根：nav.json 与 index.md 均由 zki create docs 生成，
// 侧边栏只从 nav.json 读取，避免在站点配置里维护第二份导航清单。

// NavItem 对应 nav.json 中的一条文档项。
interface NavItem {
  title: string;
  summary: string;
  output: string;
  category: string;
  source: string;
  roles: string[];
  owner: string;
}

// NavSection 对应 nav.json 中的一个导航分区。
interface NavSection {
  key: string;
  label: string;
  items: NavItem[];
}

// NavFile 是 nav.json 的顶层结构。
interface NavFile {
  generatedBy: string;
  sections: NavSection[];
}

const configDir = path.dirname(fileURLToPath(import.meta.url));
const nav = JSON.parse(
  fs.readFileSync(path.resolve(configDir, "../nav.json"), "utf8"),
) as NavFile;

// linkOf 把 nav.json 中的 docs 相对路径转换为站点路由（去掉 .md 后缀）。
// cleanUrls 下 index.md 对应所在目录的路由，而不是 .../index，否则会 404。
function linkOf(output: string): string {
  const route = output.replace(/\.md$/, "");
  const withSlash = route.startsWith("/") ? route : `/${route}`;
  if (withSlash === "/index") return "/";
  if (withSlash.endsWith("/index")) {
    return withSlash.slice(0, -"/index".length) + "/";
  }
  return withSlash;
}

// sidebarOf 由 nav.json 生成侧边栏：首项固定为总目录，其余按分区分组，
// 参考分区默认折叠，避免上百篇参考页淹没教程与指南。
function sidebarOf(): DefaultTheme.SidebarItem[] {
  const groups: DefaultTheme.SidebarItem[] = nav.sections.map((section) => ({
    text: section.label,
    collapsed: section.key === "reference",
    items: section.items.map((item) => ({
      text: item.title,
      link: linkOf(item.output),
    })),
  }));
  return [{ text: "文档总目录", link: "/" }, ...groups];
}

export default defineConfig({
  lang: "zh-CN",
  title: "zhongkui 文档",
  description: "网络流量采集、标签计算与风险告警平台",
  // 部署到子路径时通过 DOCS_BASE 覆盖，例如 DOCS_BASE=/zhongkui/ make docs-build
  base: process.env.DOCS_BASE || "/",
  cleanUrls: true,
  lastUpdated: true,
  // 模板含占位符与起草须知，只用于写作，不进入站点。
  srcExclude: ["templates/**"],
  themeConfig: {
    nav: [
      { text: "开始使用", link: "/curated/tutorials/quick-start" },
      { text: "核心概念", link: "/curated/explanation/system-overview" },
      { text: "参考", link: "/generated/reference/" },
    ],
    sidebar: sidebarOf(),
    outline: { level: [2, 3], label: "本页导航" },
    search: { provider: "local" },
    editLink: {
      pattern: "https://gitee.com/olinex/zhongkui/edit/master/docs/:path",
      text: "在 Gitee 上编辑此页",
    },
    docFooter: { prev: "上一篇", next: "下一篇" },
    lastUpdatedText: "最后更新",
    returnToTopLabel: "回到顶部",
    sidebarMenuLabel: "目录",
    darkModeSwitchLabel: "主题",
    footer: {
      message: "Apache-2.0 许可",
      copyright: "Copyright 2026 The zhongkui Authors",
    },
  },
});
