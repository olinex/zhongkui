<template>
  <!-- 作用域提醒：注册表为全局配置，常驻于列表上方，避免用户误认为随命名空间隔离 -->
  <div class="px-4 pt-4">
    <v-alert type="info">
      {{ $t("page.llmModelAdmin.scopeHint") }}
    </v-alert>
  </div>

  <search-table
    ref="searchTableRef"
    :list-request="llmModelListGet"
    :options-request="llmModelListOptions"
    :actions="getActions"
    :batch-actions="getBatchActions"
    :create="openCreateDialogAction"
    :project-fields="projectFields"
    model-name="llmModel"
  >
    <!-- 维度为只读 schema 属性：未探测到（0）时以占位符展示，避免误认为可编辑配置 -->
    <template #cell-dimensions="{ item }">
      <span v-if="item.dimensions" class="text-on-surface">{{ item.dimensions }}</span>
      <span v-else class="zk-hint">—</span>
    </template>
  </search-table>

  <confirm-dialog
    ref="deleteConfirmRef"
    :title="$t('model.llmModel.name')"
    prepend-icon="mdi-robot-outline"
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
          $t('notice.deleteConfirm', {
            category: $t('model.llmModel.name'),
            key: proxy.name,
          })
        "
      />
    </template>
  </confirm-dialog>

  <confirm-dialog
    ref="toggleConfirmRef"
    :title="$t('model.llmModel.name')"
    prepend-icon="mdi-toggle-switch"
    @confirm="itemToggleAction"
  >
    <template #body="{ proxy }">
      <confirm-alert
        :action="$t(toggleTarget === true ? 'action.activate' : 'action.deactivate')"
        :text="
          $t(toggleTarget === true ? 'notice.enableConfirm' : 'notice.disableConfirm', {
            category: $t('model.llmModel.name'),
            key: proxy.name,
          })
        "
      />
    </template>
  </confirm-dialog>

  <temp-editable-dialog
    ref="createDialogRef"
    :title="$t('model.llmModel.name')"
    :confirm-disabled="!createForm.meta.value.valid"
    force-editable
    prepend-icon="mdi-robot-outline"
    @confirm="createAction"
  >
    <template #body>
      <v-form>
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model="createName"
              v-bind="createNameProps"
              :label="$t('model.llmModel.field.name')"
            />
          </v-col>
          <v-col cols="6">
            <v-select
              v-model="createKind"
              v-bind="createKindProps"
              :items="kindOptions"
              :label="$t('model.llmModel.field.kind')"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="createBaseURL"
              v-bind="createBaseURLProps"
              :label="$t('model.llmModel.field.baseURL')"
              :hint="$t('page.llmModelAdmin.baseURLHint')"
              persistent-hint
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="createApiKey"
              v-bind="createApiKeyProps"
              :label="$t('model.llmModel.field.apiKey')"
              :type="createApiKeyRevealed ? 'text' : 'password'"
              :append-inner-icon="createApiKeyRevealed ? 'mdi-eye' : 'mdi-eye-off'"
              @click:append-inner="createApiKeyRevealed = !createApiKeyRevealed"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model="createModel"
              v-bind="createModelProps"
              :label="$t('model.llmModel.field.model')"
            />
          </v-col>
          <v-col cols="6">
            <v-number-input
              v-model="createTimeoutSeconds"
              v-bind="createTimeoutSecondsProps"
              :label="$t('model.llmModel.field.timeoutSeconds')"
              :precision="0"
              :min="1"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-number-input
              v-model="createTemperature"
              v-bind="createTemperatureProps"
              :label="$t('model.llmModel.field.temperature')"
              :step="0.1"
              :min="0"
              :max="2"
            />
          </v-col>
          <v-col cols="6">
            <v-number-input
              v-model="createMaxTokens"
              v-bind="createMaxTokensProps"
              :label="$t('model.llmModel.field.maxTokens')"
              :precision="0"
              :min="1"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-switch
              v-model="createEnabled"
              v-bind="createEnabledProps"
              :label="$t('model.llmModel.field.enabled')"
              color="primary"
            />
          </v-col>
        </v-row>
        <!-- 索引维度由后端在绑定 embedding 档位时探测写回，前端以 alert 显式提示 -->
        <v-row v-if="createKind === LLM_MODEL_KIND_EMBEDDING" dense>
          <v-col cols="12">
            <v-alert type="info">
              {{ $t("page.llmModelAdmin.dimensionsHint") }}
            </v-alert>
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="createDescription"
              v-bind="createDescriptionProps"
              :label="$t('model.llmModel.field.description')"
              rows="1"
            />
          </v-col>
        </v-row>
      </v-form>
    </template>
  </temp-editable-dialog>

  <temp-editable-dialog
    ref="editDialogRef"
    :title="$t('model.llmModel.name')"
    :confirm-disabled="!updateForm.meta.value.valid"
    prepend-icon="mdi-robot-outline"
    @confirm="itemUpdateAction"
  >
    <template #body="{ proxy, editable }">
      <v-form>
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model="updateName"
              v-bind="updateNameProps"
              :label="$t('model.llmModel.field.name')"
              :disabled="!editable"
            />
          </v-col>
          <v-col cols="6">
            <v-text-field
              :model-value="enumLabelOf(EnumClass.llmModelKind, proxy.kind)"
              :label="$t('model.llmModel.field.kind')"
              disabled
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="updateBaseURL"
              v-bind="updateBaseURLProps"
              :label="$t('model.llmModel.field.baseURL')"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="updateApiKey"
              v-bind="updateApiKeyProps"
              :label="$t('model.llmModel.field.apiKey')"
              :type="editApiKeyRevealed ? 'text' : 'password'"
              :append-inner-icon="editApiKeyRevealed ? 'mdi-eye' : 'mdi-eye-off'"
              :hint="$t('page.llmModelAdmin.apiKeyKeepHint')"
              persistent-hint
              :disabled="!editable"
              @click:append-inner="editApiKeyRevealed = !editApiKeyRevealed"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model="updateModel"
              v-bind="updateModelProps"
              :label="$t('model.llmModel.field.model')"
              :disabled="!editable"
            />
          </v-col>
          <v-col cols="6">
            <v-number-input
              v-model="updateTimeoutSeconds"
              v-bind="updateTimeoutSecondsProps"
              :label="$t('model.llmModel.field.timeoutSeconds')"
              :precision="0"
              :min="1"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-number-input
              v-model="updateTemperature"
              v-bind="updateTemperatureProps"
              :label="$t('model.llmModel.field.temperature')"
              :step="0.1"
              :min="0"
              :max="2"
              :disabled="!editable"
            />
          </v-col>
          <v-col cols="6">
            <v-number-input
              v-model="updateMaxTokens"
              v-bind="updateMaxTokensProps"
              :label="$t('model.llmModel.field.maxTokens')"
              :precision="0"
              :min="1"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-switch
              v-model="updateEnabled"
              v-bind="updateEnabledProps"
              :label="$t('model.llmModel.field.enabled')"
              color="primary"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <!-- 索引维度由后端在绑定 embedding 档位时探测写回，前端以 alert 显式提示 -->
        <v-row v-if="proxy.kind === LLM_MODEL_KIND_EMBEDDING" dense>
          <v-col cols="12">
            <v-alert type="info">
              {{ $t("page.llmModelAdmin.dimensionsHint") }}
            </v-alert>
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="updateDescription"
              v-bind="updateDescriptionProps"
              :label="$t('model.llmModel.field.description')"
              rows="1"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
      </v-form>
    </template>
  </temp-editable-dialog>
</template>

<script setup lang="ts" name="llm-model-panel">
import { useForm } from "vee-validate";

import {
  llmModelBatchDelete,
  llmModelCreate,
  llmModelDetailGet,
  llmModelListGet,
  llmModelListOptions,
  llmModelTest,
  llmModelUpdate,
} from "@/apis/llm";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { LlmModelDetailOut, LlmModelTestOut } from "@/apis/@types/llm";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { createLlmModelSchema, updateLlmModelSchema } from "@/schemas/llm";
import { useAppStore } from "@/stores/app";
import { vuetifyCfg } from "@/utils/form";
import { EnumClass, enumLabelOf, enumOptionsOf, ensureEnum } from "@/utils/enums";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema ----
const createForm = useForm({ validationSchema: createLlmModelSchema() });
const updateForm = useForm({ validationSchema: updateLlmModelSchema() });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createKind, createKindProps] = createForm.defineField("kind", vuetifyCfg);
const [createBaseURL, createBaseURLProps] = createForm.defineField("baseURL", vuetifyCfg);
const [createApiKey, createApiKeyProps] = createForm.defineField("apiKey", vuetifyCfg);
const [createModel, createModelProps] = createForm.defineField("model", vuetifyCfg);
const [createTimeoutSeconds, createTimeoutSecondsProps] = createForm.defineField(
  "timeoutSeconds",
  vuetifyCfg,
);
const [createTemperature, createTemperatureProps] = createForm.defineField(
  "temperature",
  vuetifyCfg,
);
const [createMaxTokens, createMaxTokensProps] = createForm.defineField("maxTokens", vuetifyCfg);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const [updateName, updateNameProps] = updateForm.defineField("name", vuetifyCfg);
const [updateBaseURL, updateBaseURLProps] = updateForm.defineField("baseURL", vuetifyCfg);
const [updateApiKey, updateApiKeyProps] = updateForm.defineField("apiKey", vuetifyCfg);
const [updateModel, updateModelProps] = updateForm.defineField("model", vuetifyCfg);
const [updateTimeoutSeconds, updateTimeoutSecondsProps] = updateForm.defineField(
  "timeoutSeconds",
  vuetifyCfg,
);
const [updateTemperature, updateTemperatureProps] = updateForm.defineField(
  "temperature",
  vuetifyCfg,
);
const [updateMaxTokens, updateMaxTokensProps] = updateForm.defineField("maxTokens", vuetifyCfg);
const [updateEnabled, updateEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);
const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");

// ---- 凭据明文展示开关（创建/编辑各自独立，避免互相串扰）----
const createApiKeyRevealed = ref(false);
const editApiKeyRevealed = ref(false);

// kind 下拉选项来自统一枚举注册表（label 由后端按当前语言返回）
const kindOptions = computed(() => enumOptionsOf(EnumClass.llmModelKind));
// LLM_MODEL_KIND_EMBEDDING 向量化模型在 llmModelKind 枚举中的值（仅该类型模型有索引维度）
const LLM_MODEL_KIND_EMBEDDING = "embedding";

onMounted(() => {
  void ensureEnum(EnumClass.llmModelKind);
});

// ---- 创建对话框 ----
const openCreateDialogAction = (): void => {
  const defaults = {
    name: "",
    kind: "",
    baseURL: "",
    apiKey: "",
    model: "",
    timeoutSeconds: 60,
    temperature: 0.2,
    maxTokens: 4096,
    enabled: true,
    description: "",
  };
  createForm.resetField("name", { value: defaults.name });
  createForm.resetField("kind", { value: defaults.kind });
  createForm.resetField("baseURL", { value: defaults.baseURL });
  createForm.resetField("apiKey", { value: defaults.apiKey });
  createForm.resetField("model", { value: defaults.model });
  createForm.resetField("timeoutSeconds", { value: defaults.timeoutSeconds });
  createForm.resetField("temperature", { value: defaults.temperature });
  createForm.resetField("maxTokens", { value: defaults.maxTokens });
  createForm.resetField("enabled", { value: defaults.enabled });
  createForm.resetField("description", { value: defaults.description });
  createApiKeyRevealed.value = false;
  createDialogRef.value?.open(defaults);
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    llmModelCreate({
      // 单行文本提交前去除首尾空白；apiKey 为凭据，按原样提交不做 trim
      name: (values.name as string).trim(),
      kind: values.kind,
      baseURL: (values.baseURL as string).trim(),
      apiKey: values.apiKey,
      model: (values.model as string).trim(),
      timeoutSeconds: values.timeoutSeconds,
      temperature: values.temperature,
      maxTokens: values.maxTokens,
      enabled: values.enabled,
      description: values.description,
    }),
    async (rsp) => {
      await searchTableRef.value?.refresh();
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
});

// ---- 编辑对话框 ----
let editModelId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<LlmModelDetailOut>(
    llmModelDetailGet(item.id as number),
    async (rsp) => {
      editModelId = rsp.data.id;
      updateForm.resetField("name", { value: rsp.data.name });
      updateForm.resetField("baseURL", { value: rsp.data.baseURL });
      // 凭据不回填密文：留空即表示沿用原凭据
      updateForm.resetField("apiKey", { value: "" });
      updateForm.resetField("model", { value: rsp.data.model });
      updateForm.resetField("timeoutSeconds", { value: rsp.data.timeoutSeconds });
      updateForm.resetField("temperature", { value: rsp.data.temperature });
      updateForm.resetField("maxTokens", { value: rsp.data.maxTokens });
      updateForm.resetField("enabled", { value: rsp.data.enabled });
      updateForm.resetField("description", { value: rsp.data.description });
      editApiKeyRevealed.value = false;
      editDialogRef.value?.open(rsp.data);
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
      llmModelUpdate(editModelId, {
        name: (values.name as string).trim(),
        baseURL: (values.baseURL as string).trim(),
        // 空串表示沿用原凭据（后端按留空处理）
        apiKey: values.apiKey,
        model: (values.model as string).trim(),
        timeoutSeconds: values.timeoutSeconds,
        temperature: values.temperature,
        maxTokens: values.maxTokens,
        enabled: values.enabled,
        description: values.description,
      }),
      async (rsp) => {
        await searchTableRef.value?.refresh();
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

// ---- 连通性测试 ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemTestAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<LlmModelTestOut>(
    llmModelTest({ id: item.id as number }),
    async (rsp) => {
      appStore.notify({
        text: rsp.data.success
          ? i18n.global.t("notice.llmModel.testSuccess")
          : `${i18n.global.t("notice.llmModel.testFailed")}: ${rsp.data.message}`,
        timeout: 5000,
        color: rsp.data.success ? "success" : "error",
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

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影（name 定位、enabled 启停、kind 展示），
// 避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "kind",
  "enabled",
  ...visible,
];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    test: itemTestAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => {
      batchState.value = null;
      deleteConfirmRef.value?.open(i);
    },
  };
  switch (item.enabled) {
    case false:
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      actions.activate = (i: Record<string, any>) => toggleAction(i, true);
      break;
    case true:
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      actions.deactivate = (i: Record<string, any>) => toggleAction(i, false);
      break;
  }
  return actions;
};

// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时以 error 提示失败数，否则沿用成功摘要
const applyBatchResult = (summary: string, result: BatchResultOut): void => {
  if (result.failed.length > 0) {
    appStore.notify({
      text: i18n.global.t("notice.batchResult", {
        succeeded: result.succeeded,
        failed: result.failed.length,
      }),
      timeout: 5000,
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
    llmModelBatchDelete(ids),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      searchTableRef.value?.clearSelection?.();
      await searchTableRef.value?.refresh();
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
    llmModelBatchDelete(proxy.id as number),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      await searchTableRef.value?.refresh();
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

// ---- 启用/禁用 ----
// 后端无独立的启停接口：读取详情后以整表单更新翻转载入（凭据留空即沿用原密文）
const toggleTarget = ref<boolean | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const toggleAction = (item: Record<string, any>, enabled: boolean): void => {
  toggleTarget.value = enabled;
  toggleConfirmRef.value?.open(item);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemToggleAction = async (proxy: Record<string, any>): Promise<void> => {
  if (toggleTarget.value === null) {
    return;
  }
  const target = toggleTarget.value;
  await getResponse<LlmModelDetailOut>(
    llmModelDetailGet(proxy.id as number),
    async (rsp) => {
      const detail = rsp.data;
      await getSimpleResponse(
        llmModelUpdate(detail.id, {
          name: detail.name,
          baseURL: detail.baseURL,
          apiKey: "",
          model: detail.model,
          timeoutSeconds: detail.timeoutSeconds,
          temperature: detail.temperature,
          maxTokens: detail.maxTokens,
          enabled: target,
          description: detail.description,
        }),
        async (rsp2) => {
          await searchTableRef.value?.refresh();
          appStore.notify({
            text: rsp2.summary,
            timeout: 3000,
            color: "success",
          });
        },
        async (rsp2) => {
          appStore.notify({
            text: rsp2.summary,
            timeout: 3000,
            color: "error",
          });
        },
      );
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
