<template>
  <v-card flat>
    <template #text>
      <scroll-search-field
        v-model="search"
        :order-by-field="orderByField"
        :search-headers="searchHeaders"
        :search-request="searchRequest"
      >
        <template #items="{ items, close }">
          <slot name="search" :items="items" :close="close" :appendItem="appendItem"/>
        </template>
      </scroll-search-field>
    </template>
    <v-data-table
      :items="modelValue"
      :items-per-page-options="pageSizeOptions"
      :headers="exactHeaders"
      :search="search"
      density="compact"
    >
      <template #item="{ item, index }">
        <tr class="text-no-wrap">
          <td v-show="listItemAction" key="__actions">
            <slot
              name="list-item-action"
              :item="item"
              :index="index"
              :editable="editable"
              :removeItem="() => removeItem(index)"
            />
          </td>
          <td v-for="hdr in headers" :key="hdr.key">
            <adaptive-cell :header="hdr" :item="item" />
          </td>
        </tr>
      </template>
    </v-data-table>
  </v-card>
</template>

<script setup lang="ts" name="inner-edit-table">
import type { PropType } from "vue";
import type { SearchIn, TableHeader } from "../@types/table";
import { BackendDataType, SearchAction } from "@/types";
import i18n from "@/utils/i18n";
import type { AxiosPromise } from "axios";

const pageSizeOptions = [
  { title: "10", value: 10 },
  { title: "20", value: 20 },
  { title: "50", value: 50 },
  { title: "100", value: 100 },
];

const search = ref("");

const exactHeaders = computed((): Array<TableHeader> => {
  if (props.listItemAction) {
    return [
      {
        key: "__actions",
        title: i18n.global.t(`action.remove`),
        type: BackendDataType.ACTIONS,
        align: "start",
        sortable: false,
      },
      ...props.headers,
    ];
  } else {
    return props.headers;
  }
});

const removeItem = (index: number) => {
  emits(
    "update:modelValue",
    props.modelValue.filter((_, idx) => idx !== index)
  );
};

const appendItem = (item: Record<string, any>) => {
  emits("update:modelValue", [...props.modelValue, item]);
};

const props = defineProps({
  modelValue: {
    type: Object as PropType<Array<Record<string, any>>>,
    required: true,
  },
  headers: {
    type: Object as PropType<Array<TableHeader>>,
    required: true,
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
  listItemAction: {
    type: Boolean,
    default: false,
  },
  editable: {
    type: Boolean,
    default: true,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>
