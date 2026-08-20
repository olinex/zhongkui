<template>
  <main-layout current-nav="etl">
    <template #body>
      <v-container>
        <div class="d-flex justify-end my-2">
          <action-button-group :refresh="getSchedulerListAction" collapsible>
            <column-visibility-menu
              v-model="schedulerVisibleKeys"
              v-model:order="schedulerColumnOrder"
              :headers="schedulerAllToggleable"
            />
          </action-button-group>
        </div>
        <v-row dense>
          <v-col cols="12">
            <v-data-table :headers="schedulerVisibleHeaders" :items="schedulerItems">
              <template #item="{ item }">
                <tr class="text-no-wrap">
                  <td
                    v-for="hdr in schedulerVisibleHeaders"
                    :key="hdr.key"
                    :class="
                      hdr.fixed === 'end'
                        ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                        : undefined
                    "
                  >
                    <adaptive-cell :header="hdr" :item="item" />
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
name: ETL Scheduler List
meta:
  title: title.schedulerEtl
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { getResponse } from "@/apis/base";
import type { InspectorSchedulerListItem, InspectorSchedulerListOut } from "./@types";
import { inspectorSchedulerListGet } from "@/apis/etl";
import { useAppStore } from "@/stores/app";
import type { TableHeader } from "@/components/@types/table";
import i18n from "@/utils/i18n";
import { useColumnVisibility } from "@/utils/column";
import { BackendDataType } from "@/types";

const appStore = useAppStore();
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
// 定时任务表格的列显示配置：持久化到 localStorage，表头/行渲染统一使用 visibleHeaders
const {
  allToggleable: schedulerAllToggleable,
  columnOrder: schedulerColumnOrder,
  visibleKeys: schedulerVisibleKeys,
  visibleHeaders: schedulerVisibleHeaders,
} = useColumnVisibility("model:scheduler", schedulerHeaders);
const schedulerItems = ref([] as InspectorSchedulerListItem[]);

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

onMounted(getSchedulerListAction);
</script>
