<template>
  <main-layout current-nav="notification">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="notificationTargetListGet"
        :options-request="notificationTargetListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="notificationTarget"
      >
        <!-- 渠道列显示渠道名称（与创建/编辑表单一致），而非渠道 ID -->
        <template #cell-channelId="{ item }">
          {{ item.channelName || item.channelId }}
        </template>
        <!-- 目标类型列显示可读名称（与渠道类型一致），而非原始类型键 -->
        <template #cell-type="{ value }">
          {{ enumLabelOf(EnumClass.channelType, String(value ?? "")) }}
        </template>
        <!-- 语言列显示可读名称（与创建/编辑表单及筛选下拉一致），而非原始语言键 -->
        <template #cell-language="{ value }">
          {{ $t(languageNameKey(String(value ?? ""))) }}
        </template>
        <!-- config 为加密字段，列表统一用加密字段组件展示（提供明文查看入口） -->
        <template #cell-config="{ item }">
          <encrypted-value-cell
            kind="crypto"
            :item="item"
            field-key="config"
            @view-plaintext="viewConfigPlaintext"
          />
        </template>
      </search-table>

      <!-- 删除确认 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.notificationTarget.name')"
        prepend-icon="mdi-bell-outline"
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
                category: $t(`model.notificationTarget.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 启用/禁用确认 -->
      <confirm-dialog
        ref="toggleConfirmRef"
        :title="$t('model.notificationTarget.name')"
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
                category: $t(`model.notificationTarget.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 共享明文对话框：查看目标配置明文 -->
      <plaintext-dialog ref="plaintextDialogRef" />

      <!-- 创建对话框 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.notificationTarget.name')"
        :confirm-disabled="!createFormValid"
        force-editable
        prepend-icon="mdi-bell-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  :label="$t('model.notificationTarget.field.name')"
                  :rules="requiredRule($t('model.notificationTarget.field.name'))"
                  required
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="createLanguage"
                  :items="languageOptions"
                  item-title="title"
                  item-value="value"
                  :label="$t('model.notificationTarget.field.language')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  :label="$t('model.notificationTarget.field.description')"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="createEnabled"
                  :label="$t('model.notificationTarget.field.enabled')"
                  color="primary"
                  hide-details
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="createChannelId"
                  :items="channelOptions"
                  item-title="name"
                  item-value="id"
                  :label="$t('model.notificationTarget.field.channelId')"
                  :rules="requiredRule($t('model.notificationTarget.field.channelId'))"
                  required
                  clearable
                />
              </v-col>
            </v-row>
            <v-divider class="my-2" />
            <v-row dense align="center">
              <v-col>
                <v-label>{{ $t("model.notificationTarget.field.config") }}</v-label>
              </v-col>
              <v-col v-if="createChannelType" cols="auto">
                <v-chip>
                  {{ enumLabelOf(EnumClass.channelType, createChannelType) }}
                </v-chip>
              </v-col>
            </v-row>
            <template v-for="field in createSchema" :key="field.name">
              <v-row dense>
                <v-col cols="12">
                  <v-text-field
                    v-if="field.kind === 'string' || field.kind === 'password'"
                    v-model="createConfigValues[field.name]"
                    :type="field.kind === 'password' ? 'password' : 'text'"
                    :label="$t(targetFieldLabel(createChannelType, field.name))"
                    :required="field.required"
                    :error-messages="fieldErrors(createConfigState.errors, field.name)"
                  />
                  <v-text-field
                    v-else-if="field.kind === 'number'"
                    v-model="createConfigValues[field.name]"
                    type="number"
                    :label="$t(targetFieldLabel(createChannelType, field.name))"
                    :required="field.required"
                    :error-messages="fieldErrors(createConfigState.errors, field.name)"
                  />
                  <v-textarea
                    v-else
                    v-model="createConfigValues[field.name]"
                    :label="$t(targetFieldLabel(createChannelType, field.name))"
                    :required="field.required"
                    :error-messages="fieldErrors(createConfigState.errors, field.name)"
                    rows="2"
                  />
                </v-col>
              </v-row>
            </template>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑对话框 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.notificationTarget.name')"
        :confirm-disabled="!editFormValid"
        prepend-icon="mdi-bell-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.notificationTarget.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="editLanguage"
                  :items="languageOptions"
                  item-title="title"
                  item-value="value"
                  :label="$t('model.notificationTarget.field.language')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editDescription"
                  :label="$t('model.notificationTarget.field.description')"
                  :disabled="!editable"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="editEnabled"
                  :label="$t('model.notificationTarget.field.enabled')"
                  color="primary"
                  :disabled="!editable"
                  hide-details
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  :model-value="proxy.channelId"
                  :items="channelOptions"
                  item-title="name"
                  item-value="id"
                  :label="$t('model.notificationTarget.field.channelId')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-divider class="my-2" />
            <v-row dense align="center">
              <v-col>
                <v-label>{{ $t("model.notificationTarget.field.config") }}</v-label>
              </v-col>
              <v-col v-if="editChannelType" cols="auto">
                <v-chip>
                  {{ enumLabelOf(EnumClass.channelType, editChannelType) }}
                </v-chip>
              </v-col>
            </v-row>
            <template v-for="field in editSchema" :key="field.name">
              <v-row dense>
                <v-col cols="12">
                  <v-text-field
                    v-if="field.kind === 'string' || field.kind === 'password'"
                    v-model="editConfigValues[field.name]"
                    :type="field.kind === 'password' ? 'password' : 'text'"
                    :label="$t(targetFieldLabel(editChannelType, field.name))"
                    :required="field.required"
                    :error-messages="fieldErrors(editConfigState.errors, field.name)"
                    :disabled="!editable"
                  />
                  <v-text-field
                    v-else-if="field.kind === 'number'"
                    v-model="editConfigValues[field.name]"
                    type="number"
                    :label="$t(targetFieldLabel(editChannelType, field.name))"
                    :required="field.required"
                    :error-messages="fieldErrors(editConfigState.errors, field.name)"
                    :disabled="!editable"
                  />
                  <v-textarea
                    v-else
                    v-model="editConfigValues[field.name]"
                    :label="$t(targetFieldLabel(editChannelType, field.name))"
                    :required="field.required"
                    :error-messages="fieldErrors(editConfigState.errors, field.name)"
                    :disabled="!editable"
                    rows="2"
                  />
                </v-col>
              </v-row>
            </template>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 测试发送结果对话框：触发后立即打开，展示发送进度与最终结果 -->
      <app-dialog
        v-model="testSendDialog"
        :title="$t('action.testSend')"
        icon="mdi-send-check-outline"
        size="md"
      >
        <div v-if="testSendLoading" class="d-flex align-center ga-2 text-on-surface-variant">
          <v-progress-circular indeterminate size="20" />
          <span>{{ $t("summary.dataLoading") }}</span>
        </div>
        <v-alert v-else-if="testSendError" type="error" :text="testSendError" />
        <template v-else-if="testSendResult">
          <div class="text-h6">{{ testSendResult.title }}</div>
          <pre class="zk-code-preview text-body-2">{{ testSendResult.content }}</pre>
          <v-divider class="my-2" />
          <div class="text-caption text-on-surface-variant">
            messageId: {{ testSendResult.messageId }}
          </div>
          <div class="text-caption text-on-surface-variant">
            {{ $t("model.notificationTarget.field.language") }}:
            {{ $t(languageNameKey(testSendResult.language)) }}
          </div>
          <v-divider class="my-2" />
          <div class="text-subtitle-2">{{ $t("model.notificationChannel.name") }}</div>
          <pre class="zk-code-preview text-body-2">{{
            JSON.stringify(testSendResult.channel, null, 2)
          }}</pre>
          <v-divider class="my-2" />
          <div class="text-subtitle-2">{{ $t("model.notificationTarget.name") }}</div>
          <pre class="zk-code-preview text-body-2">{{
            JSON.stringify(testSendResult.target, null, 2)
          }}</pre>
        </template>
        <template #footer>
          <v-btn variant="text" @click="testSendDialog = false">
            {{ $t("action.close") }}
          </v-btn>
        </template>
      </app-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Notification Target List
meta:
  title: title.notification.target
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="target">
import {
  notificationChannelListGet,
  notificationTargetBatchDelete,
  notificationTargetBatchSetEnabled,
  notificationTargetCreate,
  notificationTargetListGet,
  notificationTargetListOptions,
  notificationTargetTestSend,
  notificationTargetUpdate,
} from "@/apis/notification";
import { plaintextCryptoGet } from "@/apis/traffic";

import type {
  NotificationChannelItem,
  NotificationChannelListOut,
  NotificationChannelTypeField,
  NotificationTargetTestSendOut,
} from "@/apis/@types/notification";
import type { BatchResultOut } from "@/apis/@types/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { useNotificationChannelStore } from "@/stores/notificationChannel";
import { BackendDataType } from "@/types";
import { isEmptyCiphertext } from "@/utils/encrypted";
import i18n from "@/utils/i18n";
import { EnumClass, enumLabelOf } from "@/utils/enums";
import { languageNameKey, languageSelectOptions } from "@/utils/options";

const appStore = useAppStore();
const channelStore = useNotificationChannelStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");

// ---- 渠道数据：创建对话框选择渠道后据此决定目标类型与配置 schema ----
const channelOptions = ref<NotificationChannelItem[]>([]);

// 目标通知语言可选项：复用平台支持语言（SUPPORT_LANGUAGES），名称经 i18n 翻译
const languageOptions = computed(() => languageSelectOptions());

// 命名空间切换由 AppHeader 统一重载当前页（onNamespaceChange），此处无需监听
onMounted(() => {
  // 渠道类型注册表（含各类型配置 schema）由共享 store 统一提供，
  // 经 /notification/channel/types 动态拉取，新增渠道类型时前端零改动
  void channelStore.loadChannelTypes();
  void getResponse<NotificationChannelListOut>(
    notificationChannelListGet({ greedy: false, page: 1, pageSize: 100 }),
    async (rsp) => {
      channelOptions.value = rsp.data.items ?? [];
    },
  );
});

// 依据渠道类型取目标配置字段 schema（未命中时返回空列表）
const targetConfigSchema = (channelType: string): NotificationChannelTypeField[] =>
  channelStore.channelTypes.find((t) => t.type === channelType)?.config.target ?? [];

// targetFieldLabel 依据渠道类型与字段名拼接目标配置字段的 i18n label key（约 notify.channel.<type>.field.<field>）
const targetFieldLabel = (channelType: string, field: string): string =>
  `notify.channel.${channelType}.field.${field}`;

// 解析 object 类型配置字段的 JSON 文本：空串返回 null（视为未填写），非法 JSON 亦返回 null
const parseObjectValue = (raw: string): Record<string, unknown> | null => {
  const trimmed = raw.trim();
  if (!trimmed) {
    return null;
  }
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    return parsed !== null && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
};

// 校验动态配置：返回「字段名 -> 错误文案」映射（无错误字段不在映射中），
// 供各配置输入框以 error-messages 就地展示，替代原先的单条 alert 告警。
const validateConfig = (
  channelType: string,
  schema: NotificationChannelTypeField[],
  values: Record<string, string>,
): Record<string, string> => {
  const errors: Record<string, string> = {};
  for (const f of schema) {
    const raw = String(values[f.name] ?? "").trim();
    if (f.required && !raw) {
      errors[f.name] = i18n.global.t("validation.mixed.required", {
        path: i18n.global.t(targetFieldLabel(channelType, f.name)),
      });
      continue;
    }
    if (f.kind === "object" && raw && parseObjectValue(raw) === null) {
      errors[f.name] = i18n.global.t("validation.mixed.default", {
        path: i18n.global.t(targetFieldLabel(channelType, f.name)),
      });
    }
  }
  return errors;
};

// fieldErrors 返回某字段的错误文案数组（无错误时为空数组），供 error-messages 绑定。
const fieldErrors = (errors: Record<string, string>, name: string): string[] =>
  errors[name] ? [errors[name]] : [];

// 将表单原始值序列化为配置对象（object 字段经 JSON 解析、number 字段转数字）
const serializeConfig = (
  schema: NotificationChannelTypeField[],
  values: Record<string, string>,
): Record<string, unknown> => {
  const config: Record<string, unknown> = {};
  for (const f of schema) {
    // 密码字段保留原始输入（首尾空白是密码的合法字符），其余字段去除首尾空白
    const value = String(values[f.name] ?? "");
    const raw = f.kind === "password" ? value : value.trim();
    if (!raw) {
      continue;
    }
    switch (f.kind) {
      case "object":
        config[f.name] = parseObjectValue(raw);
        break;
      case "number":
        config[f.name] = Number(raw);
        break;
      default:
        config[f.name] = raw;
    }
  }
  return config;
};

// 将配置 JSON 字符串反解为表单原始值（object 字段格式化为缩进 JSON 文本）
const configToValues = (
  schema: NotificationChannelTypeField[],
  configStr: string,
): Record<string, string> => {
  let parsed: Record<string, unknown> = {};
  try {
    const data = JSON.parse(configStr || "{}") as unknown;
    if (data !== null && typeof data === "object" && !Array.isArray(data)) {
      parsed = data as Record<string, unknown>;
    }
  } catch {
    parsed = {};
  }
  const values: Record<string, string> = {};
  for (const f of schema) {
    const v = parsed[f.name];
    if (v === undefined || v === null) {
      values[f.name] = "";
    } else if (typeof v === "object") {
      values[f.name] = JSON.stringify(v, null, 2);
    } else {
      values[f.name] = String(v);
    }
  }
  return values;
};

// 从 schema 生成字段默认值（defaultValue 缺失时为空串）
const schemaDefaultValues = (schema: NotificationChannelTypeField[]): Record<string, string> => {
  const values: Record<string, string> = {};
  for (const f of schema) {
    const dv = f.defaultValue;
    if (dv === undefined || dv === null) {
      values[f.name] = "";
    } else if (typeof dv === "object") {
      values[f.name] = JSON.stringify(dv, null, 2);
    } else {
      values[f.name] = String(dv);
    }
  }
  return values;
};

// 通用必填校验规则（path 为字段可读名，用于拼接校验错误文案）
const requiredRule = (path: string) => [
  (v: unknown) =>
    (v !== null && v !== undefined && String(v).trim() !== "") ||
    i18n.global.t("validation.mixed.required", { path }),
];

// ---- create dialog ----
const createName = ref("");
const createChannelId = ref<number | null>(null);
const createEnabled = ref(true);
const createLanguage = ref("zh-Hans");
const createConfigValues = ref<Record<string, string>>({});
const createDescription = ref("");

// 选中的渠道 → 渠道类型 → 目标配置 schema
const selectedChannel = computed<NotificationChannelItem | undefined>(() =>
  channelOptions.value.find((c) => c.id === createChannelId.value),
);
const createChannelType = computed<string>(() => selectedChannel.value?.type ?? "");
const createSchema = computed<NotificationChannelTypeField[]>(() =>
  targetConfigSchema(createChannelType.value),
);

// 渠道切换后重置配置字段（按新 schema 的默认值）
watch(createChannelId, () => {
  createConfigValues.value = schemaDefaultValues(createSchema.value);
});

const createConfigState = computed<{ valid: boolean; errors: Record<string, string> }>(() => {
  const errors = validateConfig(
    createChannelType.value,
    createSchema.value,
    createConfigValues.value,
  );
  return { valid: Object.keys(errors).length === 0, errors };
});

const createFormValid = computed<boolean>(
  () =>
    !!createName.value.trim() && createChannelId.value !== null && createConfigState.value.valid,
);

const openCreateDialogAction = (): void => {
  createName.value = "";
  createChannelId.value = null;
  createEnabled.value = true;
  createLanguage.value = "zh-Hans";
  createDescription.value = "";
  createConfigValues.value = {};
  createDialogRef.value?.open({});
};

const createAction = async (): Promise<void> => {
  if (!createFormValid.value) {
    return;
  }
  await getSimpleResponse(
    notificationTargetCreate({
      // 目标名称为单行文本输入，去除首尾空白避免误输入不可见空白
      name: createName.value.trim(),
      channelId: createChannelId.value as number,
      enabled: createEnabled.value,
      config: JSON.stringify(serializeConfig(createSchema.value, createConfigValues.value)),
      language: createLanguage.value,
      description: createDescription.value,
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
};

// ---- edit dialog ----
let editTargetId = 0;
const editChannelType = ref("");
const editEnabled = ref(true);
const editLanguage = ref("zh-Hans");
const editConfigValues = ref<Record<string, string>>({});
const editDescription = ref("");

// 编辑时渠道不可变，schema 由目标的渠道类型决定
const editSchema = computed<NotificationChannelTypeField[]>(() =>
  targetConfigSchema(editChannelType.value),
);

const editConfigState = computed<{ valid: boolean; errors: Record<string, string> }>(() => {
  const errors = validateConfig(editChannelType.value, editSchema.value, editConfigValues.value);
  return { valid: Object.keys(errors).length === 0, errors };
});

const editFormValid = computed<boolean>(() => editConfigState.value.valid);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  editTargetId = item.id as number;
  editChannelType.value = String(item.channelType ?? "");
  editEnabled.value = Boolean(item.enabled);
  editLanguage.value = String(item.language ?? "zh-Hans");
  editDescription.value = String(item.description ?? "");
  // config 为 base64 密文，需经明文接口解密后再回填编辑表单
  const plaintext = await loadTargetConfigPlaintext(String(item.config ?? ""));
  editConfigValues.value = configToValues(editSchema.value, plaintext);
  editDialogRef.value?.open(item);
};

// loadTargetConfigPlaintext 经明文接口解密目标配置密文为明文 JSON 文本；
// 无密文或为空 JSON（"{}"）时返回空字符串。
const loadTargetConfigPlaintext = async (cipher: string): Promise<string> => {
  if (isEmptyCiphertext(cipher)) {
    return "";
  }
  const rsp = await plaintextCryptoGet({
    model: "notification-target",
    field: "config",
    ciphertext: cipher,
  });
  return new TextDecoder("utf-8").decode(new Uint8Array(rsp.data as ArrayBuffer));
};

// viewConfigPlaintext 列表单元格查看目标配置明文：复用共享明文对话框解密展示
const viewConfigPlaintext = async (item: Record<string, unknown>): Promise<void> => {
  const ciphertext = item.config;
  if (isEmptyCiphertext(ciphertext)) {
    return;
  }
  const label = i18n.global.t("model.notificationTarget.field.config");
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
    "notification-target",
    label,
  );
};

const itemUpdateAction = async (): Promise<void> => {
  if (!editFormValid.value) {
    return;
  }
  await getSimpleResponse(
    notificationTargetUpdate(editTargetId, {
      enabled: editEnabled.value,
      config: JSON.stringify(serializeConfig(editSchema.value, editConfigValues.value)),
      language: editLanguage.value,
      description: editDescription.value,
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
};

// ---- test-send ----
const testSendResult = ref<NotificationTargetTestSendOut | null>(null);
const testSendDialog = ref(false);
const testSendLoading = ref(false);
const testSendError = ref("");

// 触发后立即打开对话框（请求进行中显示加载态），完成后在对话框内展示成功结果，
// 或失败原因（后端以 400 + CodeNotificationSendFailed 将 details 作为具体失败原因返回）。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const testSendAction = async (item: Record<string, any>): Promise<void> => {
  testSendResult.value = null;
  testSendError.value = "";
  testSendLoading.value = true;
  testSendDialog.value = true;
  await getResponse<NotificationTargetTestSendOut>(
    notificationTargetTestSend(item.id),
    async (rsp) => {
      testSendLoading.value = false;
      testSendResult.value = rsp.data;
      appStore.notify({
        text: i18n.global.t("notice.notification.testSendSuccess"),
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      testSendLoading.value = false;
      // details 为渠道返回的具体失败原因，缺失时回退本地化的 summary
      testSendError.value = typeof rsp.details === "string" ? rsp.details : rsp.summary;
    },
  );
};

// ---- list actions ----
// projectFields 字段投影：编辑对话框直接回填列表项字段，恒定投影避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "channelType",
  "enabled",
  "language",
  "description",
  "config",
  ...visible,
];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    testSend: testSendAction,
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
    notificationTargetBatchDelete(ids),
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
    notificationTargetBatchSetEnabled(enabled, ids),
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
    notificationTargetBatchDelete([proxy.id]),
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
    notificationTargetBatchSetEnabled(toggleTarget.value, [proxy.id]),
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
