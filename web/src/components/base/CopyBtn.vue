<template>
  <v-btn
    icon="mdi-content-copy"
    :color="color"
    :variant="variant"
    :density="density"
    @click="copyToClipboard(value)"
  ></v-btn>
</template>

<script setup lang="ts" name="copy-btn">
import { copyToClipboard } from "@/utils/clipboard";
import type { PropType } from "vue";
import type { Density, Variant } from "../@types/base";

defineProps({
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
    default: "text",
  },
  density: {
    type: String as PropType<Density>,
    default: "compact",
  },
});
</script>
