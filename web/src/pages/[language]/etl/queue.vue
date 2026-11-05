<template>
  <main-layout current-nav="etl">
    <template #body>
      <!-- 列表骨架与 SearchTable 一致：扁平卡片承载「筛选行 → 操作按钮行 → 图表/表格」，
           操作按钮行独占一行右对齐，不再与筛选控件挤在同一行 -->
      <v-card flat>
        <v-card-text class="px-4 pt-3 pb-2">
          <v-form class="filter-form zk-filter-form">
            <v-row density="compact" class="align-center">
              <v-col cols="auto">
                <v-select
                  v-model="queue"
                  :label="$t('model.queue.field.queue')"
                  :items="queueOptions"
                />
              </v-col>
              <v-col cols="auto">
                <usual-days-field
                  v-model="days"
                  :label="$t('model.queue.field.days')"
                  density="compact"
                />
              </v-col>
            </v-row>
          </v-form>
        </v-card-text>
        <div class="px-4 pb-2 d-flex justify-end align-center ga-2">
          <action-button-group :refresh="refreshAction" collapsible>
            <column-visibility-menu
              v-model="queueVisibleKeys"
              v-model:order="queueColumnOrder"
              :headers="queueAllToggleable"
            />
          </action-button-group>
        </div>
        <v-container fluid class="px-4 pt-0 pb-4">
          <v-row density="compact">
            <v-col cols="6">
              <bars-chart :option="barsOption" />
            </v-col>
            <v-col cols="6">
              <line-chart :option="lineOption" />
            </v-col>
          </v-row>
          <v-row density="compact">
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
                      <enabled-switch-cell
                        v-if="hdr.key === 'running'"
                        :model-value="item.running"
                        :request="
                          (enabled: boolean) => inspectorQueuePauseToggle(item.name, !enabled)
                        "
                        @update:model-value="(v: boolean) => (item.running = v)"
                      />
                      <adaptive-cell v-else :header="hdr" :item="item" />
                    </td>
                  </tr>
                </template>
              </v-data-table>
            </v-col>
          </v-row>
        </v-container>
      </v-card>
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
import { getResponse } from "@/apis/base";
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
import type { TableHeader } from "@/components/@types/table";
import i18n, { localeDateFormat } from "@/utils/i18n";
import { useColumnVisibility } from "@/utils/column";
import { BackendDataType } from "@/types";
import type { BarsChartOption, LineChartOption } from "@/components/@types/charts";
import type { InspectorDailyStatIn } from "@/apis/@types/etl";
import { DateTime } from "luxon";

const appStore = useAppStore();
// days 每日统计的统计天数（默认近 7 天）
const days = ref(7);
// queue 当前选中的队列名（默认 default 队列）
const queue = ref("default");
// queueHeaders 队列表格列定义
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
] as Array<TableHeader>);
// 队列表格的列显示配置：持久化到 localStorage，表头/行渲染统一使用 visibleHeaders
const {
  allToggleable: queueAllToggleable,
  columnOrder: queueColumnOrder,
  visibleKeys: queueVisibleKeys,
  visibleHeaders: queueVisibleHeaders,
} = useColumnVisibility("model:queue", queueHeaders);
// statItems 按队列名索引的每日统计项
const statItems = ref({} as Record<string, InspectorDailyStatItem[]>);
// queueItems 队列列表项
const queueItems = ref([] as InspectorQueueListItem[]);
// queueOptions 队列下拉选项：由每日统计数据的队列名生成
const queueOptions = computed(() => {
  return Object.keys(statItems.value).map((key) => {
    return { title: key, value: key };
  });
});
// barsOption 各队列任务状态分布柱状图配置
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
// lineOption 所选队列的任务处理趋势折线图配置
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

// 切换统计天数时自动刷新每日统计
watch(days, (value) => {
  getDailyStatAction({ days: value });
});

// refreshAction 刷新队列列表与每日统计
const refreshAction = async (): Promise<void> => {
  await Promise.all([getQueueListAction(), getDailyStatAction({ days: days.value })]);
};

// getQueueListAction 拉取队列列表，并据暂停状态推导运行状态
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

// getDailyStatAction 按天数拉取每日队列统计
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

// 页面挂载后首次加载数据
onMounted(refreshAction);
</script>
