// navigation.ts
// Shared sidebar navigation configuration (single source of truth for the
// sidebar and the home-page quick start picker).
// Link titles are i18n keys (navbar.*); navigation targets are route name + params.

export interface NavChild {
  title: string;
  icon: string;
  routeName: string;
  params?: Record<string, string>;
}

export interface NavGroup {
  title: string;
  icon: string;
  value: string;
  childrens: NavChild[];
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
      },
      {
        title: "navbar.tagEvent.tagEventRule",
        icon: "mdi-alert-decagram",
        routeName: "Tag Event Rule List",
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
      },
      {
        title: "navbar.traffic.ipv6Tcp",
        icon: "mdi-ip-network-outline",
        routeName: "Traffic List",
        params: { type: "ipv6-tcp" },
      },
      {
        title: "navbar.traffic.httpTransport",
        icon: "mdi-web",
        routeName: "Traffic List",
        params: { type: "http-transport" },
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
      },
      {
        title: "navbar.notification.target",
        icon: "mdi-account-arrow-right-outline",
        routeName: "Notification Target List",
      },
      {
        title: "navbar.notification.group",
        icon: "mdi-account-group-outline",
        routeName: "Notification Group List",
      },
      {
        title: "navbar.notification.template",
        icon: "mdi-file-document-edit-outline",
        routeName: "Notification Template List",
      },
      {
        title: "navbar.notification.record",
        icon: "mdi-history",
        routeName: "Notification Record List",
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
    ],
  },
];
