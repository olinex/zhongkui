<template>
  <v-alert type="warning" icon="mdi-comment-question-outline" :title="action" :text="text" />
</template>

<script setup lang="ts" name="confirm-alert">
defineProps({
  action: {
    type: String,
    required: true,
  },
  text: {
    type: String,
    required: true,
  },
});
</script>
