<template>
  <v-text-field
    v-if="inputType === SearchInputType.STRING"
    :model-value="modelValue"
    @update:model-value="update"
  />
  <v-number-input
    v-else-if="inputType === SearchInputType.NUMBER"
    :model-value="modelValue"
    :min="numberProps.min"
    :max="numberProps.max"
    :precision="numberProps.precision"
    control-variant="split"
    @update:model-value="update"
  />
  <v-select
    v-else-if="inputType === SearchInputType.BOOLEAN"
    :model-value="modelValue"
    :items="booleanOptions"
    :item-title="optionTitle"
    item-value="value"
    @update:model-value="update"
  />
  <date-field
    v-else-if="inputType === SearchInputType.DATE"
    :model-value="modelValue"
    @update:model-value="update"
  />
  <time-field
    v-else-if="inputType === SearchInputType.TIME"
    :model-value="modelValue"
    @update:model-value="update"
  />
  <datetime-field
    v-else-if="inputType === SearchInputType.DATETIME"
    :model-value="modelValue"
    @update:model-value="update"
  />
  <v-select
    v-else-if="inputType === SearchInputType.SELECT"
    :model-value="modelValue"
    :items="options"
    :item-title="optionTitle"
    item-value="value"
    return-object
    @update:model-value="update"
  />
</template>

<script setup lang="ts" name="adaptive-field">
import type { PropType } from "vue";
import i18n from "@/utils/i18n";
import { SearchInputType, type Option } from "../@types/forms";
import type { NumberRange } from "@/types";

// 布尔筛选值采用与枚举一致的 v-select 下拉（是/否），保持各数据类型值输入控件外观统一。
// 布尔值与 title 键分离存储，避免将 i18n 键混入布尔数据。
const booleanOptions: Array<{ title: string; value: boolean }> = [
  { title: "model.__base.value.yes", value: true },
  { title: "model.__base.value.no", value: false },
];

// 枚举/布尔选项的 title 为 i18n 键，渲染时翻译为可读展示名。
// vuetify 展示当前选中项时会以原始值（primitive）回调 item-title，
// 需先在可选项中按 value 找回对应 option 再翻译，避免 i18n.t(undefined) 抛 Invalid arguments。
const optionTitle = (option: Option | string | number | boolean | null | undefined): string => {
  if (option === null || option === undefined) {
    return "";
  }
  if (typeof option !== "object") {
    const matched = [...props.options, ...booleanOptions].find(
      (item) => String(item.value) === String(option),
    );
    return matched ? i18n.global.t(String(matched.title ?? "")) : String(option);
  }
  const title = (option as Option).title;
  return title === null || title === undefined || title === "" ? "" : i18n.global.t(String(title));
};

const update = (value: unknown) => {
  // 字符串搜索条件统一去除首尾空白，避免误输入空白导致查询无结果
  emits("update:modelValue", typeof value === "string" ? value.trim() : value);
};

const props = defineProps({
  modelValue: {
    // Passed through to the native field as-is.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: [Object, String, null] as PropType<any>,
    required: true,
  },
  inputType: {
    type: String as PropType<SearchInputType>,
    required: true,
  },
  numberProps: {
    type: Object as PropType<NumberRange>,
    required: true,
  },
  options: {
    type: Array as PropType<Option[]>,
    default: () => [],
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
