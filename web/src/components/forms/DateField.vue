<template>
  <v-text-field
    :model-value="displayValue"
    :label="label"
    :density="density"
    :clearable="clearable"
    :disabled="disabled"
    :readonly="readonly"
    :rules="rules"
    :error-messages="errorMessages"
    min-width="180"
    prepend-inner-icon="mdi-calendar"
    @click:clear="update(null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-date-picker
            :model-value="modelValue"
            show-adjacent-months
            @update:model-value="update"
          />
        </div>
        <div class="picker-panel-footer">
          <v-btn variant="text" @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn variant="text" @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn variant="text" @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
        </div>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-date-picker
            :model-value="modelValue"
            show-adjacent-months
            @update:model-value="update"
          />
        </div>
        <div class="picker-panel-footer">
          <v-btn variant="text" @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn variant="text" @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn variant="text" @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
        </div>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-field">
import { DateTime } from "luxon";

import type { Density } from "../@types/base";
import { localeDateFormat } from "@/utils/i18n";

// 底层字段的校验规则形态：通过时返回 true，否则返回要展示的错误消息。
type Rule = (value: unknown) => boolean | string;

// 将选择结果透传给父组件。
const update = (value: unknown) => {
  emits("update:modelValue", value);
};

// 将日期回退一天。
const previousDate = () => {
  update(DateTime.now().minus({ days: 1 }).toJSDate());
};

// 将日期设为当前日期。
const currentDate = () => {
  update(DateTime.now().toJSDate());
};

// 将日期前进一天。
const nextDate = () => {
  update(DateTime.now().plus({ days: 1 }).toJSDate());
};

const props = defineProps({
  modelValue: {
    // 原样透传给日期选择器。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: [Object, String, null] as PropType<any>,
    required: true,
  },
  // 输入框标签。
  label: {
    type: String,
    default: "",
  },
  // 选择器为纯面板输入，除显式关闭外保持只读。
  clearable: {
    type: Boolean,
    default: true,
  },
  // 是否禁用输入。
  disabled: {
    type: Boolean,
    default: false,
  },
  // 是否只读。
  readonly: {
    type: Boolean,
    default: true,
  },
  // 输入框密度。
  density: {
    type: String as PropType<Density>,
    default: "compact",
  },
  // 错误消息文本。
  errorMessages: {
    type: String,
    default: "",
  },
  // 校验规则列表。
  rules: {
    type: Array as PropType<Rule[]>,
    default: () => [],
  },
  // 面板以内联菜单（true）或模态对话框（默认）渲染。
  menu: {
    type: Boolean,
  },
});

// 值变更事件。
const emits = defineEmits(["update:modelValue"]);

// displayValue 日期展示口径：ISO 日期字符串 / Date / DateTime 按当前语言本地化（与列表 DATE 列、
// 筛选条件一致），已本地化的文本原样展示，避免同一应用内出现 YYYY-MM-DD 与 YYYY/MM/DD 两种写法。
const displayValue = computed((): string => {
  const value = props.modelValue;
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "string") {
    const parsed = DateTime.fromISO(value);
    return parsed.isValid ? localeDateFormat(parsed) : value;
  }
  if (value instanceof Date) {
    return localeDateFormat(DateTime.fromJSDate(value));
  }
  if (DateTime.isDateTime(value)) {
    return localeDateFormat(value);
  }
  return String(value);
});
</script>

<style scoped>
/* 仅面板间距：边框、圆角与背景来自 .zk-panel。 */
.picker-panel-body {
  padding: var(--zk-space-4) var(--zk-space-4) var(--zk-space-2);
}

/* 底部操作在整个选择器家族中共用同一右对齐布局。 */
.picker-panel-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--zk-space-2);
  padding: var(--zk-space-2) var(--zk-space-4) var(--zk-space-4);
}
</style>
