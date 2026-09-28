<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="maxWidth"
    :persistent="persistent"
    :scrollable="scrollable"
    @update:model-value="onUpdate"
  >
    <v-card>
      <v-toolbar class="zk-dialog-toolbar">
        <v-icon v-if="icon" :icon="icon" color="primary" />
        <v-toolbar-title class="zk-section-title text-truncate">
          {{ title }}
          <span v-if="subtitle" class="zk-section-subtitle">{{ subtitle }}</span>
        </v-toolbar-title>
        <slot name="header-append" />
        <v-progress-circular v-if="loading" indeterminate size="20" />
        <v-spacer />
        <v-btn
          icon="mdi-close"
          variant="text"
          :title="$t('action.close')"
          :aria-label="$t('action.close')"
          @click="onUpdate(false)"
        />
      </v-toolbar>
      <v-divider />
      <v-card-text>
        <slot />
      </v-card-text>
      <template v-if="$slots.footer">
        <v-divider />
        <v-card-actions class="justify-end">
          <slot name="footer" />
        </v-card-actions>
      </template>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts" name="app-dialog">
const props = withDefaults(
  defineProps<{
    /** 对话框当前是否打开（v-model）。 */
    modelValue: boolean;
    /** 工具栏中渲染的对话框标题。 */
    title: string;
    /** 可选副标题，紧跟在主标题之后渲染（更弱的字重与颜色）。 */
    subtitle?: string;
    /** 可选的前置 mdi-* 图标，渲染在标题之前。 */
    icon?: string;
    /** 对话框宽度档位，映射到同名的 --zk-dialog-width-* 令牌。 */
    size?: "sm" | "md" | "lg" | "xl";
    /** 为 true 时，点击遮罩或按 ESC 不会关闭对话框。 */
    persistent?: boolean;
    /** 为 true 时，正文区域独立于工具栏与底部滚动。 */
    scrollable?: boolean;
    /** 为 true 时，在标题旁显示进度指示。 */
    loading?: boolean;
  }>(),
  {
    icon: undefined,
    subtitle: undefined,
    size: "md",
    persistent: false,
    scrollable: true,
    loading: false,
  },
);

// 对话框开关状态变更事件。
const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

// 宽度仅来自令牌层（tokens.scss 导出 --zk-dialog-width-*）。
const maxWidth = computed<string>(() => `var(--zk-dialog-width-${props.size})`);

// 将开关状态透传给父组件。
const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};
</script>
