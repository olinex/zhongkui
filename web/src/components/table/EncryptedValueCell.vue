<template>
  <!-- 加密字段（crypto）：占位“已加密”，可点击直接解密查看明文；无密文时禁用。
       S3 字段（s3）：展示对象键与存储大小，可点击读取明文；均不直接暴露 base64 密文 -->
  <v-btn
    v-if="kind === 'crypto'"
    :text="$t('model.__base.field.encrypted')"
    size="small"
    variant="tonal"
    color="warning"
    :disabled="isEmpty(value)"
    :loading="loading"
    @click.stop="emit('view-plaintext', item, fieldKey)"
  />
  <!-- S3 分支：对象键 + 存储大小 + 明文入口按同一行内基线对齐（间距走 4px 栅格 gap） -->
  <div v-else class="d-inline-flex align-center ga-2">
    <span class="font-mono">{{ value }}</span>
    <storage-size-chip :value="Number(item[`${fieldKey}Length`] ?? 0)" binary />
    <v-btn
      :text="$t('action.viewPlaintext')"
      size="small"
      variant="tonal"
      color="primary"
      :disabled="isEmpty(value)"
      :loading="loading"
      @click.stop="emit('view-plaintext', item, fieldKey)"
    />
  </div>
</template>

<script setup lang="ts" name="encrypted-value-cell">
import type { PropType } from "vue";

// Utils
import { isEmptyCiphertext } from "@/utils/encrypted";

// 列表页加密/S3 字段统一单元格：按 kind 分支渲染“已加密”占位或对象键 + 存储大小 +
// “查看明文”操作。value 取自 item[fieldKey]，s3 长度取自 item[`${fieldKey}Length`]。
// 点击统一派发 view-plaintext(item, fieldKey)，由页面按字段分发到对应的明文查看流程。
const props = defineProps({
  kind: {
    type: String as PropType<"crypto" | "s3">,
    required: true,
  },
  item: {
    // 行值异构；字段类型由 kind 决定
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Object as PropType<Record<string, any>>,
    required: true,
  },
  fieldKey: {
    type: String,
    required: true,
  },
  loading: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits<{
  // 行值异构
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  "view-plaintext": [item: Record<string, any>, fieldKey: string];
}>();

// isEmpty 判定无密文/空对象占位（含 base64 形态），避免把非密文当密文请求明文接口
const isEmpty = (value: unknown): boolean => isEmptyCiphertext(value);

const value = computed(() => props.item[props.fieldKey]);
</script>
