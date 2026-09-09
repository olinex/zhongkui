<template>
  <!-- 标签事件记录详情对话框：与流量审计等列表详情一致，通过 dialog 展示而非独立路由页；
       内容分三块呈卡片（事件基础信息 / 命中明细 / 关联发送记录）。 -->
  <v-dialog :model-value="modelValue" max-width="900" @update:model-value="onUpdate">
    <v-card>
      <v-toolbar density="compact">
        <v-toolbar-title class="text-no-wrap">
          {{ $t("title.tagEvent.recordDetail") }}
        </v-toolbar-title>
        <v-spacer />
        <v-btn icon="mdi-close" variant="text" @click="onUpdate(false)" />
      </v-toolbar>
      <div class="pa-4 overflow-y-auto" style="max-height: 72vh">
        <v-skeleton-loader v-if="loading" type="article" min-height="220" />
        <v-alert v-else-if="errorText" type="error" variant="tonal" density="compact">
          {{ errorText }}
        </v-alert>

        <template v-else-if="detail">
          <!-- 一、事件基础信息 -->
          <v-card variant="tonal" class="mb-4">
            <v-card-title class="text-subtitle-1 font-weight-medium">
              {{ $t("model.tagEventRecord.content.info") }}
            </v-card-title>
            <v-card-text>
              <v-table density="compact">
                <tbody>
                  <tr v-for="field in baseFields" :key="field.key">
                    <td class="text-no-wrap font-weight-medium" style="width: 180px">
                      {{ field.label }}
                    </td>
                    <td>{{ field.display }}</td>
                  </tr>
                </tbody>
              </v-table>
            </v-card-text>
          </v-card>

          <!-- 二、命中明细 -->
          <v-card variant="tonal" class="mb-4">
            <v-card-title class="text-subtitle-1 font-weight-medium">
              {{ $t("model.tagEventRecord.content.hits") }}
            </v-card-title>
            <v-card-text>
              <v-alert
                v-if="detail.hits.length === 0"
                type="info"
                variant="tonal"
                density="compact"
                :text="$t('model.__base.value.blank')"
              />
              <v-table v-else density="compact">
                <thead>
                  <tr>
                    <th>{{ $t("model.tagEventRecordHit.field.conditionName") }}</th>
                    <th>{{ $t("model.tagEventRecordHit.field.model") }}</th>
                    <th>{{ $t("model.tagEventRecordHit.field.dimension") }}</th>
                    <th>{{ $t("model.tagEventRecordHit.field.item") }}</th>
                    <th>{{ $t("model.tagEventRecordHit.field.count") }}</th>
                    <th>{{ $t("model.tagEventRecordHit.field.threshold") }}</th>
                    <th>{{ $t("model.tagEventRecordHit.field.window") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="hit in detail.hits" :key="hit.id">
                    <td>{{ hit.conditionName }}</td>
                    <td>{{ hit.model }}</td>
                    <td>{{ dimensionTitle(hit.dimension, hit.model) }}</td>
                    <td>{{ hit.item }}</td>
                    <td>{{ hit.count }}</td>
                    <td>{{ hit.threshold }}</td>
                    <td>{{ localDurationFormat(hit.window) }}</td>
                  </tr>
                </tbody>
              </v-table>
            </v-card-text>
          </v-card>

          <!-- 三、关联发送记录 -->
          <v-card variant="tonal">
            <v-card-title class="text-subtitle-1 font-weight-medium">
              {{ $t("model.tagEventRecord.content.sends") }}
            </v-card-title>
            <v-card-text>
              <v-alert
                v-if="detail.sends.length === 0"
                type="info"
                variant="tonal"
                density="compact"
                :text="$t('model.__base.value.blank')"
              />
              <v-table v-else density="compact">
                <thead>
                  <tr>
                    <th>{{ $t("model.tagEventRecordSend.field.targetName") }}</th>
                    <th>{{ $t("model.tagEventRecordSend.field.channelName") }}</th>
                    <th>{{ $t("model.tagEventRecordSend.field.channelType") }}</th>
                    <th>{{ $t("model.tagEventRecordSend.field.status") }}</th>
                    <th>{{ $t("model.tagEventRecordSend.field.messageId") }}</th>
                    <th>{{ $t("model.tagEventRecordSend.field.notifiedAt") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="send in detail.sends" :key="send.id">
                    <td>{{ send.targetName }}</td>
                    <td>{{ send.channelName }}</td>
                    <td>
                      <v-chip size="small" variant="tonal">
                        {{ send.channelType }}
                      </v-chip>
                    </td>
                    <td>
                      <v-chip size="small" :color="sendStatusColor(send.status)">
                        {{ $t(`model.notificationRecord.type.status.${send.status}`) }}
                      </v-chip>
                    </td>
                    <td>{{ send.messageId }}</td>
                    <td>{{ formatDatetime(send.notifiedAt) }}</td>
                  </tr>
                </tbody>
              </v-table>
            </v-card-text>
          </v-card>
        </template>
      </div>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts" name="tag-event-record-detail-dialog">
import { DateTime } from "luxon";

import { getResponse } from "@/apis/base";
import { tagEventRecordDetailGet } from "@/apis/tagEvent";

import type { TagEventRecordDetailOut } from "@/apis/@types/admin";

import { useAppStore } from "@/stores/app";
import i18n, { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";
import { modelFieldTitle, modelTypeKeyToI18nKey } from "@/utils/modelField";

const props = defineProps<{
  modelValue: boolean;
  recordId: number;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const appStore = useAppStore();

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};

const detail = ref<TagEventRecordDetailOut | null>(null);
const loading = ref(true);
const errorText = ref("");

// 打开且携带有效记录 ID 时拉取详情；切换记录（并行打开另一个）时重新请求
watch(
  () => [props.modelValue, props.recordId] as const,
  ([open, id]) => {
    if (!open) {
      return;
    }
    if (!id) {
      loading.value = false;
      errorText.value = i18n.global.t("notice.missingParam");
      return;
    }
    loading.value = true;
    errorText.value = "";
    detail.value = null;
    void getResponse<TagEventRecordDetailOut>(
      tagEventRecordDetailGet(id),
      async (rsp) => {
        detail.value = rsp.data;
        loading.value = false;
      },
      async (rsp) => {
        loading.value = false;
        errorText.value = rsp.summary ?? "";
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  },
  { immediate: true },
);

// 可空时间字段统一格式化；空值展示 null 占位
const formatDatetime = (value: string | null | undefined): string => {
  if (!value) {
    return i18n.global.t("model.__base.value.null");
  }
  return localeDatetimeFormat(DateTime.fromISO(value));
};

// dimensionTitle 标签维度列展示名：维度为枚举类型，前端按 model.<modelKey>.field.<dimension>
// 复用对应字段翻译作为 option 的 title（如 fpTcpClientSig → "客户端指纹签名"），
// 未命中时回退为模型字段标题兜底（去前缀/驼峰分词），不再直接显示原始 value。
const dimensionTitle = (dimension: string, model: string): string =>
  modelFieldTitle(modelTypeKeyToI18nKey(model), dimension);

// 事件基础信息字段（标签经 i18n，状态值含枚举翻译）
interface DetailField {
  key: string;
  label: string;
  display: string;
}

const baseFields = computed<DetailField[]>(() => {
  const item = detail.value;
  if (!item) {
    return [];
  }
  const label = (key: string): string => i18n.global.t(`model.tagEventRecord.field.${key}`);
  return [
    { key: "ruleName", label: label("ruleName"), display: item.ruleName },
    { key: "ruleId", label: label("ruleId"), display: String(item.ruleId) },
    {
      key: "matchAll",
      label: label("matchAll"),
      display: item.matchAll
        ? i18n.global.t("model.__base.value.true")
        : i18n.global.t("model.__base.value.false"),
    },
    {
      key: "status",
      label: label("status"),
      display: i18n.global.t(`model.tagEventRecord.type.status.${item.status}`),
    },
    { key: "dedupKey", label: label("dedupKey"), display: item.dedupKey || "" },
    { key: "detectedAt", label: label("detectedAt"), display: formatDatetime(item.detectedAt) },
    { key: "createdAt", label: label("createdAt"), display: formatDatetime(item.createdAt) },
    { key: "updatedAt", label: label("updatedAt"), display: formatDatetime(item.updatedAt) },
  ];
});

// 发送记录状态列 chip 颜色映射：成功=success / 失败=error / 待处理=默认
const sendStatusColor = (status: string | undefined): string | undefined => {
  switch (status) {
    case "success":
      return "success";
    case "failed":
      return "error";
    default:
      return undefined;
  }
};
</script>
