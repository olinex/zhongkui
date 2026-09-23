<template>
  <v-text-field
    :model-value="displayStr"
    :label="label"
    :density="density"
    :clearable="clearable"
    :disabled="disabled"
    :readonly="readonly"
    :rules="rules"
    :error-messages="errorMessages"
    min-width="245"
    prepend-inner-icon="mdi-clock-outline"
    @click:clear="emits('update:modelValue', null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-date-picker
            :model-value="pickedDates"
            multiple="range"
            show-adjacent-months
            @update:model-value="updateDates"
          />
          <div class="picker-panel-times">
            <v-text-field
              v-model="startTimeStr"
              :label="$t('model.__base.field.startTime')"
              type="time"
              hide-details
            />
            <v-text-field
              v-model="endTimeStr"
              :label="$t('model.__base.field.endTime')"
              type="time"
              hide-details
            />
          </div>
        </div>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-date-picker
            :model-value="pickedDates"
            multiple="range"
            show-adjacent-months
            @update:model-value="updateDates"
          />
          <div class="picker-panel-times">
            <v-text-field
              v-model="startTimeStr"
              :label="$t('model.__base.field.startTime')"
              type="time"
              hide-details
            />
            <v-text-field
              v-model="endTimeStr"
              :label="$t('model.__base.field.endTime')"
              type="time"
              hide-details
            />
          </div>
        </div>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-time-range-field">
import { DateTime } from "luxon";

import type { Density } from "../@types/base";

// Validation rule shape of the underlying field: `true` when valid,
// otherwise the error message to display.
type Rule = (value: unknown) => boolean | string;

// props/emits 须在 immediate watch 之前定义（回调同步访问 props）
const props = defineProps({
  modelValue: {
    type: [Array, null] as PropType<[DateTime, DateTime] | null>,
    required: true,
  },
  label: {
    type: String,
    default: "",
  },
  // Pickers are panel-only inputs, so they stay readonly unless opted out.
  clearable: {
    type: Boolean,
    default: true,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  readonly: {
    type: Boolean,
    default: true,
  },
  density: {
    type: String as PropType<Density>,
    default: "compact",
  },
  errorMessages: {
    type: String,
    default: "",
  },
  rules: {
    type: Array as PropType<Rule[]>,
    default: () => [],
  },
  // Render the panel as an inline menu (true) or a modal dialog (default).
  menu: {
    type: Boolean,
  },
});

const emits = defineEmits(["update:modelValue"]);

// 弹出层内部状态：日期选择器的已选日期与起始/截止时间字符串（"HH:mm"）
const pickedDates = ref<Date[]>([]);
const startTimeStr = ref<string | null>(null);
const endTimeStr = ref<string | null>(null);

// 由外部 modelValue 同步内部状态时置位，避免回声触发 emitRange 造成更新循环
let syncing = false;

// 组合当前日期范围与时间字符串，精确到分钟：
// 起始取分钟开始（:00.000），截止取分钟末尾（:59.999），保证整分钟数据不被漏查
const emitRange = (): void => {
  if (pickedDates.value.length !== 2) {
    return;
  }
  const parseTime = (raw: string | null, fallback: [number, number]): [number, number] => {
    if (raw === null) {
      return fallback;
    }
    const [hour, minute] = raw.split(":").map(Number);
    return [hour, minute];
  };
  const [startHour, startMinute] = parseTime(startTimeStr.value, [0, 0]);
  const [endHour, endMinute] = parseTime(endTimeStr.value, [23, 59]);
  const [startDay, endDay] = pickedDates.value.map((day) => DateTime.fromJSDate(day));
  const start = startDay.set({ hour: startHour, minute: startMinute, second: 0, millisecond: 0 });
  const end = endDay.set({ hour: endHour, minute: endMinute, second: 59, millisecond: 999 });
  // 值相同（仅引用更换）时不重复 emit，切断「组件 → vee-validate → 组件」回声环。
  // 注意：syncing 仅在该同步的 pre-flush 之内生效，而 pickedDates watcher 会批量派发到其后执行，
  // 此时 syncing 已复位；若仅依赖它防回显不可靠。故按值比较短路：构造出的区间与当前
  // props.modelValue 逐值相等时不 emit，保证外部回显不会再次触发 update:modelValue。
  const current = props.modelValue;
  if (
    current &&
    current.length === 2 &&
    DateTime.isDateTime(current[0]) &&
    DateTime.isDateTime(current[1]) &&
    current[0].equals(start) &&
    current[1].equals(end)
  ) {
    return;
  }
  emits("update:modelValue", [start, end]);
};

const updateDates = (value: unknown): void => {
  if (Array.isArray(value)) {
    pickedDates.value = value as Date[];
  }
};

// 外部值变化（初始化/清除/回显）时同步内部状态；范围选完整（2 个日期）或时间变化时重新发出
watch(
  () => props.modelValue,
  (range) => {
    syncing = true;
    if (range === null) {
      pickedDates.value = [];
      startTimeStr.value = null;
      endTimeStr.value = null;
    } else {
      pickedDates.value = range.map((day) => day.toJSDate());
      startTimeStr.value = range[0].toFormat("HH:mm");
      endTimeStr.value = range[1].toFormat("HH:mm");
    }
    syncing = false;
  },
  { immediate: true },
);

watch(pickedDates, (dates) => {
  if (syncing || dates.length !== 2) {
    return;
  }
  emitRange();
});

watch(startTimeStr, () => {
  if (!syncing && pickedDates.value.length === 2) {
    emitRange();
  }
});

watch(endTimeStr, () => {
  if (!syncing && pickedDates.value.length === 2) {
    emitRange();
  }
});

const displayStr = computed((): string | null => {
  if (props.modelValue === null) {
    return null;
  }
  const format = (day: DateTime): string => day.toFormat("yyyy-MM-dd HH:mm");
  return `${format(props.modelValue[0])} ~ ${format(props.modelValue[1])}`;
});
</script>

<style scoped>
/* Panel spacing only: border, radius and background come from `.zk-panel`. */
.picker-panel-body {
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
  padding: var(--zk-space-4);
}

/* Start/end time inputs sit side by side under the range calendar. */
.picker-panel-times {
  display: flex;
  gap: var(--zk-space-2);
}
</style>