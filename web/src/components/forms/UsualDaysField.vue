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

// Validation rule shape of the underlying field: `true` when valid,
// otherwise the error message to display.
type Rule = (value: unknown) => boolean | string;

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

// Deliberate variant differences from the date/time picker family
// (`DateField` / `TimeField` / `DatetimeField` / `DateRangeField` /
// `DateTimeRangeField`): this field is a `v-select` over a fixed set of day
// counts (7/15/30/90/180), which are panel-only pickers defaulting to
// `clearable: true` and `readonly: true`. Do not "fix" the following as
// omissions:
//   - no `clearable` prop: clearing would emit a non-numeric value and break
//     the downstream `days` query parameter, so the value stays required
//     (`modelValue` is `Number` and `required`).
//   - `readonly` defaults to `false`: a select is meant to be opened and
//     changed directly, unlike a picker whose value is only chosen inside a
//     panel.
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

const emits = defineEmits(["update:modelValue"]);
</script>
