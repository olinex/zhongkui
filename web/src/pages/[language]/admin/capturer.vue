<template>
  <main-layout current-nav="admin">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="capturerListGet"
        :options-request="capturerListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="capturer"
      />
      <confirm-dialog
        ref="confirmDialogRef"
        :title="$t(`model.capturer.name`)"
        prepend-icon="mdi-owl"
        @confirm="itemDeleteAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            v-if="batchState"
            :action="$t('action.delete')"
            :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
          />
          <v-alert
            v-else
            type="warning"
            icon="mdi-table-row-remove"
            :title="$t(`action.delete`)"
            :text="
              $t(`notice.deleteConfirm`, {
                category: $t(`model.capturer.name`),
                key: `${proxy.namespace}-${proxy.name}:${proxy.version}`,
              })
            "
          />
        </template>
      </confirm-dialog>
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t(`model.capturer.name`)"
        :confirm-disabled="!createForm.meta.value.dirty || !createForm.meta.value.valid"
        prepend-icon="mdi-owl"
        force-editable
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <chip-search-field
                  v-model="createNamespace"
                  v-bind="createNamespaceProps"
                  :label="$t('model.capturer.field.namespace')"
                  item-title="name"
                  item-value="name"
                  :search-headers="namespaceSearchHeaders"
                  :search-request="namespaceListGet"
                  order-by-field="name"
                  clearable
                  init
                >
                  <template #chip="{ props, item }">
                    <v-chip v-bind="props">{{ item.name }}</v-chip>
                  </template>
                  <template #item="{ props, item }">
                    <v-list-item v-bind="props" :title="item.name" :subtitle="item.description" />
                  </template>
                </chip-search-field>
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="9">
                <v-text-field
                  v-model="createName"
                  v-bind="createNameProps"
                  :label="$t('model.capturer.field.name')"
                />
              </v-col>
              <v-col cols="3">
                <v-text-field
                  v-model="createVersion"
                  v-bind="createVersionProps"
                  :label="$t('model.capturer.field.version')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="createDevice"
                  v-bind="createDeviceProps"
                  :label="$t('model.capturer.field.device')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <bpf-expr-field
                  v-model="createBpfExpr"
                  v-bind="createBpfExprProps"
                  :label="$t('model.capturer.field.bpfExpr')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-number-input
                  v-model="createSnapLen"
                  v-bind="createSnapLenProps"
                  :label="$t('model.capturer.field.snapLen')"
                />
              </v-col>
              <v-col cols="6">
                <v-number-input
                  v-model="createTimeout"
                  v-bind="createTimeoutProps"
                  :label="$t('model.capturer.field.timeout')"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-switch
                  v-model="createPromisc"
                  v-bind="createPromiscProps"
                  color="success"
                  :label="$t('model.capturer.field.promisc')"
                />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="createEnabled"
                  v-bind="createEnabledProps"
                  color="success"
                  :label="$t('model.capturer.field.enabled')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t(`model.capturer.name`)"
        :confirm-disabled="!updateForm.meta.value.dirty || !updateForm.meta.value.valid"
        prepend-icon="mdi-owl"
        @confirm="itemUpdateAction"
      >
        <template #body="{ proxy, editable }">
          <v-form>
            <v-row density="compact">
              <v-col cols="12">
                <chip-search-field
                  v-model="proxy.namespace"
                  item-title="name"
                  item-value="name"
                  :search-headers="namespaceSearchHeaders"
                  :search-request="namespaceListGet"
                  order-by-field="name"
                  :label="$t('model.capturer.field.namespace')"
                  clearable
                  disabled
                >
                  <template #chip="{ props, item }">
                    <v-chip v-bind="props" :label="item.name" />
                  </template>
                </chip-search-field>
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="9">
                <v-text-field
                  v-model="proxy.name"
                  :label="$t('model.capturer.field.name')"
                  disabled
                />
              </v-col>
              <v-col cols="3">
                <v-text-field
                  v-model="proxy.version"
                  :label="$t('model.capturer.field.version')"
                  disabled
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <v-text-field
                  v-model="updateDevice"
                  v-bind="updateDeviceProps"
                  :label="$t('model.capturer.field.device')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="12">
                <bpf-expr-field
                  v-model="updateBpfExpr"
                  v-bind="updateBpfExprProps"
                  :label="$t('model.capturer.field.bpfExpr')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-number-input
                  v-model="updateSnapLen"
                  v-bind="updateSnapLenProps"
                  :label="$t('model.capturer.field.snapLen')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-number-input
                  v-model="updateTimeout"
                  v-bind="updateTimeoutProps"
                  :label="$t('model.capturer.field.timeout')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row density="compact">
              <v-col cols="6">
                <v-switch
                  v-model="updatePromisc"
                  v-bind="updatePromiscProps"
                  color="success"
                  :label="$t('model.capturer.field.promisc')"
                  :disabled="!editable"
                />
              </v-col>
              <v-col cols="6">
                <v-switch
                  v-model="updateEnabled"
                  v-bind="updateEnabledProps"
                  color="success"
                  :label="$t('model.capturer.field.enabled')"
                  :disabled="!editable"
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
name: Admin Capturer List
meta:
  title: title.capturerAdmin
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import {
  capturerCreate,
  capturerUpdate,
  capturerDelete,
  capturerDetailGet,
  capturerListGet,
  capturerListOptions,
  namespaceListGet,
} from "@/apis/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import BpfExprField from "@/components/forms/BpfExprField.vue";
import type { CapturerDetailOut } from "./@types";
import { useAppStore } from "@/stores/app";
import type { BatchResultOut, CapturerConfigIn } from "@/apis/@types/admin";
import i18n from "@/utils/i18n";
import { useForm } from "vee-validate";
import { createCapturerSchema, updateCapturerSchema } from "@/schemas/capturer";
import { vuetifyCfg } from "@/utils/form";
import { SearchAction } from "@/types";

const createForm = useForm({
  validationSchema: createCapturerSchema,
});

const updateForm = useForm({
  validationSchema: updateCapturerSchema,
});

const [createNamespace, createNamespaceProps] = createForm.defineField("namespace", vuetifyCfg);

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);

const [createVersion, createVersionProps] = createForm.defineField("version", vuetifyCfg);

const [createDevice, createDeviceProps] = createForm.defineField("device", vuetifyCfg);

const [createBpfExpr, createBpfExprProps] = createForm.defineField("bpfExpr", vuetifyCfg);

const [createSnapLen, createSnapLenProps] = createForm.defineField("snapLen", vuetifyCfg);

const [createTimeout, createTimeoutProps] = createForm.defineField("timeout", vuetifyCfg);

const [createPromisc, createPromiscProps] = createForm.defineField("promisc", vuetifyCfg);

const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);

const [updateDevice, updateDeviceProps] = updateForm.defineField("device", vuetifyCfg);

const [updateBpfExpr, updateBpfExprProps] = updateForm.defineField("bpfExpr", vuetifyCfg);

const [updateSnapLen, updateSnapLenProps] = updateForm.defineField("snapLen", vuetifyCfg);

const [updateTimeout, updateTimeoutProps] = updateForm.defineField("timeout", vuetifyCfg);

const [updatePromisc, updatePromiscProps] = updateForm.defineField("promisc", vuetifyCfg);

const [updateEnabled, updateEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const confirmDialogRef = useTemplateRef("confirmDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const createDialogRef = useTemplateRef("createDialogRef");

const namespaceSearchHeaders: Record<string, SearchAction> = {
  name: SearchAction.CONTAIN,
};

// projectFields 字段投影：行操作依赖字段恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "namespace",
  "name",
  "version",
  ...visible,
];

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
  await getResponse<CapturerDetailOut>(
    capturerDetailGet(item.id as number),
    async (rsp) => {
      // 复位与回填同属「准备本次打开的数据」：values 完整枚举该表单全部 defineField 字段
      updateForm.resetForm({
        values: {
          device: rsp.data.config.device,
          bpfExpr: rsp.data.config.bpfExpr,
          snapLen: rsp.data.config.snapLen,
          // 后端 timeout 单位为毫秒，表单与校验规则使用秒，回填时换算
          timeout: rsp.data.config.timeout / 1000,
          promisc: rsp.data.config.promisc,
          enabled: rsp.data.config.enabled,
        },
      });
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
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    capturerDelete(ids),
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
    capturerDelete(proxy.id as number),
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
  const defaults = {
    namespace: undefined,
    name: "",
    version: "",
    device: "eth0",
    snapLen: 1600,
    promisc: false,
    timeout: 5,
    bpfExpr: "tcp port 80",
    enabled: true,
  };
  // 复位是「准备本次打开的数据」的一部分：resetForm 原子重置全部字段值与校验状态，
  // values 完整枚举该表单全部 defineField 字段，避免未列出字段被重置为初始值造成残影。
  createForm.resetForm({ values: defaults });
  createDialogRef.value?.open(defaults);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemUpdateAction = (proxy: Record<string, any>) => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      // 单行文本提交前去除首尾空白，避免误输入空白导致数据异常
      capturerUpdate(proxy.id, {
        ...values,
        device: (values.device as string).trim(),
        bpfExpr: (values.bpfExpr as string).trim(),
        // 表单使用秒，后端 CapturerConfigIn.timeout 为毫秒
        timeout: Math.round(Number(values.timeout) * 1000),
      } as CapturerConfigIn),
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

const createAction = createForm.handleSubmit(async (values): Promise<void> => {
  await getSimpleResponse(
    capturerCreate({
      // 单行文本提交前去除首尾空白，避免误输入空白导致查询失败或数据异常
      namespace: (values.namespace as string).trim(),
      name: (values.name as string).trim(),
      version: (values.version as string).trim(),
      config: {
        device: (values.device as string).trim(),
        snapLen: values.snapLen,
        promisc: values.promisc,
        // 表单使用秒，后端 CapturerConfigIn.timeout 为毫秒
        timeout: Math.round(Number(values.timeout) * 1000),
        bpfExpr: (values.bpfExpr as string).trim(),
        enabled: values.enabled,
      },
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
</script>
