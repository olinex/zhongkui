<template>
  <main-layout current-nav="notification">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="notificationTemplateListGet"
        :options-request="notificationTemplateListOptions"
        :actions="getActions"
        :create="openCreateDialogAction"
        model-name="notificationTemplate"
      >
        <!-- 事件列：展示事件的可读名称（i18n 翻译），而非后端注册表原始键 -->
        <template #cell-event="{ value }">
          {{ eventNameKey(String(value ?? "")) }}
        </template>
        <!-- 渠道类型列：展示渠道类型可读名称（与修改表单一致），而非原始类型键 -->
        <template #cell-channelType="{ value }">
          {{ channelTypeNameKey(String(value ?? "")) }}
        </template>
        <!-- 语言列：展示语言可读名称（与修改表单一致），而非原始语言键 -->
        <template #cell-language="{ value }">
          {{ $t(languageNameKey(String(value ?? ""))) }}
        </template>
        <!-- 来源列：default / override 以彩色 chip 高亮展示（i18n 翻译展示名，与筛选下拉一致） -->
        <template #cell-source="{ value }">
          <v-chip
            :color="value === 'override' ? 'primary' : 'default'"
            size="small"
            variant="tonal"
          >
            {{ sourceNameKey(String(value ?? "")) }}
          </v-chip>
        </template>
      </search-table>

      <!-- 恢复默认确认对话框 -->
      <confirm-dialog
        ref="restoreConfirmRef"
        :title="$t('model.notificationTemplate.name')"
        prepend-icon="mdi-restore"
        @confirm="itemRestoreDefaultAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.restoreDefault')"
            :text="
              $t('notice.notification.restoreDefaultConfirm', {
                event: eventNameKey(String(proxy.event)),
                language: $t(languageNameKey(String(proxy.language))),
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 创建模板对话框：事件/渠道/语言枚举必选，标题与正文必填，描述可选 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.notificationTemplate.name')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-file-document-edit-outline"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="createEvent"
                  v-bind="createEventProps"
                  :items="eventOptions"
                  :label="$t('model.notificationTemplate.field.event')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="6">
                <v-select
                  v-model="createChannelType"
                  v-bind="createChannelTypeProps"
                  :items="createChannelOptions"
                  :label="$t('model.notificationTemplate.field.channelType')"
                />
              </v-col>
              <v-col cols="6">
                <v-select
                  v-model="createLanguage"
                  v-bind="createLanguageProps"
                  :items="createLanguageOptions"
                  item-title="title"
                  item-value="value"
                  :label="$t('model.notificationTemplate.field.language')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.notificationTemplate.field.description')"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <go-template-editor
                  v-model="createTitle"
                  :fields="createEventFields"
                  :label="$t('model.notificationTemplate.field.title')"
                  :valid="editorTitleValid"
                  :error-message="editorTitleError"
                  min-height="72px"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <go-template-editor
                  v-model="createContent"
                  :fields="createEventFields"
                  :label="$t('model.notificationTemplate.field.content')"
                  :valid="editorContentValid"
                  :error-message="editorContentError"
                  min-height="140px"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <div class="d-flex align-center ga-2">
                  <v-btn
                    v-request="createValidateAction"
                    size="small"
                    variant="tonal"
                    color="primary"
                  >
                    {{ $t("action.templateValidate") }}
                  </v-btn>
                  <v-btn
                    size="small"
                    variant="tonal"
                    :loading="tmplDryRunLoading"
                    @click="createDryRunAction"
                  >
                    {{ $t("action.templateDryRun") }}
                  </v-btn>
                  <v-spacer />
                </div>
                <v-alert
                  v-if="tmplDryRunOpen && tmplDryRunError"
                  type="error"
                  variant="tonal"
                  class="mt-2"
                  :text="tmplDryRunError"
                />
                <template v-if="tmplDryRunOpen && tmplDryRun">
                  <v-divider class="my-2" />
                  <div class="text-subtitle-2">
                    {{ $t("model.notificationTemplate.field.title") }}
                  </div>
                  <pre class="render-preview">{{ tmplDryRun.title }}</pre>
                  <div class="text-subtitle-2 mt-2">
                    {{ $t("model.notificationTemplate.field.content") }}
                  </div>
                  <pre class="render-preview">{{ tmplDryRun.content }}</pre>
                </template>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑模板对话框：事件/渠道/语言只读，仅标题/正文/描述可编辑 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.notificationTemplate.name')"
        :confirm-disabled="!updateForm.meta.value.valid"
        prepend-icon="mdi-file-document-edit-outline"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  :model-value="eventNameKey(proxy.event)"
                  :label="$t('model.notificationTemplate.field.event')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="6">
                <v-text-field
                  :model-value="channelTypeNameKey(proxy.channelType)"
                  :label="$t('model.notificationTemplate.field.channelType')"
                  disabled
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  :model-value="$t(languageNameKey(proxy.language))"
                  :label="$t('model.notificationTemplate.field.language')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="updateDescription"
                  v-bind="updateDescriptionProps"
                  :label="$t('model.notificationTemplate.field.description')"
                  :disabled="!editable"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <go-template-editor
                  v-model="updateTitle"
                  :fields="updateEventFields"
                  :label="$t('model.notificationTemplate.field.title')"
                  :valid="editorTitleValid"
                  :error-message="editorTitleError"
                  :disabled="!editable"
                  min-height="72px"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <go-template-editor
                  v-model="updateContent"
                  :fields="updateEventFields"
                  :label="$t('model.notificationTemplate.field.content')"
                  :valid="editorContentValid"
                  :error-message="editorContentError"
                  :disabled="!editable"
                  min-height="140px"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <div class="d-flex align-center ga-2">
                  <v-btn
                    size="small"
                    variant="tonal"
                    color="primary"
                    :loading="tmplValidateLoading"
                    @click="updateValidateAction"
                  >
                    {{ $t("action.templateValidate") }}
                  </v-btn>
                  <v-btn
                    size="small"
                    variant="tonal"
                    :loading="tmplDryRunLoading"
                    @click="updateDryRunAction"
                  >
                    {{ $t("action.templateDryRun") }}
                  </v-btn>
                  <v-spacer />
                </div>
                <v-alert
                  v-if="tmplDryRunOpen && tmplDryRunError"
                  type="error"
                  variant="tonal"
                  class="mt-2"
                  :text="tmplDryRunError"
                />
                <template v-if="tmplDryRunOpen && tmplDryRun">
                  <v-divider class="my-2" />
                  <div class="text-subtitle-2">
                    {{ $t("model.notificationTemplate.field.title") }}
                  </div>
                  <pre class="render-preview">{{ tmplDryRun.title }}</pre>
                  <div class="text-subtitle-2 mt-2">
                    {{ $t("model.notificationTemplate.field.content") }}
                  </div>
                  <pre class="render-preview">{{ tmplDryRun.content }}</pre>
                </template>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 渲染测试对话框：选择事件/渠道/语言并编辑模板参数，实时预览渲染后的标题与正文 -->
      <v-dialog v-model="renderDialog" max-width="720">
        <v-card prepend-icon="mdi-play-box-outline" :title="$t('action.renderTest')">
          <v-card-text>
            <v-form>
              <v-row dense>
                <v-col cols="12">
                  <v-select
                    v-model="renderEvent"
                    :items="eventOptions"
                    item-title="title"
                    item-value="value"
                    :label="$t('model.notificationTemplate.field.event')"
                  />
                </v-col>
              </v-row>
              <v-row dense>
                <v-col cols="6">
                  <v-select
                    v-model="renderChannelType"
                    :items="renderChannelOptions"
                    :label="$t('model.notificationTemplate.field.channelType')"
                  />
                </v-col>
                <v-col cols="6">
                  <v-select
                    v-model="renderLanguage"
                    :items="renderLanguageOptions"
                    item-title="title"
                    item-value="value"
                    :label="$t('model.notificationTemplate.field.language')"
                  />
                </v-col>
              </v-row>
              <v-divider class="my-2" />
              <v-row dense align="center">
                <v-col>
                  <v-label>{{ $t("model.notificationTemplate.field.params") }}</v-label>
                </v-col>
                <v-col cols="auto">
                  <v-btn
                    :text="$t('action.add')"
                    size="small"
                    variant="tonal"
                    color="primary"
                    @click="addRenderParam"
                  />
                </v-col>
              </v-row>
              <v-row v-for="(row, index) in renderParams" :key="index" dense align="center">
                <v-col cols="5">
                  <v-text-field v-model="row.key" :label="$t('model.__base.field.field')" />
                </v-col>
                <v-col cols="6">
                  <v-text-field v-model="row.value" :label="$t('model.__base.field.value')" />
                </v-col>
                <v-col cols="1">
                  <v-btn
                    icon="mdi-minus"
                    variant="tonal"
                    color="error"
                    density="compact"
                    @click="removeRenderParam(index)"
                  />
                </v-col>
              </v-row>
            </v-form>
            <v-divider class="my-3" />
            <div class="d-flex ga-2">
              <v-btn
                v-request="renderTestAction"
                color="primary"
                variant="tonal"
                :disabled="!renderEvent"
              >
                {{ $t("action.renderTest") }}
              </v-btn>
              <v-spacer />
              <v-btn variant="text" @click="renderDialog = false">
                {{ $t("action.close") }}
              </v-btn>
            </div>
            <v-alert
              v-if="renderError"
              type="error"
              variant="tonal"
              class="mt-3"
              :text="renderError"
            />
            <template v-if="renderResult">
              <v-divider class="my-3" />
              <div class="text-subtitle-2">{{ $t("model.notificationTemplate.field.title") }}</div>
              <pre class="render-preview">{{ renderResult.title }}</pre>
              <div class="text-subtitle-2 mt-2">
                {{ $t("model.notificationTemplate.field.content") }}
              </div>
              <pre class="render-preview">{{ renderResult.content }}</pre>
              <div class="text-caption text-medium-emphasis mt-2">
                channelType: {{ channelTypeNameKey(renderResult.channelType) }} · language:
                {{ $t(languageNameKey(renderResult.language)) }}
              </div>
            </template>
          </v-card-text>
        </v-card>
      </v-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Notification Template List
meta:
  title: title.notification.template
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="notification-template">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  notificationTemplateCreate,
  notificationTemplateDryRun,
  notificationTemplateEventsGet,
  notificationTemplateListGet,
  notificationTemplateListOptions,
  notificationTemplateRenderTest,
  notificationTemplateRestoreDefault,
  notificationTemplateUpdate,
  notificationTemplateValidate,
} from "@/apis/notification";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type {
  NotificationTemplateDryRunIn,
  NotificationTemplateDryRunOut,
  NotificationTemplateEventsOut,
  NotificationTemplateEventItemOut,
  NotificationTemplateRenderTestOut,
  NotificationTemplateValidateOut,
} from "@/apis/@types/notification";
import type { ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { useNotificationChannelStore } from "@/stores/notificationChannel";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";
import {
  languageNameKey,
  languageSelectOptions,
  SUPPORT_LANGUAGES,
  type LanguageSelectOption,
} from "@/utils/options";

// Types
import type { ChannelTypeOption } from "@/stores/notificationChannel";

// ---- 校验 schema ----
const createTemplateSchema = yup.object({
  event: yup.string().nonNullable().required().label("model.notificationTemplate.field.event"),
  channelType: yup
    .string()
    .nonNullable()
    .required()
    .label("model.notificationTemplate.field.channelType"),
  language: yup
    .string()
    .nonNullable()
    .required()
    .label("model.notificationTemplate.field.language"),
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .label("model.notificationTemplate.field.title"),
  content: yup.string().nonNullable().required().label("model.notificationTemplate.field.content"),
  description: yup
    .string()
    .nonNullable()
    .max(256)
    .label("model.notificationTemplate.field.description"),
});

const updateTemplateSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .label("model.notificationTemplate.field.title"),
  content: yup.string().nonNullable().required().label("model.notificationTemplate.field.content"),
  description: yup
    .string()
    .nonNullable()
    .max(256)
    .label("model.notificationTemplate.field.description"),
});

const appStore = useAppStore();
const channelStore = useNotificationChannelStore();

// ---- 枚举展示名辅助：事件/渠道类型均为后端注册表决定的技术键，展示名经 i18n 翻译 ----
// eventNameKey 事件展示名（键为 model.notificationTemplate.type.event.<event>，如 tag_event_record）
const eventNameKey = (event: string): string =>
  i18n.global.t(`model.notificationTemplate.type.event.${event}`);

// channelTypeNameKey 渠道类型展示名：* 表示全部渠道（option.channel.* 翻译），
// 其余经共享 store 的全量渠道选项匹配翻译；未命中回退技术键
const channelTypeNameKey = (channelType: string): string => {
  if (channelType === "*") {
    return i18n.global.t(CHANNEL_TYPE_ALL_KEY);
  }
  return (
    channelStore.allChannelTypeOptions.find((o) => o.value === channelType)?.title ?? channelType
  );
};

// sourceNameKey 模板来源展示名（键为 model.notificationTemplate.type.source.<source>，
// 如 default / override），与筛选下拉选项的 i18n 键保持一致
const sourceNameKey = (source: string): string =>
  i18n.global.t(`model.notificationTemplate.type.source.${source}`);

const searchTableRef = useTemplateRef("searchTableRef");
const restoreConfirmRef = useTemplateRef("restoreConfirmRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");

// ---- 事件枚举：驱动创建对话框与渲染测试对话框的下拉选项 ----
const eventItems = ref<NotificationTemplateEventItemOut[]>([]);
// EventOption 事件下拉可选项：title 为 i18n 翻译展示名（model.notificationTemplate.type.event.<event>），
// value 保持后端事件注册表返回的技术键（如 tag_event_record）；新增事件仅需后端注册 + 补充翻译
const eventOptions = computed<{ title: string; value: string }[]>(() =>
  eventItems.value.map((e) => ({
    title: i18n.global.t(`model.notificationTemplate.type.event.${e.event}`),
    value: e.event,
  })),
);

const eventItemOf = (event: string): NotificationTemplateEventItemOut | undefined =>
  eventItems.value.find((e) => e.event === event);

// mergeOptions 合并多个字符串列表并去重，用于组合渠道/语言可选项（含 "*" 通配）
const mergeOptions = (...lists: (string[] | undefined)[]): string[] => [
  ...new Set(lists.flat().filter((v): v is string => !!v)),
];

// 平台默认语言（与后端 base.DefaultLanguage 一致）
const PLATFORM_DEFAULT_LANGUAGE = "zh-Hans";

// 渠道类型通配符 "*"（全部渠道）的 i18n 键，与语言通配 option.language.* 平行
const CHANNEL_TYPE_ALL_KEY = "option.channel.*";

// ---- forms ----
const createForm = useForm({ validationSchema: createTemplateSchema });
const updateForm = useForm({ validationSchema: updateTemplateSchema });

const [createEvent, createEventProps] = createForm.defineField("event", vuetifyCfg);
const [createChannelType, createChannelTypeProps] = createForm.defineField(
  "channelType",
  vuetifyCfg,
);
const [createLanguage, createLanguageProps] = createForm.defineField("language", vuetifyCfg);
const [createTitle] = createForm.defineField("title", vuetifyCfg);
const [createContent] = createForm.defineField("content", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const [updateTitle] = updateForm.defineField("title", vuetifyCfg);
const [updateContent] = updateForm.defineField("content", vuetifyCfg);
const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);

// 平台支持的语言键（始终纳入可选项，确保未选事件或事件语言受限时英文等不缺失）
const PLATFORM_LANGUAGES: string[] = SUPPORT_LANGUAGES.map((l) => l.value);

// 创建对话框的渠道可选项：通配符 "*"（全部渠道）+ 共享 store 提供的全量渠道（含占位符如 webhook，
// 模板需按渠道类型创建覆盖模板，故不随渠道页过滤）；新增渠道类型时前端零改动；语言可选项随所选事件变化
const createChannelOptions = computed<ChannelTypeOption[]>(() => [
  { title: i18n.global.t(CHANNEL_TYPE_ALL_KEY), value: "*" },
  ...channelStore.allChannelTypeOptions,
]);
const createLanguageOptions = computed<LanguageSelectOption[]>(() =>
  languageSelectOptions(
    mergeOptions(["*", ...PLATFORM_LANGUAGES], eventItemOf(createEvent.value)?.languages),
  ),
);

watch(createEvent, () => {
  createForm.resetField("channelType", { value: "*" });
  createForm.resetField("language", { value: PLATFORM_DEFAULT_LANGUAGE });
});

// ---- 编辑器校验 / 试运行（标题与正文的 Go text/template 工具）----
// 校验与试运行两个对话框共用同一组结果状态，任一时间只有一个对话框可见，
// 且对话框打开前经 resetEditorTools 清空，故不会互相污染。
const tmplValidateLoading = ref(false);
const tmplValidate = ref<NotificationTemplateValidateOut | null>(null);
const tmplDryRunLoading = ref(false);
const tmplDryRun = ref<NotificationTemplateDryRunOut | null>(null);
const tmplDryRunError = ref("");
const tmplDryRunOpen = ref(false);

// 标题/正文各自的校验错误信息回填到对应编辑器（合法或无结果时为空串）
const editorTitleError = computed(() =>
  tmplValidate.value && !tmplValidate.value.titleValid ? tmplValidate.value.titleError : "",
);
const editorContentError = computed(() =>
  tmplValidate.value && !tmplValidate.value.contentValid ? tmplValidate.value.contentError : "",
);
const editorTitleValid = computed(() => tmplValidate.value?.titleValid ?? false);
const editorContentValid = computed(() => tmplValidate.value?.contentValid ?? false);

// 编辑器智能补全的可用变量取自所选事件的字段清单（后端 events 接口的 fields）
const createEventFields = computed<string[]>(() => eventItemOf(createEvent.value)?.fields ?? []);

// 编辑对话框的事件/渠道/语言为只读展示，试运行需携带其值，故打开时缓存
const updateEvent = ref("");
const updateChannelType = ref("");
const updateLanguage = ref("");
const updateEventFields = computed<string[]>(() => eventItemOf(updateEvent.value)?.fields ?? []);

// resetEditorTools 打开对话框前重置校验与试运行状态
const resetEditorTools = (): void => {
  tmplValidate.value = null;
  tmplDryRun.value = null;
  tmplDryRunError.value = "";
  tmplDryRunOpen.value = false;
};

// validateAction 调后端 text/template 权威语法校验，标题/正文各自返回合法性（恒 200）
const validateAction = async (title: string, content: string): Promise<void> => {
  tmplValidateLoading.value = true;
  await getResponse<NotificationTemplateValidateOut>(
    notificationTemplateValidate({ title, content }),
    async (rsp) => {
      tmplValidateLoading.value = false;
      tmplValidate.value = rsp.data;
    },
    async (rsp) => {
      tmplValidateLoading.value = false;
      appStore.notify({ text: rsp.summary, timeout: 3000, color: "error" });
    },
  );
};

const createValidateAction = (): Promise<void> =>
  validateAction(createTitle.value, createContent.value);
const updateValidateAction = (): Promise<void> =>
  validateAction(updateTitle.value, updateContent.value);

// dryRunAction 用事件的内置样例参数渲染编辑器内尚未保存的标题/正文（无需手填参数）
const dryRunAction = async (inData: NotificationTemplateDryRunIn): Promise<void> => {
  tmplDryRunLoading.value = true;
  tmplDryRun.value = null;
  tmplDryRunError.value = "";
  tmplDryRunOpen.value = true;
  await getResponse<NotificationTemplateDryRunOut>(
    notificationTemplateDryRun(inData),
    async (rsp) => {
      tmplDryRunLoading.value = false;
      tmplDryRun.value = rsp.data;
    },
    async (rsp) => {
      tmplDryRunLoading.value = false;
      // details 为具体渲染失败原因（如模板非法），缺失时回退本地化 summary
      tmplDryRunError.value = typeof rsp.details === "string" ? rsp.details : rsp.summary;
    },
  );
};

const createDryRunAction = (): Promise<void> =>
  dryRunAction({
    event: createEvent.value,
    channelType: createChannelType.value,
    language: createLanguage.value,
    title: createTitle.value,
    content: createContent.value,
  });
const updateDryRunAction = (): Promise<void> =>
  dryRunAction({
    event: updateEvent.value,
    channelType: updateChannelType.value,
    language: updateLanguage.value,
    title: updateTitle.value,
    content: updateContent.value,
  });

// 标题/正文文本变更后清除上次的校验结果，避免残留过期错误提示
watch(
  () => [createTitle.value, createContent.value, updateTitle.value, updateContent.value],
  () => {
    tmplValidate.value = null;
  },
);

onMounted(() => {
  void channelStore.loadChannelTypes();
  void getResponse<NotificationTemplateEventsOut>(
    notificationTemplateEventsGet(),
    async (rsp) => {
      eventItems.value = rsp.data.events ?? [];
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

// ---- 创建模板 ----
const openCreateDialogAction = (): void => {
  createForm.resetField("event", { value: "" });
  createForm.resetField("channelType", { value: "*" });
  createForm.resetField("language", { value: PLATFORM_DEFAULT_LANGUAGE });
  createForm.resetField("title", { value: "" });
  createForm.resetField("content", { value: "" });
  createForm.resetField("description", { value: "" });
  resetEditorTools();
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    notificationTemplateCreate({
      event: values.event,
      channelType: values.channelType,
      language: values.language,
      title: values.title,
      content: values.content,
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

// ---- 编辑模板（列表项已含编辑所需全部字段，直接回填，无需额外详情接口）----
let editTemplateId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = (item: Record<string, any>): void => {
  editTemplateId = item.id as number;
  updateEvent.value = String(item.event ?? "");
  updateChannelType.value = String(item.channelType ?? "*");
  updateLanguage.value = String(item.language ?? PLATFORM_DEFAULT_LANGUAGE);
  updateForm.resetField("title", { value: String(item.title ?? "") });
  updateForm.resetField("content", { value: String(item.content ?? "") });
  updateForm.resetField("description", { value: String(item.description ?? "") });
  resetEditorTools();
  editDialogRef.value?.open(item);
};

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      notificationTemplateUpdate({
        id: editTemplateId,
        // 标题单行文本提交前去除首尾空白（正文/描述为多行文本域，空白具有语义，豁免 trim）
        title: (values.title as string).trim(),
        content: values.content,
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

// ---- 恢复默认（整事件重置为内置默认）----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemRestoreDefaultAction = async (proxy: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    notificationTemplateRestoreDefault({
      event: proxy.event as string,
      language: proxy.language as string,
    }),
    async () => {
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: i18n.global.t("notice.notification.restoreDefaultSuccess"),
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

// ---- 渲染测试 ----
const renderDialog = ref(false);
const renderEvent = ref("");
const renderChannelType = ref("*");
const renderLanguage = ref(PLATFORM_DEFAULT_LANGUAGE);
const renderParams = ref<Array<{ key: string; value: string }>>([]);
const renderLoading = ref(false);
const renderResult = ref<NotificationTemplateRenderTestOut | null>(null);
const renderError = ref("");

// 渲染测试对话框的渠道可选项：与创建对话框一致（通配符 + 注册表动态渠道，含占位符如 webhook）
const renderChannelOptions = computed<ChannelTypeOption[]>(() => [
  { title: i18n.global.t(CHANNEL_TYPE_ALL_KEY), value: "*" },
  ...channelStore.allChannelTypeOptions,
]);
const renderLanguageOptions = computed<LanguageSelectOption[]>(() =>
  languageSelectOptions(
    mergeOptions(["*", ...PLATFORM_LANGUAGES], eventItemOf(renderEvent.value)?.languages),
  ),
);

watch(renderEvent, () => {
  renderChannelType.value = "*";
  renderLanguage.value = PLATFORM_DEFAULT_LANGUAGE;
  renderResult.value = null;
  renderError.value = "";
});

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const openRenderDialog = (item: Record<string, any>): void => {
  renderEvent.value = String(item.event ?? "");
  renderChannelType.value = "*";
  renderLanguage.value = String(item.language ?? PLATFORM_DEFAULT_LANGUAGE);
  renderParams.value = [];
  renderResult.value = null;
  renderError.value = "";
  renderDialog.value = true;
};

const addRenderParam = (): void => {
  renderParams.value.push({ key: "", value: "" });
};

const removeRenderParam = (index: number): void => {
  renderParams.value.splice(index, 1);
};

// renderParamsToMap 将 params 行列表序列化为后端 text/template 渲染所需的参数字典
const renderParamsToMap = (): Record<string, unknown> => {
  const params: Record<string, unknown> = {};
  for (const row of renderParams.value) {
    // 参数 key/value 均为单行文本输入，去除首尾空白避免误输入不可见空白
    const key = row.key.trim();
    if (key) {
      params[key] = row.value.trim();
    }
  }
  return params;
};

const renderTestAction = async (): Promise<void> => {
  renderLoading.value = true;
  renderResult.value = null;
  renderError.value = "";
  await getResponse<NotificationTemplateRenderTestOut>(
    notificationTemplateRenderTest({
      event: renderEvent.value,
      channelType: renderChannelType.value,
      language: renderLanguage.value,
      params: renderParamsToMap(),
    }),
    async (rsp) => {
      renderLoading.value = false;
      renderResult.value = rsp.data;
    },
    async (rsp) => {
      renderLoading.value = false;
      // details 为具体渲染失败原因（如未知事件、模板非法），缺失时回退本地化 summary
      renderError.value = typeof rsp.details === "string" ? rsp.details : rsp.summary;
    },
  );
};

// ---- 行操作 ----
// 模板行操作：编辑 + 恢复默认 + 渲染测试（模板不可单独删除，恢复默认以重置）
const getActions = (): ItemActions => {
  return {
    edit: detailGetAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    restoreDefault: (i: Record<string, any>) => {
      restoreConfirmRef.value?.open(i);
    },
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    renderTest: (i: Record<string, any>) => {
      openRenderDialog(i);
    },
  };
};
</script>

<style scoped>
/* 渲染测试预览框：保留换行与空白，限制最大高度超出可滚动 */
.render-preview {
  white-space: pre-wrap;
  background: rgb(0 0 0 / 5%);
  border-radius: 4px;
  padding: 8px;
  font-size: 13px;
  max-height: 320px;
  overflow: auto;
}
</style>
