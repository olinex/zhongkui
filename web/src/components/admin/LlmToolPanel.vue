<template>
  <!-- 作用域提醒：注册表为全局配置，常驻于列表上方，避免用户误认为随命名空间隔离 -->
  <div class="px-4 pt-4">
    <v-alert type="info">
      {{ $t("page.llmToolAdmin.scopeHint") }}
    </v-alert>
  </div>

  <search-table
    ref="searchTableRef"
    :list-request="llmToolListGet"
    :options-request="llmToolListOptions"
    :batch-actions="getBatchActions"
    :project-fields="projectFields"
    model-name="llmTool"
  >
    <!-- 来源列：label 经枚举 store 渲染；内置来源由代码内注册表维护、不可删除，以锁形图标标识只读 -->
    <template #cell-source="{ item }">
      <div class="d-flex align-center justify-center ga-1">
        <span>{{ enumLabelOf(EnumClass.llmToolSource, item.source) }}</span>
        <v-icon
          v-if="readonlyTool(item)"
          icon="mdi-lock-outline"
          size="small"
          :title="$t('page.llmToolAdmin.builtinReadonly')"
          :aria-label="$t('page.llmToolAdmin.builtinReadonly')"
        />
      </div>
    </template>

    <!-- 启用列：开关即时写入（后端按批设置启用态接口，单条即单元素批次）；写入期间按行显示
         loading 并禁用，避免重复切换同一行 -->
    <template #cell-enabled="{ item }">
      <v-switch
        :model-value="item.enabled === true"
        color="primary"
        hide-details
        :loading="isRowPending(item)"
        :disabled="isRowPending(item)"
        @update:model-value="(value: boolean | null) => itemToggleAction(item, value === true)"
      />
    </template>
  </search-table>

  <confirm-dialog
    ref="deleteConfirmRef"
    :title="$t('model.llmTool.name')"
    prepend-icon="mdi-puzzle-outline"
    @confirm="batchDeleteConfirm"
  >
    <template #body>
      <confirm-alert
        :action="$t('action.delete')"
        :text="$t('notice.batchConfirm', { count: batchState?.items.length ?? 0 })"
      />
    </template>
  </confirm-dialog>
</template>

<script setup lang="ts" name="llm-tool-panel">
import {
  llmToolBatchDelete,
  llmToolListGet,
  llmToolListOptions,
  llmToolSetEnabled,
} from "@/apis/llm";
import type { BatchResultOut } from "@/apis/@types/admin";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { BatchActions } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import i18n from "@/utils/i18n";

const appStore = useAppStore();
const searchTableRef = useTemplateRef("searchTableRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");

onMounted(() => {
  void ensureEnum(EnumClass.llmToolSource);
});

// readonlyTool 判断工具是否只读（内置来源）：内置来源工具没有所属 MCP 服务器（后端契约：其
// mcpServerName 恒为空），生命周期由代码内注册表与 fixture 维护、不可经 API 删除；判定依据取自
// 后端下发字段，不在前端重复声明来源枚举字面量
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const readonlyTool = (item: Record<string, any>): boolean => !item.mcpServerName;

// projectFields 字段投影：来源（只读判定与展示）与启用状态为行内控件的恒定依赖字段，
// 避免用户隐藏对应列后开关与只读标识失效
const projectFields = (visible: string[]): string[] => [
  "id",
  "name",
  "source",
  "enabled",
  "mcpServerName",
  ...visible,
];

// ---- 启用开关（即时写入）----
// enabledPending 启用态写入进行中的行 id 集合：按行维度记录，使 loading 与禁用只作用于被点击的行，
// 不影响其他行与整表
const enabledPending = ref(new Set<number>());

// isRowPending 判断指定行是否正在写入启用态（开关据此显示 loading 并禁用，避免重复切换）
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const isRowPending = (item: Record<string, any>): boolean =>
  enabledPending.value.has(item.id as number);

// 乐观更新行内状态：请求失败时回退为原状态并提示，成功后无需整表刷新
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const itemToggleAction = async (item: Record<string, any>, enabled: boolean): Promise<void> => {
  const id = item.id as number;
  const previous = item.enabled === true;
  item.enabled = enabled;
  enabledPending.value.add(id);
  try {
    await getSimpleResponse(
      llmToolSetEnabled({ ids: id, enabled }),
      async (rsp) => {
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "success",
        });
      },
      async (rsp) => {
        item.enabled = previous;
        appStore.notify({
          text: rsp.summary,
          timeout: 3000,
          color: "error",
        });
      },
    );
  } finally {
    // 请求结束（无论成功或失败）清除该行的进行中标记
    enabledPending.value.delete(id);
  }
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

// getBatchActions 批量操作：选中项含内置来源工具时返回空集（后端整批拒绝删除），
// 即内置来源不提供任何删除入口
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getBatchActions = (items: Record<string, any>[]): BatchActions => {
  if (items.some((i) => readonlyTool(i))) {
    return {};
  }
  return {
    delete: () => {
      batchState.value = { items };
      deleteConfirmRef.value?.open({});
    },
  };
};

const batchDeleteConfirm = async (): Promise<void> => {
  const state = batchState.value;
  batchState.value = null;
  if (!state) {
    return;
  }
  await getResponse<BatchResultOut>(
    llmToolBatchDelete(state.items.map((i) => i.id as number)),
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
</script>
