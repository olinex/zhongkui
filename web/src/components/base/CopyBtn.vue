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

// color / density 保持 undefined，由主题层统一 VBtn 基线（density="compact"）；
// 纯图标按钮本身即扁平，故 variant 默认取 "text" 而非 Vuetify 的 "elevated"。
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
