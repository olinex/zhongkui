<template>
  <v-chip
    :color="value ? 'success' : 'error'"
    :variant="variant"
    :density="density"
  >
    <v-icon>
      {{ value ? "mdi-check" : "mdi-close" }}
    </v-icon>
    {{ label }}
  </v-chip>
</template>

<script setup lang="ts" name="boolean-chip">
import type { Density, Variant } from "../@types/base";

const props = defineProps({
  value: {
    type: Boolean,
    required: true,
  },
  variant: {
    type: String as PropType<Variant>,
  },
  density: {
    type: String as PropType<Density>,
  },
  label: {
    type: String
  }
});
</script>
