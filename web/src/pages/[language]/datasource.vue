<template>
  <main-layout current-nav="datasource">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="datasourceListGet"
        :options-request="datasourceListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="datasource"
      >
        <!-- config 为加密字段，列表统一用加密字段组件展示（提供明文查看入口，点击可解密） -->
        <template #cell-config="{ item }">
          <encrypted-value-cell
            kind="crypto"
            :item="item"
            field-key="config"
            @view-plaintext="viewConfigPlaintext"
          />
        </template>
      </search-table>

      <plaintext-dialog ref="plaintextDialogRef" />

      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.datasource.name')"
        prepend-icon="mdi-database-import-outline"
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
                category: $t('model.datasource.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <confirm-dialog
        ref="toggleConfirmRef"
        :title="$t('model.datasource.name')"
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
                category: $t('model.datasource.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.datasource.name')"
        :confirm-disabled="
          !createForm.meta.value.valid ||
          !createConfigValid ||
          (isCreateSQLite && !createSqliteFile)
        "
        force-editable
        prepend-icon="mdi-database-import-outline"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="6">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.datasource.field.name')"
                />
              </v-col>
              <v-col cols="6">
                <v-select
                  v-model="createType"
                  v-bind="createTypeProps"
                  :items="datasourceTypeOptions"
                  :label="$t('model.datasource.field.type')"
                  @update:model-value="onCreateTypeChange"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  :label="$t('model.datasource.field.enabled')"
                  color="primary"
                />
              </v-col>
            </v-row>
            <v-row v-if="isCreateSQLite" dense>
              <v-col cols="12">
                <v-file-input
                  v-model="createSqliteFile"
                  accept=".db,.sqlite,.sqlite3"
                  :label="`${$t('model.datasource.configField.path')} *`"
                  :hint="$t('model.datasource.configField.fileHint')"
                  persistent-hint
                  show-size
                />
              </v-col>
            </v-row>
            <template v-if="createConfigFields.length > 0">
              <v-divider class="my-2" />
              <v-row dense>
                <v-col cols="12">
                  <v-label>{{ $t("model.datasource.field.config") }}</v-label>
                </v-col>
              </v-row>
              <v-row v-for="f in createConfigFields" :key="f.name" dense>
                <v-col cols="12">
                  <v-select
                    v-if="f.name === 'struct'"
                    v-model="createConfigValues[f.name]"
                    :items="structFieldItems"
                    :item-title="structOptionTitle"
                    :error-messages="createConfigErrors[f.name] || ''"
                  >
                    <template #label>
                      {{ $t(configFieldLabel(createType, f.name))
                      }}<span v-if="f.required" class="text-error"> *</span>
                    </template>
                  </v-select>
                  <template v-else-if="f.kind === 'columns'">
                    <v-label class="d-block mb-1">
                      {{ $t(configFieldLabel(createType, f.name))
                      }}<span v-if="f.required" class="text-error"> *</span>
                    </v-label>
                    <datasource-schema-editor
                      :model-value="createConfigValues[f.name] as DatasourceColumn[]"
                      @update:model-value="createConfigValues[f.name] = $event"
                    />
                    <div v-if="createConfigErrors[f.name]" class="text-error text-body-2 mt-1">
                      {{ createConfigErrors[f.name] }}
                    </div>
                  </template>
                  <v-textarea
                    v-else-if="f.kind === 'object' || f.kind === 'textarea'"
                    v-model="createConfigValues[f.name]"
                    :error-messages="createConfigErrors[f.name] || ''"
                    rows="3"
                  >
                    <template #label>
                      {{ $t(configFieldLabel(createType, f.name))
                      }}<span v-if="f.required" class="text-error"> *</span>
                    </template>
                  </v-textarea>
                  <v-text-field
                    v-else
                    v-model="createConfigValues[f.name]"
                    :type="
                      f.kind === 'password' && isRevealed('create', f.name)
                        ? 'text'
                        : configFieldType(f.kind)
                    "
                    :append-inner-icon="
                      f.kind === 'password'
                        ? isRevealed('create', f.name)
                          ? 'mdi-eye-off'
                          : 'mdi-eye'
                        : undefined
                    "
                    :error-messages="createConfigErrors[f.name] || ''"
                    @click:append-inner="f.kind === 'password' && toggleReveal('create', f.name)"
                  >
                    <template #label>
                      {{ $t(configFieldLabel(createType, f.name))
                      }}<span v-if="f.required" class="text-error"> *</span>
                    </template>
                  </v-text-field>
                </v-col>
              </v-row>
            </template>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.datasource.field.description')"
                  rows="1"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.datasource.name')"
        :confirm-disabled="!updateForm.meta.value.valid || !editConfigValid"
        prepend-icon="mdi-database-import-outline"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="6">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.datasource.field.name')"
                  disabled
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  :model-value="proxy.type"
                  :label="$t('model.datasource.field.type')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="editEnabled"
                  v-bind="editEnabledProps"
                  :label="$t('model.datasource.field.enabled')"
                  color="primary"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row v-if="isEditSQLite" dense>
              <v-col cols="12">
                <v-file-input
                  v-model="editSqliteFile"
                  accept=".db,.sqlite,.sqlite3"
                  :label="$t('model.datasource.configField.path')"
                  :hint="$t('model.datasource.configField.fileHint')"
                  persistent-hint
                  show-size
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-divider class="my-2" />
            <v-row dense>
              <v-col cols="12">
                <v-label>{{ $t("model.datasource.field.config") }}</v-label>
              </v-col>
            </v-row>
            <v-row v-for="f in editConfigFields" :key="f.name" dense>
              <v-col cols="12">
                <v-select
                  v-if="f.name === 'struct'"
                  v-model="editConfigValues[f.name]"
                  :items="structFieldItems"
                  :item-title="structOptionTitle"
                  :error-messages="editConfigErrors[f.name] || ''"
                  :disabled="!editable"
                >
                  <template #label>
                    {{ $t(configFieldLabel(editType, f.name))
                    }}<span v-if="f.required" class="text-error"> *</span>
                  </template>
                </v-select>
                <template v-else-if="f.kind === 'columns'">
                  <v-label class="d-block mb-1">
                    {{ $t(configFieldLabel(editType, f.name))
                    }}<span v-if="f.required" class="text-error"> *</span>
                  </v-label>
                  <datasource-schema-editor
                    :model-value="editConfigValues[f.name] as DatasourceColumn[]"
                    :disabled="!editable"
                    @update:model-value="editConfigValues[f.name] = $event"
                  />
                  <div
                    v-if="editConfigErrors[f.name] && editable"
                    class="text-error text-body-2 mt-1"
                  >
                    {{ editConfigErrors[f.name] }}
                  </div>
                </template>
                <v-textarea
                  v-else-if="f.kind === 'object' || f.kind === 'textarea'"
                  v-model="editConfigValues[f.name]"
                  :error-messages="editConfigErrors[f.name] || ''"
                  :disabled="!editable"
                  rows="3"
                >
                  <template #label>
                    {{ $t(configFieldLabel(editType, f.name))
                    }}<span v-if="f.required" class="text-error"> *</span>
                  </template>
                </v-textarea>
                <v-text-field
                  v-else
                  v-model="editConfigValues[f.name]"
                  :type="
                    f.kind === 'password' && isRevealed('edit', f.name)
                      ? 'text'
                      : configFieldType(f.kind)
                  "
                  :append-inner-icon="
                    f.kind === 'password'
                      ? isRevealed('edit', f.name)
                        ? 'mdi-eye'
                        : 'mdi-eye-off'
                      : undefined
                  "
                  :error-messages="editConfigErrors[f.name] || ''"
                  :disabled="f.kind === 'password' ? undefined : !editable"
                  :readonly="f.kind === 'password' ? !editable : undefined"
                  @click:append-inner="f.kind === 'password' && toggleReveal('edit', f.name)"
                >
                  <template #label>
                    {{ $t(configFieldLabel(editType, f.name))
                    }}<span v-if="f.required" class="text-error"> *</span>
                  </template>
                </v-text-field>
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.datasource.field.description')"
                  :disabled="!editable"
                  rows="1"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Datasource List
meta:
  title: title.datasource.self
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="datasource-list">
import { useForm } from "vee-validate";

import {
  datasourceBatchDelete,
  datasourceBatchSetEnabled,
  datasourceCreate,
  datasourceDetailGet,
  datasourceListGet,
  datasourceListOptions,
  datasourceTypesGet,
  datasourceUpdate,
} from "@/apis/datasource";
import type { BatchResultOut } from "@/apis/@types/admin";
import type {
  DatasourceItem,
  DatasourceTypeField,
  DatasourceTypeItem,
} from "@/apis/@types/datasource";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { plaintextCryptoGet } from "@/apis/traffic";
import { createDatasourceSchema, updateDatasourceSchema } from "@/schemas/datasource";
import { useAppStore } from "@/stores/app";
import { isEmptyCiphertext } from "@/utils/encrypted";
import { vuetifyCfg } from "@/utils/form";
import { BackendDataType } from "@/types";
import i18n from "@/utils/i18n";
import { EnumClass, enumLabelOf, enumOptionsOf, ensureEnum } from "@/utils/enums";

// ---- 表单校验 schema（配置字段由动态 schema 单独校验）----
const createForm = useForm({ validationSchema: createDatasourceSchema() });
const updateForm = useForm({ validationSchema: updateDatasourceSchema() });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createType, createTypeProps] = createForm.defineField("type", vuetifyCfg);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const [editEnabled, editEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);
const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");

// ---- sqlite_file 数据源：.db 文件上传（创建必填、更新可选换新）----
const createSqliteFile = ref<File | null>(null);
const editSqliteFile = ref<File | null>(null);
// isCreateSQLite / isEditSQLite 判断当前创建/编辑表单选中的数据源是否为 sqlite_file 类型
const isCreateSQLite = computed(() => createType.value === "sqlite_file");
const isEditSQLite = computed(() => editType.value === "sqlite_file");

// ---- 数据源类型注册表（含各类型配置 schema），经 /types 动态拉取 ----
const typeItems = ref<DatasourceTypeItem[]>([]);
const typeMap = computed<Record<string, DatasourceTypeItem>>(() => {
  const map: Record<string, DatasourceTypeItem> = {};
  for (const t of typeItems.value) {
    map[t.type] = t;
  }
  return map;
});
const datasourceTypeOptions = computed(() => enumOptionsOf(EnumClass.datasourceType));

// redis 数据结构下拉选项（来自统一枚举注册表 datasourceStruct，title 为 label、value 为枚举值，
// 渲染期经 enumLabelOf 翻译，语言切换即时刷新）
const structFieldItems = computed(() => enumOptionsOf(EnumClass.datasourceStruct));
const structOptionTitle = (option: { title: string; value: string }): string =>
  enumLabelOf(EnumClass.datasourceStruct, option.value);

onMounted(() => {
  void getResponse<{ types: DatasourceTypeItem[] }>(datasourceTypesGet(), async (rsp) => {
    typeItems.value = rsp.data.types ?? [];
  });
  // 预取统一枚举注册表选项：ensureEnum 确保按当前语言缓存，enumOptionsOf 为响应式，缓存到达自动刷新
  void ensureEnum(EnumClass.datasourceType);
  void ensureEnum(EnumClass.datasourceStruct);
});

// ---- 配置动态表单 ----
// configFieldLabel 依据数据源类型与字段名拼接配置字段的 i18n label key
const configFieldLabel = (_type: string, field: string): string =>
  `model.datasource.configField.${field}`;

// DatasourceColumn 手动 schema 的一列（与后端 DatasourceColumn JSON 结构一致）
interface DatasourceColumn {
  name: string;
  type: string;
}

// configFieldType 将配置字段 schema 的 kind 映射为输入框的 type 属性
const configFieldType = (kind: string): string => {
  switch (kind) {
    case "password":
      return "password";
    case "number":
      return "number";
    default:
      return "text";
  }
};

// DatasourceConfigState 数据源配置动态表单状态（值与错误分开管理）
interface DatasourceConfigState {
  values: Ref<Record<string, string | number | DatasourceColumn[]>>;
  errors: Ref<Record<string, string>>;
  fields: ComputedRef<DatasourceTypeField[]>;
  valid: ComputedRef<boolean>;
  reset: () => void;
  fromJson: (json: string) => void;
  toJson: () => string;
  validate: () => boolean;
}

// makeConfigState 生成配置表单状态，字段 schema 随 getType() 返回的数据源类型变化
const makeConfigState = (getType: () => string): DatasourceConfigState => {
  const values = ref<Record<string, string | number | DatasourceColumn[]>>({});
  const errors = ref<Record<string, string>>({});
  const fields = computed<DatasourceTypeField[]>(() => typeMap.value[getType()]?.config ?? []);
  // 按字段 schema 生成初始值：number 用 defaultValue 兜底，columns 默认空数组，其余空串
  const reset = (): void => {
    const next: Record<string, string | number | DatasourceColumn[]> = {};
    for (const f of fields.value) {
      const d = f.defaultValue;
      next[f.name] = f.kind === "columns" ? [] : typeof d === "number" ? d : "";
    }
    values.value = next;
    errors.value = {};
  };
  // fromJson 用已解密的配置明文 JSON 回填表单：number 字段保留数值，字符串字段填充字符串，
  // columns 字段回填列定义数组，缺失或类型不符的字段回退为空，并清空上轮错误
  const fromJson = (json: string): void => {
    const parsed: Record<string, unknown> = {};
    try {
      Object.assign(parsed, JSON.parse(json || "{}"));
    } catch {
      // 非法 JSON 按空配置处理
    }
    const next: Record<string, string | number | DatasourceColumn[]> = {};
    for (const f of fields.value) {
      const v = parsed[f.name];
      if (f.kind === "columns") {
        next[f.name] = Array.isArray(v)
          ? (v as DatasourceColumn[]).filter(
              (c) => c && typeof c.name === "string" && typeof c.type === "string",
            )
          : [];
        continue;
      }
      next[f.name] = typeof v === "number" ? v : typeof v === "string" ? v : "";
    }
    values.value = next;
    errors.value = {};
  };
  // 必填字段非空校验（columns 为可选字段时跳过；required 的 columns 要求至少一列），
  // 错误信息展示在对应输入框
  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    for (const f of fields.value) {
      if (!f.required) {
        continue;
      }
      if (f.kind === "columns") {
        if (
          !Array.isArray(values.value[f.name]) ||
          (values.value[f.name] as DatasourceColumn[]).length === 0
        ) {
          errs[f.name] = i18n.global.t("validation.mixed.required", {
            path: i18n.global.t(configFieldLabel(getType(), f.name)),
          });
        }
        continue;
      }
      if (values.value[f.name] === "" || values.value[f.name] === undefined) {
        errs[f.name] = i18n.global.t("validation.mixed.required", {
          path: i18n.global.t(configFieldLabel(getType(), f.name)),
        });
      }
    }
    errors.value = errs;
    return Object.keys(errs).length === 0;
  };
  // toJson 序列化为配置 JSON：number 字段强制转数字，columns 字段仅非空时输出成数组，
  // 其余单行字符串字段去除首尾空白；password 字段保留原始输入（trim 会破坏凭据）
  const toJson = (): string => {
    const config: Record<string, string | number | DatasourceColumn[]> = {};
    for (const f of fields.value) {
      const v = values.value[f.name];
      if (f.kind === "columns") {
        // 空 schema 省略字段，后端 len(Schema) > 0 判定自动回退自省
        if (Array.isArray(v) && v.length > 0) {
          config[f.name] = v.map((c) => ({ name: c.name.trim(), type: c.type }));
        }
      } else if (f.kind === "number") {
        config[f.name] = v === "" || v === undefined ? "" : Number(v);
      } else if (f.kind === "password") {
        config[f.name] = v;
      } else {
        config[f.name] = typeof v === "string" ? v.trim() : v;
      }
    }
    return JSON.stringify(config);
  };
  const valid = computed<boolean>(() =>
    fields.value.every((f) => {
      if (!f.required) {
        return true;
      }
      if (f.kind === "columns") {
        return ((values.value[f.name] as DatasourceColumn[]) ?? []).length > 0;
      }
      return values.value[f.name] !== "" && values.value[f.name] !== undefined;
    }),
  );
  return { values, errors, fields, valid, reset, fromJson, toJson, validate };
};

// ---- 配置表单状态（创建按当前所选类型，编辑按数据源自身类型）----
const editType = ref("");
const {
  values: createConfigValues,
  errors: createConfigErrors,
  fields: createConfigFields,
  valid: createConfigValid,
  reset: resetCreateConfig,
  toJson: dumpCreateConfigJson,
  validate: validateCreateConfig,
} = makeConfigState(() => createType.value);
const {
  values: editConfigValues,
  errors: editConfigErrors,
  fields: editConfigFields,
  valid: editConfigValid,
  fromJson: loadEditConfigJson,
  toJson: dumpEditConfigJson,
  validate: validateEditConfig,
} = makeConfigState(() => editType.value);

// ---- 密码字段明文展示开关 ----
// 按字段名与打开方式（create/edit）分别记录是否明文展示，避免两套配置表单串扰
const createRevealed = ref<Record<string, boolean>>({});
const editRevealed = ref<Record<string, boolean>>({});
// isRevealed 判断某字段当前是否明文展示，用于决定 input type 与眼睛图标状态（仅密码类字段使用）
const isRevealed = (group: "create" | "edit", field: string): boolean =>
  group === "create" ? !!createRevealed.value[field] : !!editRevealed.value[field];
// toggleReveal 切换字段是否明文展示：替换对象触发响应式更新
const toggleReveal = (group: "create" | "edit", field: string): void => {
  const store = group === "create" ? createRevealed : editRevealed;
  store.value = { ...store.value, [field]: !store.value[field] };
};

// ---- 创建对话框 ----
const openCreateDialogAction = (): void => {
  createForm.resetField("name", { value: "" });
  createForm.resetField("type", { value: "" });
  createForm.resetField("enabled", { value: true });
  createForm.resetField("description", { value: "" });
  createSqliteFile.value = null;
  resetCreateConfig();
  createDialogRef.value?.open({});
};

// 数据源类型切换时重置配置表单与 sqlite 文件选择（按新类型 schema 生成默认值）
const onCreateTypeChange = (): void => {
  createSqliteFile.value = null;
  resetCreateConfig();
};

const createAction = createForm.handleSubmit(async (values) => {
  // 防御性校验：确认按钮已按 valid 禁用，此处兜底并让字段错误可见
  validateCreateConfig();
  await getSimpleResponse(
    datasourceCreate(
      {
        // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
        name: (values.name as string).trim(),
        type: values.type,
        enabled: values.enabled,
        config: dumpCreateConfigJson(),
        description: values.description,
      },
      // sqlite_file 携带 .db 文件以 multipart 上传（后端加密直传 S3），其余类型为 JSON
      isCreateSQLite.value ? createSqliteFile.value : null,
    ),
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
let editDatasourceId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<DatasourceItem>(
    datasourceDetailGet(item.id),
    async (rsp) => {
      editDatasourceId = rsp.data.id;
      editType.value = rsp.data.type;
      editSqliteFile.value = null;
      updateForm.resetField("enabled", { value: rsp.data.enabled });
      updateForm.resetField("description", { value: rsp.data.description });
      // config 为 base64 密文，需经明文接口解密后再回填编辑表单
      const plaintext = await loadDatasourceConfigPlaintext(rsp.data.config);
      loadEditConfigJson(plaintext);
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
    validateEditConfig();
    await getSimpleResponse(
      datasourceUpdate(
        editDatasourceId,
        {
          enabled: values.enabled,
          config: dumpEditConfigJson(),
          description: values.description,
        },
        // sqlite_file 选择新 .db 文件时以 multipart 覆盖上传同一 S3 对象，未选则保持既有文件
        isEditSQLite.value ? editSqliteFile.value : null,
      ),
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

// viewConfigPlaintext 列表单元格查看数据源连接配置明文：复用共享明文对话框解密展示
const viewConfigPlaintext = async (item: Record<string, unknown>): Promise<void> => {
  const ciphertext = item.config;
  if (isEmptyCiphertext(ciphertext)) {
    return;
  }
  const label = i18n.global.t("model.datasource.field.config");
  await plaintextDialogRef.value?.open(
    {
      key: "config",
      label,
      display: "",
      kind: "crypto",
      length: 0,
      value: ciphertext,
      type: BackendDataType.STRING,
    },
    "datasource",
    label,
  );
};

// loadDatasourceConfigPlaintext 经明文接口解密数据源连接配置密文为明文 JSON 文本；
// 无密文或为空 JSON（"{}"）时返回空字符串。
const loadDatasourceConfigPlaintext = async (cipher: string): Promise<string> => {
  if (isEmptyCiphertext(cipher)) {
    return "";
  }
  const rsp = await plaintextCryptoGet({
    model: "datasource",
    field: "config",
    ciphertext: cipher,
  });
  return new TextDecoder("utf-8").decode(new Uint8Array(rsp.data as ArrayBuffer));
};

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影（name 定位、enabled 开关、config 密文单元格），
// 避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "enabled",
  "config",
  ...visible,
];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
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

// ---- 批量删除/启用/禁用 ----
// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时提示失败数，否则沿用成功摘要
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
    datasourceBatchDelete(ids),
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
    datasourceBatchSetEnabled(enabled, ids),
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
    datasourceBatchDelete([proxy.id]),
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
    datasourceBatchSetEnabled(toggleTarget.value, [proxy.id]),
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
</script>
