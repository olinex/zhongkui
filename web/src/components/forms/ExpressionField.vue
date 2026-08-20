<template>
  <v-combobox
    :model-value="modelValue"
    :label="$t('model.__base.field.search')"
    density="compact"
    variant="outlined"
    return-object
    multiple
    readonly
    clearable
    @update:model-value="clearSelections"
    @click:clear="clearSelections"
    @click="addMenuOpened = true"
  >
    <template #selection="{ item, index }">
      <v-menu
        :key="index"
        location="top end"
        origin="top end"
        transition="scale-transition"
        :close-on-content-click="false"
      >
        <template #activator="{ props: activatorProps }">
          <v-chip
            v-bind="activatorProps"
            color="info"
            rounded
            closable
            @click:close="removeSelection(index)"
          >
            `{{ item.raw.field.title }}`&nbsp;{{ item.raw.action.title }}
            <span v-show="item.raw.value.value !== null">&nbsp;`{{ item.raw.value.title }}`</span>
          </v-chip>
        </template>
        <clause-field
          :model-value="item.raw"
          :selected-field-actions="selectedFieldActions"
          :headers="props.headers"
          :modify="true"
          @update:model-value="(item) => updateSelection(item, index)"
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
          <v-chip color="primary" v-bind="actProps" append-icon="mdi-plus-circle">
            <v-avatar>{{ props.modelValue.length }}</v-avatar>
            {{ $t("model.__base.content.condition") }}
          </v-chip>
        </template>
        <clause-field
          :selected-field-actions="selectedFieldActions"
          :headers="headers"
          @update:model-value="addSelection"
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
  // Value is dynamic and only passed through.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  value: { value: any; title: string };
}

const addMenuOpened = ref(false);

const selectedFieldActions = computed(() => {
  return props.modelValue.map((item) =>
    Object({ field: item.field.value, action: item.action.value }),
  );
});

const clearSelections = () => {
  emits("update:modelValue", []);
};

const removeSelection = (index: number) => {
  emits(
    "update:modelValue",
    props.modelValue.filter((_, i) => i !== index),
  );
};

const updateSelection = (item: FilterItem, index: number) => {
  emits(
    "update:modelValue",
    props.modelValue.map((i, j) => (j === index ? item : i)),
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
