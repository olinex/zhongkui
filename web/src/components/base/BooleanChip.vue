<template>
  <v-chip :color="value ? 'success' : 'error'" :variant="variant" :density="density">
    <v-icon>
      {{ value ? "mdi-check" : "mdi-close" }}
    </v-icon>
    {{ label }}
  </v-chip>
</template>

<script setup lang="ts" name="boolean-chip">
import type { Density, Variant } from "../@types/base";

defineProps({
  value: {
    type: Boolean,
    required: true,
  },
  variant: {
    type: String as PropType<Variant>,
    default: "tonal",
  },
  density: {
    type: String as PropType<Density>,
    default: "default",
  },
  label: {
    type: String,
    default: "",
  },
});
</script>
