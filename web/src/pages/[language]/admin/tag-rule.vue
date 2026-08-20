<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="tagRuleListGet"
        :options-request="tagRuleListOptions"
        :actions="getActions"
        :create="openCreateDialogAction"
        model-name="tagRule"
      />

      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.tagRule.name')"
        prepend-icon="mdi-tag"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
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
                  min-height="260px"
                />
              </v-col>
            </v-row>
            <v-row dense align="center">
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  :loading="createRegoValidating"
                  @click="validateCreateRegoNow"
                >
                  {{ $t("action.validate") }}
                </v-btn>
              </v-col>
              <v-col cols="auto">
                <v-btn variant="tonal" @click="generateCreateSkeleton">
                  {{ $t("action.generateSkeleton") }}
                </v-btn>
              </v-col>
            </v-row>
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
                  :disabled="!editable"
                  min-height="260px"
                />
              </v-col>
            </v-row>
            <v-row dense align="center">
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  :loading="editRegoValidating"
                  :disabled="!editable"
                  @click="validateEditRegoNow"
                >
                  {{ $t("action.validate") }}
                </v-btn>
              </v-col>
              <v-col cols="auto">
                <v-btn variant="tonal" :disabled="!editable" @click="reuseFieldsAction">
                  {{ $t("action.reuseFields") }}
                </v-btn>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <v-dialog v-model="dryRunDialog" max-width="800">
        <v-card>
          <v-card-title>{{ $t("action.dryRun") }}</v-card-title>
          <v-card-text>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="dryRunModel"
                  :items="modelOptions"
                  :label="$t('model.tagRule.field.model')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="dryRunSample"
                  :items="dryRunSamples.map((s) => ({ title: s.name, value: s.name }))"
                  :label="$t('action.selectSample')"
                  clearable
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <rego-editor v-model="dryRunRego" :fields="dryRunFields" min-height="200px" />
              </v-col>
            </v-row>
            <v-row dense align="center">
              <v-col cols="12">
                <v-textarea v-model="dryRunInput" label="input (JSON)" rows="8" class="monospace" />
              </v-col>
              <v-col cols="auto">
                <v-btn variant="tonal" @click="fillDryRunSample">
                  {{ $t("action.fillSample") }}
                </v-btn>
              </v-col>
            </v-row>
            <v-alert
              v-if="dryRunResult?.error"
              type="error"
              :text="dryRunResult.error"
              class="mt-2"
            />
            <v-chip v-for="tag in dryRunResult?.tags" :key="tag" color="primary" class="me-1 my-1">
              {{ tag }}
            </v-chip>
            <v-expansion-panels v-if="dryRunResult?.rules" class="mt-2">
              <v-expansion-panel v-for="(rule, name) in dryRunResult.rules" :key="name">
                <v-expansion-panel-title>
                  {{ name }} ({{ localDurationFormat(rule.duration) }})
                </v-expansion-panel-title>
                <v-expansion-panel-text>
                  <v-alert v-if="rule.error" type="error" :text="rule.error" class="mb-2" />
                  <v-chip v-for="tag in rule.tags" :key="tag" class="me-1" size="small">
                    {{ tag }}
                  </v-chip>
                </v-expansion-panel-text>
              </v-expansion-panel>
            </v-expansion-panels>
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn variant="plain" @click="dryRunDialog = false">
              {{ $t("action.close") }}
            </v-btn>
            <v-btn color="primary" :loading="dryRunLoading" @click="dryRunAction">
              {{ $t("action.run") }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

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
name: Admin Tag Rule List
meta:
  title: title.tagRuleAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { DateTime } from "luxon";

import {
  tagRuleCreate,
  tagRuleDelete,
  tagRuleDetailGet,
  tagRuleDryRun,
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
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { createTagRuleSchema, updateTagRuleSchema } from "@/schemas/tagRule";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat, localDurationFormat } from "@/utils/i18n";
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
  TagRuleDryRunOut,
  TagRuleListOut,
  TagRuleModelMetadataOut,
  TagRuleModelOptionsOut,
  TagRuleModelSamplesOut,
  TagRuleSampleOut,
  TagRuleValidateOut,
  TagRuleVersionOut,
} from "./@types";

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
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

// ---- Rego editor state (create dialog) ----
const createRego = ref("");
const createPreflightErrors = ref<RegoValidationError[]>([]);
const createBackendErrors = ref<RegoValidationError[]>([]);
const createBackendWarnings = ref<RegoValidationError[]>([]);
const createBackendValid = ref<boolean | null>(null);
const createRegoValidating = ref(false);
const createExtraFields = ref<string[]>([]);
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

// ---- list actions ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    version: versionAction,
    dryRun: openDryRunAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: (i: Record<string, any>) => deleteConfirmRef.value?.open(i),
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
      editBackendWarnings.value = [];
      void loadModelFields(editModel.value, editFields);
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
  createBackendValid.value = null;
  createBackendWarnings.value = [];
  void loadModelFields(createModel.value, createFields);
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemDeleteAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    tagRuleDelete(item.id),
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
};

const toggleTarget = ref<boolean | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const toggleAction = (item: Record<string, any>, enabled: boolean): void => {
  toggleTarget.value = enabled;
  toggleConfirmRef.value?.open(item);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemToggleAction = async (item: Record<string, any>): Promise<void> => {
  if (toggleTarget.value === null) {
    return;
  }
  await getSimpleResponse(
    tagRuleSetEnabled(item.id, toggleTarget.value),
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

// ---- dry run ----
const dryRunDialog = ref(false);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const dryRunItem = ref<Record<string, any> | null>(null);
const dryRunModel = ref(HTTP_TRANSPORT_MODEL_KEY);
const dryRunRego = ref("");
const dryRunInput = ref("");
const dryRunLoading = ref(false);
const dryRunResult = ref<TagRuleDryRunOut | null>(null);
const dryRunFields = ref<InputFieldMeta[]>([]);
const dryRunSamples = ref<TagRuleSampleOut[]>([]);
const dryRunSample = ref("");

const fillDryRunSample = (): void => {
  const sample = dryRunSamples.value.find((s) => s.name === dryRunSample.value);
  if (sample) {
    dryRunInput.value = JSON.stringify(sample.input, null, 2);
  }
};

const loadDryRunSamples = async (): Promise<void> => {
  dryRunSamples.value = [];
  dryRunSample.value = "";
  await getResponse<TagRuleModelSamplesOut>(
    tagRuleModelSamples({ model: dryRunModel.value }),
    async (rsp) => {
      dryRunSamples.value = rsp.data.items ?? [];
      const first = dryRunSamples.value[0];
      if (first) {
        dryRunSample.value = first.name;
        dryRunInput.value = JSON.stringify(first.input, null, 2);
      }
    },
  );
};

watch(dryRunModel, (val) => {
  void loadModelFields(val, dryRunFields);
  void loadDryRunSamples();
});

watch(dryRunSample, (name) => {
  if (!name) {
    return;
  }
  const sample = dryRunSamples.value.find((s) => s.name === name);
  if (sample) {
    dryRunInput.value = JSON.stringify(sample.input, null, 2);
  }
});

const dryRunAction = async (): Promise<void> => {
  if (!dryRunItem.value) {
    return;
  }
  dryRunResult.value = null;
  let input: Record<string, unknown>;
  try {
    input = JSON.parse(dryRunInput.value || "{}") as Record<string, unknown>;
  } catch {
    appStore.notify({
      text: i18n.global.t("notice.jsonInvalid"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  dryRunLoading.value = true;
  try {
    await getResponse<TagRuleDryRunOut>(
      tagRuleDryRun({
        name: dryRunItem.value.name,
        model: dryRunModel.value,
        rego: dryRunRego.value,
        input,
      }),
      async (rsp) => {
        dryRunResult.value = rsp.data;
      },
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  } finally {
    dryRunLoading.value = false;
  }
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openDryRunAction = (item: Record<string, any>): void => {
  dryRunItem.value = item;
  dryRunModel.value = item.model || HTTP_TRANSPORT_MODEL_KEY;
  dryRunRego.value = "";
  dryRunInput.value = "";
  dryRunSamples.value = [];
  dryRunSample.value = "";
  dryRunResult.value = null;
  dryRunDialog.value = true;
  void loadModelFields(dryRunModel.value, dryRunFields);
  void loadDryRunSamples();
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
.monospace :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
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
