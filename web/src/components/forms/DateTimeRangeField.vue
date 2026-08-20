<template>
  <v-text-field
    :model-value="displayStr"
    :label="label"
    min-width="245"
    prepend-inner-icon="mdi-calendar-range"
    clearable
    readonly
    @click:clear="emits('update:modelValue', null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card>
        <v-card-text>
          <v-date-picker
            :model-value="pickedDates"
            multiple="range"
            show-adjacent-months
            @update:model-value="updateDates"
          />
          <div class="d-flex ga-2 mt-2">
            <v-text-field
              v-model="startTimeStr"
              :label="$t('model.__base.field.startTime')"
              type="time"
              density="compact"
              hide-details
            />
            <v-text-field
              v-model="endTimeStr"
              :label="$t('model.__base.field.endTime')"
              type="time"
              density="compact"
              hide-details
            />
          </div>
        </v-card-text>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card>
        <v-card-text>
          <v-date-picker
            :model-value="pickedDates"
            multiple="range"
            show-adjacent-months
            @update:model-value="updateDates"
          />
          <div class="d-flex ga-2 mt-2">
            <v-text-field
              v-model="startTimeStr"
              :label="$t('model.__base.field.startTime')"
              type="time"
              density="compact"
              hide-details
            />
            <v-text-field
              v-model="endTimeStr"
              :label="$t('model.__base.field.endTime')"
              type="time"
              density="compact"
              hide-details
            />
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-time-range-field">
import { DateTime } from "luxon";

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
  emits("update:modelValue", [
    startDay.set({ hour: startHour, minute: startMinute, second: 0, millisecond: 0 }),
    endDay.set({ hour: endHour, minute: endMinute, second: 59, millisecond: 999 }),
  ]);
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
