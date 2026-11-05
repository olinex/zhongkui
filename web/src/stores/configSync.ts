// stores/configSync.ts
// 前端页面配置同步 store：将浏览器侧的页面配置（主题、命名空间、导航栏固定、
// 快速开始分组、语言、表格列）与后端保持同步，使用户在任意主机上都能获得一致的
// 页面布局。
//
// 并发控制：每次推送都携带上一次同步的哈希与版本号作为乐观锁；二者任一不匹配
// （其他浏览器已推送）后端即拒绝本次写入。任何失败都会把本地改动回滚到最近一次
// 同步的快照并报错，故同步异常绝不会静默地使页面配置产生分歧。

import consola from "consola";
import { defineStore } from "pinia";

import { getResponse } from "@/apis/base";
import { frontendConfigGet, frontendConfigSync, frontendConfigSyncEnabledSet } from "@/apis/user";
import type { FrontendConfigOut } from "@/apis/@types/user";
import { CustomCode } from "@/apis/@types/base";
import { SupportLanguage } from "@/types";
import i18n from "@/utils/i18n";

import { useAppStore } from "./app";
import { localManager } from "./local";
import { useQuickStartStore } from "./quickStart";

// 本地改动推送至后端前的防抖窗口。
const SYNC_DEBOUNCE_MS = 500;

// 表格列配置在 localStorage 中的存储键前缀（与 utils/column.ts 保持一致）
const COLUMN_STORAGE_PREFIX = "table-columns:";

// PageConfig 是从 localStorage 采集的用户可编辑页面配置。
interface PageConfig {
  theme: string;
  namespace: string;
  navPinned: boolean;
  quickStartGroups: string;
  language: string;
  // 表格列配置：键为 table-columns: 前缀的 localStorage 键，值为列配置对象
  tableColumns: Record<string, unknown>;
}

// captureColumnConfigs 收集 localStorage 中全部表格列配置；
// 键按字典序排列保证 JSON 序列化结果稳定（避免键序不同触发无谓同步）
const captureColumnConfigs = (): Record<string, unknown> => {
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key !== null && key.startsWith(COLUMN_STORAGE_PREFIX)) {
      keys.push(key);
    }
  }
  keys.sort();
  const configs: Record<string, unknown> = {};
  for (const key of keys) {
    const raw = localStorage.getItem(key);
    if (raw === null) {
      continue;
    }
    try {
      configs[key] = JSON.parse(raw);
    } catch {
      // 忽略脏数据
    }
  }
  return configs;
};

// applyColumnConfigs 将服务端下发的表格列配置写回 localStorage（幂等覆盖）
const applyColumnConfigs = (configs: unknown): void => {
  if (typeof configs !== "object" || configs === null) {
    return;
  }
  for (const [key, value] of Object.entries(configs)) {
    if (key.startsWith(COLUMN_STORAGE_PREFIX)) {
      localStorage.setItem(key, JSON.stringify(value));
    }
  }
};

const captureConfig = (): PageConfig => ({
  theme: localManager.getTheme(),
  namespace: localManager.getNamespace(),
  navPinned: localManager.getNavPinned(),
  // 空串与 null 均表示无自定义分组，统一归一化为空串避免重复同步
  quickStartGroups: localManager.getQuickStartGroups() ?? "",
  language: localManager.getLanguage(),
  tableColumns: captureColumnConfigs(),
});

// isSupportLanguage 判断字符串是否为受支持的语言枚举值
const isSupportLanguage = (lang: string): lang is SupportLanguage => {
  return Object.values(SupportLanguage).includes(lang as SupportLanguage);
};

export const useConfigSyncStore = defineStore("configSync", {
  state: () => ({
    // 同步开关状态（服务端持久化）
    enabled: false,
    // 最近一次成功同步的配置哈希与版本号（乐观锁）
    hash: "",
    version: 0,
    // 最近一次成功同步（或从服务端拉取）的配置 JSON 快照
    lastSynced: "",
    // 正在应用远端配置，避免回写触发重复同步
    applying: false,
    syncing: false,
    // 防抖定时器
    timer: undefined as ReturnType<typeof setTimeout> | undefined,
    inited: false,
  }),
  actions: {
    // init 登录后初始化：拉取服务端配置，开启同步则应用并开始监听本地变更
    async init() {
      if (this.inited) {
        return;
      }
      this.inited = true;
      await this.reload();
    },
    // reload 重新拉取服务端配置并应用；供登录初始化与并发冲突恢复使用
    async reload() {
      await getResponse<FrontendConfigOut>(
        frontendConfigGet(),
        async (rsp) => {
          this.enabled = rsp.data.syncEnabled;
          this.hash = rsp.data.hash;
          this.version = rsp.data.version;
          this.lastSynced = rsp.data.config;
          if (this.enabled) {
            this.apply(rsp.data.config);
          }
        },
        async (rsp) => {
          consola.warn(`code: ${rsp.code} - ${rsp.summary}`);
        },
      );
    },
    // apply 将服务端配置写入本地存储与各 store；为空或解析失败时跳过
    apply(config: string) {
      if (!config) {
        return;
      }
      let parsed: Partial<PageConfig>;
      try {
        parsed = JSON.parse(config) as Partial<PageConfig>;
      } catch {
        consola.warn("bad frontend config payload, ignore apply");
        return;
      }
      this.applying = true;
      try {
        const appStore = useAppStore();
        if (parsed.theme !== undefined) {
          localManager.setTheme(parsed.theme);
          appStore.theme = parsed.theme;
        }
        if (parsed.namespace !== undefined) {
          localManager.setNamespace(parsed.namespace);
          appStore.namespace = parsed.namespace;
        }
        if (parsed.navPinned !== undefined) {
          localManager.setNavPinned(parsed.navPinned);
        }
        if (parsed.tableColumns !== undefined) {
          applyColumnConfigs(parsed.tableColumns);
        }
        if (parsed.quickStartGroups !== undefined) {
          localManager.setQuickStartGroups(parsed.quickStartGroups);
          useQuickStartStore().reload();
        }
        if (parsed.language !== undefined && isSupportLanguage(parsed.language)) {
          appStore.asLanguage(parsed.language);
        }
      } finally {
        this.applying = false;
      }
    },
    // notifyLocalChange 本地配置变更入口：防抖后推送到服务端；
    // 应用远端配置（applying）或未开启同步时忽略
    notifyLocalChange() {
      if (!this.enabled || this.applying) {
        return;
      }
      if (this.timer) {
        clearTimeout(this.timer);
      }
      this.timer = setTimeout(() => this.push(), SYNC_DEBOUNCE_MS);
    },
    // push 使用乐观锁上传当前本地配置；失败时回滚本地并报错
    async push() {
      if (!this.enabled || this.applying || this.syncing) {
        return;
      }
      const config = JSON.stringify(captureConfig());
      if (config === this.lastSynced) {
        return;
      }
      this.syncing = true;
      const appStore = useAppStore();
      await getResponse<FrontendConfigOut>(
        frontendConfigSync({
          prevHash: this.hash,
          prevVersion: this.version,
          config,
        }),
        async (rsp) => {
          this.syncing = false;
          this.hash = rsp.data.hash;
          this.version = rsp.data.version;
          this.lastSynced = rsp.data.config;
          // 请求期间又发生变更则继续推送，避免变更丢失
          if (JSON.stringify(captureConfig()) !== this.lastSynced) {
            this.notifyLocalChange();
          }
        },
        async (rsp) => {
          this.syncing = false;
          // 冲突：其他浏览器已抢先更新；拉取最新配置覆盖本地并报错
          if (
            rsp.code === CustomCode.CodeFrontendConfigConflict ||
            rsp.code === CustomCode.CodeFrontendConfigSyncDisabled
          ) {
            await this.reload();
          } else {
            // 其他失败：回滚到最近一次成功同步的快照
            this.rollback();
          }
          appStore.notify({
            text: rsp.summary,
            timeout: 5000,
            color: "error",
          });
        },
        async () => {
          this.syncing = false;
          this.rollback();
          appStore.notify({
            text: i18n.global.t("notice.frontendConfigSyncFailed"),
            timeout: 5000,
            color: "error",
          });
        },
        async () => {
          this.syncing = false;
          this.rollback();
          appStore.notify({
            text: i18n.global.t("notice.frontendConfigSyncFailed"),
            timeout: 5000,
            color: "error",
          });
        },
      );
    },
    // rollback 将本地配置恢复为最近一次成功同步的快照
    rollback() {
      this.apply(this.lastSynced);
    },
    // setEnabled 切换同步开关；开启后立即上传当前本地配置
    async setEnabled(enabled: boolean) {
      const appStore = useAppStore();
      await getResponse<FrontendConfigOut>(
        frontendConfigSyncEnabledSet(enabled),
        async (rsp) => {
          this.enabled = enabled;
          if (enabled) {
            // 首次开启：本地配置即为基准，以空哈希/零版本上传
            this.hash = "";
            this.version = 0;
            this.lastSynced = "";
            await this.push();
          }
          appStore.notify({
            text: rsp.summary,
            timeout: 3000,
            color: "success",
          });
        },
        async (rsp) => {
          appStore.notify({
            text: rsp.summary,
            timeout: 3000,
            color: "error",
          });
        },
      );
    },
    // reset 退出登录时清理状态，停止同步
    reset() {
      this.inited = false;
      this.enabled = false;
      this.hash = "";
      this.version = 0;
      this.lastSynced = "";
      this.syncing = false;
      if (this.timer) {
        clearTimeout(this.timer);
        this.timer = undefined;
      }
    },
  },
});
