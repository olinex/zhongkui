<template>
  <main-layout current-nav="notification">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="notificationChannelListGet"
        :options-request="notificationChannelListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="notificationChannel"
      >
        <!-- config 为加密字段，列表统一用加密字段组件展示（提供明文查看入口） -->
        <template #cell-config="{ item }">
          <encrypted-value-cell
            kind="crypto"
            :item="item"
            field-key="config"
            @view-plaintext="viewConfigPlaintext"
          />
        </template>
        <!-- 渠道类型列显示可读名称（与创建/编辑表单及筛选下拉一致），而非原始类型键 -->
        <template #cell-type="{ value }">
          {{ $t(`model.notificationChannel.type.${String(value ?? "")}`) }}
        </template>
      </search-table>

      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.notificationChannel.name')"
        prepend-icon="mdi-email-fast-outline"
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
                category: $t('model.notificationChannel.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <confirm-dialog
        ref="toggleConfirmRef"
        :title="$t('model.notificationChannel.name')"
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
                category: $t('model.notificationChannel.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 共享明文对话框：查看渠道配置明文 -->
      <plaintext-dialog ref="plaintextDialogRef" />

      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.notificationChannel.name')"
        :confirm-disabled="!createForm.meta.value.valid || !createConfigValid"
        force-editable
        prepend-icon="mdi-email-fast-outline"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="6">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.notificationChannel.field.name')"
                />
              </v-col>
              <v-col cols="6">
                <v-select
                  v-model="createType"
                  v-bind="createTypeProps"
                  :items="channelStore.channelTypeOptions"
                  :label="$t('model.notificationChannel.field.type')"
                  @update:model-value="onCreateTypeChange"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  :label="$t('model.notificationChannel.field.enabled')"
                  color="primary"
                />
              </v-col>
            </v-row>
            <template v-if="createConfigFields.length > 0">
              <v-divider class="my-2" />
              <v-row dense>
                <v-col cols="12">
                  <v-label>{{ $t("model.notificationChannel.field.config") }}</v-label>
                </v-col>
              </v-row>
              <v-row v-for="f in createConfigFields" :key="f.name" dense>
                <v-col cols="12">
                  <v-textarea
                    v-if="f.kind === 'object' || f.kind === 'textarea'"
                    v-model="createConfigValues[f.name]"
                    :error-messages="createConfigErrors[f.name] || ''"
                    rows="3"
                  >
                    <template #label>
                      {{ $t(channelFieldLabel(createType, f.name))
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
                          ? 'mdi-eye'
                          : 'mdi-eye-off'
                        : undefined
                    "
                    :error-messages="createConfigErrors[f.name] || ''"
                    @click:append-inner="f.kind === 'password' && toggleReveal('create', f.name)"
                  >
                    <template #label>
                      {{ $t(channelFieldLabel(createType, f.name))
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
                  :label="$t('model.notificationChannel.field.description')"
                  rows="1"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.notificationChannel.name')"
        :confirm-disabled="!updateForm.meta.value.valid || !editConfigValid"
        prepend-icon="mdi-email-fast-outline"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="6">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.notificationChannel.field.name')"
                  disabled
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  :model-value="proxy.type"
                  :label="$t('model.notificationChannel.field.type')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="editEnabled"
                  v-bind="editEnabledProps"
                  :label="$t('model.notificationChannel.field.enabled')"
                  color="primary"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <template v-if="editConfigFields.length > 0">
              <v-divider class="my-2" />
              <v-row dense>
                <v-col cols="12">
                  <v-label>{{ $t("model.notificationChannel.field.config") }}</v-label>
                </v-col>
              </v-row>
              <v-row v-for="f in editConfigFields" :key="f.name" dense>
                <v-col cols="12">
                  <v-textarea
                    v-if="f.kind === 'object' || f.kind === 'textarea'"
                    v-model="editConfigValues[f.name]"
                    :error-messages="editConfigErrors[f.name] || ''"
                    :disabled="!editable"
                    rows="3"
                  >
                    <template #label>
                      {{ $t(channelFieldLabel(editType, f.name))
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
                      {{ $t(channelFieldLabel(editType, f.name))
                      }}<span v-if="f.required" class="text-error"> *</span>
                    </template>
                  </v-text-field>
                </v-col>
              </v-row>
            </template>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  v-bind="editDescriptionProps"
                  :label="$t('model.notificationChannel.field.description')"
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
name: Notification Channel List
meta:
  title: title.notification.channel
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="notification-channel-list">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  notificationChannelBatchDelete,
  notificationChannelBatchSetEnabled,
  notificationChannelCreate,
  notificationChannelDetailGet,
  notificationChannelListGet,
  notificationChannelListOptions,
  notificationChannelUpdate,
} from "@/apis/notification";
import { plaintextCryptoGet } from "@/apis/traffic";
import type { BatchResultOut } from "@/apis/@types/admin";
import type {
  NotificationChannelItem,
  NotificationChannelTypeField,
} from "@/apis/@types/notification";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { BackendDataType } from "@/types";
import { useNotificationChannelStore } from "@/stores/notificationChannel";
import { vuetifyCfg } from "@/utils/form";
import { isEmptyCiphertext } from "@/utils/encrypted";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema（配置字段由动态 schema 单独校验）----
const createChannelSchema = yup.object({
  name: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.notificationChannel.field.name"),
  type: yup.string().nonNullable().required().label("model.notificationChannel.field.type"),
  enabled: yup.bool().nonNullable().required().label("model.notificationChannel.field.enabled"),
  description: yup
    .string()
    .nonNullable()
    .max(256)
    .label("model.notificationChannel.field.description"),
});

const updateChannelSchema = yup.object({
  enabled: yup.bool().nonNullable().required().label("model.notificationChannel.field.enabled"),
  description: yup
    .string()
    .nonNullable()
    .max(256)
    .label("model.notificationChannel.field.description"),
});

const appStore = useAppStore();
const channelStore = useNotificationChannelStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");

// 渠道类型注册表（含各类型配置 schema）由共享 store 统一提供，
// 经 /notification/channel/types 动态拉取，新增渠道类型时前端零改动。
onMounted(() => {
  void channelStore.loadChannelTypes();
});

// ---- 配置动态表单 ----
// channelFieldLabel 依据渠道类型与字段名拼接配置字段的 i18n label key（约 notify.channel.<type>.field.<field>）
const channelFieldLabel = (channelType: string, field: string): string =>
  `notify.channel.${channelType}.field.${field}`;

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

// ChannelConfigState 渠道配置动态表单状态（值与错误分开管理）
interface ChannelConfigState {
  values: Ref<Record<string, string | number>>;
  errors: Ref<Record<string, string>>;
  fields: ComputedRef<NotificationChannelTypeField[]>;
  valid: ComputedRef<boolean>;
  reset: () => void;
  fromJson: (json: string) => void;
  toJson: () => string;
  validate: () => boolean;
}

// createChannelConfigState 生成配置表单状态，字段 schema 随 getType() 返回的渠道类型变化
const createChannelConfigState = (getType: () => string): ChannelConfigState => {
  const values = ref<Record<string, string | number>>({});
  const errors = ref<Record<string, string>>({});
  const fields = computed<NotificationChannelTypeField[]>(
    () => channelStore.channelTypeMap[getType()]?.config.channel ?? [],
  );
  // 按字段 schema 生成初始值：number 用 defaultValue 兜底，其余空串
  const reset = (): void => {
    const next: Record<string, string | number> = {};
    for (const f of fields.value) {
      const d = f.defaultValue;
      next[f.name] = typeof d === "number" ? d : "";
    }
    values.value = next;
    errors.value = {};
  };
  // 必填字段非空校验，错误信息展示在对应输入框
  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    for (const f of fields.value) {
      if (f.required && (values.value[f.name] === "" || values.value[f.name] === undefined)) {
        errs[f.name] = i18n.global.t("validation.mixed.required", {
          path: i18n.global.t(channelFieldLabel(getType(), f.name)),
        });
      }
    }
    errors.value = errs;
    return Object.keys(errs).length === 0;
  };
  // 回填既有 JSON 配置到表单（解析失败视为空配置）
  const fromJson = (json: string): void => {
    let parsed: Record<string, unknown> = {};
    if (json) {
      try {
        parsed = JSON.parse(json) as Record<string, unknown>;
      } catch {
        parsed = {};
      }
    }
    const next: Record<string, string | number> = {};
    for (const f of fields.value) {
      const v = parsed[f.name];
      next[f.name] = typeof v === "number" ? v : typeof v === "string" ? v : "";
    }
    values.value = next;
    errors.value = {};
  };
  // toJson 序列化为配置 JSON：密码/多行文本（object/textarea）字段保留原始输入，
  // number 字段强制转为数字（v-model 对 number 输入框返回字符串，直接透传会把 port 存成
  // "465" 字符串，导致后端 EmailChannelConfig.Port(int) 反序列化失败），
  // 其余单行字符串字段去除首尾空白（避免误输入不可见空白）
  const toJson = (): string => {
    const config: Record<string, string | number> = {};
    for (const f of fields.value) {
      const v = values.value[f.name];
      if (f.kind === "number") {
        config[f.name] = v === "" || v === undefined ? "" : Number(v);
      } else if (f.kind === "password" || f.kind === "object" || f.kind === "textarea") {
        config[f.name] = v;
      } else {
        config[f.name] = typeof v === "string" ? v.trim() : v;
      }
    }
    return JSON.stringify(config);
  };
  const valid = computed<boolean>(() =>
    fields.value.every(
      (f) => !f.required || (values.value[f.name] !== "" && values.value[f.name] !== undefined),
    ),
  );
  return { values, errors, fields, valid, reset, fromJson, toJson, validate };
};

// ---- forms ----
const createForm = useForm({ validationSchema: createChannelSchema });
const updateForm = useForm({ validationSchema: updateChannelSchema });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createType, createTypeProps] = createForm.defineField("type", vuetifyCfg);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const [editEnabled, editEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);
const [editDescription, editDescriptionProps] = updateForm.defineField("description", vuetifyCfg);

// ---- 配置表单状态（创建按当前所选类型，编辑按渠道自身类型）----
const editType = ref("");
const {
  values: createConfigValues,
  errors: createConfigErrors,
  fields: createConfigFields,
  valid: createConfigValid,
  reset: resetCreateConfig,
  toJson: dumpCreateConfigJson,
  validate: validateCreateConfig,
} = createChannelConfigState(() => createType.value);
const {
  values: editConfigValues,
  errors: editConfigErrors,
  fields: editConfigFields,
  valid: editConfigValid,
  fromJson: loadEditConfigJson,
  toJson: dumpEditConfigJson,
  validate: validateEditConfig,
} = createChannelConfigState(() => editType.value);

// ---- 密码字段明文展示开关 ----
// 按字段名与表单项（create/edit）分别记录是否明文展示，避免创建/编辑两套表单串扰
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
  resetCreateConfig();
  createDialogRef.value?.open({});
};

// 渠道类型切换时重置配置表单（按新类型 schema 生成默认值）
const onCreateTypeChange = (): void => {
  resetCreateConfig();
};

const createAction = createForm.handleSubmit(async (values) => {
  // 防御性校验：确认按钮已按 valid 禁用，此处兜底并让字段错误可见
  validateCreateConfig();
  await getSimpleResponse(
    notificationChannelCreate({
      // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      name: (values.name as string).trim(),
      type: values.type,
      enabled: values.enabled,
      config: dumpCreateConfigJson(),
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
let editChannelId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<NotificationChannelItem>(
    notificationChannelDetailGet(item.id),
    async (rsp) => {
      editChannelId = rsp.data.id;
      editType.value = rsp.data.type;
      updateForm.resetField("enabled", { value: rsp.data.enabled });
      updateForm.resetField("description", { value: rsp.data.description });
      // config 为 base64 密文，需经明文接口解密后再回填编辑表单
      const plaintext = await loadChannelConfigPlaintext(rsp.data.config);
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

// loadChannelConfigPlaintext 经明文接口解密渠道配置密文为明文 JSON 文本；
// 无密文或为空 JSON（"{}"）时返回空字符串。
const loadChannelConfigPlaintext = async (cipher: string): Promise<string> => {
  if (isEmptyCiphertext(cipher)) {
    return "";
  }
  const rsp = await plaintextCryptoGet({
    model: "notification-channel",
    field: "config",
    ciphertext: cipher,
  });
  return new TextDecoder("utf-8").decode(new Uint8Array(rsp.data as ArrayBuffer));
};

// viewConfigPlaintext 列表单元格查看渠道配置明文：复用共享明文对话框解密展示
const viewConfigPlaintext = async (item: Record<string, unknown>): Promise<void> => {
  const ciphertext = item.config;
  if (isEmptyCiphertext(ciphertext)) {
    return;
  }
  const label = i18n.global.t("model.notificationChannel.field.config");
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
    "notification-channel",
    label,
  );
};

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    validateEditConfig();
    await getSimpleResponse(
      notificationChannelUpdate(editChannelId, {
        enabled: values.enabled,
        config: dumpEditConfigJson(),
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

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "type",
  "enabled",
  "config",
  ...visible,
];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
  };
  // 占位符渠道（如 webhook）为系统固定数据，随命名空间种子化，前端隐藏删除入口
  if (!channelStore.channelTypeMap[item.type]?.placeholder) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    actions.delete = (i: Record<string, any>) => {
      batchState.value = null;
      deleteConfirmRef.value?.open(i);
    };
  }
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
    notificationChannelBatchDelete(ids),
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
    notificationChannelBatchSetEnabled(enabled, ids),
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
    notificationChannelBatchDelete([proxy.id]),
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
    notificationChannelBatchSetEnabled(toggleTarget.value, [proxy.id]),
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
