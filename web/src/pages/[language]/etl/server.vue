<template>
  <main-layout current-nav="etl">
    <template #body>
      <v-empty-state
        v-show="servers.length === 0"
        v-request="getServerListAction"
        icon="mdi-magnify"
        :headline="$t('summary.noServerYet', { name: $t('model.server.name') })"
        :action-text="$t('action.reset')"
      />
      <!-- 服务器列表：按全站展开面板基线收纳（zk-panel + v-expansion-panels + expansion-panel-title），
           刷新入口随标题行 actions 槽展示在右侧 -->
      <div v-if="servers.length > 0" class="zk-panel pa-4 mb-6">
        <v-expansion-panels>
          <v-expansion-panel v-for="server in servers" :key="server.id">
            <expansion-panel-title
              icon="mdi-server-outline"
              :title="server.host"
              :subtitle="serverSubtitle(server)"
            >
              <!-- 元信息独立成行：长标题不会与 chip 组挤在同一行换行 -->
              <template #meta>
                <div class="d-flex flex-wrap ga-2">
                  <!-- 阻断冒泡：点击 id chip 只复制，不切换面板展开状态 -->
                  <div @click.stop>
                    <hex-string-chip size="x-small" :value="server.id" prefix="id: " :length="32" />
                  </div>
                  <v-chip size="x-small"> pid: {{ server.pid }} </v-chip>
                  <v-chip size="x-small">
                    {{ $t("model.server.field.started") }}:
                    {{ localeDatetimeFormat(DateTime.fromISO(server.started)) }}
                  </v-chip>
                </div>
              </template>
              <template #actions>
                <!-- 阻断冒泡：点击刷新不应同时触发展开 / 收起 -->
                <div @click.stop>
                  <action-button-group :refresh="getServerListAction" />
                </div>
              </template>
            </expansion-panel-title>
            <v-expansion-panel-text>
              <v-row>
                <v-col class="d-flex justify-center" cols="12">
                  <pie-chart
                    height="300px"
                    width="400px"
                    :option="piesOption(server.queueWeights)"
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col cols="12">
                  <v-data-table :headers="taskHeaders" :items="server.processingTasks" />
                </v-col>
              </v-row>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
      </div>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: ETL Server List
meta:
  title: title.serverEtl
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { getResponse } from "@/apis/base";
import type { InspectorServerListItem, InspectorServerListOut } from "./@types";
import { inspectorServerListGet } from "@/apis/etl";
import { useAppStore } from "@/stores/app";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { DateTime } from "luxon";
import type { PiesChartOption } from "@/components/@types/charts";
import type { TableHeader } from "@/components/@types/table";
import { BackendDataType } from "@/types";
import { EnumClass, ensureEnum } from "@/utils/enums";

const appStore = useAppStore();
const servers = ref([] as InspectorServerListOut);

// serverSubtitle 面板标题说明行：展示服务状态（其余元信息作为标题行内 chip）
const serverSubtitle = (server: InspectorServerListItem): string =>
  `${i18n.global.t("model.server.field.status")}: ${server.status}`;
const taskHeaders = ref([
  {
    key: "id",
    title: i18n.global.t("model.task.field.id"),
    type: BackendDataType.UUID,
    align: "start",
    sortable: true,
  },
  {
    key: "queue",
    title: i18n.global.t("model.task.field.queue"),
    type: BackendDataType.STRING,
    align: "start",
    sortable: true,
  },
  {
    key: "type",
    title: i18n.global.t("model.task.field.type"),
    type: BackendDataType.STRING,
    // 处理中任务的类型按 taskType 枚举渲染 label，与任务页 / 定时任务页保持一致
    enumType: EnumClass.taskType,
    align: "start",
    sortable: true,
  },
  {
    key: "payload",
    title: i18n.global.t("model.task.field.payload"),
    type: BackendDataType.JSON,
    align: "start",
    sortable: true,
  },
  {
    key: "start",
    title: i18n.global.t("model.task.field.start"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: true,
  },
  {
    key: "deadline",
    title: i18n.global.t("model.task.field.deadline"),
    type: BackendDataType.DATETIME,
    align: "start",
    sortable: true,
  },
] as TableHeader[]);

onMounted(async () => {
  // 预取任务类型枚举，使处理中任务的类型列按 label 展示
  void ensureEnum(EnumClass.taskType);
  await getServerListAction();
});

const piesOption = (queueWeights: Record<string, number>): PiesChartOption => {
  return {
    title: {
      text: i18n.global.t("model.server.field.queueWeights"),
      left: "left",
    },
    legend: {},
    tooltip: {
      trigger: "item",
      formatter: "{b} : {c}",
    },
    series: [
      {
        type: "pie",
        data: Object.entries(queueWeights).map(([name, weight]) => {
          return {
            name: name,
            value: weight,
          };
        }),
      },
    ],
  };
};

const getServerListAction = async (): Promise<void> => {
  await getResponse<InspectorServerListOut>(
    inspectorServerListGet(),
    async (rsp) => {
      servers.value = rsp.data;
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
