<template>
  <!-- 标签事件记录详情对话框：与流量审计等列表详情一致，通过 dialog 展示而非独立路由页；
       内容分三块呈卡片（事件基础信息 / 命中明细 / 关联发送记录）。 -->
  <app-dialog
    :model-value="modelValue"
    :title="$t('title.tagEvent.recordDetail')"
    size="lg"
    @update:model-value="onUpdate"
  >
    <v-skeleton-loader v-if="loading" type="article" min-height="220" />
    <v-alert v-else-if="errorText" type="error">
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
                <td class="field-label-col text-no-wrap font-weight-medium">
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
                <th class="text-no-wrap">{{ $t("model.__base.field.__actions") }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="hit in detail.hits" :key="hit.id">
                <td>{{ hit.conditionName }}</td>
                <td>{{ enumLabelOf(EnumClass.tagRuleModel, hit.model) }}</td>
                <td>{{ enumLabelOf(EnumClass.tagEventDimension, hit.dimension) }}</td>
                <td>{{ hit.item }}</td>
                <td>{{ hit.count }}</td>
                <td>{{ hit.threshold }}</td>
                <td>{{ localDurationFormat(hit.window) }}</td>
                <td>
                  <v-btn
                    :disabled="hit.ids.length === 0"
                    :title="$t('action.viewRecords')"
                    :aria-label="$t('action.viewRecords')"
                    icon="mdi-eye"
                    size="small"
                    variant="tonal"
                    @click="openHitRecords(hit)"
                  />
                </td>
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
                  <v-chip>
                    {{ enumLabelOf(EnumClass.channelType, send.channelType) }}
                  </v-chip>
                </td>
                <td>
                  <v-chip :color="sendStatusColor(send.status)">
                    {{ enumLabelOf(EnumClass.notificationRecordStatus, send.status) }}
                  </v-chip>
                </td>
                <td>
                  {{ send.messageId || $t("model.__base.value.null") }}
                </td>
                <td>{{ formatDatetime(send.notifiedAt) }}</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
      </v-card>
    </template>
  </app-dialog>

  <!-- 命中条件原始数据反查：展示该条件命中的 http_transports 明细；
       detectedAt 随详情传入，供反查弹窗推导入库时间预筛范围 -->
  <tag-event-hit-records-dialog
    v-model="hitRecordsOpen"
    :hit="selectedHit"
    :detected-at="detail?.detectedAt ?? ''"
  />
</template>

<script setup lang="ts" name="tag-event-record-detail-dialog">
import { DateTime } from "luxon";

import { getResponse } from "@/apis/base";
import { tagEventRecordDetailGet } from "@/apis/tagEvent";

import type { TagEventRecordDetailOut, TagEventRecordHitOut } from "@/apis/@types/admin";

import { useAppStore } from "@/stores/app";
import { EnumClass, enumLabelOf } from "@/utils/enums";
import i18n, { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";

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
      display: enumLabelOf(EnumClass.tagAlertStatus, item.status),
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

// 命中条件原始数据反查弹窗：选中命中明细后打开，弹窗内按条件 ID 拉取并浏览原始记录
const hitRecordsOpen = ref(false);
const selectedHit = ref<TagEventRecordHitOut | null>(null);

// openHitRecords 打开原始数据反查弹窗：传入命中明细，按钮在 ids 非空时可用
const openHitRecords = (hit: TagEventRecordHitOut): void => {
  selectedHit.value = hit;
  hitRecordsOpen.value = true;
};
</script>

<style scoped>
/* 字段名列宽提示：固定标签列宽度，值列自适应占满剩余空间 */
.field-label-col {
  width: 180px;
}
</style>
