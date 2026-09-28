// navigation.ts
// 共享的侧边栏导航配置（侧边栏与首页快速开始选择器的单一事实来源）。
// 链接标题为 i18n 键（navbar.*）；导航目标为路由名 + 参数。

import { FEATURE_AI } from "@/stores/features";

// NavPerm 单个导航项所需的权限点（object 形如 api:/api/v1/admin/llm/model/list，
// act 为 HTTP 方法字符串，如 "POST"），与后端 RBAC 策略登记取值一致。
export interface NavPerm {
  obj: string;
  act: string;
}

// NavChild 单个导航子项（侧边栏的具体页面入口）。
export interface NavChild {
  title: string;
  icon: string;
  routeName: string;
  params?: Record<string, string>;
  // 是否受命名空间影响：为 true 时在侧边栏子项上叠加 Namespace 标识，
  // 指明该页面随命名空间切换而变化；false 或缺省表示与命名空间无关
  namespaceScoped?: boolean;
  // 所属功能开关键：设置后该子项随对应功能开关（关闭时）置为不可用态，但仍展示
  feature?: string;
  // 所需权限点：设置后该子项仅在当前用户具备该权限时可见
  perm?: NavPerm;
}

// NavGroup 导航分组（侧边栏的一级分组，含其下的导航子项）。
export interface NavGroup {
  title: string;
  icon: string;
  value: string;
  childrens: NavChild[];
  // 所属功能开关键：设置后该分组随对应功能开关（关闭时）置为不可用态，但仍展示
  feature?: string;
  // 开通入口：该功能不可用时引导用户前往执行开通的页面（路由名）。该路由须是导航中
  // 既有登记的页面，其标题与所需权限由 navChildByRoute 解析，避免二次声明标题/权限字面量；
  // 缺省表示该功能无页面级开通入口（如依赖配置文件的能力），此时「功能不可用」页不展示引导
  activationRoute?: string;
}

// NavBadgeKind 导航项标识的种类：lock 表示功能不可用，namespace 表示该页面受命名空间影响。
export type NavBadgeKind = "lock" | "namespace";

// navigationGroups 侧边栏导航的完整分组声明。
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
    title: "navbar.ai.self",
    icon: "mdi-robot-outline",
    value: "ai",
    feature: FEATURE_AI,
    // AI 能力的开通入口：LLM 模型注册与档位绑定都在 LLM 配置页完成
    activationRoute: "Admin LLM Config",
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
      {
        title: "navbar.ai.memory",
        icon: "mdi-brain",
        routeName: "AI Memory List",
        namespaceScoped: true,
        perm: { obj: "api:/api/v1/ai/memory/list", act: "POST" },
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
        title: "navbar.admin.llmConfig",
        icon: "mdi-robot-happy-outline",
        routeName: "Admin LLM Config",
        perm: { obj: "api:/api/v1/admin/llm/model/list", act: "POST" },
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

// filterNavigationByPerm 按权限点过滤导航：未声明 perm 的子项保留，判定不通过的
// 子项剔除，过滤后子项为空的分组整体丢弃。feature 过滤由调用方负责，本函数不做。
export function filterNavigationByPerm(
  groups: NavGroup[],
  can: (perm: NavPerm) => boolean,
): NavGroup[] {
  return groups
    .map((group) => {
      return {
        ...group,
        childrens: group.childrens.filter((child) => !child.perm || can(child.perm)),
      };
    })
    .filter((group) => group.childrens.length > 0);
}

// featureGroupOf 按功能键取声明该功能的导航分组：功能展示名取该分组的标题，开通入口取
// 分组上的 activationRoute，供「功能不可用」说明页与侧边栏共用同一份声明。
export function featureGroupOf(key: string): NavGroup | undefined {
  return navigationGroups.find((group) => group.feature === key);
}

// featureOfRoute 取路由所属的功能键：子项自身声明的 feature 优先，否则继承所属分组；
// 未登记在导航中的路由（如错误页、说明页本身）返回 undefined。
export function featureOfRoute(routeName: string): string | undefined {
  for (const group of navigationGroups) {
    const child = group.childrens.find((item) => item.routeName === routeName);
    if (child) {
      return child.feature ?? group.feature;
    }
  }
  return undefined;
}

// navChildByRoute 按路由名取导航子项：用于把功能开通入口的路由名解析为标题与所需权限
// （开通入口本身已在导航中登记，避免标题与权限点的字面量二次声明）。
export function navChildByRoute(routeName: string): NavChild | undefined {
  for (const group of navigationGroups) {
    const child = group.childrens.find((item) => item.routeName === routeName);
    if (child) {
      return child;
    }
  }
  return undefined;
}

// navBadgeOf 解析导航项标识：功能不可用优先于受命名空间影响——页面既然已经不可用，再说明它是否
// 随命名空间变化已无意义，故两者同时命中时只返回 lock；均未命中时无标识。父级与子级共用此规则，
// 使其标识语义始终一致。
export function navBadgeOf(
  unavailable: boolean,
  namespaceScoped: boolean,
): NavBadgeKind | undefined {
  if (unavailable) {
    return "lock";
  }
  return namespaceScoped ? "namespace" : undefined;
}
