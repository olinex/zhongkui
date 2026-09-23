<template>
  <v-text-field
    :model-value="modelValue"
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
          <v-btn @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
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
          <v-btn @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
        </div>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-field">
import { DateTime } from "luxon";

import type { Density } from "../@types/base";

// Validation rule shape of the underlying field: `true` when valid,
// otherwise the error message to display.
type Rule = (value: unknown) => boolean | string;

const update = (value: unknown) => {
  emits("update:modelValue", value);
};

const previousDate = () => {
  update(DateTime.now().minus({ days: 1 }).toJSDate());
};

const currentDate = () => {
  update(DateTime.now().toJSDate());
};

const nextDate = () => {
  update(DateTime.now().plus({ days: 1 }).toJSDate());
};

defineProps({
  modelValue: {
    // Passed through to the date picker as-is.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: [Object, String, null] as PropType<any>,
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
  padding: var(--zk-space-4) var(--zk-space-4) var(--zk-space-2);
}

/* Footer actions share one right-aligned layout across the picker family. */
.picker-panel-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--zk-space-2);
  padding: var(--zk-space-2) var(--zk-space-4) var(--zk-space-4);
}
</style>
