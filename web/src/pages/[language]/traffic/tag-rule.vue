<template>
  <main-layout current-nav="traffic">
    <template #body>
      <v-card flat>
        <v-tabs v-model="activeTab" color="primary">
          <v-tab value="rules">{{ $t("model.tagRule.name") }}</v-tab>
          <v-tab value="samples">{{ $t("model.tagRuleSample.name") }}</v-tab>
        </v-tabs>
        <v-divider />
        <v-tabs-window v-model="activeTab">
          <v-tabs-window-item value="rules">
            <search-table
              ref="searchTableRef"
              :list-request="tagRuleListGet"
              :options-request="tagRuleListOptions"
              :actions="getActions"
              :batch-actions="getBatchActions"
              :create="openCreateDialogAction"
              model-name="tagRule"
            >
              <!-- 目标模型列显示可读名称（与表单下拉一致），而非原始类型键 -->
              <template #cell-model="{ value }">
                {{ modelTypeKeyLabel(String(value ?? "")) || $t("model.__base.value.blank") }}
              </template>
            </search-table>
          </v-tabs-window-item>
          <v-tabs-window-item value="samples">
            <tag-rule-sample-panel />
          </v-tabs-window-item>
        </v-tabs-window>
      </v-card>

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
        <template #body="{ proxy }">
          <confirm-alert
            v-if="batchState"
            :action="
              $t(batchState?.action === 'activate' ? 'action.activate' : 'action.deactivate')
            "
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
          <confirm-alert
            v-else
            :action="$t(toggleTarget === true ? 'action.activate' : 'action.deactivate')"
            :text="
              $t(toggleTarget === true ? 'notice.enableConfirm' : 'notice.disableConfirm', {
                category: $t(`model.tagRule.name`),
                key: proxy.name,
              })
            "
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
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.tagRule.field.name')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="createModel"
                  v-bind="createModelProps"
                  :items="modelOptions"
                  :label="$t('model.tagRule.field.model')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.tagRule.field.description')"
                  rows="2"
                />
              </v-col>
            </v-row>
            <v-row dense>
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
            <v-row dense>
              <v-col cols="12">
                <rego-editor
                  v-model="createRego"
                  :errors="createRegoErrors"
                  :fields="createFields"
                  :extra-completions="createExtraFields"
                  :samples="createSamples"
                  min-height="260px"
                  @update:sample-input="createSampleInput = $event"
                  @view-input="createDryRunRef?.openInput()"
                />
              </v-col>
            </v-row>
            <v-row dense align="center">
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  size="small"
                  :loading="createRegoValidating"
                  @click="validateCreateRegoNow"
                >
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
                  {{ $t("action.run") }}
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
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.tagRule.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  :model-value="proxy.model"
                  :items="modelOptions"
                  :label="$t('model.tagRule.field.model')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.tagRule.field.description')"
                  :disabled="!editable"
                  rows="2"
                />
              </v-col>
            </v-row>
            <v-row dense>
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
            <v-row dense>
              <v-col cols="12">
                <rego-editor
                  v-model="editRego"
                  :errors="editRegoErrors"
                  :fields="editFields"
                  :extra-completions="editExtraFields"
                  :samples="editSamples"
                  :disabled="!editable"
                  min-height="260px"
                  @update:sample-input="editSampleInput = $event"
                  @view-input="editDryRunRef?.openInput()"
                />
              </v-col>
            </v-row>
            <v-row dense align="center">
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
                  color="primary"
                  size="small"
                  :disabled="!editable"
                  :loading="editDryRunRef?.loading"
                  @click="editDryRunRef?.run()"
                >
                  {{ $t("action.run") }}
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

      <v-dialog v-model="versionDialog" max-width="800">
        <v-card>
          <v-card-title>{{ $t("action.version") }}</v-card-title>
          <v-card-text>
            <v-list>
              <v-list-item v-for="v in versionItems" :key="v.id">
                <template #title>
                  <v-chip size="small" class="me-2"> v{{ v.version }} </v-chip>
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
                  <pre v-if="expandedVersion === v.version" class="rego-view">{{ v.rego }}</pre>
                </v-expand-transition>
              </v-list-item>
            </v-list>
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn variant="plain" @click="versionDialog = false">
              {{ $t("action.close") }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

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
name: Traffic Tag Rule List
meta:
  title: title.tagRuleTraffic
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { DateTime } from "luxon";

import {
  tagRuleCreate,
  tagRuleDelete,
  tagRuleDetailGet,
  tagRuleListGet,
  tagRuleListOptions,
  tagRuleModelMetadata,
  tagRuleModelOptions,
  tagRuleModelSamples,
  tagRuleSetEnabled,
  tagRuleUpdate,
  tagRuleValidate,
  tagRuleVersionList,
  tagRuleVersionRollback,
} from "@/apis/traffic";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import type { BatchResultOut } from "@/apis/@types/admin";
import { useAppStore } from "@/stores/app";
import { createTagRuleSchema, updateTagRuleSchema } from "@/schemas/tagRule";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { modelTypeKeyLabel } from "@/utils/modelField";
import {
  buildRegoSkeleton,
  extractInputFields,
  HTTP_TRANSPORT_MODEL_KEY,
  preflightRego,
  type InputFieldMeta,
  type RegoValidationError,
} from "@/utils/rego";
import { useForm } from "vee-validate";

import type {
  TagRuleDetailOut,
  TagRuleListOut,
  TagRuleModelMetadataOut,
  TagRuleModelOptionsOut,
  TagRuleModelSamplesOut,
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

const modelOptions = ref([
  {
    title: "HTTP Transport",
    value: HTTP_TRANSPORT_MODEL_KEY,
  },
]);

onMounted(() => {
  void getResponse<TagRuleModelOptionsOut>(tagRuleModelOptions(), async (rsp) => {
    const items = rsp.data.items ?? [];
    if (items.length > 0) {
      modelOptions.value = items.map((o) => ({ title: o.label, value: o.value }));
    }
  });
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

// ---- input model metadata ----
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

// ---- input model samples ----
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

// ---- Rego editor state (create dialog) ----
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

// ---- Rego editor state (edit dialog) ----
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
let editRuleId = 0;

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

// ---- list actions ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    version: versionAction,
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<TagRuleDetailOut>(
    tagRuleDetailGet(item.id),
    async (rsp) => {
      editRuleId = rsp.data.id;
      editModel.value = rsp.data.model || HTTP_TRANSPORT_MODEL_KEY;
      updateForm.resetField("description", { value: rsp.data.description });
      updateForm.resetField("enabled", { value: rsp.data.enabled });
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
  createForm.resetField("name", { value: "" });
  createForm.resetField("model", { value: HTTP_TRANSPORT_MODEL_KEY });
  createForm.resetField("description", { value: "" });
  createForm.resetField("enabled", { value: true });
  createRego.value = "";
  createExtraFields.value = [];
  createSamples.value = [];
  createSampleInput.value = null;
  createBackendValid.value = null;
  createBackendWarnings.value = [];
  void loadModelFields(createModel.value, createFields);
  // 显式加载样例：createModel 若与上次相同，watch 不会触发，须主动拉取
  void loadModelSamples(createModel.value, createSamples);
  createDialogRef.value?.open({});
  void validateCreateRego("");
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
      name: values.name,
      model: values.model,
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
      tagRuleUpdate(editRuleId, {
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

const toggleTarget = ref<boolean | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const toggleAction = (item: Record<string, any>, enabled: boolean): void => {
  batchState.value = null;
  toggleTarget.value = enabled;
  toggleConfirmRef.value?.open(item);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemToggleAction = async (proxy: Record<string, any>): Promise<void> => {
  const state = batchState.value;
  if (state) {
    batchState.value = null;
    await batchToggleAction(state.items, state.action === "activate");
    return;
  }
  if (toggleTarget.value === null) {
    return;
  }
  await getResponse<BatchResultOut>(
    tagRuleSetEnabled(proxy.id, toggleTarget.value),
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

// ---- reuse fields from other rules of the same model ----
const reuseFieldsAction = async (): Promise<void> => {
  await getResponse<{ items: TagRuleListOut[] }>(
    tagRuleListGet({ greedy: false, page: 1, pageSize: 100 }),
    async (rsp) => {
      const siblings = rsp.data.items.filter(
        (r) => r.id !== editRuleId && r.model === editModel.value,
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

// ---- version list & rollback ----
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
    tagRuleVersionRollback(proxy.ruleId, proxy.version),
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
/* tabs 窗口内的通用列表卡片（search-table 自带 v-card）平铺融入外层卡片：
   覆盖全局 .v-card 的边框/圆角/背景，避免「tabs 悬空 + 独立圆角卡片」的割裂观感 */
.v-tabs-window :deep(.v-card) {
  border: none;
  border-radius: 0;
  background: transparent;
}

.monospace :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
}

/* 启用开关独占一行但收缩到内容宽度，避免整行铺满留白 */
.switch-narrow {
  flex: 0 1 auto;
  width: fit-content;
}

.rego-view {
  white-space: pre-wrap;
  word-break: break-all;
  background-color: rgb(var(--v-theme-surface-variant));
  border-radius: 4px;
  padding: 12px;
  font-size: 13px;
  max-height: 320px;
  overflow: auto;
}
</style>
