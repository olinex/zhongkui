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
    prepend-inner-icon="mdi-calendar-range"
    @click:clear="emits('update:modelValue', null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-date-picker
            :model-value="dateRange"
            multiple="range"
            show-adjacent-months
            @update:model-value="update"
          />
        </div>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-date-picker
            :model-value="dateRange"
            multiple="range"
            show-adjacent-months
            @update:model-value="update"
          />
        </div>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-range-field">
import { DateTime } from "luxon";

import type { Density } from "../@types/base";

// 底层字段的校验规则形态：通过时返回 `true`，否则返回待展示的错误消息。
type Rule = (value: unknown) => boolean | string;

// 范围未选完整（仅点选起始）时保持原值，避免发出不完整范围
const update = (value: unknown): void => {
  if (Array.isArray(value) && value.length === 2) {
    const [start, end] = value as Date[];
    emits("update:modelValue", [DateTime.fromJSDate(start), DateTime.fromJSDate(end)]);
  }
};

// displayStr 将选定范围格式化为「起始 ~ 结束」的展示文本。
const displayStr = computed((): string | null => {
  if (props.modelValue === null) {
    return null;
  }
  const format = (day: DateTime): string => day.toFormat("yyyy-MM-dd");
  return `${format(props.modelValue[0])} ~ ${format(props.modelValue[1])}`;
});

// dateRange 将 DateTime 范围转换为日期选择器所需的 JS Date 数组。
const dateRange = computed((): Date[] | null => {
  if (props.modelValue === null) {
    return null;
  }
  return props.modelValue.map((day) => day.toJSDate());
});

const props = defineProps({
  // 已选定的日期范围（起始、结束），未选择时为 null
  modelValue: {
    type: [Array, null] as PropType<[DateTime, DateTime] | null>,
    required: true,
  },
  // 输入框标签
  label: {
    type: String,
    default: "",
  },
  // 选择器为面板式输入，除显式放开外均保持只读
  clearable: {
    type: Boolean,
    default: true,
  },
  // 是否禁用
  disabled: {
    type: Boolean,
    default: false,
  },
  // 是否只读
  readonly: {
    type: Boolean,
    default: true,
  },
  // 控件密度
  density: {
    type: String as PropType<Density>,
    default: "compact",
  },
  // 外部传入的错误消息
  errorMessages: {
    type: String,
    default: "",
  },
  // 校验规则列表
  rules: {
    type: Array as PropType<Rule[]>,
    default: () => [],
  },
  // 面板以内联菜单（true）或模态对话框（默认）方式渲染
  menu: {
    type: Boolean,
  },
});

// emits 向上同步选定的日期范围。
const emits = defineEmits(["update:modelValue"]);
</script>

<style scoped>
/* 仅设置面板间距：边框、圆角与背景取自 `.zk-panel`。 */
.picker-panel-body {
  padding: var(--zk-space-4);
}
</style>
