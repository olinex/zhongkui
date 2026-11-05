<template>
  <v-select
    :model-value="modelValue"
    :label="label"
    :items="options"
    :density="density"
    :return-object="returnObject"
    :disabled="disabled"
    :readonly="readonly"
    :rules="rules"
    :error-messages="errorMessages"
    prepend-inner-icon="mdi-timer-sand"
    @update:model-value="emits('update:modelValue', $event)"
  />
</template>

<script setup lang="ts" name="usual-days-field">
import i18n from "@/utils/i18n";

import type { Density } from "../@types/base";

// 底层字段的校验规则形状：合法时返回 `true`，否则返回要展示的错误信息。
type Rule = (value: unknown) => boolean | string;

// options 常用天数下拉项（7/15/30/90/180），单位经 i18n 翻译。
const options = computed(() => {
  const unit = i18n.global.t("unit.day");
  return [
    { title: `7 ${unit}`, value: 7 },
    { title: `15 ${unit}`, value: 15 },
    { title: `30 ${unit}`, value: 30 },
    { title: `90 ${unit}`, value: 90 },
    { title: `180 ${unit}`, value: 180 },
  ];
});

// 与日期/时间选择器族（`DateField` / `TimeField` / `DatetimeField` / `DateRangeField` /
// `DateTimeRangeField`）有意保持的差异：本字段是对固定天数集合（7/15/30/90/180）的
// `v-select`，而选择器族为仅面板选择、默认 `clearable: true` 与 `readonly: true`。
// 请勿将以下差异当作遗漏「修复」：
//   - 无 `clearable` 属性：清空会发出非数值并破坏下游 `days` 查询参数，故取值恒为必填
//     （`modelValue` 为 `Number` 且 `required`）。
//   - `readonly` 默认为 `false`：下拉框本就应可直接展开修改，不同于只能在面板内选值的
//     选择器。
defineProps({
  modelValue: {
    type: Number,
    required: true,
  },
  label: {
    type: String,
    default: "",
  },
  density: {
    type: String as PropType<Density>,
    default: "compact",
  },
  returnObject: {
    type: Boolean,
    default: false,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  readonly: {
    type: Boolean,
    default: false,
  },
  errorMessages: {
    type: String,
    default: "",
  },
  rules: {
    type: Array as PropType<Rule[]>,
    default: () => [],
  },
});

// emits 值变更事件，供父组件以 v-model 绑定。
const emits = defineEmits(["update:modelValue"]);
</script>
