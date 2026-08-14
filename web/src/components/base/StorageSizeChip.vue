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

const props = defineProps({
  value: {
    type: Number,
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
  binary: {
    type: Boolean,
    default: false,
  },
});
</script>
