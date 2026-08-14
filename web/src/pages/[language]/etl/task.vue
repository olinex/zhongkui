<template>
  <main-layout current-nav="etl">
    <template #body>
      <v-container>
        <v-row class="justify-between align-top" dense>
          <v-col cols="6">
            <v-tabs v-model="tab" color="primary" density="compact">
              <v-tab value="queue">{{ $t("model.queue.name") }}</v-tab>
              <v-tab value="task">{{ $t("model.task.name") }}</v-tab>
              <v-tab value="scheduler">{{ $t("model.scheduler.name") }}</v-tab>
            </v-tabs>
          </v-col>
          <v-spacer />
          <v-col v-show="tab !== 'scheduler'" cols="2">
            <v-select
              v-model="queue"
              :label="$t('model.queue.field.queue')"
              :items="queueOptions"
              density="compact"
            />
          </v-col>
          <v-col v-show="tab === 'queue'" cols="2">
            <usual-days-field
              v-model="days"
              :label="$t('model.queue.field.days')"
              density="compact"
            />
          </v-col>
          <v-col v-show="tab === 'task'" cols="2">
            <v-select
              v-model="taskState"
              :label="$t('model.task.field.state')"
              :items="taskTypeOptions"
              density="compact"
            />
          </v-col>
        </v-row>
        <v-row v-if="tab === 'queue'" dense>
          <v-col cols="6">
            <bars-chart :option="barsOption" />
          </v-col>
          <v-col cols="6">
            <line-chart :option="lineOption" />
          </v-col>
        </v-row>
      </v-container>
      <v-tabs-window v-model="tab">
        <v-tabs-window-item value="queue">
          <v-container>
            <v-divider />
            <v-row dense>
              <v-col cols="12">
                <v-data-table :headers="queueHeaders" :items="queueItems">
                  <template #item="{ item }">
                    <tr class="text-no-wrap">
                      <td v-for="hdr in queueHeaders" :key="hdr.key">
                        <adaptive-cell :header="hdr" :item="item" :actions="getQueueActions" />
                      </td>
                    </tr>
                  </template>
                </v-data-table>
              </v-col>
            </v-row>
          </v-container>
        </v-tabs-window-item>
        <v-tabs-window-item value="scheduler">
          <v-container>
            <v-divider />
            <v-row dense>
              <v-col cols="12">
                <v-data-table :headers="schedulerHeaders" :items="schedulerItems">
                  <template #item="{ item }">
                    <tr class="text-no-wrap">
                      <td v-for="hdr in schedulerHeaders" :key="hdr.key">
                        <adaptive-cell :header="hdr" :item="item" />
                      </td>
                    </tr>
                  </template>
                </v-data-table>
              </v-col>
            </v-row>
          </v-container>
        </v-tabs-window-item>
        <v-tabs-window-item value="task">
          <v-container>
            <v-divider />
            <v-row dense>
              <v-col cols="12">
                <v-data-table-server
                  v-model:page="page"
                  v-model:items-per-page="pageSize"
                  :headers="taskHeaders"
                  :items="taskItems"
                  :items-length="taskTotal"
                  :items-per-page-options="pageSizeOptions"
                  select-strategy="page"
                >
                  <template #top>
                    <v-toolbar :collapse="taskBtnCollapse" density="compact">
                      <template #prepend>
                        <v-btn icon="mdi-refresh" @click="refreshAction" />
                        <v-btn
                          v-show="!taskBtnCollapse && taskState !== 'active'"
                          icon="mdi-delete-forever"
                          @click="confirmStateDeleteTaskDialogRef?.open({})"
                        />
                        <v-btn
                          v-show="
                            !taskBtnCollapse &&
                            ['pending', 'scheduled', 'retry', 'aggregating'].includes(taskState)
                          "
                          icon="mdi-archive"
                          @click="confirmStateArchiveTaskDialogRef?.open({})"
                        />
                        <v-btn
                          v-show="
                            !taskBtnCollapse &&
                            ['scheduled', 'retry', 'archived', 'aggregating'].includes(taskState)
                          "
                          icon="mdi-menu-right"
                          @click="confirmStateRunTaskDialogRef?.open({})"
                        />
                        <v-btn
                          :icon="taskBtnCollapse ? 'mdi-chevron-right' : 'mdi-chevron-left'"
                          @click="taskBtnCollapse = !taskBtnCollapse"
                        />
                      </template>
                    </v-toolbar>
                  </template>
                  <template #item="{ item }">
                    <tr class="text-no-wrap">
                      <td v-for="hdr in taskHeaders" :key="hdr.key">
                        <adaptive-cell :header="hdr" :item="item" :actions="getTaskActions" />
                      </td>
                    </tr>
                  </template>
                </v-data-table-server>
              </v-col>
            </v-row>
          </v-container>
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
        </v-tabs-window-item>
      </v-tabs-window>
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
import {
  type InspectorQueueListOut,
  type InspectorQueueListItem,
  type InspectorDailyStatListOut,
  type InspectorDailyStatItem,
  type InspectorTaskListItem,
  type TaskState,
  type InspectorTaskListOut,
  type InspectorSchedulerListOut,
  type InspectorSchedulerListItem,
} from "./@types";
import {
  inspectorDailyStatListGet,
  inspectorQueueListGet,
  inspectorQueuePauseToggle,
  inspectorSchedulerListGet,
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
import i18n, { localeDateFormat } from "@/utils/i18n";
import { BackendDataType } from "@/types";
import type { BarsChartOption, LineChartOption } from "@/components/@types/charts";
import type { InspectorDailyStatIn, InspectorTaskListIn } from "@/apis/@types/etl";
import { DateTime } from "luxon";

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
const tab = ref("chart");
const days = ref(7);
const queue = ref("default");
const taskState = ref("archived" as TaskState);
const page = ref(1);
const pageSize = ref(10);
const taskBtnCollapse = ref(true);
const queueHeaders = ref([
  {
    key: "name",
    title: i18n.global.t("model.queue.field.queue"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: true,
  },
  {
    key: "running",
    title: i18n.global.t("model.queue.field.running"),
    type: BackendDataType.BOOLEAN,
    align: "start",
    sortable: false,
  },
  {
    key: "memoryUsage",
    title: i18n.global.t("model.queue.field.memoryUsage"),
    type: BackendDataType.BINARY_BYTE,
    align: "start",
    sortable: true,
  },
  {
    key: "latency",
    title: i18n.global.t("model.queue.field.latency"),
    type: BackendDataType.DURATION,
    align: "start",
    sortable: true,
  },
  {
    key: "processed",
    title: i18n.global.t("model.queue.field.processed"),
    align: "start",
    sortable: true,
  },
  {
    key: "size",
    title: i18n.global.t("model.queue.field.size"),
    type: BackendDataType.INT64,
    align: "start",
    sortable: true,
  },
  {
    key: "failed",
    title: i18n.global.t("model.queue.field.failed"),
    type: BackendDataType.INT64,
    align: "start",
    sortable: true,
  },
  {
    key: "retrievalTime",
    title: i18n.global.t("model.queue.field.retrievalTime"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: true,
  },
  {
    key: "__actions",
    title: i18n.global.t("model.__base.field.__actions"),
    type: BackendDataType.ACTIONS,
    align: "start",
    sortable: false,
  },
] as Array<TableHeader>);
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
  },
] as Array<TableHeader>);
const schedulerHeaders = ref([
  {
    key: "id",
    title: i18n.global.t("model.scheduler.field.id"),
    type: BackendDataType.UUID,
    align: "start",
    sortable: true,
  },
  {
    key: "spec",
    title: i18n.global.t("model.scheduler.field.spec"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: true,
  },
  {
    key: "next",
    title: i18n.global.t("model.scheduler.field.next"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: true,
  },
  {
    key: "prev",
    title: i18n.global.t("model.scheduler.field.prev"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: true,
  },
  {
    key: "typeName",
    title: i18n.global.t("model.scheduler.field.typeName"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: true,
  },
  {
    key: "payload",
    title: i18n.global.t("model.scheduler.field.payload"),
    type: BackendDataType.JSON,
    align: "start",
    sortable: true,
  },
] as Array<TableHeader>);
const statItems = ref({} as Record<string, InspectorDailyStatItem[]>);
const queueItems = ref([] as InspectorQueueListItem[]);
const taskItems = ref([] as InspectorTaskListItem[]);
const schedulerItems = ref([] as InspectorSchedulerListItem[]);
const taskTotal = computed(() => {
  return queueItems.value
    .map((item) => {
      return item[taskState.value];
    })
    .reduce((total, cur) => {
      return total + cur;
    });
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
const barsOption = computed((): BarsChartOption => {
  const active = i18n.global.t("chart.field.active");
  const pending = i18n.global.t("chart.field.pending");
  const retring = i18n.global.t("chart.field.retring");
  const scheduled = i18n.global.t("chart.field.scheduled");
  const archived = i18n.global.t("chart.field.archived");
  return {
    title: {
      text: i18n.global.t("chart.title.queueSize"),
      left: "center",
    },
    tooltip: {
      trigger: "item",
      formatter: "{a} <br/>{b} : {c}",
    },
    legend: {
      orient: "horizontal",
      left: "center",
      data: queueItems.value.map((item) => item.name),
    },
    xAxis: {
      type: "category",
      name: i18n.global.t("chart.axis.queue"),
      data: queueItems.value.map((item) => item.name),
    },
    yAxis: {
      type: "value",
      name: `${i18n.global.t("chart.axis.size")} (${i18n.global.t("unit.count")})`,
    },
    series: queueItems.value.map((item) => {
      return {
        name: item.name,
        type: "bar",
        data: [
          { name: active, value: item.active },
          { name: pending, value: item.pending },
          { name: retring, value: item.retry },
          { name: scheduled, value: item.scheduled },
          { name: archived, value: item.archived },
        ],
      };
    }),
  };
});
const lineOption = computed((): LineChartOption => {
  const processed = i18n.global.t("chart.field.processed");
  const failed = i18n.global.t("chart.field.failed");
  return {
    title: {
      text: i18n.global.t("chart.title.taskProcess"),
      left: "center",
    },
    tooltip: {
      trigger: "item",
      formatter: "{a} <br/>{b} : {c}",
    },
    legend: {
      orient: "horizontal",
      left: "center",
      data: [processed, failed],
    },
    xAxis: {
      type: "category",
      name: i18n.global.t("chart.axis.date"),
      data:
        statItems.value && statItems.value[queue.value]
          ? statItems.value[queue.value].map((item) =>
              localeDateFormat(DateTime.fromISO(item.date)),
            )
          : [],
    },
    yAxis: {
      type: "value",
      name: `${i18n.global.t("chart.axis.size")} (${i18n.global.t("unit.count")})`,
    },
    series: [
      {
        name: processed,
        type: "line",
        smooth: true,
        data:
          statItems.value && statItems.value[queue.value]
            ? statItems.value[queue.value].map((item) => item.processed)
            : [],
      },
      {
        name: failed,
        type: "line",
        smooth: true,
        data:
          statItems.value && statItems.value[queue.value]
            ? statItems.value[queue.value].map((item) => item.failed)
            : [],
      },
    ],
  };
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

const refreshAction = async (): Promise<void> => {
  await Promise.all([
    getQueueListAction(),
    getSchedulerListAction(),
    getDailyStatAction({ days: days.value }),
    getTaskListAction({
      page: page.value,
      pageSize: pageSize.value,
      queue: queue.value,
      state: taskState.value,
    }),
  ]);
};

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

const getSchedulerListAction = async (): Promise<void> => {
  await getResponse<InspectorSchedulerListOut>(
    inspectorSchedulerListGet(),
    async (rsp) => {
      schedulerItems.value = rsp.data;
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
const pauseToggleQueueAction = async (item: Record<string, any>): Promise<void> => {
  await getSimpleResponse(
    inspectorQueuePauseToggle(item.name, item.running),
    async (rsp) => {
      queueItems.value = queueItems.value.map((queue) => {
        return {
          ...queue,
          running: queue.name === item.name ? !item.running : item.running,
        };
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const getQueueActions = (item: Record<string, any>): ItemActions => {
  const actions: ItemActions = {};
  if (item.running) {
    actions.pause = pauseToggleQueueAction;
  } else {
    actions.unpause = pauseToggleQueueAction;
  }
  return actions;
};

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
