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
    @view-plaintext="viewApiKeyPlaintext"
  >
    <!-- 维度为只读 schema 属性：未探测到（0）时以占位符展示，避免误认为可编辑配置 -->
    <template #cell-dimensions="{ item }">
      <span v-if="item.dimensions">{{ item.dimensions }}</span>
      <span v-else class="zk-hint">{{ INVALID_PLACEHOLDER }}</span>
    </template>

    <!-- token 数量列按 K/M 单位展示：与输入组件、只读用量同一套换算口径（utils/token） -->
    <template #cell-maxTokens="{ item }">
      {{ formatTokenCount(item.maxTokens) }}
    </template>

    <!-- 上下文容量仅 chat 模型有意义：embedding 为 0，以占位符展示，避免误认为容量为 0 -->
    <template #cell-contextTokens="{ item }">
      <span v-if="item.contextTokens">{{ formatTokenCount(item.contextTokens) }}</span>
      <span v-else class="zk-hint">{{ INVALID_PLACEHOLDER }}</span>
    </template>

    <!-- API 模式仅 chat 模型有意义：缺省（历史模型未下发）按 chat 展示 -->
    <template #cell-apiMode="{ item }">
      {{ apiModeLabel(item.apiMode) }}
    </template>

    <!-- 凭据为加密列（后端 kind=crypto）：由 SearchTable 自动渲染加密单元格，点击经事件查看明文 -->

    <!-- 启用态：行内开关就地改写（后端无独立启停接口，内部走「读详情 + 整表单更新」） -->
    <template #cell-enabled="{ item }">
      <enabled-switch-cell
        v-model="item.enabled"
        :request="(enabled: boolean) => setEnabledRequest(item, enabled)"
      />
    </template>
  </search-table>

  <!-- 共享明文对话框：查看模型端点凭据明文 -->
  <plaintext-dialog ref="plaintextDialogRef" />

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
        <!-- 索引维度由后端在绑定 embedding 档位时探测写回，前端以 alert 显式提示，置于表单开头避免被下方字段淹没 -->
        <v-row v-if="createKind === LLM_MODEL_KIND_EMBEDDING" density="compact">
          <v-col cols="12">
            <v-alert type="info" class="mb-3">
              {{ $t("page.llmModelAdmin.dimensionsHint") }}
            </v-alert>
          </v-col>
        </v-row>
        <v-row density="compact">
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
        <!-- API 模式仅对话模型可选：chat 为默认，responses 走 Agentic 消息协议 -->
        <v-row v-if="createKind === LLM_MODEL_KIND_CHAT" density="compact">
          <v-col cols="6">
            <v-select
              v-model="createApiMode"
              v-bind="createApiModeProps"
              :items="apiModeOptions"
              :label="$t('model.llmModel.field.apiMode')"
              :hint="$t('page.llmModelAdmin.apiModeHint')"
              persistent-hint
            />
          </v-col>
        </v-row>
        <v-row density="compact">
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
        <v-row density="compact">
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
        <v-row density="compact">
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
        <v-row density="compact">
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
            <token-count-field
              v-model="createMaxTokens"
              v-bind="createMaxTokensProps"
              :label="$t('model.llmModel.field.maxTokens')"
              :min="1"
            />
          </v-col>
        </v-row>
        <v-row density="compact" class="align-center">
          <v-col cols="6">
            <v-switch
              v-model="createEnabled"
              v-bind="createEnabledProps"
              :label="$t('model.llmModel.field.enabled')"
              color="primary"
            />
          </v-col>
          <!-- 上下文容量仅 chat 模型需要：agent 据此计算上下文占用比例并判定自动压缩 -->
          <v-col v-if="createKind === LLM_MODEL_KIND_CHAT" cols="6">
            <token-count-field
              v-model="createContextTokens"
              v-bind="createContextTokensProps"
              :label="$t('model.llmModel.field.contextTokens')"
              :min="1"
            />
          </v-col>
        </v-row>
        <v-row density="compact">
          <v-col cols="12">
            <v-textarea
              v-model="createDescription"
              v-bind="createDescriptionProps"
              :label="$t('model.llmModel.field.description')"
              rows="3"
            />
          </v-col>
        </v-row>
        <!-- 高级配置（额外请求字段、思考开关）仅对话模型适用，且均为可选高级项：
             默认收起，避免长说明文本与非常用字段占据表单主视线 -->
        <div v-if="createKind === LLM_MODEL_KIND_CHAT" class="zk-panel">
          <v-expansion-panels v-model="createAdvancedPanel">
            <v-expansion-panel value="advanced">
              <expansion-panel-title
                icon="mdi-tune-variant"
                :title="$t('page.llmModelAdmin.advancedTitle')"
                :subtitle="$t('page.llmModelAdmin.advancedHint')"
              />
              <v-expansion-panel-text>
                <v-row density="compact">
                  <v-col cols="12">
                    <v-text-field
                      v-model="createThinkingField"
                      v-bind="createThinkingFieldProps"
                      :label="$t('model.llmModel.field.thinkingField')"
                      :hint="$t('page.llmModelAdmin.thinkingHint')"
                      persistent-hint
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-text-field
                      v-model="createThinkingOnValue"
                      v-bind="createThinkingOnValueProps"
                      :label="$t('model.llmModel.field.thinkingOnValue')"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="createThinkingOffValue"
                      v-bind="createThinkingOffValueProps"
                      :label="$t('model.llmModel.field.thinkingOffValue')"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="12">
                    <json-editor
                      v-model="createExtraBody"
                      :label="$t('model.llmModel.field.extraBody')"
                      :hint="$t('page.llmModelAdmin.extraBodyHint')"
                      :error-message="createForm.errors.value.extraBody"
                    />
                  </v-col>
                </v-row>
              </v-expansion-panel-text>
            </v-expansion-panel>
          </v-expansion-panels>
        </div>
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
        <!-- 索引维度由后端在绑定 embedding 档位时探测写回，前端以 alert 显式提示，置于表单开头避免被下方字段淹没 -->
        <v-row v-if="proxy.kind === LLM_MODEL_KIND_EMBEDDING" density="compact">
          <v-col cols="12">
            <v-alert type="info" class="mb-3">
              {{ $t("page.llmModelAdmin.dimensionsHint") }}
            </v-alert>
          </v-col>
        </v-row>
        <v-row density="compact">
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
        <!-- API 模式仅对话模型可选：chat 为默认，responses 走 Agentic 消息协议 -->
        <v-row v-if="proxy.kind === LLM_MODEL_KIND_CHAT" density="compact">
          <v-col cols="6">
            <v-select
              v-model="updateApiMode"
              v-bind="updateApiModeProps"
              :items="apiModeOptions"
              :label="$t('model.llmModel.field.apiMode')"
              :hint="$t('page.llmModelAdmin.apiModeHint')"
              persistent-hint
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row density="compact">
          <v-col cols="12">
            <v-text-field
              v-model="updateBaseURL"
              v-bind="updateBaseURLProps"
              :label="$t('model.llmModel.field.baseURL')"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row density="compact">
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
        <v-row density="compact">
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
        <v-row density="compact">
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
            <token-count-field
              v-model="updateMaxTokens"
              v-bind="updateMaxTokensProps"
              :label="$t('model.llmModel.field.maxTokens')"
              :min="1"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row density="compact" class="align-center">
          <v-col cols="6">
            <v-switch
              v-model="updateEnabled"
              v-bind="updateEnabledProps"
              :label="$t('model.llmModel.field.enabled')"
              color="primary"
              :disabled="!editable"
            />
          </v-col>
          <!-- 上下文容量仅 chat 模型需要：agent 据此计算上下文占用比例并判定自动压缩 -->
          <v-col v-if="proxy.kind === LLM_MODEL_KIND_CHAT" cols="6">
            <token-count-field
              v-model="updateContextTokens"
              v-bind="updateContextTokensProps"
              :label="$t('model.llmModel.field.contextTokens')"
              :min="1"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row density="compact">
          <v-col cols="12">
            <v-textarea
              v-model="updateDescription"
              v-bind="updateDescriptionProps"
              :label="$t('model.llmModel.field.description')"
              rows="3"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <!-- 高级配置（额外请求字段、思考开关）仅对话模型适用，且均为可选高级项：
             默认收起，避免长说明文本与非常用字段占据表单主视线 -->
        <div v-if="proxy.kind === LLM_MODEL_KIND_CHAT" class="zk-panel">
          <v-expansion-panels v-model="updateAdvancedPanel">
            <v-expansion-panel value="advanced">
              <expansion-panel-title
                icon="mdi-tune-variant"
                :title="$t('page.llmModelAdmin.advancedTitle')"
                :subtitle="$t('page.llmModelAdmin.advancedHint')"
              />
              <v-expansion-panel-text>
                <v-row density="compact">
                  <v-col cols="12">
                    <v-text-field
                      v-model="updateThinkingField"
                      v-bind="updateThinkingFieldProps"
                      :label="$t('model.llmModel.field.thinkingField')"
                      :hint="$t('page.llmModelAdmin.thinkingHint')"
                      persistent-hint
                      :disabled="!editable"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="6">
                    <v-text-field
                      v-model="updateThinkingOnValue"
                      v-bind="updateThinkingOnValueProps"
                      :label="$t('model.llmModel.field.thinkingOnValue')"
                      :disabled="!editable"
                    />
                  </v-col>
                  <v-col cols="6">
                    <v-text-field
                      v-model="updateThinkingOffValue"
                      v-bind="updateThinkingOffValueProps"
                      :label="$t('model.llmModel.field.thinkingOffValue')"
                      :disabled="!editable"
                    />
                  </v-col>
                </v-row>
                <v-row density="compact">
                  <v-col cols="12">
                    <json-editor
                      v-model="updateExtraBody"
                      :label="$t('model.llmModel.field.extraBody')"
                      :hint="$t('page.llmModelAdmin.extraBodyHint')"
                      :error-message="updateForm.errors.value.extraBody"
                      :disabled="!editable"
                    />
                  </v-col>
                </v-row>
              </v-expansion-panel-text>
            </v-expansion-panel>
          </v-expansion-panels>
        </div>
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
import { BackendDataType } from "@/types";
import { INVALID_PLACEHOLDER } from "@/utils/base";
import { isEmptyCiphertext } from "@/utils/encrypted";
import { vuetifyCfg } from "@/utils/form";
import { EnumClass, enumLabelOf, enumOptionsOf, ensureEnum } from "@/utils/enums";
import i18n from "@/utils/i18n";
import { formatTokenCount } from "@/utils/token";

// ---- 表单校验 schema ----
const createForm = useForm({ validationSchema: createLlmModelSchema() });
const updateForm = useForm({ validationSchema: updateLlmModelSchema() });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createKind, createKindProps] = createForm.defineField("kind", vuetifyCfg);
const [createApiMode, createApiModeProps] = createForm.defineField("apiMode", vuetifyCfg);
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
const [createContextTokens, createContextTokensProps] = createForm.defineField(
  "contextTokens",
  vuetifyCfg,
);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
const [createExtraBody] = createForm.defineField("extraBody", vuetifyCfg);
const [createThinkingField, createThinkingFieldProps] = createForm.defineField(
  "thinkingField",
  vuetifyCfg,
);
const [createThinkingOnValue, createThinkingOnValueProps] = createForm.defineField(
  "thinkingOnValue",
  vuetifyCfg,
);
const [createThinkingOffValue, createThinkingOffValueProps] = createForm.defineField(
  "thinkingOffValue",
  vuetifyCfg,
);

const [updateName, updateNameProps] = updateForm.defineField("name", vuetifyCfg);
const [updateApiMode, updateApiModeProps] = updateForm.defineField("apiMode", vuetifyCfg);
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
const [updateContextTokens, updateContextTokensProps] = updateForm.defineField(
  "contextTokens",
  vuetifyCfg,
);
const [updateEnabled, updateEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);
const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);
const [updateExtraBody] = updateForm.defineField("extraBody", vuetifyCfg);
const [updateThinkingField, updateThinkingFieldProps] = updateForm.defineField(
  "thinkingField",
  vuetifyCfg,
);
const [updateThinkingOnValue, updateThinkingOnValueProps] = updateForm.defineField(
  "thinkingOnValue",
  vuetifyCfg,
);
const [updateThinkingOffValue, updateThinkingOffValueProps] = updateForm.defineField(
  "thinkingOffValue",
  vuetifyCfg,
);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");

// ---- 凭据明文展示开关（创建/编辑各自独立，避免互相串扰）----
const createApiKeyRevealed = ref(false);
const editApiKeyRevealed = ref(false);

// ---- 高级配置面板的展开态（创建 / 编辑各自独立）----
// 面板为手风琴式，v-model 必须是单值（空串 = 收起）；高级字段默认收起，不占表单主视线
const createAdvancedPanel = ref("");
const updateAdvancedPanel = ref("");

// kind 下拉选项来自统一枚举注册表（label 由后端按当前语言返回）
const kindOptions = computed(() => enumOptionsOf(EnumClass.llmModelKind));
// LLM_MODEL_KIND_EMBEDDING 向量化模型在 llmModelKind 枚举中的值（仅该类型模型有索引维度）
const LLM_MODEL_KIND_EMBEDDING = "embedding";
// LLM_MODEL_KIND_CHAT 对话模型在 llmModelKind 枚举中的值（仅该类型模型需填上下文容量）
const LLM_MODEL_KIND_CHAT = "chat";
// API_MODE_CHAT / API_MODE_RESPONSES 对话模型的 API 模式：Chat Completions / Responses API
const API_MODE_CHAT = "chat";
const API_MODE_RESPONSES = "responses";

// apiMode 下拉选项：取值由后端契约固定为 chat / responses，文案经 i18n 渲染
const apiModeOptions = computed(() => [
  { title: i18n.global.t("model.llmModel.apiMode.chat"), value: API_MODE_CHAT },
  { title: i18n.global.t("model.llmModel.apiMode.responses"), value: API_MODE_RESPONSES },
]);

// apiModeLabel 列表 / 详情展示：历史模型未下发该字段时按 chat 兜底
const apiModeLabel = (value: string | undefined): string =>
  i18n.global.t(
    value === API_MODE_RESPONSES
      ? "model.llmModel.apiMode.responses"
      : "model.llmModel.apiMode.chat",
  );

onMounted(() => {
  void ensureEnum(EnumClass.llmModelKind);
});

// ---- 创建对话框 ----
// 出厂默认值：每次打开都以此复位，因此上一次填写的内容不会残留
const createDefaults = {
  name: "",
  kind: "",
  // 对话模型默认走 Chat Completions
  apiMode: API_MODE_CHAT,
  baseURL: "",
  apiKey: "",
  model: "",
  timeoutSeconds: 60,
  temperature: 0.2,
  maxTokens: 4096,
  // 上下文容量仅 chat 模型需要：默认 0，选中 chat 后由用户填写正整数
  contextTokens: 0,
  enabled: true,
  description: "",
  // 高级请求配置与思考开关仅 chat 模型适用：默认留空
  extraBody: "",
  thinkingField: "",
  thinkingOnValue: "",
  thinkingOffValue: "",
};

// 复位是「准备本次打开的数据」的一部分，与填充默认值同在打开动作里完成。
// 用 resetForm 而非逐字段 resetField：前者原子重置值 + errors/touched/dirty/validated/submitCount，不留残影。
const openCreateDialogAction = (): void => {
  createForm.resetForm({ values: createDefaults });
  createApiKeyRevealed.value = false;
  createAdvancedPanel.value = "";
  createDialogRef.value?.open(createDefaults);
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    llmModelCreate({
      // 单行文本提交前去除首尾空白；apiKey 为凭据，按原样提交不做 trim
      name: (values.name as string).trim(),
      kind: values.kind,
      // API 模式仅对话模型有效：非对话模型提交 chat，由后端按缺省处理
      apiMode: values.kind === LLM_MODEL_KIND_CHAT ? values.apiMode : API_MODE_CHAT,
      baseURL: (values.baseURL as string).trim(),
      apiKey: values.apiKey,
      model: (values.model as string).trim(),
      timeoutSeconds: values.timeoutSeconds,
      temperature: values.temperature,
      maxTokens: values.maxTokens,
      // 上下文容量仅 chat 模型需要，embedding 模型提交 0（后端亦强制置 0）
      contextTokens: values.kind === LLM_MODEL_KIND_CHAT ? values.contextTokens : 0,
      enabled: values.enabled,
      description: values.description,
      // 高级请求配置与思考开关仅 chat 模型适用：embedding 模型一律提交空串
      extraBody: values.kind === LLM_MODEL_KIND_CHAT ? values.extraBody : "",
      thinkingField: values.kind === LLM_MODEL_KIND_CHAT ? values.thinkingField : "",
      thinkingOnValue: values.kind === LLM_MODEL_KIND_CHAT ? values.thinkingOnValue : "",
      thinkingOffValue: values.kind === LLM_MODEL_KIND_CHAT ? values.thinkingOffValue : "",
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
let editModelID = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<LlmModelDetailOut>(
    llmModelDetailGet(item.id as number),
    async (rsp) => {
      editModelID = rsp.data.id;
      // 复位与回填同属「准备本次打开的数据」：resetForm 原子重置值 + 校验状态，再写入本次详情
      updateForm.resetForm({
        values: {
          name: rsp.data.name,
          // 历史模型可能未下发 apiMode，按 chat 兜底
          apiMode: rsp.data.apiMode || API_MODE_CHAT,
          baseURL: rsp.data.baseURL,
          // 凭据不回填密文：留空即表示沿用原凭据
          apiKey: "",
          model: rsp.data.model,
          timeoutSeconds: rsp.data.timeoutSeconds,
          temperature: rsp.data.temperature,
          maxTokens: rsp.data.maxTokens,
          contextTokens: rsp.data.contextTokens,
          enabled: rsp.data.enabled,
          description: rsp.data.description,
          extraBody: rsp.data.extraBody,
          thinkingField: rsp.data.thinkingField,
          thinkingOnValue: rsp.data.thinkingOnValue,
          thinkingOffValue: rsp.data.thinkingOffValue,
        },
      });
      editApiKeyRevealed.value = false;
      updateAdvancedPanel.value = "";
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
      llmModelUpdate(editModelID, {
        name: (values.name as string).trim(),
        apiMode: values.apiMode || API_MODE_CHAT,
        baseURL: (values.baseURL as string).trim(),
        // 空串表示沿用原凭据（后端按留空处理）
        apiKey: values.apiKey,
        model: (values.model as string).trim(),
        timeoutSeconds: values.timeoutSeconds,
        temperature: values.temperature,
        maxTokens: values.maxTokens,
        contextTokens: values.contextTokens,
        enabled: values.enabled,
        description: values.description,
        // 高级请求配置与思考开关随更新请求发送（embedding 模型由详情回填后原样提交）
        extraBody: values.extraBody,
        thinkingField: values.thinkingField,
        thinkingOnValue: values.thinkingOnValue,
        thinkingOffValue: values.thinkingOffValue,
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

// ---- 凭据明文查看 ----
// viewApiKeyPlaintext 以列表下发的密文为凭据，经全局明文接口解密查看端点凭据；
// 明文不随列表下发，解密须经 plaintext:crypto:llm-model:apiKey 权限点授权
const viewApiKeyPlaintext = async (item: Record<string, unknown>): Promise<void> => {
  const ciphertext = item.apiKey;
  if (isEmptyCiphertext(ciphertext)) {
    return;
  }
  const label = i18n.global.t("model.llmModel.field.apiKey");
  await plaintextDialogRef.value?.open(
    {
      key: "apiKey",
      label,
      display: "",
      kind: "crypto",
      length: 0,
      value: ciphertext,
      type: BackendDataType.STRING,
    },
    "llm-model",
    label,
  );
};

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影（name 定位、enabled 启停、kind 展示），
// 避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "kind",
  "apiMode",
  "enabled",
  ...visible,
];

const getActions = (): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    test: itemTestAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => {
      batchState.value = null;
      deleteConfirmRef.value?.open(i);
    },
  };
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
// 后端无独立的启停接口：读取详情后以整表单更新翻转载入（凭据留空即沿用原密文）。
// 该复合请求交由 EnabledSwitchCell 行内开关执行：组件负责乐观更新、按行 pending 与失败回退。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const setEnabledRequest = async (item: Record<string, any>, enabled: boolean) => {
  // 详情接口返回统一信封 { code, summary, data }：axios 的 rsp.data 是整个信封，
  // 直接取它会把信封当详情（其 id 为 undefined），更新请求因此打成
  // /admin/llm/model/undefined/update 并返回 400。故需再取一层信封内的 data 才是详情载荷。
  const detail = (await llmModelDetailGet(item.id as number)).data.data as LlmModelDetailOut;
  return llmModelUpdate(detail.id, {
    name: detail.name,
    apiMode: detail.apiMode || API_MODE_CHAT,
    baseURL: detail.baseURL,
    apiKey: "",
    model: detail.model,
    timeoutSeconds: detail.timeoutSeconds,
    temperature: detail.temperature,
    maxTokens: detail.maxTokens,
    contextTokens: detail.contextTokens,
    enabled: enabled,
    description: detail.description,
    // 高级请求配置与思考开关随整表单更新原样回传，避免启停操作误清空
    extraBody: detail.extraBody,
    thinkingField: detail.thinkingField,
    thinkingOnValue: detail.thinkingOnValue,
    thinkingOffValue: detail.thinkingOffValue,
  });
};
</script>
