<template>
  <v-chip
    :color="color"
    :variant="variant"
    :size="size"
    prepend-icon="mdi-content-copy"
    @click="copyToClipboard(value)"
  >
    <span class="font-mono">{{ prefix }}{{ value.slice(0, length) }}{{ suffix }}</span>
  </v-chip>
</template>

<script setup lang="ts" name="hex-string-chip">
import { copyToClipboard } from "@/utils/clipboard";
import type { Variant } from "../@types/base";

defineProps({
  value: {
    type: String,
    required: true,
  },
  size: {
    type: String,
    default: "default",
  },
  color: {
    type: String,
    default: "default",
  },
  variant: {
    type: String as PropType<Variant>,
    default: "tonal",
  },
  prefix: {
    type: String,
    default: "",
  },
  suffix: {
    type: String,
    default: "",
  },
  length: {
    type: Number,
    default: 6,
  },
});
</script>
