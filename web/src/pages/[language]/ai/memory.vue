<template>
  <main-layout current-nav="ai">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="agentMemoryListGet"
        :options-request="agentMemoryListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="agentMemory"
      />

      <!-- 删除确认对话框：单条删除与批量删除共用 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.agentMemory.name')"
        prepend-icon="mdi-brain"
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
                category: $t('model.agentMemory.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 新增记忆对话框：Agent 类型 / 类别 / 标题 / 正文 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.agentMemory.name')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-brain"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12" sm="6">
                <v-select
                  v-model="createAgentKind"
                  v-bind="createAgentKindProps"
                  :items="agentKindOptions"
                  :label="$t('model.agentMemory.field.agentKind')"
                  item-title="title"
                  item-value="value"
                />
              </v-col>
              <v-col cols="12" sm="6">
                <v-select
                  v-model="createKind"
                  v-bind="createKindProps"
                  :items="kindOptions"
                  :label="$t('model.agentMemory.field.kind')"
                  item-title="title"
                  item-value="value"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createTitle"
                  v-bind="createTitleProps"
                  :label="$t('model.agentMemory.field.title')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createContent"
                  v-bind="createContentProps"
                  :label="$t('model.agentMemory.field.content')"
                  :hint="$t('page.aiMemory.contentHint')"
                  persistent-hint
                  rows="8"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑记忆对话框：更新接口仅提交非空字段，空值表示不修改 -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.agentMemory.name')"
        :confirm-disabled="!updateForm.meta.value.valid"
        prepend-icon="mdi-brain-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-select
                  v-model="editKind"
                  v-bind="editKindProps"
                  :items="kindOptions"
                  :label="$t('model.agentMemory.field.kind')"
                  item-title="title"
                  item-value="value"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="editTitle"
                  v-bind="editTitleProps"
                  :label="$t('model.agentMemory.field.title')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editContent"
                  v-bind="editContentProps"
                  :label="$t('model.agentMemory.field.content')"
                  :hint="$t('page.aiMemory.contentHint')"
                  persistent-hint
                  rows="8"
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
name: AI Memory List
meta:
  title: title.aiMemory
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-memory-list">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  agentMemoryCreate,
  agentMemoryDelete,
  agentMemoryDetailGet,
  agentMemoryListGet,
  agentMemoryListOptions,
  agentMemoryStatusSet,
  agentMemoryUpdate,
} from "@/apis/ai";
import type {
  AgentMemoryCreateIn,
  AgentMemoryDetailOut,
  AgentMemoryUpdateIn,
} from "@/apis/@types/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { EnumClass, enumOptionsOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema（标题为单行文本提交前 trim；正文为多行文本域，空白具语义不 trim）----
const createMemorySchema = yup.object({
  agentKind: yup.string().nonNullable().label("model.agentMemory.field.agentKind"),
  kind: yup.string().nonNullable().label("model.agentMemory.field.kind"),
  title: yup.string().trim().nonNullable().max(255).label("model.agentMemory.field.title"),
  content: yup.string().nonNullable().required().min(1).label("model.agentMemory.field.content"),
});

const updateMemorySchema = yup.object({
  kind: yup.string().nonNullable().label("model.agentMemory.field.kind"),
  title: yup.string().trim().nonNullable().max(255).label("model.agentMemory.field.title"),
  content: yup.string().nonNullable().label("model.agentMemory.field.content"),
});

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");

// ---- forms ----
const createForm = useForm({ validationSchema: createMemorySchema });
const updateForm = useForm({ validationSchema: updateMemorySchema });

const [createAgentKind, createAgentKindProps] = createForm.defineField("agentKind", vuetifyCfg);
const [createKind, createKindProps] = createForm.defineField("kind", vuetifyCfg);
const [createTitle, createTitleProps] = createForm.defineField("title", vuetifyCfg);
const [createContent, createContentProps] = createForm.defineField("content", vuetifyCfg);

const [editKind, editKindProps] = updateForm.defineField("kind", vuetifyCfg);
const [editTitle, editTitleProps] = updateForm.defineField("title", vuetifyCfg);
const [editContent, editContentProps] = updateForm.defineField("content", vuetifyCfg);

// ---- 枚举选项（取值集合与文案一律来自后端枚举接口，前端不声明字面量）----
const agentKindOptions = computed(() => enumOptionsOf(EnumClass.agentKind));
const kindOptions = computed(() => enumOptionsOf(EnumClass.memoryKind));

// ---- 新增对话框 ----
const openCreateDialogAction = async (): Promise<void> => {
  // 预取枚举类后默认选中后端返回的首项（返回顺序即展示顺序），避免在前端书写默认值字面量
  await Promise.all([ensureEnum(EnumClass.agentKind), ensureEnum(EnumClass.memoryKind)]);
  createForm.resetField("agentKind", { value: enumOptionsOf(EnumClass.agentKind)[0]?.value ?? "" });
  createForm.resetField("kind", { value: enumOptionsOf(EnumClass.memoryKind)[0]?.value ?? "" });
  createForm.resetField("title", { value: "" });
  createForm.resetField("content", { value: "" });
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  const payload: AgentMemoryCreateIn = { content: values.content };
  if (values.agentKind) {
    payload.agentKind = values.agentKind;
  }
  if (values.kind) {
    payload.kind = values.kind;
  }
  // 标题为单行文本，提交前去除首尾空白；为空则不提交，由后端留空
  const title = (values.title as string).trim();
  if (title) {
    payload.title = title;
  }
  await getSimpleResponse(
    agentMemoryCreate(payload),
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
let editMemoryId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<AgentMemoryDetailOut>(
    agentMemoryDetailGet(item.id),
    async (rsp) => {
      editMemoryId = rsp.data.id;
      updateForm.resetField("kind", { value: rsp.data.kind });
      updateForm.resetField("title", { value: rsp.data.title });
      updateForm.resetField("content", { value: rsp.data.content });
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
    // 仅提交非空字段：空值表示不修改对应字段（正文非空时后端重新向量化）
    const payload: AgentMemoryUpdateIn = {};
    if (values.kind) {
      payload.kind = values.kind;
    }
    const title = (values.title as string).trim();
    if (title) {
      payload.title = title;
    }
    if (values.content) {
      payload.content = values.content;
    }
    await getSimpleResponse(
      agentMemoryUpdate(editMemoryId, payload),
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

// ---- 启停记忆（active ⇄ archived）----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemStatusAction = async (item: Record<string, any>): Promise<void> => {
  // 目标状态取当前状态的反面：生效记忆归档、归档记忆重新启用
  const status = item.status === "active" ? "archived" : "active";
  await getSimpleResponse(
    agentMemoryStatusSet(item.id, { status }),
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

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖 id/title/status 恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "title", "status", ...visible];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {
    edit: () => detailGetAction(item),
    delete: () => {
      batchState.value = null;
      deleteConfirmRef.value?.open(item);
    },
  };
  // 启停动作按当前状态动态命名：生效 → 归档，归档 → 启用（名称经 action.<key> 渲染）
  actions[item.status === "active" ? "archive" : "activate"] = () => itemStatusAction(item);
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
    agentMemoryDelete(ids),
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
    agentMemoryDelete([proxy.id]),
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

onMounted(() => {
  // 表单下拉的枚举类按当前语言预取（表格筛选/表头的枚举类由 SearchTable 自行确保）
  void ensureEnum(EnumClass.agentKind);
  void ensureEnum(EnumClass.memoryKind);
});
</script>
