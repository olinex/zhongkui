<template>
  <!-- 行详情对话框：只读明细以全屏对话框展示，便于浏览大字段与长报文；
       字段按 kind 分支渲染，crypto/s3 提供明文查看（通过 view-plaintext 事件交给调用方） -->
  <v-dialog :model-value="modelValue" fullscreen transition="dialog-bottom-transition" @update:model-value="onUpdate">
    <v-card>
      <v-toolbar density="compact">
        <v-toolbar-title class="text-no-wrap">
          {{ title }}
        </v-toolbar-title>
        <v-spacer />
        <v-btn icon="mdi-close" variant="text" @click="onUpdate(false)" />
      </v-toolbar>
      <div class="px-6 py-4 overflow-y-auto" style="height: calc(100vh - 48px)">
        <v-table density="compact">
          <thead>
            <tr>
              <th class="text-no-wrap">{{ $t("model.__base.field.field") }}</th>
              <th>{{ $t("model.__base.field.value") }}</th>
              <th class="text-no-wrap">{{ $t("model.__base.field.action") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="field in fields" :key="field.key">
              <td class="text-no-wrap">
                {{ field.label }}
              </td>
              <td>
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
                  {{ field.display }}
                </template>
              </td>
              <td class="text-no-wrap">
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

defineProps<{
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
</script>
