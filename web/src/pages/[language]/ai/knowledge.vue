<template>
  <main-layout current-nav="ai">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="knowledgeDocumentListGet"
        :options-request="knowledgeDocumentListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :create="openCreateDialogAction"
        :project-fields="projectFields"
        model-name="aiDocument"
      />

      <!-- 删除确认对话框：单条删除与批量删除共用 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('model.aiDocument.name')"
        prepend-icon="mdi-book-open-variant"
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
                category: $t('model.aiDocument.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 重新索引确认对话框 -->
      <confirm-dialog
        ref="reindexConfirmRef"
        :title="$t('action.reindex')"
        prepend-icon="mdi-database-sync-outline"
        @confirm="itemReindexAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t('action.reindex')"
            :text="
              $t('notice.reindexConfirm', {
                category: $t('model.aiDocument.name'),
                key: proxy.title,
              })
            "
          />
        </template>
      </confirm-dialog>

      <!-- 创建知识文档对话框 -->
      <temp-editable-dialog
        ref="createDialogRef"
        :title="$t('model.aiDocument.name')"
        :confirm-disabled="!createForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-book-plus"
        @confirm="createAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createTitle"
                  v-bind="createTitleProps"
                  :label="$t('model.aiDocument.field.title')"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="createContent"
                  v-bind="createContentProps"
                  :label="$t('model.aiDocument.field.content')"
                  rows="10"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 编辑知识文档对话框：标题与正文均可修改（更新接口触发重新索引） -->
      <temp-editable-dialog
        ref="editDialogRef"
        :title="$t('model.aiDocument.name')"
        :confirm-disabled="!updateForm.meta.value.valid"
        prepend-icon="mdi-book-edit"
        @confirm="itemUpdateAction"
      >
        <template #body="{ editable }">
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="editTitle"
                  v-bind="editTitleProps"
                  :label="$t('model.aiDocument.field.title')"
                  :disabled="!editable"
                />
              </v-col>
            </v-row>
            <v-row dense>
              <v-col cols="12">
                <v-textarea
                  v-model="editContent"
                  v-bind="editContentProps"
                  :label="$t('model.aiDocument.field.content')"
                  :disabled="!editable"
                  rows="10"
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
name: AI Knowledge List
meta:
  title: title.aiKnowledge
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-knowledge-list">
import * as yup from "yup";
import { useForm } from "vee-validate";

import {
  knowledgeDocumentCreate,
  knowledgeDocumentDelete,
  knowledgeDocumentDetailGet,
  knowledgeDocumentListGet,
  knowledgeDocumentListOptions,
  knowledgeDocumentReindex,
  knowledgeDocumentUpdate,
} from "@/apis/ai";
import type { KnowledgeDocumentDetail, KnowledgeDocumentReindexOut } from "@/apis/@types/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { vuetifyCfg } from "@/utils/form";
import i18n from "@/utils/i18n";

// ---- 表单校验 schema（正文为多行文本域，内容中的空白具有语义，不做 trim）----
const createDocSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiDocument.field.title"),
  content: yup.string().nonNullable().required().min(1).label("model.aiDocument.field.content"),
});

const updateDocSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiDocument.field.title"),
  content: yup.string().nonNullable().required().min(1).label("model.aiDocument.field.content"),
});

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const createDialogRef = useTemplateRef("createDialogRef");
const editDialogRef = useTemplateRef("editDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const reindexConfirmRef = useTemplateRef("reindexConfirmRef");

// ---- forms ----
const createForm = useForm({ validationSchema: createDocSchema });
const updateForm = useForm({ validationSchema: updateDocSchema });

const [createTitle, createTitleProps] = createForm.defineField("title", vuetifyCfg);
const [createContent, createContentProps] = createForm.defineField("content", vuetifyCfg);

const [editTitle, editTitleProps] = updateForm.defineField("title", vuetifyCfg);
const [editContent, editContentProps] = updateForm.defineField("content", vuetifyCfg);

// ---- 创建对话框 ----
const openCreateDialogAction = (): void => {
  createForm.resetField("title", { value: "" });
  createForm.resetField("content", { value: "" });
  createDialogRef.value?.open({});
};

const createAction = createForm.handleSubmit(async (values) => {
  await getSimpleResponse(
    knowledgeDocumentCreate({
      // 标题为单行文本，提交前去除首尾空白，避免误输入空白创建出看似相同实则不同的记录
      title: (values.title as string).trim(),
      content: values.content,
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
let editDocumentId = 0;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const detailGetAction = async (item: Record<string, any>): Promise<void> => {
  await getResponse<KnowledgeDocumentDetail>(
    knowledgeDocumentDetailGet(item.id),
    async (rsp) => {
      editDocumentId = rsp.data.id;
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
    await getSimpleResponse(
      knowledgeDocumentUpdate(editDocumentId, {
        id: editDocumentId,
        title: (values.title as string).trim(),
        content: values.content,
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
const projectFields = (visible: string[]): string[] => ["id", "title", ...visible];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  return {
    edit: () => detailGetAction(item),
    reindex: () => {
      batchState.value = null;
      reindexConfirmRef.value?.open(item);
    },
    delete: () => {
      batchState.value = null;
      deleteConfirmRef.value?.open(item);
    },
  };
};

// ---- 重新索引 ----
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemReindexAction = async (proxy: Record<string, any>): Promise<void> => {
  await getResponse<KnowledgeDocumentReindexOut>(
    knowledgeDocumentReindex(proxy.id),
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
type BatchState = { items: Record<string, any>[]; action: "delete" };
const batchState = ref<BatchState | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  return {
    delete: () => {
      batchState.value = { items, action: "delete" };
      deleteConfirmRef.value?.open({});
    },
  };
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const batchDeleteAction = async (items: Record<string, any>[]): Promise<void> => {
  const ids = items.map((i) => i.id as number);
  await getResponse<BatchResultOut>(
    knowledgeDocumentDelete(ids),
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
    knowledgeDocumentDelete([proxy.id]),
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
