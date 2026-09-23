<template>
  <v-btn
    icon="mdi-content-copy"
    :color="color"
    :variant="variant"
    :density="density"
    :title="$t('action.copy')"
    :aria-label="$t('action.copy')"
    @click="copyToClipboard(value)"
  ></v-btn>
</template>

<script setup lang="ts" name="copy-btn">
import { copyToClipboard } from "@/utils/clipboard";
import type { PropType } from "vue";
import type { Density, Variant } from "../@types/base";

// color / density stay undefined so the theme layer owns the VBtn baseline
// (density="compact"); an icon-only button is flat by definition, so `variant`
// defaults to "text" instead of Vuetify's "elevated".
defineProps({
  value: {
    type: String,
    required: true,
  },
  color: {
    type: String,
    default: undefined,
  },
  variant: {
    type: String as PropType<Variant>,
    default: "text",
  },
  density: {
    type: String as PropType<Density>,
    default: undefined,
  },
});
</script>
