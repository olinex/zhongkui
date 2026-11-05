<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="namespaceListGet"
        :options-request="namespaceListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="namespace"
      />
      <confirm-dialog
        ref="confirmDialogRef"
        :title="$t(`model.namespace.name`)"
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
                category: $t(`model.namespace.name`),
                key: proxy.name,
              })
            "
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t(`model.namespace.name`)"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-group"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.namespace.field.name')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="createDescription"
                  v-bind="createDescriptionProps"
                  :label="$t('model.namespace.field.description')"
                  rows="3"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t(`model.namespace.name`)"
        :confirm-disabled="!updateForm.meta.value.dirty || !updateForm.meta.value.valid"
        prepend-icon="mdi-group"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="proxy.name"
                  :label="$t('model.namespace.field.name')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-textarea
                  v-model="updateDescription"
                  v-bind="updateDescriptionProps"
                  :label="$t('model.namespace.field.description')"
                  :disabled="!editable"
                  rows="3"
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
name: Admin Namespace List
meta:
  title: title.namespaceAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  namespaceCreate,
  namespaceDetailGet,
  namespaceDelete,
  namespaceListGet,
  namespaceListOptions,
  namespaceUpdate,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import type { BatchResultOut } from "@/apis/@types/admin";
import { useAppStore } from "@/stores/app";
import { useUserStore } from "@/stores/user";
import type { NamespaceDetailOut } from "./@types";
import i18n from "@/utils/i18n";
import { useForm } from "vee-validate";
import { createNamespaceSchema, updateNamespaceSchema } from "@/schemas/namespace";
import { vuetifyCfg } from "@/utils/form";

const updateForm = useForm({
  validationSchema: updateNamespaceSchema,
});

const createForm = useForm({
  validationSchema: createNamespaceSchema,
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
const userStore = useUserStore();
const searchTableRef = useTemplateRef("searchTableRef");
const confirmDialogRef = useTemplateRef("confirmDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const createDialogRef = useTemplateRef("createDialogRef");

// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "name", ...visible];

const getActions = (): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: function (item: Record<string, any>): void {
      batchState.value = null;
      confirmDialogRef.value?.open(item);
    },
  };
  return actions;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<NamespaceDetailOut>(
    namespaceDetailGet(item.name as string),
    async (rsp) => {
      // 复位与回填同属打开动作：原子重置全部字段与校验状态
      updateForm.resetForm({ values: { description: rsp.data.description } });
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
      namespaceUpdate(proxy.name, {
        description: values.description,
      }),
      async (rsp) => {
        await searchTableRef.value?.refresh();
        // 刷新授权快照，保证右上角命名空间切换列表实时反映增删改结果
        await userStore.refreshAuthorization();
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

// refreshAfterNamespaceChange 刷新列表并同步授权快照（命名空间增删影响右上角切换列表）
const refreshAfterNamespaceChange = async (): Promise<void> => {
  await searchTableRef.value?.refresh();
  await userStore.refreshAuthorization();
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const names = items.map((i) => i.name as string);
  await getResponse<BatchResultOut>(
    namespaceDelete(names),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      searchTableRef.value?.clearSelection?.();
      await refreshAfterNamespaceChange();
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
    namespaceDelete(proxy.name as string),
    async (rsp) => {
      applyBatchResult(rsp.summary, rsp.data);
      await refreshAfterNamespaceChange();
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
  // 打开动作里原子复位全部字段
  const defaults = { name: "", description: "" };
  createForm.resetForm({ values: defaults });
  createDialogRef.value?.open(defaults);
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    namespaceCreate({
      // 名称单行文本提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      name: (values.name as string).trim(),
      description: values.description,
    }),
    async (rsp) => {
      await searchTableRef.value?.refresh();
      // 刷新授权快照，保证右上角命名空间切换列表实时反映增删改结果
      await userStore.refreshAuthorization();
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
</script>
