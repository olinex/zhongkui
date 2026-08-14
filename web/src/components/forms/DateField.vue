<template>
  <v-text-field
    :model-value="modelValue"
    :label="label"
    min-width="180"
    prepend-inner-icon="mdi-clock"
    clearable
    readonly
    @click:clear="update(null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card>
        <v-card-text>
          <v-date-picker
            :model-value="modelValue"
            show-adjacent-months
            @update:model-value="update"
          />
        </v-card-text>
        <v-card-actions>
          <v-btn @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card>
        <v-card-text>
          <v-date-picker
            :model-value="modelValue"
            show-adjacent-months
            @update:model-value="update"
          />
        </v-card-text>
        <v-card-actions>
          <v-btn @click="previousDate">{{ $t("action.previousDate") }}</v-btn>
          <v-btn @click="currentDate">{{ $t("action.currentDate") }}</v-btn>
          <v-btn @click="nextDate">{{ $t("action.nextDate") }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="date-field">
import { DateTime } from "luxon";

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
  menu: {
    type: Boolean,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
