<template>
  <main-layout current-nav="notification">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="notificationGroupListGet"
        :options-request="notificationGroupListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        model-name="notificationGroup"
      >
        <!-- 默认分组列：默认分组以彩色 chip 高亮展示 -->
        <template #cell-isDefault="{ value }">
          <v-chip v-if="value" color="primary" size="small" variant="tonal">
            {{ $t("model.notificationGroup.field.isDefault") }}
          </v-chip>
          <template v-else>
            {{ $t("model.__base.value.no") }}
          </template>
        </template>
      </search-table>

      <!-- 删除确认对话框：单条删除与批量删除共用 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.notificationGroup.name')"
        prepend-icon="mdi-account-group-outline"
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
                category: $t('model.notificationGroup.name'),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 设为默认分组确认对话框 -->
      <confirm-dialog
        ref="setDefaultConfirmRef"
        :title="$t('model.notificationGroup.name')"
        prepend-icon="mdi-star-outline"
        @confirm="itemSetDefaultAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.setDefault')"
            :text="$t('notice.notification.setDefaultConfirm', { key: proxy.name })"
          />
        </template>
      </confirm-dialog>

      <!-- 创建分组对话框：名称必填，isDefault 开关，描述可选 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.notificationGroup.name')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-account-group-outline"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.notificationGroup.field.name')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.notificationGroup.field.description')"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  v-model="createIsDefault"
                  v-bind="createIsDefaultProps"
                  :label="$t('model.notificationGroup.field.isDefault')"
                  color="primary"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <inner-edit-table
                  v-model="createTargetItems"
                  :headers="targetHeaders"
                  :search-headers="targetSearchHeaders"
                  :search-request="notificationTargetListGet"
                  :list-item-action="true"
                  order-by-field="id"
                  item-key-field="id"
                  editable
                >
                  <template #list-item-action="{ removeItem }">
                    <v-btn
                      icon="mdi-minus"
                      variant="tonal"
                      color="error"
                      density="compact"
                      @click="removeItem"
                    />
                  </template>
                  <template #item-subtitle="{ item }">
                    {{ item.channelName }}
                  </template>
                  <template #item-append="{ item, appendItem, editable: listEditable, bound }">
                    <v-btn
                      :disabled="!listEditable || bound"
                      :text="$t('action.add')"
                      color="primary"
                      density="compact"
                      variant="tonal"
                      @click="appendItem(item)"
                    />
                  </template>
                </inner-edit-table>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑分组对话框：名称与默认标记禁用，描述与绑定目标可编辑 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.notificationGroup.name')"
        :confirm-disabled="
          (!updateForm.meta.value.dirty && !editTargetsChanged) || !updateForm.meta.value.valid
        "
        prepend-icon="mdi-account-group-outline"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  :model-value="proxy.name"
                  :label="$t('model.notificationGroup.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-switch
                  :model-value="proxy.isDefault"
                  :label="$t('model.notificationGroup.field.isDefault')"
                  color="primary"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="updateDescription"
                  v-bind="updateDescriptionProps"
                  :label="$t('model.notificationGroup.field.description')"
                  :disabled="!editable"
                  rows="1"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <inner-edit-table
                  v-model="editTargetItems"
                  :headers="targetHeaders"
                  :search-headers="targetSearchHeaders"
                  :search-request="notificationTargetListGet"
                  :list-item-action="true"
                  :editable="editable"
                  order-by-field="id"
                  item-key-field="id"
                >
                  <template #list-item-action="{ removeItem }">
                    <v-btn
                      :disabled="!editable"
                      icon="mdi-minus"
                      variant="tonal"
                      color="error"
                      density="compact"
                      @click="removeItem"
                    />
                  </template>
                  <template #item-subtitle="{ item }">
                    {{ item.channelName }}
                  </template>
                  <template #item-append="{ item, appendItem, editable: listEditable, bound }">
                    <v-btn
                      :disabled="!listEditable || bound"
                      :text="$t('action.add')"
                      color="primary"
                      density="compact"
                      variant="tonal"
                      @click="appendItem(item)"
                    />
                  </template>
                </inner-edit-table>
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Notification Group List
meta:
  title: title.notification.group
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="notification-group">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  notificationGroupBatchDelete,
  notificationGroupCreate,
  notificationGroupDetailGet,
  notificationGroupListGet,
  notificationGroupListOptions,
  notificationGroupSetDefault,
  notificationGroupUpdate,
  notificationTargetListGet,
} from "@/apis/notification";
import { getResponse, getSimpleResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";

import { BackendDataType, SearchAction } from "@/types";
import type { BatchResultOut } from "@/apis/@types/admin";
import type {
  NotificationGroupDetailOut,
  NotificationGroupTargetItem,
} from "@/apis/@types/notification";
import type { BatchActions, ItemActions, TableHeader } from "@/components/@types/table";

// ---- 校验 schema：名称必填；isDefault 默认 false；描述可选（最大 256）----
const createNotificationGroupSchema = yup.object({
  name: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(3)
    .max(32)
    .label("model.notificationGroup.field.name"),
  isDefault: yup
    .bool()
    .nonNullable()
    .default(false)
    .label("model.notificationGroup.field.isDefault"),
  description: yup
    .string()
    .nonNullable()
    .max(256)
    .label("model.notificationGroup.field.description"),
});

const updateNotificationGroupSchema = yup.object({
  description: yup
    .string()
    .nonNullable()
    .max(256)
    .label("model.notificationGroup.field.description"),
});

// ---- forms ----
const createForm = useForm({ validationSchema: createNotificationGroupSchema });
const updateForm = useForm({ validationSchema: updateNotificationGroupSchema });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createIsDefault, createIsDefaultProps] = createForm.defineField("isDefault", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);
const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const setDefaultConfirmRef = useTemplateRef("setDefaultConfirmRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");

// ---- 绑定目标表格：已绑定目标分页展示 + 移除；候选目标按 name 服务端搜索后追加 ----
// 通知目标表格列：仅展示名称 + 所属渠道，末位固定操作为「移除」
const targetHeaders: Array<TableHeader> = [
  {
    key: "name",
    title: i18n.global.t("model.notificationTarget.field.name"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "channelName",
    title: i18n.global.t("model.notificationTarget.field.channelName"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
];
// 追加候选目标：按名称服务端关键字搜索
const targetSearchHeaders: Record<string, SearchAction> = { name: SearchAction.CONTAIN };

// ---- 创建分组 ----
const createTargetItems = ref<NotificationGroupTargetItem[]>([]);

const openCreateDialogAction = (): void => {
  createForm.resetField("name", { value: "" });
  createForm.resetField("isDefault", { value: false });
  createForm.resetField("description", { value: "" });
  createTargetItems.value = [];
  createDialogRef.value?.open({ name: "", isDefault: false, description: "" });
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    notificationGroupCreate({
      // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      name: (values.name as string).trim(),
      isDefault: values.isDefault,
      description: values.description,
      targetIds: createTargetItems.value.map((t) => t.id),
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

// ---- 编辑分组（描述 + 绑定目标）----
const editTargetItems = ref<NotificationGroupTargetItem[]>([]);
// 打开编辑对话框时的初始绑定目标，用于判断绑定目标是否被修改（targetIds 不在
// vee-validate 表单内，无法复用 form.meta.dirty，需单独跟踪以驱动确认按钮可用态）
let editTargetsInitial: number[] = [];

const editTargetsChanged = computed<boolean>(() => {
  const current = [...editTargetItems.value].map((t) => t.id).sort((a, b) => a - b);
  const initial = [...editTargetsInitial].sort((a, b) => a - b);
  return current.length !== initial.length || current.some((v, i) => v !== initial[i]);
});

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<NotificationGroupDetailOut>(
    notificationGroupDetailGet(item.id),
    async (rsp) => {
      updateForm.resetField("description", { value: rsp.data.description });
      editTargetsInitial = (rsp.data.targets ?? []).map((t) => t.id);
      editTargetItems.value = rsp.data.targets ?? [];
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemUpdateAction = (proxy: Record<string, any>) => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      notificationGroupUpdate(proxy.id, {
        description: values.description,
        targetIds: editTargetItems.value.map((t) => t.id),
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

// ---- 设为默认分组 ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemSetDefaultAction = async (proxy: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    notificationGroupSetDefault(proxy.id),
    async () => {
      await searchTableRef.value?.refresh();
      appStore.notify({
        text: i18n.global.t("notice.notification.setDefaultSuccess"),
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

// ---- 行操作 ----
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
  // 仅非默认分组提供「设为默认」操作
  if (!item.isDefault) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    actions.setDefault = (i: Record<string, any>) => {
      batchState.value = null;
      setDefaultConfirmRef.value?.open(i);
    };
  }
  return actions;
};

// ---- 批量删除 ----
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
    notificationGroupBatchDelete(ids),
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
    notificationGroupBatchDelete([proxy.id]),
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
