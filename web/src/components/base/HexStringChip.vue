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

// size / variant / color are intentionally unset by default: the theme layer
// already provides the VChip baseline (small + tonal), so leaving them
// undefined keeps every chip in the family identical unless a caller really
// needs a semantic override (e.g. color="info" for hashes).
defineProps({
  value: {
    type: String,
    required: true,
  },
  size: {
    type: String,
    default: undefined,
  },
  color: {
    type: String,
    default: undefined,
  },
  variant: {
    type: String as PropType<Variant>,
    default: undefined,
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
