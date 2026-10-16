<template>
  <!-- 知识库分块明细对话框：分块展示逻辑（索引摘要 + 分块列表）已提取为只读视图组件，
       本文件仅作薄包装，负责对话框外壳与开关 → documentID 的传参。
       关闭对话框时传 documentID=0，等价于原「open && documentID」判定，避免关闭时仍发请求。
       chunksRequest 由调用方按来源分栏下发（系统知识库 → 只读系统接口），缺省走常规接口。 -->
  <app-dialog
    :model-value="modelValue"
    :title="$t('page.aiKnowledge.chunks')"
    icon="mdi-file-document-multiple-outline"
    size="lg"
    :subtitle="documentTitle"
    @update:model-value="onUpdate"
  >
    <knowledge-chunks-view
      :documentID="modelValue ? documentID : 0"
      :chunks-request="chunksRequest"
    />
  </app-dialog>
</template>

<script setup lang="ts" name="knowledge-chunks-dialog">
import type { AxiosPromise } from "axios";

defineProps<{
  modelValue: boolean;
  documentID: number;
  documentTitle: string;
  // 分块请求由调用方覆盖：系统知识库分栏需走只读系统接口，缺省为文档级常规接口
  chunksRequest?: (id: number) => AxiosPromise;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};
</script>
