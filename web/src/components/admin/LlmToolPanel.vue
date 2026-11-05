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

    <!-- 启用列：开关即时写入（后端按批设置启用态接口，单条即单元素批次）；写入与失败回退
         由共享开关组件按行处理 -->
    <template #cell-enabled="{ item }">
      <enabled-switch-cell
        v-model="item.enabled"
        :request="(enabled: boolean) => llmToolSetEnabled({ ids: item.id, enabled })"
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
import { getResponse } from "@/apis/base";
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
