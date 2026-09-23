<template>
  <!-- v-btn-group 分体式按钮组：固定边界，切换自动刷新开关/loading 时组形态保持不变 -->
  <v-btn-group density="compact" class="zk-refresh-group" divided>
    <!-- 第一段：定时刷新间隔选择（下拉） -->
    <v-menu>
      <template #activator="{ props }">
        <v-btn
          v-bind="props"
          width="56"
          height="32"
          variant="outlined"
          :title="$t('action.refreshInterval')"
          :aria-label="$t('action.refreshInterval')"
        >
          <span class="text-caption font-weight-medium">{{ intervalLabel }}</span>
          <v-icon size="small" class="ml-1">mdi-chevron-down</v-icon>
        </v-btn>
      </template>
      <v-list class="pa-0">
        <v-list-item
          v-for="opt in intervalOptions"
          :key="opt.value"
          :active="interval === opt.value"
          @click="selectInterval(opt.value)"
        >
          <v-list-item-title class="text-caption">{{ opt.title }}</v-list-item-title>
        </v-list-item>
      </v-list>
    </v-menu>

    <!-- 第二段：自动刷新开关（启用时环形倒计时包围暂停图标） -->
    <v-btn
      width="44"
      height="32"
      variant="outlined"
      :title="enabled ? $t('action.disableAutoRefresh') : $t('action.enableAutoRefresh')"
      :aria-label="enabled ? $t('action.disableAutoRefresh') : $t('action.enableAutoRefresh')"
      @click="enabled = !enabled"
    >
      <v-progress-circular
        v-if="enabled"
        :model-value="remainingPct"
        :size="22"
        :width="3"
        color="primary"
      >
        <v-icon size="small" class="text-on-surface">mdi-pause</v-icon>
      </v-progress-circular>
      <v-icon v-else size="small" class="text-on-surface">mdi-play</v-icon>
    </v-btn>

    <!-- 第三段：手动刷新 -->
    <v-btn
      height="32"
      width="44"
      :loading="loading"
      :disabled="loading"
      icon="mdi-refresh"
      variant="outlined"
      :title="$t('action.refresh')"
      :aria-label="$t('action.refresh')"
      @click="onManualRefresh"
    />
  </v-btn-group>
</template>

<script setup lang="ts" name="refresh-control">
// RefreshControl 自动刷新控制：
// - 按钮组第一段选择定时刷新间隔（默认展示当前间隔文本），v-model:interval 双向绑定；
// - 第二段为自动刷新开关：启用状态下环形进度（v-progress-circular）实时倒计时距下次刷新的
//   剩余时间，归零即触发 auto-refresh 事件并重新起算；点击环形可停止自动刷新；
// - 第三段手动刷新触发 refresh 事件（手动刷新后倒计时从完整间隔重新起算）。
// 倒计时计时器由本组件自持，调用方仅消费 refresh / auto-refresh 事件与 loading 状态。
// 采用 v-btn-group 分体按钮组保证切换开关时组边界固定，避免视图形态伸缩（jank）。

// 倒计时刷新 tick 间隔（毫秒）：驱动环形进度条平滑递减
const TickMs = 100;

defineProps<{
  /** 手动刷新 loading 态 */
  loading?: boolean;
}>();

/** 当前刷新间隔（毫秒），支持 v-model:interval */
const interval = defineModel<number>("interval", { required: true });

const emit = defineEmits<{ refresh: []; "auto-refresh": [] }>();

// 定时刷新间隔选项（5s / 10s / 15s / 30s / 1m）
const intervalOptions = [
  { title: "5s", value: 5000 },
  { title: "10s", value: 10000 },
  { title: "15s", value: 15000 },
  { title: "30s", value: 30000 },
  { title: "1m", value: 60000 },
];

// 当前间隔对应的展示文本（如 "5s"），未命中时回退为数值本身。
const intervalLabel = computed<string>(() => {
  return (
    intervalOptions.find((opt) => opt.value === interval.value)?.title ?? String(interval.value)
  );
});

// 自动刷新开关：默认开启，点击环形中心播放/暂停按钮切换
const enabled = ref(true);
// 距下次自动刷新的剩余毫秒数
const remaining = ref(interval.value);
let tickTimer: number | undefined;

// 剩余时间占间隔的百分比，驱动环形进度条（0-100）
const remainingPct = computed<number>(() => {
  if (interval.value <= 0) {
    return 0;
  }
  return Math.round((remaining.value / interval.value) * 100);
});

// resetCountdown 将倒计时重置为完整间隔。
const resetCountdown = (): void => {
  remaining.value = interval.value;
};

// startCountdown 启动倒计时：每个 tick 递减剩余时间，归零触发 auto-refresh 并重新起算。
const startCountdown = (): void => {
  stopCountdown();
  resetCountdown();
  tickTimer = window.setInterval(() => {
    remaining.value = Math.max(0, remaining.value - TickMs);
    if (remaining.value <= 0) {
      emit("auto-refresh");
      resetCountdown();
    }
  }, TickMs);
};

// stopCountdown 停止倒计时计时器。
const stopCountdown = (): void => {
  if (tickTimer !== undefined) {
    window.clearInterval(tickTimer);
    tickTimer = undefined;
  }
};

// onManualRefresh 手动刷新：触发 refresh 事件；自动刷新开启时倒计时重新起算。
const onManualRefresh = (): void => {
  emit("refresh");
  if (enabled.value) {
    resetCountdown();
  }
};

// selectInterval 选择刷新间隔并更新模型（菜单点选后自动闭合，无需手动关闭）。
const selectInterval = (value: number): void => {
  interval.value = value;
};

// 开关切换时启停倒计时。
watch(enabled, (value) => {
  if (value) {
    startCountdown();
  } else {
    stopCountdown();
  }
});

// 间隔变化时按新间隔重置倒计时。
watch(interval, () => {
  if (enabled.value) {
    resetCountdown();
  }
});

onMounted(() => {
  if (enabled.value) {
    startCountdown();
  }
});

onBeforeUnmount(() => {
  stopCountdown();
});
</script>
