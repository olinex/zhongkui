<template>
  <main-layout current-nav="etl">
    <template #body>
      <v-container>
        <v-form class="filter-form">
          <v-row dense align-center>
            <v-col cols="auto">
              <v-select
                v-model="queue"
                :label="$t('model.queue.field.queue')"
                :items="queueOptions"
                density="compact"
              />
            </v-col>
            <v-col cols="auto">
              <usual-days-field
                v-model="days"
                :label="$t('model.queue.field.days')"
                density="compact"
              />
            </v-col>
            <action-button-group class="ml-auto" :refresh="refreshAction" collapsible>
              <column-visibility-menu
                v-model="queueVisibleKeys"
                v-model:order="queueColumnOrder"
                :headers="queueAllToggleable"
              />
            </action-button-group>
          </v-row>
        </v-form>
        <v-divider class="my-2" />
        <v-row dense>
          <v-col cols="6">
            <bars-chart :option="barsOption" />
          </v-col>
          <v-col cols="6">
            <line-chart :option="lineOption" />
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-data-table :headers="queueVisibleHeaders" :items="queueItems">
              <template #item="{ item }">
                <tr class="text-no-wrap">
                  <td
                    v-for="hdr in queueVisibleHeaders"
                    :key="hdr.key"
                    :class="
                      hdr.fixed === 'end'
                        ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                        : undefined
                    "
                  >
                    <adaptive-cell :header="hdr" :item="item" :actions="getQueueActions" />
                  </td>
                </tr>
              </template>
            </v-data-table>
          </v-col>
        </v-row>
      </v-container>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: ETL Queue List
meta:
  title: title.queueEtl
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
} from "./@types";
import {
  inspectorDailyStatListGet,
  inspectorQueueListGet,
  inspectorQueuePauseToggle,
} from "@/apis/etl";
import { useAppStore } from "@/stores/app";
import type { ItemActions, TableHeader } from "@/components/@types/table";
import i18n, { localeDateFormat } from "@/utils/i18n";
import { useColumnVisibility } from "@/utils/column";
import { BackendDataType } from "@/types";
import type { BarsChartOption, LineChartOption } from "@/components/@types/charts";
import type { InspectorDailyStatIn } from "@/apis/@types/etl";
import { DateTime } from "luxon";

const appStore = useAppStore();
const days = ref(7);
const queue = ref("default");
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
    fixed: "end",
  },
] as Array<TableHeader>);
// 队列表格的列显示配置：持久化到 localStorage，表头/行渲染统一使用 visibleHeaders
const {
  allToggleable: queueAllToggleable,
  columnOrder: queueColumnOrder,
  visibleKeys: queueVisibleKeys,
  visibleHeaders: queueVisibleHeaders,
} = useColumnVisibility("model:queue", queueHeaders);
const statItems = ref({} as Record<string, InspectorDailyStatItem[]>);
const queueItems = ref([] as InspectorQueueListItem[]);
const queueOptions = computed(() => {
  return Object.keys(statItems.value).map((key) => {
    return { title: key, value: key };
  });
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

const refreshAction = async (): Promise<void> => {
  await Promise.all([getQueueListAction(), getDailyStatAction({ days: days.value })]);
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

onMounted(refreshAction);
</script>

<style scoped>
/* 折叠筛选输入的空消息区，使按钮组与输入控件保持同一水平线，
   与流量页 filter-form 的垂直间距保持一致 */
/* stylelint-disable-next-line selector-class-pattern */
.filter-form :deep(.v-input__details:has(.v-messages:empty)) {
  min-height: 0;
  padding-top: 0;
}

.filter-form :deep(.v-messages:empty) {
  min-height: 0;
}
</style>
