<template>
  <div class="schema-editor-wrap">
    <v-label class="mb-1">{{ $t("model.datasource.configField.schemaHint") }}</v-label>
    <div
      v-for="(col, index) in columns"
      :key="index"
      class="schema-row"
    >
      <v-text-field
        :model-value="col.name"
        :label="$t('model.datasource.configField.columnName')"
        density="compact"
        variant="outlined"
        hide-details
        :disabled="disabled"
        @update:model-value="updateName(index, $event)"
      />
      <v-select
        :model-value="col.type"
        :items="columnTypeItems"
        :label="$t('model.datasource.configField.columnType')"
        density="compact"
        variant="outlined"
        hide-details
        :disabled="disabled"
        @update:model-value="updateType(index, $event)"
      />
      <v-btn
        icon="mdi-delete-outline"
        variant="text"
        size="small"
        :disabled="disabled"
        :title="$t('action.delete')"
        @click="removeColumn(index)"
      />
    </div>
    <v-btn
      variant="tonal"
      size="small"
      prepend-icon="mdi-plus"
      class="mt-2"
      :disabled="disabled"
      @click="addColumn"
    >
      {{ $t("model.datasource.configField.addColumn") }}
    </v-btn>
  </div>
</template>

<script setup lang="ts" name="datasource-schema-editor">
import i18n from "@/utils/i18n";

// 手动 schema 的一列：与后端 DatasourceColumn JSON 结构一致
interface DatasourceColumn {
  name: string;
  type: string;
}

const props = withDefaults(
  defineProps<{
    modelValue: DatasourceColumn[];
    disabled?: boolean;
  }>(),
  { disabled: false },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: DatasourceColumn[]): void;
}>();

// 后端 columnValueType 允许的类型键（与 sqlitevtab.ValueType 映射一致）
const COLUMN_TYPES = ["any", "integer", "float", "string", "blob", "bool"] as const;

const columnTypeItems = computed(() =>
  COLUMN_TYPES.map((t) => ({
    title: i18n.global.t(`model.datasource.columnType.${t}`),
    value: t,
  })),
);

// columns 为本地响应式副本（替代直接改 props.modelValue），变更统一经 emit 上报
const columns = ref<DatasourceColumn[]>([]);

watch(
  () => props.modelValue,
  (val) => {
    // 深拷贝，避免与父层共享数组引用造成意外串扰
    columns.value = (val ?? []).map((c) => ({ name: c.name, type: c.type }));
  },
  { immediate: true },
);

const commit = (): void => emit("update:modelValue", columns.value.map((c) => ({ ...c })));

const addColumn = (): void => {
  columns.value.push({ name: "", type: "string" });
  commit();
};

const removeColumn = (index: number): void => {
  columns.value.splice(index, 1);
  commit();
};

const updateName = (index: number, value: unknown): void => {
  columns.value[index].name = (value as string) ?? "";
  commit();
};

const updateType = (index: number, value: unknown): void => {
  columns.value[index].type = (value as string) ?? "string";
  commit();
};
</script>

<style scoped>
.schema-editor-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.schema-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>