<template>
  <main-layout current-nav="admin">
    <template #body>
      <v-card class="mb-4">
        <v-card-title class="text-h6 text-on-surface">
          {{ $t("model.service.name") }}
        </v-card-title>
        <v-data-table :items="serviceItems" :headers="serviceHeaders" item-value="instanceId">
          <template #[`item.startedAt`]="{ item }">
            <span v-if="isValidDateTime(item.startedAt)">{{
              formatStartedAt(item.startedAt)
            }}</span>
            <span v-else>{{ item.startedAt }}</span>
          </template>
          <template #[`item.running`]="{ item }">
            {{ formatRunning(item.startedAt) }}
          </template>
          <template #[`item.live`]="{ item }">
            <BooleanChip
              :value="item.live"
              :label="$t(item.live ? 'model.__base.value.yes' : 'model.__base.value.no')"
            />
          </template>
          <template #[`item.ready`]="{ item }">
            <BooleanChip
              :value="item.ready"
              :label="$t(item.ready ? 'model.__base.value.yes' : 'model.__base.value.no')"
            />
          </template>
          <template #[`item.started`]="{ item }">
            <BooleanChip
              :value="item.started"
              :label="$t(item.started ? 'model.__base.value.yes' : 'model.__base.value.no')"
            />
          </template>
        </v-data-table>
      </v-card>

      <v-card>
        <v-card-title class="text-h6 text-on-surface">
          {{ $t("model.capturer.name") }}
        </v-card-title>
        <v-data-table :items="capturerItems" :headers="capturerHeaders" item-value="namespace">
          <template #[`item.status`]="{ item }">
            {{ $t(`model.capturer.option.status.${item.status}`) }}
          </template>
          <template #[`item.lastSeen`]="{ item }">
            <span v-if="isValidDateTime(item.lastSeen)">{{ formatLastSeen(item.lastSeen) }}</span>
            <span v-else>{{ item.lastSeen }}</span>
          </template>
        </v-data-table>
      </v-card>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: Admin System Status
meta:
  title: title.systemStatus
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts">
import { getResponse } from "@/apis/base";
import { capturerStatusListGet } from "@/apis/admin";
import { serviceListGet } from "@/apis/service";
import type { CapturerStatusItemOut, CapturerStatusListOut } from "@/apis/admin";
import type { ServiceItemOut, ServiceListOut } from "@/apis/service";
import { useAppStore } from "@/stores/app";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";
import { DateTime } from "luxon";

const appStore = useAppStore();
const serviceItems = ref<ServiceItemOut[]>([]);
const capturerItems = ref<CapturerStatusItemOut[]>([]);

// nowTick 每 30 秒更新一次，驱动运行时长列刷新。
const nowTick = ref(0);
let timer: number | undefined;

// 服务状态列标题跟随语言变化，故用 computed 声明。
const serviceHeaders = computed(() => [
  { key: "serviceName", title: i18n.global.t("model.service.field.serviceName"), sortable: true },
  { key: "instanceId", title: i18n.global.t("model.service.field.instanceId"), sortable: true },
  { key: "host", title: i18n.global.t("model.service.field.host"), sortable: true },
  { key: "version", title: i18n.global.t("model.service.field.version"), sortable: true },
  { key: "pid", title: i18n.global.t("model.service.field.pid"), sortable: true },
  { key: "startedAt", title: i18n.global.t("model.service.field.startedAt"), sortable: true },
  { key: "running", title: i18n.global.t("model.service.field.running"), sortable: false },
  { key: "live", title: i18n.global.t("model.service.field.live"), sortable: true },
  { key: "ready", title: i18n.global.t("model.service.field.ready"), sortable: true },
  { key: "started", title: i18n.global.t("model.service.field.started"), sortable: true },
]);

// 采集器状态列标题跟随语言变化，故用 computed 声明。
const capturerHeaders = computed(() => [
  { key: "namespace", title: i18n.global.t("model.capturer.field.namespace"), sortable: true },
  { key: "name", title: i18n.global.t("model.capturer.field.name"), sortable: true },
  { key: "version", title: i18n.global.t("model.capturer.field.version"), sortable: true },
  { key: "sessionId", title: i18n.global.t("model.capturer.field.sessionId"), sortable: true },
  { key: "status", title: i18n.global.t("model.capturer.field.status"), sortable: true },
  {
    key: "configVersion",
    title: i18n.global.t("model.capturer.field.configVersion"),
    sortable: true,
  },
  {
    key: "globalVersion",
    title: i18n.global.t("model.capturer.field.globalVersion"),
    sortable: true,
  },
  { key: "lastSeen", title: i18n.global.t("model.capturer.field.lastSeen"), sortable: true },
]);

const isValidDateTime = (value: string): boolean => DateTime.fromISO(value).isValid;

const formatStartedAt = (startedAt: string): string => {
  return localeDatetimeFormat(DateTime.fromISO(startedAt));
};

const formatLastSeen = (lastSeen: string): string => {
  return localeDatetimeFormat(DateTime.fromISO(lastSeen));
};

// formatRunning 根据 startedAt 计算运行时长（如 "1h 5m 30s"）；读取 nowTick 以保证周期性刷新。
const formatRunning = (startedAt: string): string => {
  void nowTick.value;
  const started = DateTime.fromISO(startedAt);
  if (!started.isValid) {
    return startedAt;
  }
  const totalSecs = Math.max(0, Math.floor(DateTime.now().diff(started, "seconds").seconds));
  const days = Math.floor(totalSecs / 86400);
  const hours = Math.floor((totalSecs % 86400) / 3600);
  const minutes = Math.floor((totalSecs % 3600) / 60);
  const seconds = totalSecs % 60;
  const parts: string[] = [];
  if (days > 0) {
    parts.push(`${days}d`);
  }
  if (hours > 0 || days > 0) {
    parts.push(`${hours}h`);
  }
  if (minutes > 0 || hours > 0 || days > 0) {
    parts.push(`${minutes}m`);
  }
  parts.push(`${seconds}s`);
  return parts.join(" ");
};

const getServiceListAction = async (): Promise<void> => {
  await getResponse<ServiceListOut>(
    serviceListGet(),
    async (rsp) => {
      serviceItems.value = rsp.data.items;
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

const getCapturerStatusListAction = async (): Promise<void> => {
  await getResponse<CapturerStatusListOut>(
    capturerStatusListGet(),
    async (rsp) => {
      capturerItems.value = rsp.data.items;
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

// 每 30 秒刷新：nowTick 驱动运行时长列，同时重新拉取列表反映服务/采集器上线下线。
onMounted(async () => {
  await Promise.all([getServiceListAction(), getCapturerStatusListAction()]);
  timer = window.setInterval(() => {
    nowTick.value = Date.now();
    void getServiceListAction();
    void getCapturerStatusListAction();
  }, 30000);
});

onBeforeUnmount(() => {
  if (timer !== undefined) {
    window.clearInterval(timer);
  }
});
</script>
