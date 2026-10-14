<template>
  <!-- 命中条件原始数据反查对话框：展示命中条件对应的 http_transports 原始记录明细。
       单条记录 + 上一条/下一条切换；字段按 kind 分支渲染三列自适应列表
       （字段名 / 字段值 / 操作），crypto 与 s3 字段经操作列查看明文。
       detectedAt 来自事件详情，用于推导原始记录入库时间预筛范围（后端 ClickHouse 分区裁剪）。 -->
  <app-dialog
    :model-value="modelValue"
    :title="$t('title.tagEvent.hitRecords')"
    size="lg"
    @update:model-value="onUpdate"
  >
    <template #header-append>
      <v-spacer />
      <div v-if="items.length > 0" class="d-flex align-center ga-2">
        <v-btn
          icon="mdi-chevron-left"
          size="small"
          variant="text"
          :disabled="currentIndex <= 0"
          :title="$t('action.previousRecord')"
          :aria-label="$t('action.previousRecord')"
          @click="currentIndex -= 1"
        />
        <span class="text-body-small text-no-wrap"
          >{{ currentIndex + 1 }} / {{ items.length }}</span
        >
        <v-btn
          icon="mdi-chevron-right"
          size="small"
          variant="text"
          :disabled="currentIndex >= items.length - 1"
          :title="$t('action.nextRecord')"
          :aria-label="$t('action.nextRecord')"
          @click="currentIndex += 1"
        />
      </div>
    </template>
    <v-skeleton-loader v-if="loading" type="article" min-height="220" />
    <v-alert v-else-if="errorText" type="error">
      {{ errorText }}
    </v-alert>
    <template v-else>
      <!-- 命中摘要 -->
      <v-card variant="tonal" class="mb-4">
        <v-card-title class="text-body-large font-weight-medium">
          {{ $t("model.tagEventRecordHit.name") }}
        </v-card-title>
        <v-card-text>
          <v-table density="compact">
            <tbody>
              <tr v-for="field in summaryFields" :key="field.key">
                <td class="field-label-col text-no-wrap font-weight-medium">
                  {{ field.label }}
                </td>
                <td>{{ field.display }}</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
      </v-card>

      <!-- 原始数据明细 -->
      <template v-if="items.length > 0">
        <v-table density="compact">
          <thead>
            <tr>
              <th class="text-no-wrap">{{ $t("model.__base.field.field") }}</th>
              <th>{{ $t("model.__base.field.value") }}</th>
              <th class="text-no-wrap">{{ $t("model.__base.field.action") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="field in detailFields" :key="field.key">
              <td class="text-no-wrap font-weight-medium">{{ field.label }}</td>
              <td class="detail-value">
                <template v-if="field.kind === 'crypto'">
                  <v-chip color="warning">
                    {{ $t("model.__base.field.encrypted") }}
                  </v-chip>
                </template>
                <template v-else-if="field.kind === 's3'">
                  <span class="font-mono">{{ field.display }}</span>
                  <storage-size-chip
                    class="ml-2"
                    :value="field.length"
                    binary
                    density="comfortable"
                  />
                </template>
                <template v-else>
                  <adaptive-cell :header="plainHeader(field)" :item="plainItem(field)" />
                </template>
              </td>
              <td class="text-no-wrap">
                <template
                  v-if="field.kind === 'crypto' || (field.kind === 's3' && field.length > 0)"
                >
                  <v-btn
                    :text="$t('action.viewPlaintext')"
                    size="small"
                    variant="tonal"
                    color="primary"
                    :loading="plaintextLoading === field.key"
                    @click="viewPlaintextAction(field)"
                  />
                </template>
                <template v-else-if="field.kind === 's3'">
                  <v-chip>{{ $t("notice.plaintextEmpty") }}</v-chip>
                </template>
              </td>
            </tr>
          </tbody>
        </v-table>
      </template>
      <v-alert v-else type="info" :text="$t('model.__base.value.blank')" />
    </template>
  </app-dialog>

  <!-- 明文查看对话框（共享组件：加密/S3 字段解密展示与下载） -->
  <plaintext-dialog ref="plaintextDialogRef" />
</template>

<script setup lang="ts" name="tag-event-hit-records-dialog">
import { DateTime } from "luxon";

import { getResponse } from "@/apis/base";
import { tagEventRecordHitRecordsGet } from "@/apis/tagEvent";
import {
  TrafficCryptoFields,
  TrafficFieldTypes,
  TrafficS3Fields,
  trafficTypeFromModelKey,
} from "@/apis/traffic";

import type { TagEventRecordHitOut, TagEventRecordHitRecordsOut } from "@/apis/@types/admin";
import type { TrafficType } from "@/apis/@types/traffic";
import type { DetailField } from "@/components/@types/detail";
import type { TableHeader } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { BackendDataType } from "@/types";
import { durationToMillis } from "@/utils/duration";
import { EnumClass, enumLabelOf } from "@/utils/enums";
import i18n, { localDurationFormat } from "@/utils/i18n";
import { modelFieldTitle, modelTypeKeyToI18nKey } from "@/utils/modelField";

const props = defineProps<{
  modelValue: boolean;
  hit: TagEventRecordHitOut | null;
  // detectedAt 事件检测时间，用于推导原始记录入库时间预筛范围（后端 ClickHouse 分区裁剪）
  detectedAt: string;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const appStore = useAppStore();
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};

// 原始数据明细（http_transports 全字段行）与当前浏览索引
const items = ref<Record<string, unknown>[]>([]);
const currentIndex = ref(0);
const loading = ref(true);
const errorText = ref("");
const plaintextLoading = ref("");

// 打开时按命中条件透传参数拉取原始记录；切换命中条件时重置浏览位置。
// 入参 model/ids 来自命中明细；createdAtFrom/createdAtTo 由事件检测时间 detectedAt
// 与条件窗口 window 推导（[detectedAt - window, detectedAt]），后端据此预筛
// ClickHouse created_at 分区（http_transports 按小时分区），避免全表扫描。
watch(
  () => [props.modelValue, props.hit, props.detectedAt] as const,
  ([open, hit, detectedAt]) => {
    if (!open || !hit || !detectedAt) {
      return;
    }
    currentIndex.value = 0;
    items.value = [];
    errorText.value = "";
    loading.value = true;
    const detected = DateTime.fromISO(detectedAt);
    const windowMs = durationToMillis(hit.window);
    // 时间解析失败时回退为空串：后端视为时间不限制（保持全范围查询行为）
    const createdAtFrom = Number.isNaN(windowMs)
      ? ""
      : windowMs > 0
        ? (detected.minus(windowMs).toISO() ?? "")
        : (detected.toISO() ?? "");
    const createdAtTo = detected.toISO() ?? "";
    void getResponse<TagEventRecordHitRecordsOut>(
      tagEventRecordHitRecordsGet({
        model: hit.model,
        ids: hit.ids,
        createdAtFrom,
        createdAtTo,
      }),
      async (rsp) => {
        items.value = rsp.data?.items ?? [];
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

// 命中模型 → 流量类型（决定字段渲染类型与加密/S3 字段清单）；未注册模型返回 undefined
const trafficType = computed<TrafficType | undefined>(() => {
  return props.hit ? trafficTypeFromModelKey(props.hit.model) : undefined;
});

// 命中摘要字段（条件名/模型/维度/维度值/次数/阈值/窗口）
interface SummaryField {
  key: string;
  label: string;
  display: string;
}

const summaryFields = computed<SummaryField[]>(() => {
  const hit = props.hit;
  if (!hit) {
    return [];
  }
  const label = (key: string): string => i18n.global.t(`model.tagEventRecordHit.field.${key}`);
  return [
    { key: "conditionName", label: label("conditionName"), display: hit.conditionName },
    {
      key: "model",
      label: label("model"),
      display: enumLabelOf(EnumClass.tagRuleModel, hit.model),
    },
    {
      key: "dimension",
      label: label("dimension"),
      display: enumLabelOf(EnumClass.tagEventDimension, hit.dimension),
    },
    { key: "item", label: label("item"), display: hit.item },
    { key: "count", label: label("count"), display: String(hit.count) },
    { key: "threshold", label: label("threshold"), display: String(hit.threshold) },
    { key: "window", label: label("window"), display: localDurationFormat(hit.window) },
  ];
});

// 当前原始记录字段列表：plain 经 AdaptiveCell 渲染，crypto/s3 展示占位并经操作列查看明文
const detailFields = computed<DetailField[]>(() => {
  const typeName = trafficType.value;
  const item = items.value[currentIndex.value];
  if (!typeName || !item) {
    return [];
  }
  const fieldTypes = TrafficFieldTypes[typeName] ?? {};
  const cryptoKeys = new Set(TrafficCryptoFields[typeName] ?? []);
  const s3Keys = new Set(TrafficS3Fields[typeName] ?? []);
  const modelKey = modelTypeKeyToI18nKey(props.hit?.model ?? "");
  return Object.entries(item)
    .filter(([, value]) => value !== null && value !== undefined && value !== "")
    .map(([key, value]) => {
      const label = modelFieldTitle(modelKey, key);
      const fieldType = fieldTypes[key] ?? BackendDataType.STRING;
      if (cryptoKeys.has(key)) {
        return {
          key,
          label,
          display: "",
          kind: "crypto" as const,
          length: 0,
          value,
          type: fieldType,
        };
      }
      if (s3Keys.has(key)) {
        return {
          key,
          label,
          display: String(value),
          kind: "s3" as const,
          length: Number(item[`${key}Length`] ?? 0),
          value,
          type: fieldType,
        };
      }
      return { key, label, display: "", kind: "plain" as const, length: 0, value, type: fieldType };
    });
});

// plainHeader / plainItem 构造 plain 字段的表头与单字段行数据，供 AdaptiveCell 按列表页方式渲染
const plainHeader = (field: DetailField): TableHeader => {
  return { key: field.key, title: field.label, type: field.type };
};

const plainItem = (field: DetailField): Record<string, unknown> => {
  return { [field.key]: field.value };
};

// viewPlaintextAction 统一入口：交共享明文对话框解密展示；model 段为流量类型 slug
// （与后端权限点 plaintext:{crypto|s3}:{model}:{field} 一致），loading 反馈给操作按钮
const viewPlaintextAction = async (field: DetailField): Promise<void> => {
  const model = trafficType.value ?? "";
  plaintextLoading.value = field.key;
  try {
    await plaintextDialogRef.value?.open(field, model, field.label);
  } finally {
    plaintextLoading.value = "";
  }
};
</script>

<style scoped>
/* 字段名列宽提示：固定标签列宽度，值列自适应占满剩余空间 */
.field-label-col {
  width: 180px;
}

/* 详情值单元格：长文本（如 base64 报文、请求头）自动换行，避免撑破紧凑对话框 */
.detail-value {
  word-break: break-all;
}
</style>
