<template>
  <v-combobox
    :model-value="modelValue"
    :label="$t('model.__base.field.search')"
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
            `{{ fieldTitleOf(asFilterItem(item)) }}`&nbsp;{{ asFilterItem(item).action.title }}
            <span v-show="asFilterItem(item).value.value !== null"
              >&nbsp;`{{ asFilterItem(item).value.title }}`</span
            >
          </v-chip>
        </template>
        <clause-field
          :model-value="asFilterItem(item)"
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
            <!-- 计数徽标：头像落在 prepend 槽并显式 start —— 只有 `v-avatar--start` 才带
                 Vuetify 为「chip 内前导头像」准备的内建间距（margin-inline-end）。
                 默认槽里的裸头像只会落进 .v-chip__content（inline-flex 且无 gap），
                 与文本紧贴；而 prepend 槽自身不注入 start 默认值（VChip 仅在设置了
                 prepend-icon / prepend-avatar 属性时才启用该 defaults），故必须显式声明 -->
            <template #prepend>
              <v-avatar start>{{ props.modelValue.length }}</v-avatar>
            </template>
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

// FilterItem 单个筛选条件项：字段、动作与取值，title 均为 i18n 键。
export interface FilterItem {
  field: { value: string; title: string };
  action: { value: string; title: string };
  // value 为动态值，仅做透传，不在此处加工。
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  value: { value: any; title: string };
}

// Vuetify 的 selection 槽运行期传入的是原始筛选项（VSelect.js 里 `item: item.raw`），
// 但槽位的静态类型被标注为 ListItem<any>，声明与实现不符——直接访问 .field / .action
// 会被报「类型上不存在该属性」。此处按运行时事实收口为 FilterItem，避免把类型断言散进模板。
const asFilterItem = (item: unknown): FilterItem => item as FilterItem;

const addMenuOpened = ref(false);

// fieldTitleOf 条件字段名：优先取当前筛选元信息中的标题，使链路槽位等
// 随上下文变化的字段名在已选条件 chip 中实时刷新；缺失时回退加入条件时的标题
const fieldTitleOf = (item: FilterItem): string => {
  return props.headers[item.field.value]?.title ?? item.field.title;
};

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
  // modelValue 已选筛选条件列表
  modelValue: {
    type: Array as PropType<FilterItem[]>,
    default: () => [],
  },
  // headers 可筛选字段及其可选动作的面板配置
  headers: {
    type: Object as PropType<Record<string, FilterByOption>>,
    required: true,
  },
});

// emits 更新已选筛选条件列表
const emits = defineEmits(["update:modelValue"]);
</script>
