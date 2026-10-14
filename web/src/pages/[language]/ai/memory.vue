<template>
  <main-layout current-nav="ai">
    <template #body>
      <search-table
        ref="searchTableRef"
        :list-request="agentMemoryListGet"
        :options-request="agentMemoryListOptions"
        :actions="getActions"
        :batch-actions="getBatchActions"
        :project-fields="projectFields"
        model-name="agentMemory"
      >
        <!-- 状态列：字符串 status 映射为布尔开关（active ⇄ archived），开关即时写入启停 -->
        <template #cell-status="{ item }">
          <enabled-switch-cell
            :model-value="item.status === 'active'"
            :request="
              (enabled: boolean) =>
                agentMemoryStatusSet(item.id, { status: enabled ? 'active' : 'archived' })
            "
            @update:model-value="(v: boolean) => (item.status = v ? 'active' : 'archived')"
          />
        </template>
      </search-table>

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
import {
  agentMemoryDelete,
  agentMemoryListGet,
  agentMemoryListOptions,
  agentMemoryStatusSet,
} from "@/apis/ai";
import { getResponse } from "@/apis/base";
import type { BatchResultOut } from "@/apis/@types/admin";
import type { BatchActions, ItemActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import i18n from "@/utils/i18n";

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");

// ---- 列表操作 ----
// projectFields 字段投影：行操作依赖 id/title/status 恒定投影，避免用户隐藏对应列后行操作失效
const projectFields = (visible: string[]): string[] => ["id", "title", "status", ...visible];

// 记忆由 Agent 运行期按会话生成，管理端不提供新增/编辑，仅保留删除；启停已改为状态列内联开关。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getActions = (item: Record<string, any>): ItemActions => {
  return {
    delete: () => {
      batchState.value = null;
      deleteConfirmRef.value?.open(item);
    },
  };
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
</script>
