<template>
  <v-autocomplete
    :model-value="modelValue"
    @update:model-value="emits('update:modelValue', $event)"
    @update:focused="initAction"
    @update:search="searchAction"
    @blur="searchAction.clear"
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
  >
    <template #chip="{ props, item }">
      <slot name="chip" :props="props" :item="item" />
    </template>
    <template #item="{ props, item }">
      <slot name="item" :props="props" :item="item" />
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
const items = ref([] as Array<Record<string, any>>);

const initAction = async () => {
  if (props.init && items.value.length === 0) {
    searchAction.immediate();
  }
}

const searchAction = debounce(async (val: string) => {
  loading.value = true;
  items.value = [];
  getResponse<SearchOut<Record<string, any>>>(
    props.searchRequest({
      greedy: true,
      page: 1,
      pageSize: 100,
      orderBy: [{ name: props.orderByField, order: 1 }],
      filterBy: Object.entries(props.searchHeaders).reduce(
        (prev, [field, action]) => {
          if (props.modelValue === "") {
            return prev;
          }
          prev[field] = {
            action: action,
            value: props.modelValue,
          };
          return prev;
        },
        {} as Record<string, FilterByItem>
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
    }
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
  }
});

const emits = defineEmits(["update:modelValue"]);
</script>
