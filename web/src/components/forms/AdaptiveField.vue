<template>
  <v-text-field
    v-if="inputType === SearchInputType.STRING"
    :model-value="modelValue"
    @update:model-value="update"
    :density="density"
  />
  <v-number-input
    v-else-if="inputType === SearchInputType.NUMBER"
    :model-value="modelValue"
    @update:model-value="update"
    :min="numberProps.min"
    :max="numberProps.max"
    :precision="numberProps.precision"
    :density="density"
    control-variant="stacked"
  />
  <v-switch
    v-else-if="inputType === SearchInputType.BOOLEAN"
    :model-value="modelValue"
    :density="density"
    @update:model-value="update"
    color="primary"
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
</template>

<script setup lang="ts" name="adaptive-field">
import type { PropType } from "vue";
import { SearchInputType } from "../@types/forms";
import { type Density } from "../@types/base";
import type { NumberRange } from "@/types";

const update = (value: any) => {
  emits("update:modelValue", value);
};

const props = defineProps({
  modelValue: {
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
  density: {
    type: Object as PropType<Density>,
    default: null,
  }
});

const emits = defineEmits(["update:modelValue"]);
</script>
