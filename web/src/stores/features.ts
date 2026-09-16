// 全局功能开关（feature flags）状态存储。
// Pinia store for global feature flags.
import { defineStore } from "pinia";

import { featuresListGet } from "@/apis/features";
import { getResponse } from "@/apis/base";

// 功能键常量：与后端 features.Key 保持一致。
// FEATURE_PASSWORD_RESET 密码重置功能（依赖系统 SMTP），关闭时前端隐藏"忘记密码"入口。
export const FEATURE_PASSWORD_RESET = "passwordReset";
// FEATURE_AI AI 助手功能（RAG + Agent），关闭时前端隐藏 AI 相关导航入口。
export const FEATURE_AI = "ai";

export const useFeaturesStore = defineStore("features", {
  state: () => ({
    // enabled 功能键 -> 是否开启
    enabled: {} as Record<string, boolean>,
    // loaded 是否已从后端加载过功能开关
    loaded: false,
  }),
  actions: {
    // load 拉取一次功能开关列表并缓存；已加载则直接返回，避免重复请求。
    async load() {
      if (this.loaded) {
        return;
      }
      await getResponse(
        featuresListGet(),
        async (rsp) => {
          const { data } = rsp as { data?: { key: string; enabled: boolean }[] };
          const map: Record<string, boolean> = {};
          for (const item of data ?? []) {
            map[item.key] = item.enabled;
          }
          this.enabled = map;
          this.loaded = true;
        },
        async () => {
          // 拉取失败时按"全部关闭"处理，避免误展示不可用功能入口。
          this.enabled = {};
          this.loaded = true;
        },
      );
    },
    // isEnable 判断某项功能是否开启；未加载/未注册统一视为关闭。
    isEnable(key: string): boolean {
      return this.enabled[key] ?? false;
    },
  },
});
