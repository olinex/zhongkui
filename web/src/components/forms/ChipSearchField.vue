<template>
  <v-autocomplete
    :model-value="modelValue"
    :label="label"
    :items="items"
    :loading="loading"
    :item-title="itemTitle"
    :item-value="itemValue"
    :return-object="returnObject"
    :disabled="disabled"
    :multiple="multiple"
    :clearable="clearable"
    :closable-chip="clearable"
    chips
    @update:model-value="emits('update:modelValue', $event)"
    @update:focused="initAction"
    @update:search="searchAction"
    @blur="searchAction.clear"
  >
    <template #chip="{ props: chipProps, item }">
      <slot name="chip" :props="chipProps" :item="item" />
    </template>
    <template #item="{ props: itemProps, item }">
      <slot name="item" :props="itemProps" :item="item" />
    </template>
  </v-autocomplete>
</template>

<script setup lang="ts" name="chip-search-field">
import type { SearchAction } from "@/types";
import type { FilterByItem, SearchIn, SearchOut } from "../@types/table";
import type { AxiosPromise } from "axios";
import { getResponse } from "@/apis/base";
import { useAppStore } from "@/stores/app";
import { debounce } from "vuetify/lib/util/helpers.mjs";
import type { PropType } from "vue";

const appStore = useAppStore();
const loading = ref(false);
// Server response items are dynamic.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const items = ref([] as Array<Record<string, any>>);

const initAction = async () => {
  if (props.init && items.value.length === 0) {
    searchAction.immediate();
  }
};

const searchAction = debounce(async () => {
  loading.value = true;
  items.value = [];
  // Server response items are dynamic.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  getResponse<SearchOut<Record<string, any>>>(
    props.searchRequest({
      greedy: true,
      page: 1,
      pageSize: 100,
      orderBy: [{ name: props.orderByField, order: 1 }],
      filterBy: Object.entries(props.searchHeaders).reduce(
        (prev, [field, action]) => {
          // 搜索关键字去除首尾空白，避免误输入空白导致查询无结果
          const keyword =
            typeof props.modelValue === "string" ? props.modelValue.trim() : props.modelValue;
          if (keyword === "" || keyword === undefined || keyword === null) {
            return prev;
          }
          prev[field] = {
            action: action,
            value: keyword,
          };
          return prev;
        },
        {} as Record<string, FilterByItem>,
      ),
    }),
    async (rsp) => {
      items.value = rsp.data.items;
      loading.value = false;
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
      loading.value = false;
    },
  );
}, 3000);

const props = defineProps({
  modelValue: {
    type: [String, Number, Boolean, null],
    required: false,
    default: undefined,
  },
  orderByField: {
    type: String,
    default: "id",
  },
  searchHeaders: {
    type: Object as PropType<Record<string, SearchAction>>,
    required: true,
  },
  searchRequest: {
    type: Function as PropType<(req: SearchIn) => AxiosPromise>,
    required: true,
  },
  label: {
    type: String,
    required: true,
  },
  itemTitle: {
    type: String,
    default: "name",
  },
  itemValue: {
    type: String,
    default: "id",
  },
  multiple: {
    type: Boolean,
    default: false,
  },
  returnObject: {
    type: Boolean,
    default: false,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  clearable: {
    type: Boolean,
    default: false,
  },
  init: {
    type: Boolean,
    default: false,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
