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
            :model-value="dateRange"
            multiple="range"
            show-adjacent-months
            @update:model-value="update"
          />
        </v-card-text>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card>
        <v-card-text>
          <v-date-picker
            :model-value="dateRange"
            multiple="range"
            show-adjacent-months
            @update:model-value="update"
          />
        </v-card-text>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-range-field">
import { DateTime } from "luxon";

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
  menu: {
    type: Boolean,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
