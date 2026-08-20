<template>
  <div>
    <v-chip :color="color" :variant="variant" :density="density" @click="dialog = true">
      <span class="font-mono">{{ short }}</span>
    </v-chip>
    <v-dialog v-model="dialog" max-width="600">
      <template #default>
        <v-card>
          <v-card-text>
            <VueJsonPretty :data="value && JSON.parse(value)" />
          </v-card-text>
          <v-card-actions>
            <v-btn @click="close">
              {{ $t("action.close") }}
            </v-btn>
            <v-btn color="info" @click="copy">
              {{ $t("action.copy") }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </template>
    </v-dialog>
  </div>
</template>

<script setup lang="ts" name="json-chip">
import VueJsonPretty from "vue-json-pretty";
import i18n from "@/utils/i18n";
import type { Density, Variant } from "../@types/base";
import "vue-json-pretty/lib/styles.css";
import { copyToClipboard } from "@/utils/clipboard";

const dialog = ref(false);

const short = computed(() => {
  const raw = props.value.trim();
  if (raw === "") {
    return i18n.global.t("model.__base.value.blank");
  }
  const firstChar = raw.charAt(0);
  switch (firstChar) {
    case "[":
      return "[...]";
    case "{":
      return "{...}";
    default:
      return raw;
  }
});

const close = () => {
  dialog.value = false;
};

const copy = async () => {
  copyToClipboard(props.value);
  close();
};

const props = defineProps({
  value: {
    type: String,
    required: true,
  },
  color: {
    type: String,
    default: "default",
  },
  variant: {
    type: String as PropType<Variant>,
    default: "tonal",
  },
  density: {
    type: String as PropType<Density>,
    default: "default",
  },
});
</script>
