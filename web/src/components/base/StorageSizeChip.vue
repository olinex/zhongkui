<template>
  <v-chip :color="color" :variant="variant" :density="density">
    <span class="font-mono">{{ humanReadableValue }}</span>
  </v-chip>
</template>

<script setup lang="ts" name="storage-size-chip">
import numeral from "numeral";
import type { Density, Variant } from "../@types/base";

const humanReadableValue = computed((): string => {
  if (props.binary) {
    return numeral(props.value).format("0.00 ib");
  } else {
    return numeral(props.value).format("0.00 b");
  }
});

// Defaults stay undefined so the theme layer owns the VChip baseline
// (small + tonal); callers only override for a real semantic difference.
const props = defineProps({
  value: {
    type: Number,
    required: true,
  },
  color: {
    type: String,
    default: undefined,
  },
  variant: {
    type: String as PropType<Variant>,
    default: undefined,
  },
  density: {
    type: String as PropType<Density>,
    default: undefined,
  },
  binary: {
    type: Boolean,
    default: false,
  },
});
</script>
