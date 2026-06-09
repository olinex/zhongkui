<template>
  <v-text-field
    :model-value="datetimeStr"
    :label="label"
    min-width="245"
    prepend-inner-icon="mdi-clock"
    @click:clear="emits('update:modelValue', null)"
    clearable
    readonly
  >
    <v-menu
      v-if="menu"
      :close-on-content-click="false"
      activator="parent"
      min-width="0"
    >
      <v-card>
        <v-card-text>
          <v-tabs v-model="tab">
            <v-tab value="date">{{ $t("model.__base.field.date") }}</v-tab>
            <v-tab value="time">{{ $t("model.__base.field.time") }}</v-tab>
          </v-tabs>
          <v-divider />
          <v-tabs-window v-model="tab">
            <v-tabs-window-item value="date">
              <v-date-picker
                :model-value="dateRaw"
                @update:model-value="updateDate"
                show-adjacent-months
              />
            </v-tabs-window-item>
            <v-tabs-window-item value="time">
              <v-time-picker
                :model-value="timeStr"
                @update:model-value="updateTime"
                format="24hr"
                use-seconds
              />
            </v-tabs-window-item>
          </v-tabs-window>
        </v-card-text>
        <v-card-actions>
          <v-btn v-show="tab === 'date'" @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
          <v-btn v-show="tab === 'time'" @click="currentTime">{{ $t("action.currentTime") }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card>
        <v-card-text>
          <v-tabs v-model="tab">
            <v-tab value="date">{{ $t("model.__base.field.date") }}</v-tab>
            <v-tab value="time">{{ $t("model.__base.field.time") }}</v-tab>
          </v-tabs>
          <v-divider />
          <v-tabs-window v-model="tab">
            <v-tabs-window-item value="date">
              <v-date-picker
                :model-value="dateRaw"
                @update:model-value="updateDate"
                show-adjacent-months
              />
            </v-tabs-window-item>
            <v-tabs-window-item value="time">
              <v-time-picker
                :model-value="timeStr"
                @update:model-value="updateTime"
                format="24hr"
                use-seconds
              />
            </v-tabs-window-item>
          </v-tabs-window>
        </v-card-text>
        <v-card-actions>
          <v-btn v-show="tab === 'date'" @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn v-show="tab === 'date'" @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
          <v-btn v-show="tab === 'time'" @click="currentTime">{{ $t("action.currentTime") }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="datetime-field">
import { localeDatetimeFormat, localeTimeFormat } from "@/utils/i18n";
import { DateTime } from "luxon";

const tab = ref("date");

const datetimeStr = computed((): string | null => {
  if (props.modelValue === null) {
    return null;
  } else if (props.modelValue instanceof DateTime) {
    return props.modelValue.toLocaleString(
      DateTime.DATETIME_SHORT_WITH_SECONDS
    );
  } else if (props.modelValue instanceof Date) {
    return DateTime.fromJSDate(props.modelValue).toLocaleString(
      DateTime.DATETIME_SHORT_WITH_SECONDS
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
    return localeDatetimeFormat(props.modelValue)
  } else if (props.modelValue instanceof Date) {
    return localeDatetimeFormat(DateTime.fromJSDate(props.modelValue))
  } else if (typeof props.modelValue === "string") {
    return localeDatetimeFormat(DateTime.fromISO(props.modelValue))
  } else {
    return "unknown";
  }
});

const previousDate = () => {
  updateDate(DateTime.now().minus({days: 1}).toJSDate())
}

const currentDate = () => {
  updateDate(DateTime.now().toJSDate())
}

const nextDate = () => {
  updateDate(DateTime.now().plus({days: 1}).toJSDate())
}

const currentTime = () => {
  updateTime(localeTimeFormat(DateTime.now()))
}

const updateDate = (value: Date | null) => {
  if (value === null) {
    emits("update:modelValue", null);
  } else if (props.modelValue instanceof DateTime) {
    let date = DateTime.fromJSDate(value);
    date.set({
      hour: props.modelValue.hour,
      minute: props.modelValue.minute,
      second: props.modelValue.second,
      millisecond: props.modelValue.millisecond,
    });
    emits("update:modelValue", date);
  } else if (props.modelValue instanceof Date) {
    let date = DateTime.fromJSDate(value);
    const origin = DateTime.fromJSDate(props.modelValue);
    date.set({
      hour: origin.hour,
      minute: origin.minute,
      second: origin.second,
      millisecond: origin.millisecond,
    });
    emits("update:modelValue", date.toJSDate());
  } else if (typeof props.modelValue === "string") {
    let date = DateTime.fromJSDate(value);
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
    let date = DateTime.fromISO(value);
    date.set({
      year: props.modelValue.year,
      month: props.modelValue.month,
      day: props.modelValue.day,
    });
    emits("update:modelValue", date);
  } else if (props.modelValue instanceof Date) {
    let date = DateTime.fromISO(value);
    const origin = DateTime.fromJSDate(props.modelValue);
    date.set({
      year: origin.year,
      month: origin.month,
      day: origin.day,
    });
    emits("update:modelValue", date.toJSDate());
  } else if (typeof props.modelValue === "string") {
    let date = DateTime.fromISO(value);
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
    type: [DateTime, Date, String, null] as PropType<
      DateTime | Date | String | null
    >,
    required: true,
  },
  label: {
    type: String,
  },
  menu: {
    type: Boolean,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
