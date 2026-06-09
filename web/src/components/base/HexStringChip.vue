<template>
  <v-chip
    @click="copyToClipboard(value)"
    :color="color"
    :size="size"
    prepend-icon="mdi-content-copy"
  >
    {{ prefix }}{{ value.slice(0, length) }}{{ suffix }}
  </v-chip>
</template>

<script setup lang="ts" name="hex-string-chip">
import { copyToClipboard } from "@/utils/clipboard";

const props = defineProps({
  value: {
    type: String,
    required: true,
  },
  size: {
    type: String,
  },
  color: {
    type: String,
  },
  prefix: {
    type: String,
  },
  suffix: {
    type: String,
  },
  length: {
    type: Number,
    default: 6,
  },
});
</script>
