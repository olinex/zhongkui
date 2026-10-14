<template>
  <main-layout current-nav="admin">
    <template #body>
      <!-- 服务/采集器两块内容经 tab 拆分：消除多卡片垂直排列时的背景间隙，RefreshControl 置于 tab 栏下方右侧共享 -->
      <tab-view
        v-model="tab"
        :items="[
          { key: 'service', label: $t('model.service.name') },
          { key: 'capturer', label: $t('model.capturer.name') },
        ]"
      >
        <template #actions>
          <RefreshControl
            v-model:interval="refreshInterval"
            :loading="refreshing"
            @refresh="refreshAll(true)"
            @auto-refresh="refreshAll()"
          />
        </template>

        <template #service>
          <v-data-table :items="serviceItems" :headers="serviceHeaders" item-value="instanceID">
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
              <BooleanChip :value="item.live" />
            </template>
            <template #[`item.ready`]="{ item }">
              <BooleanChip :value="item.ready" />
            </template>
            <template #[`item.started`]="{ item }">
              <BooleanChip :value="item.started" />
            </template>
          </v-data-table>
        </template>

        <template #capturer>
          <v-data-table :items="capturerItems" :headers="capturerHeaders" item-value="namespace">
            <template #[`item.status`]="{ item }">
              {{ $t(`model.capturer.option.status.${item.status}`) }}
            </template>
            <template #[`item.lastSeen`]="{ item }">
              <span v-if="isValidDateTime(item.lastSeen)">{{ formatLastSeen(item.lastSeen) }}</span>
              <span v-else>{{ item.lastSeen }}</span>
            </template>
          </v-data-table>
        </template>
      </tab-view>
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
// 当前激活的 tab：service（服务状态）或 capturer（采集器状态）
const tab = ref("service");
const serviceItems = ref<ServiceItemOut[]>([]);
const capturerItems = ref<CapturerStatusItemOut[]>([]);

// nowTick 每次刷新时更新，驱动运行时长列重新计算。
const nowTick = ref(0);
const refreshInterval = ref(30000);
// refreshing 仅用于手动刷新按钮的 loading 态，定时刷新不触发。
const refreshing = ref(false);

// 服务状态列标题跟随语言变化，故用 computed 声明。
const serviceHeaders = computed(() => [
  { key: "serviceName", title: i18n.global.t("model.service.field.serviceName"), sortable: true },
  { key: "instanceID", title: i18n.global.t("model.service.field.instanceID"), sortable: true },
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
  { key: "sessionID", title: i18n.global.t("model.capturer.field.sessionID"), sortable: true },
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

// refreshAll 立即刷新：重新拉取服务/采集器列表并更新运行时长基准。
// showLoading 为 true 时点亮刷新按钮 loading（手动触发），定时触发不显示。
const refreshAll = async (showLoading = false): Promise<void> => {
  if (showLoading) {
    refreshing.value = true;
  }
  nowTick.value = Date.now();
  try {
    await Promise.all([getServiceListAction(), getCapturerStatusListAction()]);
  } finally {
    refreshing.value = false;
  }
};

// 定时刷新由 RefreshControl 组件自持（环形倒计时），此处仅在挂载时做一次初始加载；
// 后续自动刷新经 auto-refresh 事件回调 refreshAll()。
onMounted(async () => {
  await refreshAll();
});
</script>
