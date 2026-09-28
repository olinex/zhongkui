<template>
  <v-chip :color="color" :variant="variant" :density="density">
    <span class="font-mono">{{ humanReadableValue }}</span>
  </v-chip>
</template>

<script setup lang="ts" name="storage-size-chip">
import numeral from "numeral";
import type { Density, Variant } from "../@types/base";

// 将字节数格式化为易读的存储容量文本（二进制或十进制单位）。
const humanReadableValue = computed((): string => {
  if (props.binary) {
    return numeral(props.value).format("0.00 ib");
  } else {
    return numeral(props.value).format("0.00 b");
  }
});

// 各外观属性默认保持 undefined，由主题层决定 VChip 基线外观（small + tonal）；
// 调用方仅在存在真实语义差异时才覆盖。
const props = defineProps({
  // value 字节数原始值
  value: {
    type: Number,
    required: true,
  },
  // color 颜色（缺省由主题层决定）
  color: {
    type: String,
    default: undefined,
  },
  // variant 外观变体（缺省由主题层决定）
  variant: {
    type: String as PropType<Variant>,
    default: undefined,
  },
  // density 密度（缺省由主题层决定）
  density: {
    type: String as PropType<Density>,
    default: undefined,
  },
  // binary 是否按二进制单位换算展示
  binary: {
    type: Boolean,
    default: false,
  },
});
</script>
