<template>
  <main-layout current-nav="etl">
    <template #body>
      <!-- 列表骨架与 SearchTable 一致：本页无筛选条件，扁平卡片承载「操作按钮行 → 表格」 -->
      <v-card flat>
        <div class="px-4 pt-3 pb-2 d-flex justify-end align-center ga-2">
          <action-button-group
            :refresh="getSchedulerListAction"
            :create="openCreateDialogAction"
            collapsible
          >
            <column-visibility-menu
              v-model="schedulerVisibleKeys"
              v-model:order="schedulerColumnOrder"
              :headers="schedulerAllToggleable"
            />
          </action-button-group>
        </div>
        <v-container fluid class="px-4 pt-0 pb-4">
          <v-row density="compact">
            <v-col cols="12">
              <v-data-table :headers="schedulerVisibleHeaders" :items="schedulerItems">
                <template #item="{ item }">
                  <tr class="text-no-wrap">
                    <td
                      v-for="hdr in schedulerVisibleHeaders"
                      :key="hdr.key"
                      :class="
                        hdr.fixed === 'end'
                          ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                          : undefined
                      "
                    >
                      <enabled-switch-cell
                        v-if="hdr.key === 'enabled'"
                        v-model="item.enabled"
                        :request="
                          (enabled: boolean) =>
                            schedulerTaskToggle({ typeName: item.typeName, enabled })
                        "
                      />
                      <adaptive-cell
                        v-else
                        :header="hdr"
                        :item="item"
                        :actions="getSchedulerActions"
                      />
                    </td>
                  </tr>
                </template>
              </v-data-table>
            </v-col>
          </v-row>
        </v-container>
      </v-card>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.scheduler.name')"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-calendar-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createTypeName"
                  v-bind="createTypeNameProps"
                  :label="$t('model.scheduler.field.typeName')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <cron-field
                  v-model="createCronSpec"
                  v-bind="createCronSpecProps"
                  :label="$t('model.scheduler.field.cronSpec')"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="createInterval"
                  v-bind="createIntervalProps"
                  :label="$t('model.scheduler.field.interval')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createQueue"
                  v-bind="createQueueProps"
                  :label="$t('model.scheduler.field.queue')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="createTimeout"
                  v-bind="createTimeoutProps"
                  :label="$t('model.scheduler.field.timeout')"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="createMaxRetry"
                  v-bind="createMaxRetryProps"
                  type="number"
                  :label="$t('model.scheduler.field.maxRetry')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="createRetention"
                  v-bind="createRetentionProps"
                  :label="$t('model.scheduler.field.retention')"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="createLookback"
                  v-bind="createLookbackProps"
                  :label="$t('model.scheduler.field.lookback')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  :label="$t('model.scheduler.field.enabled')"
                  color="primary"
                  hide-details
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createPayload"
                  :label="$t('model.scheduler.field.payload')"
                  rows="3"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-switch
                  v-model="createBacklogEnabled"
                  v-bind="createBacklogEnabledProps"
                  :label="$t('model.scheduler.field.backlogEnabled')"
                  color="primary"
                  hide-details
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="createBacklogWindow"
                  v-bind="createBacklogWindowProps"
                  :label="$t('model.scheduler.field.backlogWindow')"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="createBacklogLimit"
                  v-bind="createBacklogLimitProps"
                  type="number"
                  :label="$t('model.scheduler.field.backlogLimit')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.scheduler.name')"
        :confirm-disabled="!editForm.meta.value.valid"
        prepend-icon="mdi-calendar-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  :model-value="editTypeName"
                  :label="$t('model.scheduler.field.typeName')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <cron-field
                  v-model="editCronSpec"
                  v-bind="editCronSpecProps"
                  :label="$t('model.scheduler.field.cronSpec')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="editInterval"
                  v-bind="editIntervalProps"
                  :label="$t('model.scheduler.field.interval')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="editQueue"
                  v-bind="editQueueProps"
                  :label="$t('model.scheduler.field.queue')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="editTimeout"
                  v-bind="editTimeoutProps"
                  :label="$t('model.scheduler.field.timeout')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="editMaxRetry"
                  v-bind="editMaxRetryProps"
                  type="number"
                  :label="$t('model.scheduler.field.maxRetry')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="editRetention"
                  v-bind="editRetentionProps"
                  :label="$t('model.scheduler.field.retention')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="editLookback"
                  v-bind="editLookbackProps"
                  :label="$t('model.scheduler.field.lookback')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-switch
                  v-model="editEnabled"
                  v-bind="editEnabledProps"
                  :label="$t('model.scheduler.field.enabled')"
                  color="primary"
                  hide-details
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="editPayload"
                  :label="$t('model.scheduler.field.payload')"
                  rows="3"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-switch
                  v-model="editBacklogEnabled"
                  v-bind="editBacklogEnabledProps"
                  :label="$t('model.scheduler.field.backlogEnabled')"
                  color="primary"
                  hide-details
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-text-field
                  v-model="editBacklogWindow"
                  v-bind="editBacklogWindowProps"
                  :label="$t('model.scheduler.field.backlogWindow')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-text-field
                  v-model="editBacklogLimit"
                  v-bind="editBacklogLimitProps"
                  type="number"
                  :label="$t('model.scheduler.field.backlogLimit')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.scheduler.name')"
        prepend-icon="mdi-calendar-remove"
        @confirm="deleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.delete')"
            :text="
              $t('notice.deleteConfirm', {
                category: $t('model.scheduler.name'),
                key: proxy.typeName,
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="runConfirmRef"
        :title="$t('model.scheduler.name')"
        prepend-icon="mdi-calendar-play"
        @confirm="runAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.run')"
            :text="
              $t('notice.runConfirm', {
                category: $t('model.scheduler.name'),
                key: proxy.typeName,
              })
            "
          />
        </template>
      </confirm-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: ETL Scheduler List
meta:
  title: title.schedulerEtl
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { InspectorSchedulerListItem, InspectorSchedulerListOut } from "./@types";
import {
  inspectorSchedulerListGet,
  schedulerTaskCreate,
  schedulerTaskDelete,
  schedulerTaskRun,
  schedulerTaskToggle,
  schedulerTaskUpdate,
} from "@/apis/etl";
import { useAppStore } from "@/stores/app";
import type { ItemActions, TableHeader } from "@/components/@types/table";
import i18n from "@/utils/i18n";
import { useColumnVisibility } from "@/utils/column";
import { BackendDataType } from "@/types";
import { EnumClass, ensureEnum } from "@/utils/enums";
import { useForm } from "vee-validate";
import { schedulerTaskSchema } from "@/schemas/scheduler";
import { vuetifyCfg } from "@/utils/form";
import type { SchedulerTaskIn } from "@/apis/@types/etl";

const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const runConfirmRef = useTemplateRef("runConfirmRef");

// 创建定时任务的表单与字段
const createForm = useForm({ validationSchema: schedulerTaskSchema });
const [createTypeName, createTypeNameProps] = createForm.defineField("typeName", vuetifyCfg);
const [createCronSpec, createCronSpecProps] = createForm.defineField("cronSpec", vuetifyCfg);
const [createInterval, createIntervalProps] = createForm.defineField("interval", vuetifyCfg);
const [createQueue, createQueueProps] = createForm.defineField("queue", vuetifyCfg);
const [createTimeout, createTimeoutProps] = createForm.defineField("timeout", vuetifyCfg);
const [createMaxRetry, createMaxRetryProps] = createForm.defineField("maxRetry", vuetifyCfg);
const [createRetention, createRetentionProps] = createForm.defineField("retention", vuetifyCfg);
const [createLookback, createLookbackProps] = createForm.defineField("lookback", vuetifyCfg);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);
const [createBacklogEnabled, createBacklogEnabledProps] = createForm.defineField(
  "backlogEnabled",
  vuetifyCfg,
);
const [createBacklogWindow, createBacklogWindowProps] = createForm.defineField(
  "backlogWindow",
  vuetifyCfg,
);
const [createBacklogLimit, createBacklogLimitProps] = createForm.defineField(
  "backlogLimit",
  vuetifyCfg,
);
const createPayload = ref("{}");

// 编辑定时任务的表单与字段
const editForm = useForm({ validationSchema: schedulerTaskSchema });
const [editCronSpec, editCronSpecProps] = editForm.defineField("cronSpec", vuetifyCfg);
const [editInterval, editIntervalProps] = editForm.defineField("interval", vuetifyCfg);
const [editQueue, editQueueProps] = editForm.defineField("queue", vuetifyCfg);
const [editTimeout, editTimeoutProps] = editForm.defineField("timeout", vuetifyCfg);
const [editMaxRetry, editMaxRetryProps] = editForm.defineField("maxRetry", vuetifyCfg);
const [editRetention, editRetentionProps] = editForm.defineField("retention", vuetifyCfg);
const [editLookback, editLookbackProps] = editForm.defineField("lookback", vuetifyCfg);
const [editEnabled, editEnabledProps] = editForm.defineField("enabled", vuetifyCfg);
const [editBacklogEnabled, editBacklogEnabledProps] = editForm.defineField(
  "backlogEnabled",
  vuetifyCfg,
);
const [editBacklogWindow, editBacklogWindowProps] = editForm.defineField(
  "backlogWindow",
  vuetifyCfg,
);
const [editBacklogLimit, editBacklogLimitProps] = editForm.defineField("backlogLimit", vuetifyCfg);
const editPayload = ref("{}");
const editTypeName = ref("");

const appStore = useAppStore();
const schedulerHeaders = ref([
  {
    key: "typeName",
    title: i18n.global.t("model.scheduler.field.typeName"),
    type: BackendDataType.STRING,
    // 任务类型名称按 taskType 枚举渲染 label，避免与任务页的类型展示不一致
    enumType: EnumClass.taskType,
    align: "start",
    sortable: true,
  },
  {
    key: "cronSpec",
    title: i18n.global.t("model.scheduler.field.cronSpec"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: true,
  },
  {
    key: "interval",
    title: i18n.global.t("model.scheduler.field.interval"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: true,
  },
  {
    key: "queue",
    title: i18n.global.t("model.scheduler.field.queue"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: true,
  },
  {
    key: "timeout",
    title: i18n.global.t("model.scheduler.field.timeout"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: true,
  },
  {
    key: "maxRetry",
    title: i18n.global.t("model.scheduler.field.maxRetry"),
    type: BackendDataType.INT,
    align: "start",
    sortable: true,
  },
  {
    key: "retention",
    title: i18n.global.t("model.scheduler.field.retention"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: true,
  },
  {
    key: "lookback",
    title: i18n.global.t("model.scheduler.field.lookback"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: true,
  },
  {
    key: "enabled",
    title: i18n.global.t("model.scheduler.field.enabled"),
    type: BackendDataType.BOOLEAN,
    align: "start",
    sortable: true,
  },
  {
    key: "payload",
    title: i18n.global.t("model.scheduler.field.payload"),
    type: BackendDataType.JSON,
    align: "start",
    sortable: true,
  },
  {
    key: "backlogEnabled",
    title: i18n.global.t("model.scheduler.field.backlogEnabled"),
    type: BackendDataType.BOOLEAN,
    align: "start",
    sortable: true,
  },
  {
    key: "backlogWindow",
    title: i18n.global.t("model.scheduler.field.backlogWindow"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: true,
  },
  {
    key: "backlogLimit",
    title: i18n.global.t("model.scheduler.field.backlogLimit"),
    type: BackendDataType.INT,
    align: "start",
    sortable: true,
  },
  {
    key: "__actions",
    title: i18n.global.t("model.__base.field.__actions"),
    type: BackendDataType.ACTIONS,
    sortable: false,
    fixed: "end",
  },
] as Array<TableHeader>);
// 定时任务表格的列显示配置：持久化到 localStorage，表头/行渲染统一使用 visibleHeaders
const {
  allToggleable: schedulerAllToggleable,
  columnOrder: schedulerColumnOrder,
  visibleKeys: schedulerVisibleKeys,
  visibleHeaders: schedulerVisibleHeaders,
} = useColumnVisibility("model:scheduler", schedulerHeaders);
const schedulerItems = ref([] as InspectorSchedulerListItem[]);

const getSchedulerListAction = async (): Promise<void> => {
  await getResponse<InspectorSchedulerListOut>(
    inspectorSchedulerListGet(),
    async (rsp) => {
      schedulerItems.value = rsp.data;
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

// parsePayload 将负载文本解析为对象；空文本返回 undefined，非法 JSON 返回 null
const parsePayload = (text: string): Record<string, unknown> | null | undefined => {
  if (text.trim() === "") {
    return undefined;
  }
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
};

// scheduleModeError 校验 cronSpec 与 interval 的互斥关系，非法时返回 i18n 键，合法返回空
const scheduleModeError = (cronSpec: string, interval: string): string => {
  const hasCron = cronSpec.trim() !== "";
  const hasInterval = interval.trim() !== "";
  if (hasCron && hasInterval) {
    return "notice.schedulerScheduleConflict";
  }
  if (!hasCron && !hasInterval) {
    return "notice.schedulerScheduleEmpty";
  }
  return "";
};

const openCreateDialogAction = (): void => {
  // 复位是「准备本次打开的数据」的一部分：resetForm 原子重置值 + 校验状态，避免上次填写内容残留
  createForm.resetForm({
    values: {
      typeName: "",
      cronSpec: "",
      interval: "",
      queue: "",
      timeout: "",
      maxRetry: 0,
      retention: "1h",
      lookback: "5m",
      enabled: true,
      backlogEnabled: true,
      backlogWindow: "0",
      backlogLimit: 1000,
    },
  });
  createPayload.value = "{}";
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  const payload = parsePayload(createPayload.value);
  if (payload === null) {
    appStore.notify({
      text: i18n.global.t("notice.payloadInvalid"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  const scheduleError = scheduleModeError(values.cronSpec ?? "", values.interval ?? "");
  if (scheduleError !== "") {
    appStore.notify({
      text: i18n.global.t(scheduleError),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  // 单行文本字段提交前统一去除首尾空白，避免误输入空白导致查询失败或数据异常
  const typeName = (values.typeName as string).trim();
  const cronSpec = (values.cronSpec as string).trim();
  const interval = (values.interval as string).trim();
  const queue = (values.queue as string).trim();
  const timeout = (values.timeout as string).trim();
  const data: SchedulerTaskIn = {
    typeName,
    cronSpec: cronSpec !== "" ? cronSpec : undefined,
    interval: interval !== "" ? interval : undefined,
    queue,
    timeout: timeout !== "" ? timeout : undefined,
    maxRetry: values.maxRetry,
    enabled: values.enabled ?? true,
    payload,
    backlogEnabled: values.backlogEnabled ?? true,
    backlogWindow: (values.backlogWindow as string)?.trim() ?? "0",
    backlogLimit: values.backlogLimit ?? 1000,
    retention: (values.retention as string)?.trim() || "1h",
    lookback: (values.lookback as string)?.trim() || "5m",
  };
  await getSimpleResponse(
    schedulerTaskCreate(data),
    async (rsp) => {
      await getSchedulerListAction();
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

const openEditDialogAction = (item: InspectorSchedulerListItem): void => {
  editTypeName.value = item.typeName;
  // 复位与回填同属「准备本次打开的数据」：resetForm 原子重置值 + 校验状态，再写入本行详情
  editForm.resetForm({
    values: {
      cronSpec: item.cronSpec ?? "",
      interval: item.interval ?? "",
      queue: item.queue ?? "",
      timeout: item.timeout ?? "",
      maxRetry: item.maxRetry ?? 0,
      retention: item.retention ?? "1h",
      lookback: item.lookback ?? "5m",
      enabled: item.enabled ?? true,
      backlogEnabled: item.backlogEnabled ?? true,
      backlogWindow: item.backlogWindow ?? "0",
      backlogLimit: item.backlogLimit ?? 1000,
    },
  });
  editPayload.value = JSON.stringify(item.payload ?? {});
  editDialogRef.value?.open({ typeName: item.typeName });
};

const itemUpdateAction = editForm.handleSubmit(async (values) => {
  const payload = parsePayload(editPayload.value);
  if (payload === null) {
    appStore.notify({
      text: i18n.global.t("notice.payloadInvalid"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  const scheduleError = scheduleModeError(values.cronSpec ?? "", values.interval ?? "");
  if (scheduleError !== "") {
    appStore.notify({
      text: i18n.global.t(scheduleError),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  // 单行文本字段提交前统一去除首尾空白，避免误输入空白导致查询失败或数据异常
  const cronSpec = (values.cronSpec as string).trim();
  const interval = (values.interval as string).trim();
  const queue = (values.queue as string).trim();
  const timeout = (values.timeout as string).trim();
  const data: SchedulerTaskIn = {
    typeName: editTypeName.value,
    cronSpec: cronSpec !== "" ? cronSpec : undefined,
    interval: interval !== "" ? interval : undefined,
    queue,
    timeout: timeout !== "" ? timeout : undefined,
    maxRetry: values.maxRetry,
    enabled: values.enabled ?? true,
    payload,
    backlogEnabled: values.backlogEnabled ?? true,
    backlogWindow: (values.backlogWindow as string)?.trim() ?? "0",
    backlogLimit: values.backlogLimit ?? 1000,
    retention: (values.retention as string)?.trim() || "1h",
    lookback: (values.lookback as string)?.trim() || "5m",
  };
  await getSimpleResponse(
    schedulerTaskUpdate(data),
    async (rsp) => {
      await getSchedulerListAction();
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getSchedulerActions = (item: Record<string, any>): ItemActions => {
  // 行操作：修改/立即执行/删除（启停改为列表内联开关）
  const actions: ItemActions = {
    edit: () => openEditDialogAction(item as InspectorSchedulerListItem),
    run: () => runConfirmRef.value?.open(item),
    delete: () => deleteConfirmRef.value?.open(item),
  };
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const deleteAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    schedulerTaskDelete({ typeName: item.typeName }),
    async (rsp) => {
      await getSchedulerListAction();
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const runAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    schedulerTaskRun({ typeName: item.typeName }),
    async (rsp) => {
      await getSchedulerListAction();
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

onMounted(() => {
  // 预取任务类型枚举，使列表中的 typeName 列按 label 展示
  void ensureEnum(EnumClass.taskType);
  void getSchedulerListAction();
});
</script>
