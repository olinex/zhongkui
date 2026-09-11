// Utilities
import { defineStore } from "pinia";

import { getResponse } from "@/apis/base";
import { notificationChannelTypesGet } from "@/apis/notification";
import { DynamicEnums, dynamicEnumOptions } from "@/constants/dynamicEnums";

// Types
import type {
  NotificationChannelTypeItem,
  NotificationChannelTypesOut,
} from "@/apis/@types/notification";

import { useAppStore } from "./app";

// ChannelTypeOption 渠道类型下拉可选项：title 为经 i18n 翻译的展示名（经动态枚举注册表），
// value 为渠道类型键。供 channel / target / template 三个页面统一消费后端渠道类型注册表。
export interface ChannelTypeOption {
  title: string;
  value: string;
}

// useNotificationChannelStore 通知渠道类型注册表共享 store（Options Store 风格）。
// 单一数据源：仅首访问时拉取后端 /notification/channel/types，成功后缓存，
// 后续页面不再重复请求；渠道类型由后端注册表动态决定，新增渠道类型时前端零改动。
export const useNotificationChannelStore = defineStore("notificationChannel", {
  state: () => ({
    // channelTypes 后端渠道类型注册表（含占位符渠道与各类型配置 schema）
    channelTypes: [] as NotificationChannelTypeItem[],
    // loaded 是否已成功加载（失败保持 false 以便重试）
    loaded: false,
  }),
  getters: {
    // channelTypeMap 按类型键索引的渠道类型映射，便于按类型键查找条目
    channelTypeMap(): Record<string, NotificationChannelTypeItem> {
      return Object.fromEntries(this.channelTypes.map((t) => [t.type, t]));
    },
    // channelTypeOptions 渠道类型下拉选项：过滤占位符渠道（系统固定数据，前端不提供创建入口），
    // 供 channel / target 创建与编辑下拉使用；展示名经动态枚举注册表统一翻译
    channelTypeOptions(): ChannelTypeOption[] {
      return dynamicEnumOptions(
        DynamicEnums.channelType,
        this.channelTypes.filter((t) => !t.placeholder),
        "type",
      );
    },
    // allChannelTypeOptions 全量渠道类型下拉选项（含占位符渠道），
    // 供 template 页创建/渲染测试模板使用（模板渠道类型需覆盖全部渠道，如 webhook）
    allChannelTypeOptions(): ChannelTypeOption[] {
      return dynamicEnumOptions(DynamicEnums.channelType, this.channelTypes, "type");
    },
  },
  actions: {
    // loadChannelTypes 拉取渠道类型注册表（仅首次请求，成功即缓存）；
    // 失败时保持未加载态以允许重试，并以 snackbar 提示错误。
    async loadChannelTypes(): Promise<void> {
      if (this.loaded) {
        return;
      }
      await getResponse<NotificationChannelTypesOut>(
        notificationChannelTypesGet(),
        async (rsp) => {
          this.channelTypes = rsp.data.types ?? [];
          this.loaded = true;
        },
        async (rsp) => {
          useAppStore().notify({
            text: rsp.summary,
            timeout: 3000,
            color: "error",
          });
        },
      );
    },
  },
});
