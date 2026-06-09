<template>
  <v-select
    @update:model-value="emits('update:modelValue', $event)"
    :model-value="modelValue"
    :label="label"
    :items="options"
    :variant="variant"
    :density="density"
    :return-object="returnObject"
    :disabled="disabled"
  />
</template>

<script setup lang="ts" name="usual-days-field">
import i18n from "@/utils/i18n";
import type { Density, FieldVariant } from "../@types/base";

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

const props = defineProps({
  modelValue: {
    type: Number,
    required: true,
  },
  label: {
    type: String,
    default: ""
  },
  variant: {
    type: String as PropType<FieldVariant>,
    default: "filled",
  },
  density: {
    type: String as PropType<Density>,
    default: "default",
  },
  returnObject: {
    type: Boolean,
    default: false,
  },
  disabled: {
    type: Boolean,
    default: false,
  }
});

const emits = defineEmits(["update:modelValue"]);
</script>
