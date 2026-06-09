<template>
  <v-combobox
    :model-value="modelValue"
    @update:modelValue="clearSelections"
    @click:clear="clearSelections"
    @click="addMenuOpened = true"
    :label="$t('model.__base.field.search')"
    density="compact"
    variant="outlined"
    return-object
    multiple
    readonly
    clearable
    rounded
  >
    <template #selection="{ item, index }">
      <v-menu
        :key="index"
        location="top end"
        origin="top end"
        transition="scale-transition"
        :close-on-content-click="false"
      >
        <template #activator="{ props }">
          <v-chip
            v-bind="props"
            @click:close="removeSelection(index)"
            color="info"
            rounded
            closable
          >
            `{{ item.raw.field.title }}`&nbsp;{{ item.raw.action.title }}
            <span v-show="item.raw.value.value !== null"
              >&nbsp;`{{ item.raw.value.title }}`</span
            >
          </v-chip>
        </template>
        <clause-field
          :model-value="item.raw"
          @update:modelValue="(item) => updateSelection(item, index)"
          :selected-field-actions="selectedFieldActions"
          :headers="props.headers"
          :modify="true"
        />
      </v-menu>
    </template>
    <template #append-inner>
      <v-menu
        v-model="addMenuOpened"
        location="top end"
        origin="top end"
        transition="scale-transition"
        :close-on-content-click="false"
      >
        <template #activator="{ props: actProps }">
          <v-chip
            color="success"
            v-bind="actProps"
            append-icon="mdi-plus-circle"
          >
            <v-avatar>{{ props.modelValue.length }}</v-avatar>
            {{ $t("model.__base.content.condition") }}
          </v-chip>
        </template>
        <clause-field
          @update:modelValue="addSelection"
          :selected-field-actions="selectedFieldActions"
          :headers="headers"
        >
        </clause-field>
      </v-menu>
    </template>
  </v-combobox>
</template>

<script setup lang="ts" name="expression-field">
import type { FilterByOption } from "../@types/table";

export interface FilterItem {
  field: { value: string; title: string };
  action: { value: string; title: string };
  value: { value: any; title: string };
}

const addMenuOpened = ref(false);

const selectedFieldActions = computed(() => {
  return props.modelValue.map((item) =>
    Object({ field: item.field.value, action: item.action.value })
  );
});

const clearSelections = () => {
  emits("update:modelValue", []);
};

const removeSelection = (index: number) => {
  emits(
    "update:modelValue",
    props.modelValue.filter((_, i) => i !== index)
  );
};

const updateSelection = (item: FilterItem, index: number) => {
  emits(
    "update:modelValue",
    props.modelValue.map((i, j) => (j === index ? item : i))
  );
};

const addSelection = (item: FilterItem) => {
  emits("update:modelValue", [
    ...props.modelValue,
    {
      field: { value: item.field.value, title: item.field.title },
      action: { value: item.action.value, title: item.action.title },
      value: { value: item.value.value, title: item.value.title },
    },
  ]);
  addMenuOpened.value = false;
};

const props = defineProps({
  modelValue: {
    type: Array as PropType<FilterItem[]>,
    default: () => [],
  },
  headers: {
    type: Object as PropType<Record<string, FilterByOption>>,
    required: true,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
