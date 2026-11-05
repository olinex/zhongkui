<template>
  <main-layout current-nav="tag-event">
    <template #body>
      <tab-view
        v-model="activeTab"
        :items="[
          { key: 'rules', label: $t('model.tagRule.name') },
          { key: 'samples', label: $t('model.tagRuleSample.name') },
        ]"
      >
        <template #rules>
          <search-table
            ref="searchTableRef"
            :list-request="tagRuleListGet"
            :options-request="tagRuleListOptions"
            :actions="getActions"
            :batch-actions="getBatchActions"
            :create="openCreateDialogAction"
            :project-fields="projectFields"
            model-name="tagRule"
          >
            <!-- 目标模型列显示可读名称（与表单下拉一致），而非原始类型键 -->
            <template #cell-model="{ value }">
              {{
                enumLabelOf(EnumClass.tagRuleModel, String(value ?? "")) ||
                $t("model.__base.value.blank")
              }}
            </template>
            <!-- 启用态：开关即时写入（乐观更新，失败回退） -->
            <template #cell-enabled="{ item }">
              <enabled-switch-cell
                v-model="item.enabled"
                :request="(enabled: boolean) => tagRuleSetEnabled(item.id, enabled)"
              />
            </template>
          </search-table>
        </template>
        <template #samples>
          <tag-rule-sample-panel />
        </template>
      </tab-view>

      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.tagRule.name')"
        prepend-icon="mdi-tag"
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
                category: $t(`model.tagRule.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <confirm-dialog
        ref="toggleConfirmRef"
        :title="$t('model.tagRule.name')"
        prepend-icon="mdi-toggle-switch"
        @confirm="itemToggleAction"
      >
        <template #body>
          <confirm-alert
            v-if="batchState"
            :action="
              $t(batchState?.action === 'activate' ? 'action.activate' : 'action.deactivate')
            "
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
        </template>
      </confirm-dialog>

      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.tagRule.name')"
        :confirm-disabled="!createForm.meta.value.valid || !createRegoValid || createRegoValidating"
        force-editable
        prepend-icon="mdi-tag-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.tagRule.field.name')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-select
                  v-model="createModel"
                  v-bind="createModelProps"
                  :items="modelOptions"
                  :item-title="modelOptionTitle"
                  :label="$t('model.tagRule.field.model')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.tagRule.field.description')"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  :label="$t('model.tagRule.field.enabled')"
                  color="primary"
                  hide-details
                  class="switch-narrow"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <rego-editor
                  v-model="createRego"
                  :errors="createRegoErrors"
                  :fields="createFields"
                  :extra-completions="createExtraFields"
                  :functions="customFunctions"
                  :sqlite-datasources="sqliteDatasources"
                  :samples="createSamples"
                  :min-height="'var(--zk-editor-min-height)'"
                  @update:sample-input="createSampleInput = $event"
                  @view-input="createDryRunRef?.openInput()"
                />
              </v-col>
            </v-row>
            <v-row density="compact" class="align-center">
              <v-col cols="auto">
                <v-btn v-request="validateCreateRegoNow" variant="tonal" size="small">
                  {{ $t("action.validate") }}
                </v-btn>
              </v-col>
              <v-col cols="auto">
                <v-btn variant="tonal" size="small" @click="generateCreateSkeleton">
                  {{ $t("action.generateSkeleton") }}
                </v-btn>
              </v-col>
              <v-col cols="auto">
                <v-btn
                  color="primary"
                  size="small"
                  :loading="createDryRunRef?.loading"
                  @click="createDryRunRef?.run()"
                >
                  {{ $t("action.dryRun") }}
                </v-btn>
              </v-col>
            </v-row>
            <rego-dry-run-panel
              ref="createDryRunRef"
              :model="createModel"
              :rego="createRego"
              :sample-input="createSampleInput"
            />
          </v-form>
        </template>
      </temp-editable-dialog>

      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.tagRule.name')"
        :confirm-disabled="!updateForm.meta.value.valid || !editRegoValid || editRegoValidating"
        prepend-icon="mdi-tag-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.tagRule.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-select
                  :model-value="proxy.model"
                  :items="modelOptions"
                  :item-title="modelOptionTitle"
                  :label="$t('model.tagRule.field.model')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.tagRule.field.description')"
                  :disabled="!editable"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-switch
                  v-model="editEnabled"
                  v-bind="editEnabledProps"
                  :label="$t('model.tagRule.field.enabled')"
                  color="primary"
                  :disabled="!editable"
                  hide-details
                  class="switch-narrow"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <rego-editor
                  v-model="editRego"
                  :errors="editRegoErrors"
                  :fields="editFields"
                  :extra-completions="editExtraFields"
                  :functions="customFunctions"
                  :sqlite-datasources="sqliteDatasources"
                  :samples="editSamples"
                  :disabled="!editable"
                  :min-height="'var(--zk-editor-min-height)'"
                  @update:sample-input="editSampleInput = $event"
                  @view-input="editDryRunRef?.openInput()"
                />
              </v-col>
            </v-row>
            <v-row density="compact" class="align-center">
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  size="small"
                  :loading="editRegoValidating"
                  :disabled="!editable"
                  @click="validateEditRegoNow"
                >
                  {{ $t("action.validate") }}
                </v-btn>
              </v-col>
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  size="small"
                  :disabled="!editable"
                  @click="reuseFieldsAction"
                >
                  {{ $t("action.reuseFields") }}
                </v-btn>
              </v-col>
              <v-col cols="auto">
                <v-btn
                  v-request="
                    async () => {
                      await editDryRunRef?.run();
                    }
                  "
                  color="primary"
                  size="small"
                  :disabled="!editable"
                >
                  {{ $t("action.dryRun") }}
                </v-btn>
              </v-col>
            </v-row>
            <rego-dry-run-panel
              ref="editDryRunRef"
              :model="editModel"
              :rego="editRego"
              :sample-input="editSampleInput"
            />
          </v-form>
        </template>
      </temp-editable-dialog>

      <app-dialog v-model="versionDialog" :title="$t('action.version')" size="md">
        <v-alert v-if="versionItems.length === 0" type="info" :text="$t('notice.noVersion')" />
        <v-list v-else>
          <v-list-item v-for="v in versionItems" :key="v.id">
            <template #title>
              <v-chip class="me-2"> v{{ v.version }} </v-chip>
              {{ v.name }}
            </template>
            <template #subtitle>
              {{ localeDatetimeFormat(DateTime.fromISO(v.createdAt)) }}
            </template>
            <template #append>
              <v-btn
                variant="tonal"
                size="small"
                class="me-2"
                @click="expandedVersion = expandedVersion === v.version ? null : v.version"
              >
                {{ $t("action.expand") }}
              </v-btn>
              <v-btn variant="tonal" size="small" color="error" @click="openRollbackConfirm(v)">
                {{ $t("action.rollback") }}
              </v-btn>
            </template>
            <v-expand-transition>
              <pre v-if="expandedVersion === v.version" class="zk-code-preview text-body-medium">{{
                v.rego
              }}</pre>
            </v-expand-transition>
          </v-list-item>
        </v-list>
        <template #footer>
          <v-btn variant="text" @click="versionDialog = false">
            {{ $t("action.close") }}
          </v-btn>
        </template>
      </app-dialog>

      <confirm-dialog
        ref="rollbackConfirmRef"
        :title="$t('action.rollback')"
        prepend-icon="mdi-history"
        @confirm="itemRollbackAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.rollback')"
            :text="
              $t(`notice.rollbackConfirm`, {
                category: $t(`model.tagRule.name`),
                key: proxy.name,
                version: proxy.version,
              })
            "
          />
        </template>
      </confirm-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Tag Rule List
meta:
  title: title.tagRuleTagEvent
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { DateTime } from "luxon";

import {
  tagRuleCreate,
  tagRuleDelete,
  tagRuleDetailGet,
  tagRuleFunctions,
  tagRuleListGet,
  tagRuleListOptions,
  tagRuleModelMetadata,
  tagRuleModelSamples,
  tagRuleSetEnabled,
  tagRuleSqliteMeta,
  tagRuleUpdate,
  tagRuleValidate,
  tagRuleVersionList,
  tagRuleVersionRollback,
} from "@/apis/tagEvent";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import type { BatchResultOut } from "@/apis/@types/admin";
import { useAppStore } from "@/stores/app";
import { createTagRuleSchema, updateTagRuleSchema } from "@/schemas/tagRule";
import { EnumClass, enumLabelOf, enumOptionsOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import {
  buildRegoSkeleton,
  extractInputFields,
  HTTP_TRANSPORT_MODEL_KEY,
  preflightRego,
  type InputFieldMeta,
  type RegoFunctionMeta,
  type RegoValidationError,
  type SqliteDatasourceMeta,
} from "@/utils/rego";
import { useForm } from "vee-validate";

import type {
  TagRuleDetailOut,
  TagRuleFunctionOut,
  TagRuleListOut,
  TagRuleModelMetadataOut,
  TagRuleModelSamplesOut,
  TagRuleSqliteMetaOut,
  TagRuleValidateOut,
  TagRuleVersionOut,
} from "./@types";

const appStore = useAppStore();

// 规则 / 样例两个内容区由 tab 分页栏切换，默认停在规则页
const activeTab = ref("rules");

const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const createDryRunRef = useTemplateRef("createDryRunRef");
const editDryRunRef = useTemplateRef("editDryRunRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const rollbackConfirmRef = useTemplateRef("rollbackConfirmRef");

// 模型下拉选项：统一经 tagRuleModel 枚举类从枚举 store 解析（value 为完整类型键），
// 展示名由 modelOptionTitle 按值经 enumLabelOf 翻译（enum.tagRuleModel.<ModelKey>）。
const modelOptions = computed(() => enumOptionsOf(EnumClass.tagRuleModel));

// modelOptionTitle 模型下拉展示名：按 tagRuleModel 枚举类 value 解析 label（语言切换即时生效）。
const modelOptionTitle = (option: { title: string; value: string }): string =>
  enumLabelOf(EnumClass.tagRuleModel, option.value);

// ---- 自定义 OPA 内置函数 / sqlite vtable 元信息（Rego 编辑器补全、悬停与 SQL 提示）----
const customFunctions = ref<RegoFunctionMeta[]>([]);
const sqliteDatasources = ref<SqliteDatasourceMeta[]>([]);

const loadRegoMeta = async (): Promise<void> => {
  await getResponse<{ functions: TagRuleFunctionOut[] }>(
    tagRuleFunctions(),
    async (rsp) => {
      customFunctions.value = (rsp.data.functions ?? []).map((f) => ({
        name: f.name,
        description: f.description,
        signature: f.signature,
        example: f.example,
      }));
    },
    async () => {
      customFunctions.value = [];
    },
  );
  await getResponse<TagRuleSqliteMetaOut>(
    tagRuleSqliteMeta(),
    async (rsp) => {
      sqliteDatasources.value = (rsp.data.datasources ?? []).map((d) => ({
        name: d.name,
        columns: d.columns,
      }));
    },
    async () => {
      sqliteDatasources.value = [];
    },
  );
};

onMounted(() => {
  void loadRegoMeta();
  // 目标模型下拉与列表列 label 需 tagRuleModel 枚举类已缓存；enumLabelOf 响应式，缓存到达自动刷新
  void ensureEnum(EnumClass.tagRuleModel);
});

const createForm = useForm({ validationSchema: createTagRuleSchema });
const updateForm = useForm({ validationSchema: updateTagRuleSchema });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createModel, createModelProps] = createForm.defineField("model", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);

const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);
const [editEnabled, editEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);

// ---- 输入模型元信息 ----
const createFields = ref<InputFieldMeta[]>([]);

const loadModelFields = async (model: string, target: Ref<InputFieldMeta[]>): Promise<void> => {
  await getResponse<TagRuleModelMetadataOut>(
    tagRuleModelMetadata({ model }),
    async (rsp) => {
      target.value = rsp.data.fields ?? [];
    },
    async () => {
      target.value = [];
    },
  );
};

watch(createModel, (val) => {
  void loadModelFields(val, createFields);
});

// ---- 输入模型样例 ----
// 样例与 rego 编辑器联动：按 model 加载样例（含完整 input），编辑器内预选样例后
// 对 `input.<field>` 的补全/悬停提示展示实际值，试运行面板同步该 input。
type SampleOption = { name: string; input: Record<string, unknown> };

const loadModelSamples = async (model: string, target: Ref<SampleOption[]>): Promise<void> => {
  await getResponse<TagRuleModelSamplesOut>(
    tagRuleModelSamples({ model }),
    async (rsp) => {
      target.value = (rsp.data.items ?? []).map((s) => ({
        name: s.name,
        input: s.input ?? {},
      }));
    },
    async () => {
      target.value = [];
    },
  );
};

// ---- Rego 编辑器状态（创建对话框） ----
const createRego = ref("");
const createPreflightErrors = ref<RegoValidationError[]>([]);
const createBackendErrors = ref<RegoValidationError[]>([]);
const createBackendWarnings = ref<RegoValidationError[]>([]);
const createBackendValid = ref<boolean | null>(null);
const createRegoValidating = ref(false);
const createExtraFields = ref<string[]>([]);
const createSamples = ref<SampleOption[]>([]);
const createSampleInput = ref<Record<string, unknown> | null>(null);
let createTimer: ReturnType<typeof setTimeout> | undefined;

const createRegoErrors = computed<RegoValidationError[]>(() => {
  const errs: RegoValidationError[] = [];
  if (createBackendValid.value === false) {
    errs.push(...createBackendErrors.value);
  }
  errs.push(...createBackendWarnings.value);
  errs.push(...createPreflightErrors.value);
  const seen = new Set<string>();
  return errs.filter((e) => {
    const key = `${e.line}:${e.column}:${e.message}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
});

const createRegoValid = computed<boolean>(() => {
  if (createBackendValid.value !== null) {
    return createBackendValid.value;
  }
  return createPreflightErrors.value.length === 0;
});

const validateCreateRego = async (src: string): Promise<void> => {
  createRegoValidating.value = true;
  try {
    await getResponse<TagRuleValidateOut>(
      tagRuleValidate({ rego: src, model: createModel.value }),
      async (rsp) => {
        createBackendValid.value = rsp.data.valid;
        createBackendErrors.value = rsp.data.errors ?? [];
        createBackendWarnings.value = rsp.data.warnings ?? [];
      },
      async () => {
        createBackendValid.value = null;
      },
    );
  } finally {
    createRegoValidating.value = false;
  }
};

const validateCreateRegoNow = async (): Promise<void> => {
  createPreflightErrors.value = preflightRego(createRego.value);
  await validateCreateRego(createRego.value);
  if (createBackendValid.value === true) {
    appStore.notify({
      text: i18n.global.t("action.validatePassed"),
      timeout: 3000,
      color: "success",
    });
  } else if (createBackendValid.value === false) {
    appStore.notify({
      text: i18n.global.t("action.validateFailed"),
      timeout: 3000,
      color: "error",
    });
  }
};

watch(createRego, (val) => {
  createPreflightErrors.value = preflightRego(val);
  if (createTimer) {
    clearTimeout(createTimer);
  }
  // 空/空白规则仅做本地 preflight，不发起后端验证（空串必然校验失败，属无意义请求）
  if (!val.trim()) {
    createBackendValid.value = null;
    createBackendErrors.value = [];
    createBackendWarnings.value = [];
    return;
  }
  createTimer = setTimeout(() => void validateCreateRego(val), 800);
});

watch(createModel, (val) => {
  void loadModelSamples(val, createSamples);
});

const generateCreateSkeleton = (): void => {
  const skeleton = buildRegoSkeleton(createName.value || "tag");
  createRego.value = createRego.value.trim()
    ? `${createRego.value.trimEnd()}\n\n${skeleton}`
    : skeleton;
};

// ---- Rego 编辑器状态（编辑对话框） ----
const editRego = ref("");
const editPreflightErrors = ref<RegoValidationError[]>([]);
const editBackendErrors = ref<RegoValidationError[]>([]);
const editBackendWarnings = ref<RegoValidationError[]>([]);
const editBackendValid = ref<boolean | null>(null);
const editRegoValidating = ref(false);
const editExtraFields = ref<string[]>([]);
const editFields = ref<InputFieldMeta[]>([]);
const editSamples = ref<SampleOption[]>([]);
const editSampleInput = ref<Record<string, unknown> | null>(null);
const editModel = ref("");
let editTimer: ReturnType<typeof setTimeout> | undefined;
let editRuleID = 0;

const editRegoErrors = computed<RegoValidationError[]>(() => {
  const errs: RegoValidationError[] = [];
  if (editBackendValid.value === false) {
    errs.push(...editBackendErrors.value);
  }
  errs.push(...editBackendWarnings.value);
  errs.push(...editPreflightErrors.value);
  const seen = new Set<string>();
  return errs.filter((e) => {
    const key = `${e.line}:${e.column}:${e.message}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
});

const editRegoValid = computed<boolean>(() => {
  if (editBackendValid.value !== null) {
    return editBackendValid.value;
  }
  return editPreflightErrors.value.length === 0;
});

const validateEditRego = async (src: string): Promise<void> => {
  editRegoValidating.value = true;
  try {
    await getResponse<TagRuleValidateOut>(
      tagRuleValidate({ rego: src, model: editModel.value }),
      async (rsp) => {
        editBackendValid.value = rsp.data.valid;
        editBackendErrors.value = rsp.data.errors ?? [];
        editBackendWarnings.value = rsp.data.warnings ?? [];
      },
      async () => {
        editBackendValid.value = null;
      },
    );
  } finally {
    editRegoValidating.value = false;
  }
};

const validateEditRegoNow = async (): Promise<void> => {
  editPreflightErrors.value = preflightRego(editRego.value);
  await validateEditRego(editRego.value);
  if (editBackendValid.value === true) {
    appStore.notify({
      text: i18n.global.t("action.validatePassed"),
      timeout: 3000,
      color: "success",
    });
  } else if (editBackendValid.value === false) {
    appStore.notify({
      text: i18n.global.t("action.validateFailed"),
      timeout: 3000,
      color: "error",
    });
  }
};

watch(editRego, (val) => {
  editPreflightErrors.value = preflightRego(val);
  if (editTimer) {
    clearTimeout(editTimer);
  }
  editTimer = setTimeout(() => void validateEditRego(val), 800);
});

watch(editModel, (val) => {
  void loadModelSamples(val, editSamples);
});

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "name", "enabled", ...visible];

const getActions = (): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    version: versionAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => {
      batchState.value = null;
      deleteConfirmRef.value?.open(i);
    },
  };
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<TagRuleDetailOut>(
    tagRuleDetailGet(item.id),
    async (rsp) => {
      editRuleID = rsp.data.id;
      editModel.value = rsp.data.model || HTTP_TRANSPORT_MODEL_KEY;
      // 复位与回填同属「准备本次打开的数据」：完整枚举 defineField 字段，原子重置值 + 校验状态
      updateForm.resetForm({
        values: {
          description: rsp.data.description,
          enabled: rsp.data.enabled,
        },
      });
      editRego.value = rsp.data.rego;
      editExtraFields.value = [];
      editFields.value = [];
      editSamples.value = [];
      editSampleInput.value = null;
      editBackendWarnings.value = [];
      void loadModelFields(editModel.value, editFields);
      // 显式加载样例：editModel 若与上次相同，watch 不会触发，须主动拉取
      void loadModelSamples(editModel.value, editSamples);
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

const openCreateDialogAction = (): void => {
  // 复位用原子 API resetForm：完整枚举全部 defineField 字段，值 + 校验状态一次性重置
  createForm.resetForm({
    values: {
      name: "",
      model: HTTP_TRANSPORT_MODEL_KEY,
      description: "",
      enabled: true,
    },
  });
  createRego.value = "";
  createExtraFields.value = [];
  createSamples.value = [];
  createSampleInput.value = null;
  createBackendValid.value = null;
  createBackendErrors.value = [];
  createBackendWarnings.value = [];
  void loadModelFields(createModel.value, createFields);
  // 显式加载样例：createModel 若与上次相同，watch 不会触发，须主动拉取
  void loadModelSamples(createModel.value, createSamples);
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  if (!createRegoValid.value) {
    appStore.notify({
      text: i18n.global.t("notice.regoInvalid"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  await getSimpleResponse(
    tagRuleCreate({
      // 单行文本提交前去除首尾空白，避免误输入空白导致查询失败或数据异常
      name: (values.name as string).trim(),
      model: (values.model as string).trim(),
      description: values.description,
      rego: createRego.value,
      enabled: values.enabled,
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

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    if (!editRegoValid.value) {
      appStore.notify({
        text: i18n.global.t("notice.regoInvalid"),
        timeout: 3000,
        color: "error",
      });
      return;
    }
    await getSimpleResponse(
      tagRuleUpdate(editRuleID, {
        description: values.description,
        rego: editRego.value,
        enabled: values.enabled,
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

// ---- 批量删除/启用/禁用 ----
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
type BatchState = { items: Record<string, any>[]; action: "delete" | "activate" | "deactivate" };
const batchState = ref<BatchState | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    delete: () => {
      batchState.value = { items, action: "delete" };
      deleteConfirmRef.value?.open({});
    },
    activate: () => {
      batchState.value = { items, action: "activate" };
      toggleConfirmRef.value?.open({});
    },
    deactivate: () => {
      batchState.value = { items, action: "deactivate" };
      toggleConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    tagRuleDelete(ids),
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
const batchToggleAction = async (items: Record<string, any>[], enabled: boolean): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    tagRuleSetEnabled(ids, enabled),
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
    tagRuleDelete(proxy.id),
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

const itemToggleAction = async (): Promise<void> => {
  const state = batchState.value;
  if (state) {
    batchState.value = null;
    await batchToggleAction(state.items, state.action === "activate");
  }
};

// ---- 复用同一模型下其他规则的字段 ----
const reuseFieldsAction = async (): Promise<void> => {
  await getResponse<{ items: TagRuleListOut[] }>(
    tagRuleListGet({ greedy: false, page: 1, pageSize: 100 }),
    async (rsp) => {
      const siblings = rsp.data.items.filter(
        (r) => r.id !== editRuleID && r.model === editModel.value,
      );
      const fields = new Set<string>();
      for (const rule of siblings) {
        await getResponse<TagRuleDetailOut>(tagRuleDetailGet(rule.id), async (r2) => {
          for (const f of extractInputFields(r2.data.rego)) {
            fields.add(f);
          }
        });
      }
      editExtraFields.value = [...fields];
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

// ---- 版本列表与回滚 ----
const versionDialog = ref(false);
const versionItems = ref<TagRuleVersionOut[]>([]);
const expandedVersion = ref<number | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const versionAction = async (item: Record<string, any>): Promise<void> => {
  versionDialog.value = true;
  versionItems.value = [];
  expandedVersion.value = null;
  await getResponse<{ items: TagRuleVersionOut[] }>(
    tagRuleVersionList(item.id),
    async (rsp) => {
      versionItems.value = rsp.data.items ?? [];
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

const openRollbackConfirm = (v: TagRuleVersionOut): void => {
  rollbackConfirmRef.value?.open(v);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemRollbackAction = async (proxy: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    tagRuleVersionRollback(proxy.ruleID, proxy.version),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      versionDialog.value = false;
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
/* 启用开关独占一行但收缩到内容宽度，避免整行铺满留白 */
.switch-narrow {
  flex: 0 1 auto;
  width: fit-content;
}
</style>
