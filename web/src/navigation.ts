// navigation.ts
// Shared sidebar navigation configuration (single source of truth for the
// sidebar and the home-page quick start picker).
// Link titles are i18n keys (navbar.*); navigation targets are route name + params.

import { FEATURE_AI } from "@/stores/features";

export interface NavChild {
  title: string;
  icon: string;
  routeName: string;
  params?: Record<string, string>;
  // 是否受命名空间影响：为 true 时在侧边栏子项上叠加 Namespace 标识，
  // 指明该页面随命名空间切换而变化；false 或缺省表示与命名空间无关
  namespaceScoped?: boolean;
  // 所属功能开关键：设置后该子项随对应功能开关（关闭时）一并隐藏
  feature?: string;
}

export interface NavGroup {
  title: string;
  icon: string;
  value: string;
  childrens: NavChild[];
  // 所属功能开关键：设置后该分组随对应功能开关（关闭时）一并隐藏
  feature?: string;
}

export const navigationGroups: NavGroup[] = [
  {
    title: "navbar.tagEvent.self",
    icon: "mdi-tag-multiple",
    value: "tagEvent",
    childrens: [
      {
        title: "navbar.tagEvent.tagRule",
        icon: "mdi-label",
        routeName: "Tag Rule List",
        namespaceScoped: true,
      },
      {
        title: "navbar.tagEvent.tagEventRule",
        icon: "mdi-alert-decagram",
        routeName: "Tag Event Rule List",
        namespaceScoped: true,
      },
      {
        title: "navbar.tagEvent.record",
        icon: "mdi-history",
        routeName: "Tag Event Record List",
        namespaceScoped: true,
      },
    ],
  },
  {
    title: "navbar.traffic.self",
    icon: "mdi-wifi-marker",
    value: "traffic",
    childrens: [
      {
        title: "navbar.traffic.ipv4Tcp",
        icon: "mdi-ip-network",
        routeName: "Traffic List",
        params: { type: "ipv4-tcp" },
        namespaceScoped: true,
      },
      {
        title: "navbar.traffic.ipv6Tcp",
        icon: "mdi-ip-network-outline",
        routeName: "Traffic List",
        params: { type: "ipv6-tcp" },
        namespaceScoped: true,
      },
      {
        title: "navbar.traffic.httpTransport",
        icon: "mdi-web",
        routeName: "Traffic List",
        params: { type: "http-transport" },
        namespaceScoped: true,
      },
    ],
  },
  {
    title: "navbar.notification.self",
    icon: "mdi-bell-outline",
    value: "notification",
    childrens: [
      {
        title: "navbar.notification.channel",
        icon: "mdi-email-multiple",
        routeName: "Notification Channel List",
        namespaceScoped: true,
      },
      {
        title: "navbar.notification.target",
        icon: "mdi-account-arrow-right-outline",
        routeName: "Notification Target List",
        namespaceScoped: true,
      },
      {
        title: "navbar.notification.group",
        icon: "mdi-account-group-outline",
        routeName: "Notification Group List",
        namespaceScoped: true,
      },
      {
        title: "navbar.notification.template",
        icon: "mdi-file-document-edit-outline",
        routeName: "Notification Template List",
        namespaceScoped: true,
      },
      {
        title: "navbar.notification.record",
        icon: "mdi-history",
        routeName: "Notification Record List",
        namespaceScoped: true,
      },
    ],
  },
  {
    title: "navbar.etl.self",
    icon: "mdi-timetable",
    value: "etl",
    childrens: [
      {
        title: "navbar.etl.server",
        icon: "mdi-server",
        routeName: "ETL Server List",
      },
      {
        title: "navbar.etl.queue",
        icon: "mdi-timeline",
        routeName: "ETL Queue List",
      },
      {
        title: "navbar.etl.task",
        icon: "mdi-timetable",
        routeName: "ETL Task List",
      },
      {
        title: "navbar.etl.scheduler",
        icon: "mdi-calendar-clock",
        routeName: "ETL Scheduler List",
      },
    ],
  },
  {
    title: "navbar.ai.self",
    icon: "mdi-robot-outline",
    value: "ai",
    feature: FEATURE_AI,
    childrens: [
      {
        title: "navbar.ai.knowledge",
        icon: "mdi-book-open-variant",
        routeName: "AI Knowledge List",
        namespaceScoped: true,
      },
      {
        title: "navbar.ai.assistant",
        icon: "mdi-robot",
        routeName: "AI Assistant",
        namespaceScoped: true,
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
        icon: "mdi-account",
        routeName: "Admin User List",
      },
      {
        title: "navbar.admin.group",
        icon: "mdi-account-group",
        routeName: "Admin Group List",
      },
      {
        title: "navbar.admin.namespace",
        icon: "mdi-tag-multiple",
        routeName: "Admin Namespace List",
      },
      {
        title: "navbar.admin.capturer",
        icon: "mdi-lan",
        routeName: "Admin Capturer List",
      },
      {
        title: "navbar.admin.systemStatus",
        icon: "mdi-server",
        routeName: "Admin System Status",
      },
      {
        title: "navbar.datasource.self",
        icon: "mdi-database-import-outline",
        routeName: "Datasource List",
        namespaceScoped: true,
      },
    ],
  },
];
