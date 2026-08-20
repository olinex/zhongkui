<template>
  <v-text-field
    :model-value="modelValue"
    :label="label"
    min-width="160"
    prepend-inner-icon="mdi-clock"
    clearable
    readonly
    @click:clear="update(null)"
  >
    <v-menu v-if="menu" :close-on-content-click="false" activator="parent" min-width="0">
      <v-card>
        <v-card-text>
          <v-time-picker
            :model-value="modelValue"
            format="24hr"
            use-seconds
            @update:model-value="update"
          />
        </v-card-text>
        <v-card-actions>
          <v-btn @click="currentTime">{{ $t("action.currentTime") }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-menu>
    <v-dialog v-else activator="parent" width="auto">
      <v-card>
        <v-card-text>
          <v-time-picker
            :model-value="modelValue"
            format="24hr"
            use-seconds
            @update:model-value="update"
          />
        </v-card-text>
        <v-card-actions>
          <v-btn @click="currentTime">{{ $t("action.currentTime") }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-text-field>
</template>

<script setup lang="ts" name="time-field">
import { localeTimeFormat } from "@/utils/i18n";
import { DateTime } from "luxon";

const update = (value: unknown) => {
  emits("update:modelValue", value);
};

const currentTime = () => {
  update(localeTimeFormat(DateTime.now()));
};

defineProps({
  modelValue: {
    // Passed through to the time picker as-is.
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
