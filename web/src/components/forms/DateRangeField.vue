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

// Validation rule shape of the underlying field: `true` when valid,
// otherwise the error message to display.
type Rule = (value: unknown) => boolean | string;

// 范围未选完整（仅点选起始）时保持原值，避免发出不完整范围
const update = (value: unknown): void => {
  if (Array.isArray(value) && value.length === 2) {
    const [start, end] = value as Date[];
    emits("update:modelValue", [DateTime.fromJSDate(start), DateTime.fromJSDate(end)]);
  }
};

const displayStr = computed((): string | null => {
  if (props.modelValue === null) {
    return null;
  }
  const format = (day: DateTime): string => day.toFormat("yyyy-MM-dd");
  return `${format(props.modelValue[0])} ~ ${format(props.modelValue[1])}`;
});

const dateRange = computed((): Date[] | null => {
  if (props.modelValue === null) {
    return null;
  }
  return props.modelValue.map((day) => day.toJSDate());
});

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
</script>

<style scoped>
/* Panel spacing only: border, radius and background come from `.zk-panel`. */
.picker-panel-body {
  padding: var(--zk-space-4);
}
</style>