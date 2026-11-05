<template>
  <!-- 有意采用纯图标呈现：语义由图标形状、主题色以及 title/aria-label 文本承载，
       不渲染 "true"/"false" 字样。由外层包裹元素承载无障碍名称
       （在无点击事件时 VIcon 自身为 aria-hidden）。 -->
  <span :title="label" :aria-label="label" role="img">
    <v-icon
      :icon="value ? 'mdi-check-circle' : 'mdi-close-circle'"
      :color="value ? 'success' : 'error'"
    />
  </span>
</template>

<script setup lang="ts" name="boolean-chip">
import i18n from "@/utils/i18n";

const props = defineProps({
  // value 待展示的布尔值
  value: {
    type: Boolean,
    required: true,
  },
});

// 复用既有的通用布尔文案键（model.__base.value.true|false）。
const label = computed((): string => {
  return i18n.global.t(props.value ? "model.__base.value.true" : "model.__base.value.false");
});
</script>
