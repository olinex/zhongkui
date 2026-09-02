<template>
  <!-- 行详情对话框：只读明细以紧凑卡片对话框展示（与发送记录详情一致，便于聚焦字段）；
       字段按 kind 分支渲染；存在加密/S3 等操作字段时保留操作列，否则退化为两列；
       plain 字段按列表页方式（AdaptiveCell）依 type 渲染，而非纯文本 -->
  <v-dialog :model-value="modelValue" max-width="760" @update:model-value="onUpdate">
    <v-card>
      <v-toolbar density="compact">
        <v-toolbar-title class="text-no-wrap">
          {{ title }}
        </v-toolbar-title>
        <v-spacer />
        <v-btn icon="mdi-close" variant="text" @click="onUpdate(false)" />
      </v-toolbar>
      <div class="px-6 py-4 overflow-y-auto" style="max-height: 70vh">
        <v-table density="compact">
          <thead v-if="showActions">
            <tr>
              <th class="text-no-wrap">{{ $t("model.__base.field.field") }}</th>
              <th>{{ $t("model.__base.field.value") }}</th>
              <th class="text-no-wrap">{{ $t("model.__base.field.action") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="field in fields" :key="field.key">
              <td class="text-no-wrap font-weight-medium">
                {{ field.label }}
              </td>
              <td class="detail-value">
                <template v-if="field.kind === 'crypto'">
                  <v-chip color="warning" variant="tonal" size="small">
                    {{ $t("model.__base.field.encrypted") }}
                  </v-chip>
                </template>
                <template v-else-if="field.kind === 's3'">
                  <span class="font-mono">{{ field.display }}</span>
                  <storage-size-chip
                    class="ml-2"
                    :value="field.length"
                    binary
                    density="comfortable"
                  />
                </template>
                <template v-else>
                  <adaptive-cell :header="plainHeader(field)" :item="plainItem(field)" />
                </template>
              </td>
              <td v-if="showActions" class="text-no-wrap">
                <template
                  v-if="field.kind === 'crypto' || (field.kind === 's3' && field.length > 0)"
                >
                  <v-btn
                    :text="$t('action.viewPlaintext')"
                    size="small"
                    variant="tonal"
                    color="primary"
                    :loading="plaintextLoading === field.key"
                    @click="$emit('view-plaintext', field)"
                  />
                </template>
                <template v-else-if="field.kind === 's3'">
                  <v-chip size="small">{{ $t("notice.plaintextEmpty") }}</v-chip>
                </template>
              </td>
            </tr>
          </tbody>
        </v-table>
      </div>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts" name="model-detail-dialog">
import type { DetailField } from "@/components/@types/detail";
import type { TableHeader } from "@/components/@types/table";

const props = defineProps<{
  modelValue: boolean;
  title: string;
  fields: DetailField[];
  plaintextLoading?: string | null;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: boolean];
  "view-plaintext": [field: DetailField];
}>();

const onUpdate = (value: boolean): void => {
  emits("update:modelValue", value);
};

// showActions 是否需要操作列：存在加密/S3 字段（有查看明文等操作）时保留操作列
const showActions = computed(() => {
  return props.fields.some((field) => field.kind === "crypto" || field.kind === "s3");
});

// plainHeader 构造 plain 字段的表头（key/title/type），供 AdaptiveCell 按列表页方式渲染
const plainHeader = (field: DetailField): TableHeader => {
  return { key: field.key, title: field.label, type: field.type };
};

// plainItem 构造 plain 字段的单字段行数据（{ [key]: value }），供 AdaptiveCell 取值
const plainItem = (field: DetailField): Record<string, unknown> => {
  return { [field.key]: field.value };
};
</script>

<style scoped>
/* 详情值单元格：长文本（如 base64 报文、请求头）自动换行，避免撑破紧凑对话框 */
.detail-value {
  word-break: break-all;
}
</style>
