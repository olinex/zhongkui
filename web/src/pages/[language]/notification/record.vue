<template>
  <main-layout current-nav="notification">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="notificationRecordListGet"
        :options-request="notificationRecordListOptions"
        :actions="getActions"
        :project-fields="projectFields"
        model-name="notificationRecord"
      >
        <!-- 状态列：按发送状态着色（成功=success / 失败=error / 待处理=默认） -->
        <template #cell-status="{ item }">
          <v-chip :color="statusColor(item.status)">
            {{ enumLabelOf(EnumClass.notificationRecordStatus, item.status) }}
          </v-chip>
        </template>
        <!-- 渠道类型列：展示可读的渠道类型名 -->
        <template #cell-channelType="{ item }">
          <v-chip>
            {{ enumLabelOf(EnumClass.channelType, item.channelType) }}
          </v-chip>
        </template>
        <!-- 事件类型列：展示经过 i18n 翻译的枚举 label（键为事件注册表返回的 event 键） -->
        <template #cell-eventType="{ item }">
          <v-chip>
            {{ enumLabelOf(EnumClass.notificationEvent, item.eventType) }}
          </v-chip>
        </template>
      </search-table>

      <!-- 记录详情对话框：只读展示基础字段、payload（美化 JSON）与失败原因（如有） -->
      <app-dialog v-model="detailDialog" :title="$t('model.notificationRecord.name')" size="md">
        <v-table density="compact">
          <tbody>
            <tr v-for="field in detailFields" :key="field.key">
              <td class="text-no-wrap font-weight-medium">{{ field.label }}</td>
              <td>{{ field.display }}</td>
            </tr>
          </tbody>
        </v-table>

        <v-divider class="my-3" />

        <div class="text-subtitle-2 font-weight-medium mb-2">
          {{ $t("model.notificationRecord.field.payload") }}
        </div>
        <pre class="zk-code-preview text-body-2">{{ detailPayload }}</pre>

        <v-alert v-if="detailError" type="error" class="mt-3">
          <div class="text-subtitle-2 font-weight-medium mb-1">
            {{ $t("model.notificationRecord.field.error") }}
          </div>
          <div class="text-body-2 error-view">{{ detailError }}</div>
        </v-alert>
      </app-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Notification Record List
meta:
  title: title.notification.record
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="notification-record">
import { DateTime } from "luxon";

import { getResponse } from "@/apis/base";
import {
  notificationRecordDetailGet,
  notificationRecordListGet,
  notificationRecordListOptions,
} from "@/apis/notification";
import { useAppStore } from "@/stores/app";
import { EnumClass, enumLabelOf } from "@/utils/enums";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

import type { NotificationRecordDetailOut } from "@/apis/@types/notification";
import type { ItemActions } from "@/components/@types/table";

const appStore = useAppStore();

// projectFields 字段投影：行操作依赖字段恒定投影（id 定位详情），避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", ...visible];

// ---- 行操作：仅查看详情（只读页面，无创建/批量操作）----
const getActions = (): ItemActions => {
  return {
    viewDetail: detailGetAction,
  };
};

// ---- 详情对话框 ----
const detailDialog = ref(false);
const detailItem = ref<NotificationRecordDetailOut | null>(null);

// 状态列 chip 颜色映射：成功/失败/待处理与全局色彩语义一致，未知状态回退默认色
const statusColor = (status: string | undefined): string | undefined => {
  switch (status) {
    case "success":
      return "success";
    case "failed":
      return "error";
    default:
      return undefined;
  }
};

// payload 美化：合法 JSON 做 pretty-print，非法时原样回退
const formatPayload = (payload: string | undefined | null): string => {
  if (!payload) {
    return "";
  }
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
};

// 可空时间字段统一格式化；空值展示 null 占位
const formatDatetime = (value: string | null | undefined): string => {
  if (!value) {
    return i18n.global.t("model.__base.value.null");
  }
  return localeDatetimeFormat(DateTime.fromISO(value));
};

// 详情基础字段（标签经 i18n，值含枚举翻译）
interface DetailField {
  key: string;
  label: string;
  display: string;
}

// eventTypeLabel 事件类型展示名（键为后端事件注册表返回的 event 键，如 tag_event_record）
const eventTypeLabel = (eventType: string): string =>
  enumLabelOf(EnumClass.notificationEvent, eventType);

const detailFields = computed<DetailField[]>(() => {
  const item = detailItem.value;
  if (!item) {
    return [];
  }
  const label = (key: string): string => i18n.global.t(`model.notificationRecord.field.${key}`);
  return [
    { key: "eventType", label: label("eventType"), display: eventTypeLabel(item.eventType) },
    { key: "eventId", label: label("eventId"), display: String(item.eventId) },
    { key: "targetName", label: label("targetName"), display: item.targetName },
    { key: "channelName", label: label("channelName"), display: item.channelName },
    {
      key: "channelType",
      label: label("channelType"),
      display: enumLabelOf(EnumClass.channelType, item.channelType),
    },
    {
      key: "status",
      label: label("status"),
      display: enumLabelOf(EnumClass.notificationRecordStatus, item.status),
    },
    { key: "messageId", label: label("messageId"), display: item.messageId },
    { key: "notifiedAt", label: label("notifiedAt"), display: formatDatetime(item.notifiedAt) },
  ];
});

const detailPayload = computed<string>(() => formatPayload(detailItem.value?.payload));
const detailError = computed<string>(() => detailItem.value?.error ?? "");

// 拉取详情失败时仅提示错误，不打开对话框
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<NotificationRecordDetailOut>(
    notificationRecordDetailGet(item.id),
    async (rsp) => {
      detailItem.value = rsp.data;
      detailDialog.value = true;
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};
</script>

<style scoped>
/* payload 美化 JSON 视图的外观（弱化底色 + 换行 + 断词 + 内边距）统一走
   .zk-code-preview，超高内容由 AppDialog 的滚动承载，不再本地限高 */

/* 失败原因长文本自动换行 */
.error-view {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
