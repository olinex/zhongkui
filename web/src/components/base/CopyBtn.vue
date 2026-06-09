<template>
  <v-btn
    @click="copyToClipboard(value)"
    icon="mdi-content-copy"
    :color="color"
    :variant="variant"
    :density="density"
  ></v-btn>
</template>

<script setup lang="ts" name="copy-btn">
import { copyToClipboard } from "@/utils/clipboard";
import type { PropType } from "vue";
import type { Density, Variant } from "../@types/base";

const props = defineProps({
  value: {
    type: String,
    required: true,
  },
  color: {
    type: String,
  },
  variant: {
    type: String as PropType<Variant>,
    default: "text",
  },
  density: {
    type: String as PropType<Density>,
  },
});
</script>
