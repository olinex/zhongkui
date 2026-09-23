<template>
  <!-- Icon-only by design: the semantics live in the icon shape, its theme color
       and the title/aria-label text; no "true"/"false" wording is rendered.
       The wrapper carries the accessible name (VIcon itself is aria-hidden when
       it has no click handler). -->
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
  value: {
    type: Boolean,
    required: true,
  },
});

// Reuses the existing generic boolean wording keys (model.__base.value.true|false).
const label = computed((): string => {
  return i18n.global.t(props.value ? "model.__base.value.true" : "model.__base.value.false");
});
</script>
