<template>
  <search-table
    ref="sampleTableRef"
    :list-request="tagRuleSampleListGet"
    :options-request="tagRuleSampleListOptions"
    :actions="getSampleActions"
    model-name="tagRuleSample"
  />

  <confirm-dialog
    ref="deleteConfirmRef"
    :title="$t('model.tagRuleSample.name')"
    prepend-icon="mdi-tag-remove"
    @confirm="itemDeleteAction"
  >
    <template #body="{ proxy }">
      <confirm-alert
        :action="$t('action.delete')"
        :text="
          $t(`notice.deleteConfirm`, {
            category: $t(`model.tagRuleSample.name`),
            key: proxy.name,
          })
        "
      />
    </template>
  </confirm-dialog>

  <temp-editable-dialog
    ref="editDialogRef"
    :title="$t('model.tagRuleSample.name')"
    :confirm-disabled="!updateForm.meta.value.valid"
    prepend-icon="mdi-tag-edit"
    @confirm="itemUpdateAction"
  >
    <template #body="{ proxy, editable }">
      <v-form>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="editName"
              v-bind="editNameProps"
              :label="$t('model.tagRuleSample.field.name')"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              :model-value="proxy.model"
              :label="$t('model.tagRuleSample.field.model')"
              disabled
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="editDescription"
              v-bind="editDescriptionProps"
              :label="$t('model.tagRuleSample.field.description')"
              :disabled="!editable"
              rows="2"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              :model-value="proxy.inputJson"
              :label="$t('model.tagRuleSample.field.input')"
              readonly
              rows="12"
              class="monospace"
            />
          </v-col>
        </v-row>
      </v-form>
    </template>
  </temp-editable-dialog>

  <model-detail-dialog
    v-model="detailDialog"
    :title="$t('model.tagRuleSample.name')"
    :fields="detailFields"
    :plaintext-loading="plaintextLoading"
    @view-plaintext="viewPlaintextAction"
  />

  <!-- 明文查看对话框（共享组件：加密/S3 字段解密展示与下载） -->
  <plaintext-dialog ref="plaintextDialogRef" />
</template>

<script setup lang="ts" name="tag-rule-sample-panel">
import { DateTime } from "luxon";
import { useForm } from "vee-validate";

import {
  tagRuleSampleDelete,
  tagRuleSampleDetailGet,
  tagRuleSampleListGet,
  tagRuleSampleListOptions,
  tagRuleSampleUpdate,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import { TrafficCryptoFields, TrafficS3Fields, trafficTypeFromModelKey } from "@/apis/traffic";
import type { DetailField } from "@/components/@types/detail";
import type { ItemActions } from "@/components/@types/table";
import type { TagRuleSampleDetailOut } from "@/pages/[language]/admin/@types";
import { updateTagRuleSampleSchema } from "@/schemas/tagRuleSample";
import { useAppStore } from "@/stores/app";
import { vuetifyCfg } from "@/utils/form";
import { localeDatetimeFormat } from "@/utils/i18n";
import { modelFieldTitle, modelTypeKeyToI18nKey } from "@/utils/modelField";

const DATETIME_FIELD_KEYS = new Set(["timestamp", "createdAt"]);

const appStore = useAppStore();
const sampleTableRef = useTemplateRef("sampleTableRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const editDialogRef = useTemplateRef("editDialogRef");

const detailDialog = ref(false);
// 详情数据来自 detail 接口（含 input 文档），类型未知按记录处理
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailItem = ref<Record<string, any> | null>(null);

const plaintextDialogRef = useTemplateRef("plaintextDialogRef");
const plaintextLoading = ref("");

let editSampleId = 0;

const updateForm = useForm({ validationSchema: updateTagRuleSampleSchema });
const [editName, editNameProps] = updateForm.defineField("name", vuetifyCfg);
const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);

// ---- detail ----
const formatValue = (value: unknown): string => {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
};

// detailFields 样例详情字段：遍历 input 文档按模型类型翻译字段名；
// 加密/S3 字段经模型类型键逆映射识别后提供明文查看，其余字段对象值
// JSON 序列化，timestamp/createdAt 等 ISO 串本地化格式化
const detailFields = computed<DetailField[]>(() => {
  const item = detailItem.value;
  if (!item) {
    return [];
  }
  const modelKey = modelTypeKeyToI18nKey(String(item.model));
  // 加密/S3 字段识别：model 为 tagengine 类型键，逆映射回流量类型后查字段清单
  const trafficType = trafficTypeFromModelKey(String(item.model));
  const cryptoKeys = new Set(trafficType ? (TrafficCryptoFields[trafficType] ?? []) : []);
  const s3Keys = new Set(trafficType ? (TrafficS3Fields[trafficType] ?? []) : []);
  const input = (item.input ?? {}) as Record<string, unknown>;
  return Object.entries(input).map(([key, value]) => {
    const label = modelFieldTitle(modelKey, key);
    // 加密字段：密文须为非空字符串才提供明文查看（预置样例空对象 {} 按普通值展示）
    if (cryptoKeys.has(key) && typeof value === "string" && value !== "") {
      return { key, label, display: "", kind: "crypto" as const, length: 0, value };
    }
    // S3 字段：对象键须为非空字符串才提供明文查看；长度取 input 中 {key}Length 字段
    if (s3Keys.has(key) && typeof value === "string" && value !== "") {
      return {
        key,
        label,
        display: value,
        kind: "s3" as const,
        length: Number(input[`${key}Length`] ?? 0),
        value,
      };
    }
    let display = formatValue(value);
    if (DATETIME_FIELD_KEYS.has(key) && typeof value === "string") {
      const dt = DateTime.fromISO(value);
      if (dt.isValid) {
        display = localeDatetimeFormat(dt);
      }
    }
    return { key, label, display, kind: "plain" as const, length: 0, value };
  });
});

// viewPlaintextAction 明文查看统一入口：model 参数用流量类型名（与后端权限点
// plaintext:{crypto|s3}:{model}:{field} 对应），loading 反馈给详情行按钮
const viewPlaintextAction = async (field: DetailField): Promise<void> => {
  const trafficType = trafficTypeFromModelKey(String(detailItem.value?.model ?? ""));
  if (!trafficType) {
    return;
  }
  plaintextLoading.value = field.key;
  try {
    await plaintextDialogRef.value?.open(field, trafficType, field.label);
  } finally {
    plaintextLoading.value = "";
  }
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openDetailAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<TagRuleSampleDetailOut>(
    tagRuleSampleDetailGet(item.id),
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

// ---- list actions ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getSampleActions = (item: Record<string, any>): ItemActions => {
  return {
    viewDetail: () => void openDetailAction(item),
    edit: editAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => deleteConfirmRef.value?.open(i),
  };
};

// ---- edit ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const editAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<TagRuleSampleDetailOut>(
    tagRuleSampleDetailGet(item.id),
    async (rsp) => {
      editSampleId = rsp.data.id;
      updateForm.resetField("name", { value: rsp.data.name });
      updateForm.resetField("description", { value: rsp.data.description });
      editDialogRef.value?.open({
        ...rsp.data,
        inputJson: JSON.stringify(rsp.data.input ?? {}, null, 2),
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
};

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      tagRuleSampleUpdate(editSampleId, {
        name: values.name,
        description: values.description,
      }),
      async (rsp) => {
        await sampleTableRef.value?.refresh();
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
  })();
};

// ---- delete ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDeleteAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    tagRuleSampleDelete(item.id),
    async (rsp) => {
      await sampleTableRef.value?.refresh();
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
};
</script>

<style scoped>
.monospace :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
}
</style>
