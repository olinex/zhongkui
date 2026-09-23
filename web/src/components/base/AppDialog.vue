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
    /** Whether the dialog is currently open (v-model). */
    modelValue: boolean;
    /** Dialog title rendered in the toolbar. */
    title: string;
    /** Optional leading mdi-* icon rendered before the title. */
    icon?: string;
    /** Dialog width tier, mapped to the --zk-dialog-width-* token of the same name. */
    size?: "sm" | "md" | "lg" | "xl";
    /** When true, overlay click and ESC do not close the dialog. */
    persistent?: boolean;
    /** When true, the body scrolls independently of the toolbar and footer. */
    scrollable?: boolean;
    /** When true, a progress indicator is shown next to the title. */
    loading?: boolean;
  }>(),
  {
    icon: undefined,
    size: "md",
    persistent: false,
    scrollable: true,
    loading: false,
  },
);

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

// Width comes from the token layer only (tokens.scss exports --zk-dialog-width-*).
const maxWidth = computed<string>(() => `var(--zk-dialog-width-${props.size})`);

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};
</script>
