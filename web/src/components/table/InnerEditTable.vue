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
          <slot name="search" :items="items" :close="close" :append-item="appendItem">
            <!-- 统一候选条目渲染：紧凑 v-list-item 风格，通过 item-title / item-subtitle / item-append 插槽定制 -->
            <v-list :items="items">
              <v-list-item v-for="item in items" :key="item[itemKeyField]" class="px-1">
                <v-list-item-title class="text-body-medium">
                  <slot name="item-title" :item="item">
                    <span class="font-weight-medium">{{ item.name }}</span>
                  </slot>
                </v-list-item-title>
                <template v-if="subtitleField || $slots['item-subtitle']">
                  <v-list-item-subtitle>
                    <slot name="item-subtitle" :item="item">
                      {{ item[subtitleField] }}
                    </slot>
                  </v-list-item-subtitle>
                </template>
                <template #append>
                  <div class="d-flex align-center ga-1">
                    <slot
                      name="item-append"
                      :item="item"
                      :append-item="appendItem"
                      :close="close"
                      :editable="editable"
                      :bound="isBound(item)"
                    >
                      <v-btn
                        :disabled="!editable || isBound(item)"
                        :text="$t('action.add')"
                        color="primary"
                        variant="tonal"
                        size="small"
                        @click="appendItem(item)"
                      />
                    </slot>
                  </div>
                </template>
              </v-list-item>
            </v-list>
          </slot>
        </template>
      </scroll-search-field>
    </template>
    <v-data-table
      :items="modelValue"
      :items-per-page-options="pageSizeOptions"
      :headers="exactHeaders"
      :search="search"
      virtual-scrolling
      fixed-header
      max-height="360px"
    >
      <template #item="{ item, index }">
        <tr class="text-no-wrap">
          <td
            v-show="listItemAction"
            key="__actions"
            :class="[
              'v-data-table-column--align-center',
              listItemAction
                ? 'v-data-table-column--fixed v-data-table-column--last-fixed'
                : undefined,
            ]"
          >
            <slot
              name="list-item-action"
              :item="item"
              :index="index"
              :editable="editable"
              :remove-item="() => removeItem(index)"
            />
          </td>
          <td
            v-for="hdr in headers"
            :key="hdr.key"
            :class="hdr.align ? `v-data-table-column--align-${hdr.align}` : undefined"
          >
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
        // 行级按钮列统一水平居中（固定列保持在左端的移除按钮）
        align: "center",
        sortable: false,
        fixed: "start",
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
    props.modelValue.filter((_, idx) => idx !== index),
  );
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const appendItem = (item: Record<string, any>) => {
  emits("update:modelValue", [...props.modelValue, item]);
};

const props = defineProps({
  modelValue: {
    // 行载荷为动态结构。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
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
  // 候选条目循环与去重使用的唯一键字段
  itemKeyField: {
    type: String,
    default: "id",
  },
  // 默认候选条目副标题字段名（未配置且未提供 item-subtitle 插槽时不显示副标题）
  subtitleField: {
    type: String,
    default: "",
  },
});

const emits = defineEmits(["update:modelValue"]);

// 候选条目是否已被选（按 itemKeyField 比对），供默认「添加」按钮与 item-append 插槽使用
// 行载荷为动态结构。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const isBound = (item: Record<string, any>): boolean => {
  return props.modelValue.some((x) => x[props.itemKeyField] === item[props.itemKeyField]);
};
</script>
