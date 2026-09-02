<template>
  <main-layout current-nav="tag-event">
    <template #body>
      <search-table
        :list-request="tagEventRecordListGet"
        :options-request="tagEventRecordListOptions"
        :actions="getActions"
        model-name="tagEventRecord"
      >
        <!-- 状态列：按事件状态着色（告警=error / 抑制=warning / 待处理与关闭=默认） -->
        <template #cell-status="{ item }">
          <v-chip size="small" :color="statusColor(item.status)">
            {{ $t(`model.tagEventRecord.type.status.${item.status}`) }}
          </v-chip>
        </template>
      </search-table>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Tag Event Record List
meta:
  title: title.tagEvent.record
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="tag-event-record">
import { useRoute, useRouter } from "vue-router";

import { tagEventRecordListGet, tagEventRecordListOptions } from "@/apis/tagEvent";

import type { ItemActions } from "@/components/@types/table";

const router = useRouter();
const route = useRoute();

// 当前语言参数（自动路由下各页面 params 形态不一，统一取 language，缺省留空由 i18n 兜底）
const language = (route.params as { language?: string }).language ?? "";

// ---- 行操作：仅查看详情（只读页面，点击跳转独立详情路由页）----
const getActions = (): ItemActions => {
  return {
    viewDetail: viewDetailAction,
  };
};

// 状态列 chip 颜色映射：告警=error / 抑制=warning / 待处理与关闭=默认
const statusColor = (status: string | undefined): string | undefined => {
  switch (status) {
    case "alerting":
      return "error";
    case "suppressed":
      return "warning";
    default:
      return undefined;
  }
};

// 跳转详情：记录 ID 经 query.recordId 传递给独立详情路由页
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const viewDetailAction = (item: Record<string, any>): void => {
  void router.push({
    name: "Tag Event Record Detail",
    params: { language },
    query: { recordId: item.id },
  });
};
</script>
