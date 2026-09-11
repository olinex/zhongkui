<template>
  <main-layout current-nav="tag-event">
    <template #body>
      <search-table
        :list-request="tagEventRecordListGet"
        :options-request="tagEventRecordListOptions"
        :actions="getActions"
        :row-click="openDetail"
        :project-fields="projectFields"
        model-name="tagEventRecord"
      >
        <!-- 状态列：按事件状态着色（告警=error / 抑制=warning / 待处理与关闭=默认） -->
        <template #cell-status="{ item }">
          <v-chip size="small" :color="statusColor(item.status)">
            {{ enumValueTitle("tagEventRecord", item.status) }}
          </v-chip>
        </template>
      </search-table>

      <!-- 记录详情对话框（只读）；与流量审计等列表一致，通过 row 点击/操作按钮打开 -->
      <tag-event-record-detail-dialog v-model="detailDialog" :record-id="detailId" />
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
import { tagEventRecordListGet, tagEventRecordListOptions } from "@/apis/tagEvent";
import { enumValueTitle } from "@/utils/options";

import type { ItemActions } from "@/components/@types/table";

// 详情对话框可见性与当前查看的记录 ID
const detailDialog = ref(false);
const detailId = ref(0);

// projectFields 字段投影：行操作依赖字段恒定投影（id 定位详情），避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", ...visible];

// ---- 行操作：仅查看详情（只读页面，点击打开展示在对话框而非独立路由页）----
const getActions = (): ItemActions => {
  return {
    viewDetail: (
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      item: Record<string, any>,
    ) => openDetail(item),
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

// 打开详情对话框：记录 ID 由行数据提供
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openDetail = (item: Record<string, any>): void => {
  detailId.value = Number(item.id);
  detailDialog.value = true;
};
</script>
