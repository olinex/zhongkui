<template>
  <v-dialog v-model="dialog" max-width="600">
    <v-card :prepend-icon="prependIcon" :title="title">
      <v-card-text>
        <slot name="body" :proxy="proxy" :editable="forceEditable || editable" />
      </v-card-text>
      <v-divider />
      <v-card-actions>
        <v-spacer />
        <v-btn :text="$t(`action.close`)" variant="plain" @click="close" />
        <v-btn
          v-show="!forceEditable && !editable"
          :text="$t(`action.edit`)"
          variant="tonal"
          @click="editable = true"
        />
        <v-btn
          v-show="forceEditable || editable"
          color="primary"
          :text="$t(`action.confirm`)"
          :disabled="confirmDisabled"
          @click="confirm"
        />
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts" name="temp-editable-dialog">
const dialog = ref(false);
const editable = ref(false);
const proxy = ref({} as Record<string, any>);

const open = (item: Record<string, any>) => {
  proxy.value = { ...item };
  dialog.value = true;
};

const close = () => {
  editable.value = false;
  dialog.value = false;
};

const confirm = async () => {
  emits("confirm", proxy.value);
  close();
};

const emits = defineEmits(["confirm"]);

const props = defineProps({
  prependIcon: {
    type: String,
  },
  title: {
    type: String,
    required: true,
  },
  forceEditable: {
    type: Boolean,
    default: false,
  },
  confirmDisabled: {
    type: Boolean,
    default: false,
  }
});

defineExpose({ open });
</script>
