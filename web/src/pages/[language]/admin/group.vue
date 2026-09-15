<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="groupListGet"
        :options-request="groupListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="group"
      />
      <confirm-dialog
        ref="confirmDialogRef"
        :title="$t(`model.group.name`)"
        prepend-icon="mdi-group"
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
                category: $t(`model.group.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.group.name')"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-account-multiple"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.group.field.name')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.group.field.description')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.group.name')"
        :confirm-disabled="!updateForm.meta.value.dirty || !updateForm.meta.value.valid"
        prepend-icon="mdi-account-multiple"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field v-model="proxy.name" :label="$t('model.group.field.name')" disabled />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="updateDescription"
                  v-bind="updateDescriptionProps"
                  :label="$t('model.group.field.description')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <inner-edit-table
                  v-model="proxy.policies"
                  :headers="groupPolicyHeaders"
                  :search-headers="groupPolicySearchHeaders"
                  :search-request="rbacPolicyTemplateListGet"
                  :list-item-action="true"
                  :editable="editable"
                  order-by-field="createdAt"
                >
                  <template #list-item-action="{ item, editable: listEditable, removeItem }">
                    <v-btn
                      :disabled="!listEditable"
                      icon="mdi-minus"
                      variant="tonal"
                      color="error"
                      density="compact"
                      @click="removePolicyAction(proxy.name, item, removeItem)"
                    />
                  </template>
                  <template #item-title="{ item }">
                    <span class="font-weight-medium">{{ item.action }}</span>
                    &nbsp;{{ item.target }}
                  </template>
                  <template #item-append="{ item, appendItem, close, editable: listEditable }">
                    <v-select
                      v-model="item.effect"
                      :items="policyEffectOptions"
                      :item-title="policyEffectTitle"
                      :disabled="!listEditable"
                      variant="outlined"
                      density="compact"
                      hide-details
                      single-line
                      style="width: 108px"
                    />
                    <v-btn
                      v-request="
                        () =>
                          addPolicyAction(
                            proxy.name,
                            item.target,
                            item.action,
                            item.effect,
                            appendItem,
                            close,
                          )
                      "
                      :disabled="!(listEditable && item.effect)"
                      :text="$t('action.add')"
                      color="primary"
                      variant="tonal"
                      size="small"
                      density="compact"
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
name: Admin Group List
meta:
  title: title.groupAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  groupCreate,
  groupDelete,
  groupDetailGet,
  groupListGet,
  groupListOptions,
  groupRbacPolicyAdd,
  groupRbacPolicyRemove,
  groupUpdate,
  rbacPolicyTemplateListGet,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions, TableHeader } from "@/components/@types/table";
import type { GroupDetailOut, GroupRbacPolicyDetailOut } from "./@types";
import type { BatchResultOut } from "@/apis/@types/admin";
import { useAppStore } from "@/stores/app";
import { BackendDataType, SearchAction } from "@/types";
import i18n from "@/utils/i18n";
import { useForm } from "vee-validate";
import { updateGroupSchema, createGroupSchema } from "@/schemas/group";
import { vuetifyCfg } from "@/utils/form";
import { EnumClass, enumLabelOf, enumOptionsOf } from "@/utils/enums";

// policyEffectOptions 权限 effect 下拉可选项：来自统一枚举注册表 policyEffect，
// title 为 label、value 为枚举值，语言切换即时刷新
const policyEffectOptions = computed(() => enumOptionsOf(EnumClass.policyEffect));

// policyEffectTitle 权限 effect 下拉项展示名：value 为枚举值，渲染期经统一枚举注册表 policyEffect 翻译
const policyEffectTitle = (option: { title: string; value: string }): string =>
  enumLabelOf(EnumClass.policyEffect, option.value);

const updateForm = useForm({
  validationSchema: updateGroupSchema,
});

const createForm = useForm({
  validationSchema: createGroupSchema,
});

const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);

const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const confirmDialogRef = useTemplateRef("confirmDialogRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const groupPolicyHeaders: Array<TableHeader> = [
  {
    key: "target",
    title: i18n.global.t("model.policy.field.target"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "action",
    title: i18n.global.t("model.policy.field.action"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "effect",
    title: i18n.global.t("model.policy.field.effect"),
    align: "start",
    type: BackendDataType.STRING,
    sortable: false,
  },
  {
    key: "createdAt",
    title: i18n.global.t("model.policy.field.createdAt"),
    align: "start",
    type: BackendDataType.DATETIME,
    sortable: false,
  },
  {
    key: "updatedAt",
    title: i18n.global.t("model.policy.field.updatedAt"),
    align: "start",
    type: BackendDataType.DATETIME,
    sortable: false,
  },
];

const groupPolicySearchHeaders: Record<string, SearchAction> = {
  target: SearchAction.CONTAIN,
  action: SearchAction.CONTAIN,
};

// projectFields 字段投影：行操作依赖字段恒定投影（name 定位），避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "name", ...visible];

const getActions = (): ItemActions => {
  return {
    edit: detailGetAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: function (item: Record<string, any>): void {
      batchState.value = null;
      confirmDialogRef.value?.open(item);
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<GroupDetailOut>(
    groupDetailGet(item.name),
    async (rsp) => {
      updateForm.resetField("description", { value: rsp.data.description });
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
      groupUpdate(proxy.name, {
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
      confirmDialogRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const names = items.map((i) => i.name as string);
  await getResponse<BatchResultOut>(
    groupDelete(names),
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
    groupDelete(proxy.name as string),
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

const openCreateDialogAction = () => {
  createDialogRef.value?.open({ name: "", description: "" });
  createForm.resetField("name", { value: "" });
  createForm.resetField("description", { value: "" });
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    groupCreate({
      // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      name: (values.name as string).trim(),
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

const addPolicyAction = async (
  groupName: string,
  target: string,
  action: string,
  effect: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  appendItem: (item: Record<string, any>) => void,
  close: () => void,
): Promise<void> => {
  await getResponse<GroupRbacPolicyDetailOut>(
    groupRbacPolicyAdd(groupName, {
      target,
      action,
      effect,
    }),
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
      appendItem(rsp.data);
      close();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
      close();
    },
  );
};

const removePolicyAction = async (
  groupName: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  item: Record<string, any>,
  callback: () => void,
): Promise<void> => {
  await getSimpleResponse(
    groupRbacPolicyRemove(groupName, {
      target: item.target,
      action: item.action,
      effect: item.effect,
    }),
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
      callback();
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
