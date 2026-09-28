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

// size / variant / color 默认刻意留空：主题层已提供 VChip 基线（small + tonal），
// 保持 undefined 可让同族 chip 外观一致，除非调用方确有语义覆盖需求
// （如哈希使用 color="info"）。
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
