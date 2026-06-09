<template>
  <main-layout current-nav="etl">
    <template #body>
      <v-empty-state
        v-show="servers.length === 0"
        icon="mdi-magnify"
        :headline="$t('summary.noServerYet', { name: $t('model.server.name') })"
        :text="$t('summary.noServerYetHelp', { btn: $t('action.reset') })"
        :action-text="$t('action.reset')"
        @click:action="getServerListAction"
      />
      <v-card
        v-for="server in servers"
        :key="server.id"
        dense
      >
        <template #title>
          <span class="font-weight-black">{{ server.host }}</span>
        </template>
        <template #subtitle>
          <v-chip-group>
            <hex-string-chip
              size="x-small"
              :value="server.id"
              prefix="id: "
              :length="32"
            />
            <v-chip size="x-small"> pid: {{ server.pid }} </v-chip>
            <v-chip size="x-small">
              {{ $t("model.server.field.status") }}: {{ server.status }}
            </v-chip>
            <v-chip size="x-small">
              {{ $t("model.server.field.started") }}:
              {{ localeDatetimeFormat(DateTime.fromISO(server.started)) }}
            </v-chip>
          </v-chip-group>
        </template>
        <template #actions>
          <v-btn
            @click="server.show = !server.show"
            :color="server.show ? 'text' : 'success'"
            :text="server.show ? $t('action.close') : $t('action.expand')"
            :append-icon="server.show ? 'mdi-chevron-up' : 'mdi-chevron-down'"
            block
          />
        </template>
        <template #text>
          <v-expand-transition>
            <div v-show="server.show">
              <v-divider />
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
                  <v-data-table
                    :headers="taskHeaders"
                    :items="server.processingTasks"
                  >
                  </v-data-table>
                </v-col>
              </v-row>
            </div>
          </v-expand-transition>
        </template>
      </v-card>
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
import type { InspectorServerListOut } from "./@types";
import { inspectorServerListGet } from "@/apis/etl";
import { useAppStore } from "@/stores/app";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { DateTime } from "luxon";
import type { PiesChartOption } from "@/components/@types/charts";
import type { TableHeader } from "@/components/@types/table";
import { BackendDataType } from "@/types";

const appStore = useAppStore();
const servers = ref([] as InspectorServerListOut);
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
    }
  );
};
</script>
