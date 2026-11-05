// 全局功能开关（feature flags）状态存储。
import { defineStore } from "pinia";

import { featuresListGet } from "@/apis/features";
import type { FeatureOut } from "@/apis/features";
import { getResponse } from "@/apis/base";

import { useAppStore } from "./app";

// 功能键常量：与后端 features.Key 保持一致。
// FEATURE_PASSWORD_RESET 密码重置功能（依赖系统 SMTP），关闭时前端隐藏"忘记密码"入口。
export const FEATURE_PASSWORD_RESET = "passwordReset";
// FEATURE_AI AI 助手功能（RAG + Agent），关闭时 AI 相关入口置为不可用态并引导开通。
export const FEATURE_AI = "ai";

export const useFeaturesStore = defineStore("features", {
  state: () => ({
    // enabled 功能键 -> 是否开启
    enabled: {} as Record<string, boolean>,
    // reasons 功能键 -> 不可用原因（后端按请求语言下发；功能可用时为空串）
    reasons: {} as Record<string, string>,
    // loadedLang 已加载的功能开关所属语言：不可用原因随请求语言本地化，语言切换后须重新拉取
    loadedLang: "",
  }),
  actions: {
    // load 拉取一次功能开关列表（含按当前语言下发的不可用原因）并缓存；已按当前语言加载时
    // 直接返回，避免重复请求，语言切换后自动重新拉取。force 为 true 时强制刷新：用户刚在
    // 开通入口完成开通后，下一次导航即可取到最新可用性。
    async load(force = false) {
      const lang = useAppStore().language;
      if (!force && this.loadedLang === lang) {
        return;
      }
      await getResponse(
        featuresListGet(),
        async (rsp) => {
          const { data } = rsp as { data?: FeatureOut[] };
          const enabled: Record<string, boolean> = {};
          const reasons: Record<string, string> = {};
          for (const item of data ?? []) {
            enabled[item.key] = item.enabled;
            reasons[item.key] = item.reason;
          }
          this.enabled = enabled;
          this.reasons = reasons;
          this.loadedLang = lang;
        },
        async () => {
          // 拉取失败时按"全部关闭"处理，避免误展示不可用功能入口；不标记已加载，
          // 使下一次导航自动重试（功能可用性的最终裁决方始终是后端）。
          this.enabled = {};
          this.reasons = {};
          this.loadedLang = "";
        },
      );
    },
    // isEnable 判断某项功能是否开启；未加载/未注册统一视为关闭。
    isEnable(key: string): boolean {
      return this.enabled[key] ?? false;
    },
    // reasonOf 取某项功能不可用的原因文案（后端按语言下发）；功能可用、未注册或后端未提供
    // 原因时为空串，调用方据此回退到通用提示。
    reasonOf(key: string): string {
      return this.reasons[key] ?? "";
    },
  },
});
