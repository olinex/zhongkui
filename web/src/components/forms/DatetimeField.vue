<template>
  <v-text-field
    :model-value="datetimeStr"
    :label="label"
    :density="density"
    :clearable="clearable"
    :disabled="disabled"
    :readonly="readonly"
    :rules="rules"
    :error-messages="errorMessages"
    min-width="245"
    prepend-inner-icon="mdi-calendar-clock"
    @click:clear="emits('update:modelValue', null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-tabs v-model="tab">
            <v-tab value="date">{{ $t("model.__base.field.date") }}</v-tab>
            <v-tab value="time">{{ $t("model.__base.field.time") }}</v-tab>
          </v-tabs>
          <v-divider />
          <v-tabs-window v-model="tab">
            <v-tabs-window-item value="date">
              <v-date-picker
                :model-value="dateRaw"
                show-adjacent-months
                @update:model-value="updateDate"
              />
            </v-tabs-window-item>
            <v-tabs-window-item value="time">
              <v-time-picker
                :model-value="timeStr"
                format="24hr"
                use-seconds
                @update:model-value="updateTime"
              />
            </v-tabs-window-item>
          </v-tabs-window>
        </div>
        <div class="picker-panel-footer">
          <v-btn v-show="tab === 'date'" @click="previousDate">{{
            $t("action.previousDate")
          }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
          <v-btn v-show="tab === 'time'" @click="currentTime">{{ $t("action.currentTime") }}</v-btn>
        </div>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card class="zk-panel">
        <div class="picker-panel-body">
          <v-tabs v-model="tab">
            <v-tab value="date">{{ $t("model.__base.field.date") }}</v-tab>
            <v-tab value="time">{{ $t("model.__base.field.time") }}</v-tab>
          </v-tabs>
          <v-divider />
          <v-tabs-window v-model="tab">
            <v-tabs-window-item value="date">
              <v-date-picker
                :model-value="dateRaw"
                show-adjacent-months
                @update:model-value="updateDate"
              />
            </v-tabs-window-item>
            <v-tabs-window-item value="time">
              <v-time-picker
                :model-value="timeStr"
                format="24hr"
                use-seconds
                @update:model-value="updateTime"
              />
            </v-tabs-window-item>
          </v-tabs-window>
        </div>
        <div class="picker-panel-footer">
          <v-btn v-show="tab === 'date'" @click="previousDate">{{
            $t("action.previousDate")
          }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
          <v-btn v-show="tab === 'time'" @click="currentTime">{{ $t("action.currentTime") }}</v-btn>
        </div>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="datetime-field">
import { DateTime } from "luxon";

import type { Density } from "../@types/base";
import { localeDatetimeFormat, localeTimeFormat } from "@/utils/i18n";

// Validation rule shape of the underlying field: `true` when valid,
// otherwise the error message to display.
type Rule = (value: unknown) => boolean | string;

const tab = ref("date");

const datetimeStr = computed((): string | null => {
  if (props.modelValue === null) {
    return null;
  } else if (props.modelValue instanceof DateTime) {
    return props.modelValue.toLocaleString(DateTime.DATETIME_SHORT_WITH_SECONDS);
  } else if (props.modelValue instanceof Date) {
    return DateTime.fromJSDate(props.modelValue).toLocaleString(
      DateTime.DATETIME_SHORT_WITH_SECONDS,
    );
  } else if (typeof props.modelValue === "string") {
    return props.modelValue;
  } else {
    return "unknown";
  }
});

const dateRaw = computed((): Date | null => {
  if (props.modelValue === null) {
    return null;
  } else if (props.modelValue instanceof DateTime) {
    return props.modelValue.toJSDate();
  } else if (props.modelValue instanceof Date) {
    return props.modelValue;
  } else if (typeof props.modelValue === "string") {
    return DateTime.fromISO(props.modelValue).toJSDate();
  } else {
    return null;
  }
});

const timeStr = computed((): string | null => {
  if (props.modelValue === null) {
    return null;
  } else if (props.modelValue instanceof DateTime) {
    return localeDatetimeFormat(props.modelValue);
  } else if (props.modelValue instanceof Date) {
    return localeDatetimeFormat(DateTime.fromJSDate(props.modelValue));
  } else if (typeof props.modelValue === "string") {
    return localeDatetimeFormat(DateTime.fromISO(props.modelValue));
  } else {
    return "unknown";
  }
});

const previousDate = () => {
  updateDate(DateTime.now().minus({ days: 1 }).toJSDate());
};

const currentDate = () => {
  updateDate(DateTime.now().toJSDate());
};

const nextDate = () => {
  updateDate(DateTime.now().plus({ days: 1 }).toJSDate());
};

const currentTime = () => {
  updateTime(localeTimeFormat(DateTime.now()));
};

const updateDate = (value: Date | null) => {
  if (value === null) {
    emits("update:modelValue", null);
  } else if (props.modelValue instanceof DateTime) {
    const date = DateTime.fromJSDate(value);
    date.set({
      hour: props.modelValue.hour,
      minute: props.modelValue.minute,
      second: props.modelValue.second,
      millisecond: props.modelValue.millisecond,
    });
    emits("update:modelValue", date);
  } else if (props.modelValue instanceof Date) {
    const date = DateTime.fromJSDate(value);
    const origin = DateTime.fromJSDate(props.modelValue);
    date.set({
      hour: origin.hour,
      minute: origin.minute,
      second: origin.second,
      millisecond: origin.millisecond,
    });
    emits("update:modelValue", date.toJSDate());
  } else if (typeof props.modelValue === "string") {
    const date = DateTime.fromJSDate(value);
    const origin = DateTime.fromISO(props.modelValue);
    date.set({
      hour: origin.hour,
      minute: origin.minute,
      second: origin.second,
      millisecond: origin.millisecond,
    });
    emits("update:modelValue", date.toISO());
  } else {
    emits("update:modelValue", value);
  }
};

const updateTime = (value: string) => {
  if (value === null) {
    emits("update:modelValue", null);
  } else if (props.modelValue instanceof DateTime) {
    const date = DateTime.fromISO(value);
    date.set({
      year: props.modelValue.year,
      month: props.modelValue.month,
      day: props.modelValue.day,
    });
    emits("update:modelValue", date);
  } else if (props.modelValue instanceof Date) {
    const date = DateTime.fromISO(value);
    const origin = DateTime.fromJSDate(props.modelValue);
    date.set({
      year: origin.year,
      month: origin.month,
      day: origin.day,
    });
    emits("update:modelValue", date.toJSDate());
  } else if (typeof props.modelValue === "string") {
    const date = DateTime.fromISO(value);
    const origin = DateTime.fromISO(props.modelValue);
    date.set({
      year: origin.year,
      month: origin.month,
      day: origin.day,
    });
    emits("update:modelValue", date.toISO());
  } else {
    emits("update:modelValue", value);
  }
};

const props = defineProps({
  modelValue: {
    type: [DateTime, Date, String, null] as PropType<DateTime | Date | string | null>,
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
