<template>
  <search-table
    ref="sampleTableRef"
    :list-request="tagRuleSampleListGet"
    :options-request="tagRuleSampleListOptions"
    :actions="getSampleActions"
    :batch-actions="getBatchActions"
    :project-fields="projectFields"
    model-name="tagRuleSample"
  >
    <!-- 目标模型列显示可读名称（与表单下拉一致），而非原始类型键 -->
    <template #cell-model="{ value }">
      {{
        enumLabelOf(EnumClass.tagRuleModel, String(value ?? "")) || $t("model.__base.value.blank")
      }}
    </template>
  </search-table>

  <confirm-dialog
    ref="deleteConfirmRef"
    :title="$t('model.tagRuleSample.name')"
    prepend-icon="mdi-tag-remove"
    @confirm="itemDeleteAction"
  >
    <template #body="{ proxy }">
      <confirm-alert
        v-if="batchState"
        :action="$t('action.delete')"
        :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
      />
      <confirm-alert
        v-else
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
              class="font-mono"
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
import { useForm } from "vee-validate";

import {
  tagRuleSampleDelete,
  tagRuleSampleDetailGet,
  tagRuleSampleListGet,
  tagRuleSampleListOptions,
  tagRuleSampleUpdate,
} from "@/apis/tagEvent";
import {
  TrafficCryptoFields,
  TrafficFieldTypes,
  TrafficS3Fields,
  trafficTypeFromModelKey,
} from "@/apis/traffic";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { DetailField } from "@/components/@types/detail";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import type { TagRuleSampleDetailOut } from "@/pages/[language]/tag-event/@types";
import { updateTagRuleSampleSchema } from "@/schemas/tagRuleSample";
import { useAppStore } from "@/stores/app";
import { BackendDataType } from "@/types";
import { EnumClass, enumLabelOf } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";
import { modelFieldTitle, modelTypeKeyToI18nKey } from "@/utils/modelField";

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
// detailFields 样例详情字段：遍历 input 文档按模型类型翻译字段名；
// 加密/S3 字段经模型类型键逆映射识别后提供明文查看，其余字段按流量模型类型
// （TrafficFieldTypes）由 AdaptiveCell 以列表页方式渲染
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
  // 字段渲染类型：按流量类型查 TrafficFieldTypes，未知类型/字段回退 STRING
  const fieldTypes = trafficType ? TrafficFieldTypes[trafficType] : {};
  const input = (item.input ?? {}) as Record<string, unknown>;
  return Object.entries(input).map(([key, value]) => {
    const label = modelFieldTitle(modelKey, key);
    const fieldType = fieldTypes[key] ?? BackendDataType.STRING;
    // 加密字段：密文须为非空字符串才提供明文查看（预置样例空对象 {} 按普通值展示）
    if (cryptoKeys.has(key) && typeof value === "string" && value !== "") {
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
    // S3 字段：对象键须为非空字符串才提供明文查看；长度取 input 中 {key}Length 字段
    if (s3Keys.has(key) && typeof value === "string" && value !== "") {
      return {
        key,
        label,
        display: value,
        kind: "s3" as const,
        length: Number(input[`${key}Length`] ?? 0),
        value,
        type: fieldType,
      };
    }
    return { key, label, display: "", kind: "plain" as const, length: 0, value, type: fieldType };
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
// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "name", ...visible];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getSampleActions = (item: Record<string, any>): ItemActions => {
  return {
    viewDetail: () => void openDetailAction(item),
    edit: editAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => {
      batchState.value = null;
      deleteConfirmRef.value?.open(i);
    },
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
        // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
        name: (values.name as string).trim(),
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

// ---- 批量删除 ----
// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时以 error 提示失败数，否则沿用成功摘要
const applyBatchResult = (summary: string, result: BatchResultOut): void => {
  if (result.failed.length > 0) {
    appStore.notify({
      text: i18n.global.t("notice.batchResult", {
        succeeded: result.succeeded,
        failed: result.failed.length,
      }),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  appStore.notify({
    text: summary,
    timeout: 3000,
    color: "success",
  });
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type BatchState = { items: Record<string, any>[] };
const batchState = ref<BatchState | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    delete: () => {
      batchState.value = { items };
      deleteConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    tagRuleSampleDelete(ids),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      sampleTableRef.value?.clearSelection?.();
      await sampleTableRef.value?.refresh();
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDeleteAction = async (proxy: Record<string, any>): Promise<void> => {
  const state = batchState.value;
  if (state) {
    batchState.value = null;
    await batchDeleteAction(state.items);
    return;
  }
  await getResponse<BatchResultOut>(
    tagRuleSampleDelete(proxy.id),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      await sampleTableRef.value?.refresh();
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
