<template>
  <app-dialog v-model="dialog" :title="title" :icon="prependIcon" size="sm">
    <slot name="body" :proxy="proxy" :editable="forceEditable || editable" />
    <template #footer>
      <v-btn :text="$t(`action.close`)" variant="text" @click="close" />
      <v-btn
        v-show="!forceEditable && !editable"
        :text="$t(`action.edit`)"
        variant="tonal"
        @click="editable = true"
      />
      <v-btn
        v-show="forceEditable || editable"
        v-request="confirm"
        color="primary"
        :text="$t(`action.confirm`)"
        :disabled="confirmDisabled"
      />
    </template>
  </app-dialog>
</template>

<script setup lang="ts" name="temp-editable-dialog">
const dialog = ref(false);
const editable = ref(false);
// 对话框负载为动态结构。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const proxy = ref({} as Record<string, any>);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const open = (item: Record<string, any>) => {
  proxy.value = { ...item };
  dialog.value = true;
};

const close = () => {
  editable.value = false;
  dialog.value = false;
};

// confirm 等待父级 @confirm 处理器返回的 Promise（若为异步请求）结束后再关窗，
// 配合 v-request 指令实现"点击后禁用 + 加载中动画"，防止慢接口重复提交。
const confirm = async () => {
  const result = emits("confirm", proxy.value);
  await Promise.resolve(result);
  close();
};

const emits = defineEmits(["confirm"]);

defineProps({
  prependIcon: {
    type: String,
    default: "",
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
  },
});

defineExpose({ open });
</script>
