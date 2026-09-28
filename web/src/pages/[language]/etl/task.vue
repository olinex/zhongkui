<template>
  <main-layout current-nav="etl">
    <template #body>
      <!-- 列表骨架与 SearchTable 一致：扁平卡片承载「筛选行 → 操作按钮行 → 表格」，
           操作按钮行独占一行右对齐，不再与筛选控件挤在同一行 -->
      <v-card flat>
        <v-card-text class="px-4 pt-3 pb-2">
          <v-form class="filter-form zk-filter-form">
            <v-row dense align-center>
              <v-col cols="auto">
                <v-select
                  v-model="queue"
                  :label="$t('model.queue.field.queue')"
                  :items="queueOptions"
                />
              </v-col>
              <v-col cols="auto">
                <v-select
                  v-model="taskState"
                  :label="$t('model.task.field.state')"
                  :items="taskTypeOptions"
                />
              </v-col>
            </v-row>
          </v-form>
        </v-card-text>
        <div class="px-4 pb-2 d-flex justify-end align-center ga-2">
          <action-button-group :refresh="refreshAction" :actions="taskToolbarActions" collapsible>
            <column-visibility-menu
              v-model="taskVisibleKeys"
              v-model:order="taskColumnOrder"
              :headers="taskAllToggleable"
            />
          </action-button-group>
        </div>
        <v-container fluid class="px-4 pt-0 pb-4">
          <v-row dense>
            <v-col cols="12">
              <v-data-table-server
                v-model:page="page"
                v-model:items-per-page="pageSize"
                :headers="taskVisibleHeaders"
                :items="taskItems"
                :items-length="taskTotal"
                :items-per-page-options="pageSizeOptions"
                select-strategy="page"
              >
                <template #item="{ item }">
                  <tr class="text-no-wrap">
                    <td
                      v-for="hdr in taskVisibleHeaders"
                      :key="hdr.key"
                      :class="
                        hdr.fixed === 'end'
                          ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                          : undefined
                      "
                    >
                      <adaptive-cell :header="hdr" :item="item" :actions="getTaskActions" />
                    </td>
                  </tr>
                </template>
              </v-data-table-server>
            </v-col>
          </v-row>
        </v-container>
      </v-card>
      <confirm-dialog
        ref="confirmStateDeleteTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="deleteStateTaskAction"
      >
        <template #body>
          <confirm-alert
            :action="$t('action.delete')"
            :text="
              $t(`notice.deleteConfirm`, {
                category: $t(`model.queue.field.${taskState}`),
                key: $t(`model.task.name`),
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="confirmStateArchiveTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="archiveStateTaskAction"
      >
        <template #body>
          <confirm-alert
            :action="$t(`action.archive`)"
            :text="
              $t(`notice.archiveConfirm`, {
                category: $t(`model.queue.field.${taskState}`),
                key: $t(`model.task.name`),
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="confirmStateRunTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="runStateTaskAction"
      >
        <template #body>
          <confirm-alert
            :action="$t(`action.run`)"
            :text="
              $t(`notice.runConfirm`, {
                category: $t(`model.queue.field.${taskState}`),
                key: $t(`model.task.name`),
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="confirmSingleDeleteTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="deleteTaskAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t(`action.delete`)"
            :text="
              $t(`notice.deleteConfirm`, {
                category: $t(`model.task.name`),
                key: proxy.id,
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="confirmSingleArchiveTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="archiveTaskAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t(`action.archive`)"
            :text="
              $t(`notice.archiveConfirm`, {
                category: $t(`model.task.name`),
                key: proxy.id,
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="confirmSingleRunTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="runTaskAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t(`action.run`)"
            :text="
              $t(`notice.runConfirm`, {
                category: $t(`model.task.name`),
                key: proxy.id,
              })
            "
          />
        </template>
      </confirm-dialog>
      <confirm-dialog
        ref="confirmSingleCancelTaskDialogRef"
        :title="$t(`model.task.name`)"
        prepend-icon="mdi-format-list-bulleted"
        @confirm="cancelPorcessingTaskAction"
      >
        <template #body="{ proxy }">
          <confirm-alert
            :action="$t(`action.cancel`)"
            :text="
              $t(`notice.cancelConfirm`, {
                category: $t(`model.task.name`),
                key: proxy.id,
              })
            "
          />
        </template>
      </confirm-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: ETL Task List
meta:
  title: title.taskEtl
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { getResponse, getSimpleResponse } from "@/apis/base";
import type {
  InspectorDailyStatItem,
  InspectorDailyStatListOut,
  InspectorQueueListItem,
  InspectorQueueListOut,
  InspectorTaskListItem,
  InspectorTaskListOut,
  TaskState,
} from "./@types";
import {
  inspectorDailyStatListGet,
  inspectorQueueListGet,
  inspectorStateTaskArchive,
  inspectorStateTaskDelete,
  inspectorStateTaskRun,
  inspectorTaskArchive,
  inspectorTaskCancelProcessing,
  inspectorTaskDelete,
  inspectorTaskListGet,
  inspectorTaskRun,
} from "@/apis/etl";
import { useAppStore } from "@/stores/app";
import type { ItemActions, TableHeader } from "@/components/@types/table";
import type { ActionButton } from "@/components/base/ActionButtonGroup.vue";
import i18n from "@/utils/i18n";
import { useColumnVisibility } from "@/utils/column";
import { BackendDataType } from "@/types";
import type { InspectorDailyStatIn, InspectorTaskListIn } from "@/apis/@types/etl";

const confirmStateDeleteTaskDialogRef = useTemplateRef("confirmStateDeleteTaskDialogRef");

const confirmStateArchiveTaskDialogRef = useTemplateRef("confirmStateArchiveTaskDialogRef");

const confirmStateRunTaskDialogRef = useTemplateRef("confirmStateRunTaskDialogRef");

const confirmSingleDeleteTaskDialogRef = useTemplateRef("confirmSingleDeleteTaskDialogRef");

const confirmSingleArchiveTaskDialogRef = useTemplateRef("confirmSingleArchiveTaskDialogRef");

const confirmSingleRunTaskDialogRef = useTemplateRef("confirmSingleRunTaskDialogRef");

const confirmSingleCancelTaskDialogRef = useTemplateRef("confirmSingleCancelTaskDialogRef");

const pageSizeOptions = [
  { title: "10", value: 10 },
  { title: "20", value: 20 },
  { title: "50", value: 50 },
  { title: "100", value: 100 },
];

const appStore = useAppStore();
const days = ref(7);
const queue = ref("default");
const taskState = ref("archived" as TaskState);
const page = ref(1);
const pageSize = ref(10);
const taskHeaders = ref([
  {
    key: "id",
    title: i18n.global.t("model.task.field.id"),
    type: BackendDataType.UUID,
    align: "start",
    sortable: false,
  },
  {
    key: "queue",
    title: i18n.global.t("model.task.field.queue"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: false,
  },
  {
    key: "type",
    title: i18n.global.t("model.task.field.type"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: false,
  },
  {
    key: "payload",
    title: i18n.global.t("model.task.field.payload"),
    type: BackendDataType.JSON,
    align: "start",
    sortable: false,
  },
  {
    key: "result",
    title: i18n.global.t("model.task.field.result"),
    type: BackendDataType.JSON,
    align: "start",
    sortable: false,
  },
  {
    key: "retried",
    title: i18n.global.t("model.task.field.retried"),
    type: BackendDataType.INT,
    align: "start",
    sortable: false,
  },
  {
    key: "lastErr",
    title: i18n.global.t("model.task.field.lastErr"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: false,
  },
  {
    key: "lastFailedAt",
    title: i18n.global.t("model.task.field.lastFailedAt"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: false,
  },
  {
    key: "nextProcessAt",
    title: i18n.global.t("model.task.field.nextProcessAt"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: false,
  },
  {
    key: "completedAt",
    title: i18n.global.t("model.task.field.completedAt"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: false,
  },
  {
    key: "deadline",
    title: i18n.global.t("model.task.field.deadline"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: false,
  },
  {
    key: "timeout",
    title: i18n.global.t("model.task.field.timeout"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: false,
  },
  {
    key: "retention",
    title: i18n.global.t("model.task.field.retention"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: false,
  },
  {
    key: "group",
    title: i18n.global.t("model.task.field.group"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: false,
  },
  {
    key: "isOrphaned",
    title: i18n.global.t("model.task.field.isOrphaned"),
    type: BackendDataType.BOOLEAN,
    align: "start",
    sortable: false,
  },
  {
    key: "__actions",
    title: i18n.global.t("model.__base.field.__actions"),
    type: BackendDataType.ACTIONS,
    align: "start",
    sortable: false,
    fixed: "end",
  },
] as Array<TableHeader>);
// 任务表格的列显示配置：持久化到 localStorage，表头/行渲染统一使用 visibleHeaders
const {
  allToggleable: taskAllToggleable,
  columnOrder: taskColumnOrder,
  visibleKeys: taskVisibleKeys,
  visibleHeaders: taskVisibleHeaders,
} = useColumnVisibility("model:task", taskHeaders);
// 队列列表（各状态计数）用于计算任务总数；每日统计键用于队列筛选项
const statItems = ref({} as Record<string, InspectorDailyStatItem[]>);
const queueItems = ref([] as InspectorQueueListItem[]);
const taskItems = ref([] as InspectorTaskListItem[]);
const taskTotal = computed(() => {
  // 队列数据未返回（或尚无队列）时队列列表为空，reduce 必须带初始值以免空数组抛 TypeError
  return queueItems.value
    .map((item) => {
      return item[taskState.value];
    })
    .reduce((total, cur) => {
      return total + cur;
    }, 0);
});
const queueOptions = computed(() => {
  return Object.keys(statItems.value).map((key) => {
    return { title: key, value: key };
  });
});
const taskTypeOptions = computed(() => {
  return [
    { title: i18n.global.t("model.queue.field.active"), value: "active" },
    { title: i18n.global.t("model.queue.field.pending"), value: "pending" },
    { title: i18n.global.t("model.queue.field.retry"), value: "retry" },
    { title: i18n.global.t("model.queue.field.scheduled"), value: "scheduled" },
    { title: i18n.global.t("model.queue.field.archived"), value: "archived" },
    {
      title: i18n.global.t("model.queue.field.aggregating"),
      value: "aggregating",
    },
    { title: i18n.global.t("model.queue.field.completed"), value: "completed" },
  ];
});
// 任务页的次级操作按钮：随任务状态动态展示，收起时隐藏（刷新按钮常驻）
const taskToolbarActions = computed<ActionButton[]>(() => {
  const actions: ActionButton[] = [];
  if (taskState.value !== "active") {
    actions.push({
      icon: "mdi-delete-forever",
      title: i18n.global.t("action.delete"),
      onClick: () => confirmStateDeleteTaskDialogRef.value?.open({}),
    });
  }
  if (["pending", "scheduled", "retry", "aggregating"].includes(taskState.value)) {
    actions.push({
      icon: "mdi-archive",
      title: i18n.global.t("action.archive"),
      onClick: () => confirmStateArchiveTaskDialogRef.value?.open({}),
    });
  }
  if (["scheduled", "retry", "archived", "aggregating"].includes(taskState.value)) {
    actions.push({
      icon: "mdi-menu-right",
      title: i18n.global.t("action.run"),
      onClick: () => confirmStateRunTaskDialogRef.value?.open({}),
    });
  }
  return actions;
});

watch(days, (value) => {
  getDailyStatAction({ days: value });
});

watch(page, (value) => {
  getTaskListAction({
    page: value,
    pageSize: pageSize.value,
    queue: queue.value,
    state: taskState.value,
  });
});

watch(pageSize, (value) => {
  page.value = 1;
  getTaskListAction({
    page: page.value,
    pageSize: value,
    queue: queue.value,
    state: taskState.value,
  });
});

watch(queue, (value) => {
  page.value = 1;
  getTaskListAction({
    page: page.value,
    pageSize: pageSize.value,
    queue: value,
    state: taskState.value,
  });
});

watch(taskState, (value) => {
  page.value = 1;
  getTaskListAction({
    page: page.value,
    pageSize: pageSize.value,
    queue: queue.value,
    state: value,
  });
});

// 队列列表加载后，若当前选中的队列并非真实存在的队列（如硬编码的默认 "default"
// 与 asynq 实际队列不符），自动切换到第一个真实队列，避免持续查询不存在的队列。
watch(
  queueItems,
  (items) => {
    if (items.length === 0) return;
    const names = items.map((item) => item.name);
    if (!names.includes(queue.value)) {
      queue.value = names[0];
    }
  },
  { immediate: true },
);

// refreshAction 并行刷新队列列表、每日统计与任务列表。
const refreshAction = async (): Promise<void> => {
  await Promise.all([
    getQueueListAction(),
    getDailyStatAction({ days: days.value }),
    getTaskListAction({
      page: page.value,
      pageSize: pageSize.value,
      queue: queue.value,
      state: taskState.value,
    }),
  ]);
};

// getQueueListAction 拉取队列列表并推导各队列的运行状态。
const getQueueListAction = async (): Promise<void> => {
  await getResponse<InspectorQueueListOut>(
    inspectorQueueListGet(),
    async (rsp) => {
      queueItems.value = Object.entries(rsp.data).map(([name, item]) => {
        return {
          name,
          ...item,
          running: !item.paused,
        };
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

// getDailyStatAction 按天数拉取每日统计，作为队列筛选项的数据来源。
const getDailyStatAction = async (data: InspectorDailyStatIn): Promise<void> => {
  await getResponse<InspectorDailyStatListOut>(
    inspectorDailyStatListGet(data),
    async (rsp) => {
      statItems.value = rsp.data;
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

// getTaskListAction 按筛选条件分页拉取任务列表。
const getTaskListAction = async (data: InspectorTaskListIn): Promise<void> => {
  await getResponse<InspectorTaskListOut>(
    inspectorTaskListGet(data),
    async (rsp) => {
      taskItems.value = rsp.data;
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

// getTaskActions 按当前任务状态动态生成可用的行操作。
const getTaskActions = (): ItemActions => {
  const actions: ItemActions = {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete: function (item: Record<string, any>): void {
      confirmSingleDeleteTaskDialogRef.value?.open(item);
    },
  };
  if (taskState.value !== "archived") {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    actions.archive = function (item: Record<string, any>): void {
      confirmSingleArchiveTaskDialogRef.value?.open(item);
    };
  }
  if (!["archived", "completed"].includes(taskState.value)) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    actions.cancel = function (item: Record<string, any>): void {
      confirmSingleCancelTaskDialogRef.value?.open(item);
    };
  }
  if (["scheduled", "retry", "archived", "aggregating"].includes(taskState.value)) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    actions.run = function (item: Record<string, any>): void {
      confirmSingleRunTaskDialogRef.value?.open(item);
    };
  }
  return actions;
};

// deleteStateTaskAction 删除当前状态下的整批任务并刷新列表。
const deleteStateTaskAction = async (): Promise<void> => {
  await getResponse<number>(
    inspectorStateTaskDelete({ queue: queue.value, state: taskState.value }),
    async (rsp) => {
      page.value = 1;
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
      });
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

// deleteTaskAction 删除单条任务并刷新队列与任务列表。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const deleteTaskAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    inspectorTaskDelete(item.id, item.queue),
    async () => {
      await getQueueListAction();
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
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

// archiveStateTaskAction 归档当前状态下的整批任务并刷新列表。
const archiveStateTaskAction = async (): Promise<void> => {
  await getResponse<number>(
    inspectorStateTaskArchive({ queue: queue.value, state: taskState.value }),
    async (rsp) => {
      page.value = 1;
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
      });
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

// archiveTaskAction 归档单条任务并刷新队列与任务列表。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const archiveTaskAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    inspectorTaskArchive(item.id, item.queue),
    async () => {
      await getQueueListAction();
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
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

// runStateTaskAction 立即执行当前状态下的整批任务并刷新列表。
const runStateTaskAction = async (): Promise<void> => {
  await getResponse<number>(
    inspectorStateTaskRun({ queue: queue.value, state: taskState.value }),
    async (rsp) => {
      page.value = 1;
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
      });
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

// runTaskAction 立即执行单条任务并刷新队列与任务列表。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const runTaskAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    inspectorTaskRun(item.id, item.queue),
    async () => {
      await getQueueListAction();
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
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

// cancelPorcessingTaskAction 取消单条正在处理的任务并刷新列表。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const cancelPorcessingTaskAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    inspectorTaskCancelProcessing(item.id),
    async (rsp) => {
      await getTaskListAction({
        page: page.value,
        pageSize: pageSize.value,
        queue: queue.value,
        state: taskState.value,
      });
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

onMounted(refreshAction);
</script>
