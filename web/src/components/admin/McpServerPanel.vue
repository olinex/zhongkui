<template>
  <!-- 作用域提醒：注册表为全局配置，常驻于列表上方，避免用户误认为随命名空间隔离 -->
  <div class="px-4 pt-4">
    <v-alert type="info">
      {{ $t("page.mcpServerAdmin.scopeHint") }}
    </v-alert>
  </div>

  <search-table
    ref="searchTableRef"
    :list-request="mcpServerListGet"
    :options-request="mcpServerListOptions"
    :actions="getActions"
    :batch-actions="getBatchActions"
    :create="openCreateDialogAction"
    :project-fields="projectFields"
    model-name="mcpServer"
  >
    <!-- 请求头为加密列：列表出参为密文，仅以「已加密」标识占位，点击解密查看明文 -->
    <template #cell-headers="{ item }">
      <encrypted-value-cell
        kind="crypto"
        :item="item"
        field-key="headers"
        @view-plaintext="viewHeadersPlaintext"
      />
    </template>
  </search-table>

  <!-- 共享明文对话框：查看 MCP 服务器请求头明文 -->
  <plaintext-dialog ref="plaintextDialogRef" />

  <confirm-dialog
    ref="deleteConfirmRef"
    :title="$t('model.mcpServer.name')"
    prepend-icon="mdi-server-outline"
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
            category: $t('model.mcpServer.name'),
            key: proxy.name,
          })
        "
      />
    </template>
  </confirm-dialog>

  <confirm-dialog
    ref="toggleConfirmRef"
    :title="$t('model.mcpServer.name')"
    prepend-icon="mdi-toggle-switch"
    @confirm="itemToggleAction"
  >
    <template #body="{ proxy }">
      <confirm-alert
        :action="$t(toggleTarget === true ? 'action.activate' : 'action.deactivate')"
        :text="
          $t(toggleTarget === true ? 'notice.enableConfirm' : 'notice.disableConfirm', {
            category: $t('model.mcpServer.name'),
            key: proxy.name,
          })
        "
      />
    </template>
  </confirm-dialog>

  <temp-editable-dialog
    ref="createDialogRef"
    :title="$t('model.mcpServer.name')"
    :confirm-disabled="!createForm.meta.value.valid"
    force-editable
    prepend-icon="mdi-server-outline"
    @confirm="createAction"
  >
    <template #body>
      <v-form>
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model="createName"
              v-bind="createNameProps"
              :label="$t('model.mcpServer.field.name')"
            />
          </v-col>
          <v-col cols="6">
            <v-select
              v-model="createTransport"
              v-bind="createTransportProps"
              :items="transportOptions"
              :label="$t('model.mcpServer.field.transport')"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="createUrl"
              v-bind="createUrlProps"
              :label="$t('model.mcpServer.field.url')"
              :hint="$t('page.mcpServerAdmin.urlHint')"
              persistent-hint
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="createHeaders"
              v-bind="createHeadersProps"
              :label="$t('model.mcpServer.field.headers')"
              :hint="$t('page.mcpServerAdmin.headersHint')"
              persistent-hint
              rows="2"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-number-input
              v-model="createTimeoutSeconds"
              v-bind="createTimeoutSecondsProps"
              :label="$t('model.mcpServer.field.timeoutSeconds')"
              :precision="0"
              :min="1"
            />
          </v-col>
          <v-col cols="6">
            <v-switch
              v-model="createEnabled"
              v-bind="createEnabledProps"
              :label="$t('model.mcpServer.field.enabled')"
              color="primary"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="createDescription"
              v-bind="createDescriptionProps"
              :label="$t('model.mcpServer.field.description')"
              rows="1"
            />
          </v-col>
        </v-row>
      </v-form>
    </template>
  </temp-editable-dialog>

  <temp-editable-dialog
    ref="editDialogRef"
    :title="$t('model.mcpServer.name')"
    :confirm-disabled="!updateForm.meta.value.valid"
    prepend-icon="mdi-server-outline"
    @confirm="itemUpdateAction"
  >
    <template #body="{ editable }">
      <v-form>
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model="updateName"
              v-bind="updateNameProps"
              :label="$t('model.mcpServer.field.name')"
              :disabled="!editable"
            />
          </v-col>
          <v-col cols="6">
            <v-select
              v-model="updateTransport"
              v-bind="updateTransportProps"
              :items="transportOptions"
              :label="$t('model.mcpServer.field.transport')"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-text-field
              v-model="updateUrl"
              v-bind="updateUrlProps"
              :label="$t('model.mcpServer.field.url')"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="updateHeaders"
              v-bind="updateHeadersProps"
              :label="$t('model.mcpServer.field.headers')"
              :hint="$t('page.mcpServerAdmin.headersKeepHint')"
              persistent-hint
              rows="2"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="6">
            <v-number-input
              v-model="updateTimeoutSeconds"
              v-bind="updateTimeoutSecondsProps"
              :label="$t('model.mcpServer.field.timeoutSeconds')"
              :precision="0"
              :min="1"
              :disabled="!editable"
            />
          </v-col>
          <v-col cols="6">
            <v-switch
              v-model="updateEnabled"
              v-bind="updateEnabledProps"
              :label="$t('model.mcpServer.field.enabled')"
              color="primary"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-textarea
              v-model="updateDescription"
              v-bind="updateDescriptionProps"
              :label="$t('model.mcpServer.field.description')"
              rows="1"
              :disabled="!editable"
            />
          </v-col>
        </v-row>
      </v-form>
    </template>
  </temp-editable-dialog>
</template>

<script setup lang="ts" name="mcp-server-panel">
import { useForm } from "vee-validate";

import {
  mcpServerBatchDelete,
  mcpServerCreate,
  mcpServerDetailGet,
  mcpServerListGet,
  mcpServerListOptions,
  mcpServerSync,
  mcpServerTest,
  mcpServerUpdate,
} from "@/apis/llm";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { McpServerDetailOut, McpServerSyncOut, McpServerTestOut } from "@/apis/@types/llm";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { mcpServerSchema } from "@/schemas/llm";
import { useAppStore } from "@/stores/app";
import { BackendDataType } from "@/types";
import { isEmptyCiphertext } from "@/utils/encrypted";
import { vuetifyCfg } from "@/utils/form";
import { EnumClass, enumOptionsOf, ensureEnum } from "@/utils/enums";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema ----
const createForm = useForm({ validationSchema: mcpServerSchema() });
const updateForm = useForm({ validationSchema: mcpServerSchema() });

const [createName, createNameProps] = createForm.defineField("name", vuetifyCfg);
const [createTransport, createTransportProps] = createForm.defineField("transport", vuetifyCfg);
const [createUrl, createUrlProps] = createForm.defineField("url", vuetifyCfg);
const [createHeaders, createHeadersProps] = createForm.defineField("headers", vuetifyCfg);
const [createTimeoutSeconds, createTimeoutSecondsProps] = createForm.defineField(
  "timeoutSeconds",
  vuetifyCfg,
);
const [createEnabled, createEnabledProps] = createForm.defineField("enabled", vuetifyCfg);
const [createDescription, createDescriptionProps] = createForm.defineField(
  "description",
  vuetifyCfg,
);

const [updateName, updateNameProps] = updateForm.defineField("name", vuetifyCfg);
const [updateTransport, updateTransportProps] = updateForm.defineField("transport", vuetifyCfg);
const [updateUrl, updateUrlProps] = updateForm.defineField("url", vuetifyCfg);
const [updateHeaders, updateHeadersProps] = updateForm.defineField("headers", vuetifyCfg);
const [updateTimeoutSeconds, updateTimeoutSecondsProps] = updateForm.defineField(
  "timeoutSeconds",
  vuetifyCfg,
);
const [updateEnabled, updateEnabledProps] = updateForm.defineField("enabled", vuetifyCfg);
const [updateDescription, updateDescriptionProps] = updateForm.defineField(
  "description",
  vuetifyCfg,
);

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const toggleConfirmRef = useTemplateRef("toggleConfirmRef");
const plaintextDialogRef = useTemplateRef("plaintextDialogRef");

// transport 下拉选项来自统一枚举注册表（label 由后端按当前语言返回）
const transportOptions = computed(() => enumOptionsOf(EnumClass.mcpTransport));

onMounted(() => {
  void ensureEnum(EnumClass.mcpTransport);
});

// ---- 创建对话框 ----
const openCreateDialogAction = (): void => {
  const defaults = {
    name: "",
    // 传输方式初值取枚举类首项（展示顺序以后端返回顺序为准，前端不声明默认值字面量）
    transport: transportOptions.value[0]?.value ?? "",
    url: "",
    headers: "",
    timeoutSeconds: 60,
    enabled: true,
    description: "",
  };
  createForm.resetField("name", { value: defaults.name });
  createForm.resetField("transport", { value: defaults.transport });
  createForm.resetField("url", { value: defaults.url });
  createForm.resetField("headers", { value: defaults.headers });
  createForm.resetField("timeoutSeconds", { value: defaults.timeoutSeconds });
  createForm.resetField("enabled", { value: defaults.enabled });
  createForm.resetField("description", { value: defaults.description });
  createDialogRef.value?.open(defaults);
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    mcpServerCreate({
      // 单行文本提交前去除首尾空白；headers 承载凭据，按原样提交不做 trim
      name: (values.name as string).trim(),
      transport: values.transport,
      url: (values.url as string).trim(),
      headers: values.headers,
      timeoutSeconds: values.timeoutSeconds,
      enabled: values.enabled,
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
let editServerId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<McpServerDetailOut>(
    mcpServerDetailGet(item.id as number),
    async (rsp) => {
      editServerId = rsp.data.id;
      updateForm.resetField("name", { value: rsp.data.name });
      updateForm.resetField("transport", { value: rsp.data.transport });
      updateForm.resetField("url", { value: rsp.data.url });
      // 请求头不回填密文：留空即表示沿用原凭据
      updateForm.resetField("headers", { value: "" });
      updateForm.resetField("timeoutSeconds", { value: rsp.data.timeoutSeconds });
      updateForm.resetField("enabled", { value: rsp.data.enabled });
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

const itemUpdateAction = () => {
  updateForm.handleSubmit(async (values) => {
    await getSimpleResponse(
      mcpServerUpdate(editServerId, {
        name: (values.name as string).trim(),
        transport: values.transport,
        url: (values.url as string).trim(),
        // 空串表示沿用原凭据（后端按留空处理）
        headers: values.headers,
        timeoutSeconds: values.timeoutSeconds,
        enabled: values.enabled,
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

// ---- 连通性测试 ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemTestAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<McpServerTestOut>(
    mcpServerTest({ id: item.id as number }),
    async (rsp) => {
      appStore.notify({
        text: i18n.global.t("notice.mcpServer.testSuccess", { count: rsp.data.toolCount }),
        timeout: 5000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: `${i18n.global.t("notice.mcpServer.testFailed")}: ${rsp.summary}`,
        timeout: 5000,
        color: "error",
      });
    },
  );
};

// ---- 同步工具 ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemSyncAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<McpServerSyncOut>(
    mcpServerSync(item.id as number),
    async (rsp) => {
      appStore.notify({
        text: i18n.global.t("notice.mcpServer.syncSuccess", {
          created: rsp.data.created,
          updated: rsp.data.updated,
          removed: rsp.data.removed,
        }),
        timeout: 5000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 5000,
        color: "error",
      });
    },
  );
};

// ---- 请求头明文查看 ----
// viewHeadersPlaintext 以列表下发的密文为凭据，经全局明文接口解密查看请求头；
// 明文不随列表下发，解密须经 plaintext:crypto:mcp-server:headers 权限点授权
const viewHeadersPlaintext = async (item: Record<string, unknown>): Promise<void> => {
  const ciphertext = item.headers;
  if (isEmptyCiphertext(ciphertext)) {
    return;
  }
  const label = i18n.global.t("model.mcpServer.field.headers");
  await plaintextDialogRef.value?.open(
    {
      key: "headers",
      label,
      display: "",
      kind: "crypto",
      length: 0,
      value: ciphertext,
      type: BackendDataType.STRING,
    },
    "mcp-server",
    label,
  );
};

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖字段恒定投影（name 定位、enabled 启停），
// 避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "name", "enabled", ...visible];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: detailGetAction,
    test: itemTestAction,
    sync: itemSyncAction,
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

// applyBatchResult 统一处理 BatchResultOut 响应：存在失败条目时以 error 提示失败数，否则沿用成功摘要
const applyBatchResult = (summary: string, result: BatchResultOut): void => {
  if (result.failed.length > 0) {
    appStore.notify({
      text: i18n.global.t("notice.batchResult", {
        succeeded: result.succeeded,
        failed: result.failed.length,
      }),
      timeout: 5000,
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
    mcpServerBatchDelete(ids),
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
    mcpServerBatchDelete(proxy.id as number),
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

// ---- 启用/禁用 ----
// 后端无独立的启停接口：读取详情后以整表单更新翻转载入（凭据留空即沿用原密文）
const toggleTarget = ref<boolean | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const toggleAction = (item: Record<string, any>, enabled: boolean): void => {
  toggleTarget.value = enabled;
  toggleConfirmRef.value?.open(item);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemToggleAction = async (proxy: Record<string, any>): Promise<void> => {
  if (toggleTarget.value === null) {
    return;
  }
  const target = toggleTarget.value;
  await getResponse<McpServerDetailOut>(
    mcpServerDetailGet(proxy.id as number),
    async (rsp) => {
      const detail = rsp.data;
      await getSimpleResponse(
        mcpServerUpdate(detail.id, {
          name: detail.name,
          transport: detail.transport,
          url: detail.url,
          headers: "",
          timeoutSeconds: detail.timeoutSeconds,
          enabled: target,
          description: detail.description,
        }),
        async (rsp2) => {
          await searchTableRef.value?.refresh();
          appStore.notify({
            text: rsp2.summary,
            timeout: 3000,
            color: "success",
          });
        },
        async (rsp2) => {
          appStore.notify({
            text: rsp2.summary,
            timeout: 3000,
            color: "error",
          });
        },
      );
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
