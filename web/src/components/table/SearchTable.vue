<template>
  <v-card flat>
    <v-card-title class="d-flex align-center pe-2">
      {{ title }}
      <v-spacer />
      <slot name="top" :refresh="tableDataRefresh" />
      <v-btn
        :disabled="loading"
        icon="mdi-refresh"
        variant="text"
        @click="tableDataRefresh"
      />
    </v-card-title>
    <v-card-text>
      <expression-field v-model="filterBy" :headers="searchHeaders" />
    </v-card-text>
    <v-container>
      <v-data-table-server
        v-model="selectItems"
        v-model:page="page"
        v-model:items-per-page="pageSize"
        v-model:sort-by="orderBy"
        :items="items"
        :items-length="total"
        :items-per-page-options="pageSizeOptions"
        :headers="tableHeaders"
        :loading="loading"
        select-strategy="page"
        show-select
        return-object
      >
        <template #loading>
          <v-skeleton-loader :type="`table-row@${total || pageSize}`" />
        </template>
        <template
          #item="{ index, item, internalItem, isSelected, toggleSelect }"
        >
          <tr class="text-no-wrap">
            <td>
              <v-checkbox-btn
                :model-value="isSelected(internalItem)"
                color="primary"
                @update:model-value="toggleSelect(internalItem, index)"
              ></v-checkbox-btn>
            </td>
            <td v-for="hdr in tableHeaders" :key="hdr.key">
              <adaptive-cell
                :header="hdr"
                :item="item"
                :actions="actions"
                :refresh="tableDataRefresh"
              />
            </td>
          </tr>
        </template>
      </v-data-table-server>
    </v-container>
  </v-card>
</template>

<script setup lang="ts" name="search-table">
import type { AxiosPromise } from "axios";
import { getResponse } from "@/apis/base";
import i18n from "@/utils/i18n";
import type { FilterItem } from "@/components/forms/ExpressionField.vue";
import type { SortItem } from "vuetify/lib/components/VDataTable/composables/sort.mjs";
import type {
  BaseHeaders,
  ErrorResponse,
  Status,
  SuccessResponse,
} from "@/apis/@types/base";
import type {
  Align,
  FilterByItem,
  FilterByOption,
  ItemActions,
  SearchIn,
  SearchOptionsOut,
  SearchOut,
  TableHeader,
} from "../@types/table";
import { BackendDataType } from "@/types";

async function getSearchResponse<Item>(
  request: AxiosPromise,
  success: (
    rsp: SuccessResponse<SearchOut<Item>>,
    headers: BaseHeaders
  ) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>
): Promise<Status> {
  return await getResponse<SearchOut<Item>>(
    request,
    success,
    failed,
    noResponse,
    noRequest,
    final
  );
}

async function getSearchOptionsResponse(
  request: AxiosPromise,
  success: (
    rsp: SuccessResponse<SearchOptionsOut>,
    headers: BaseHeaders
  ) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>
): Promise<Status> {
  return await getResponse<SearchOptionsOut>(
    request,
    success,
    failed,
    noResponse,
    noRequest,
    final
  );
}

function extractSearchHeaders(
  out: SearchOptionsOut
): Record<string, FilterByOption> {
  return out.filterBy.reduce((prev, cur) => {
    prev[cur.name] = {
      name: cur.name,
      title: i18n.global.t(`model.${props.modelName}.field.${cur.name}`),
      actions: cur.actions,
      type: cur.type,
    };
    return prev;
  }, {} as Record<string, FilterByOption>);
}

function extractTableHeaders(out: SearchOptionsOut): Array<TableHeader> {
  const headers = out.headers.map((hdr, idx) => {
    let align: Align = "center";
    switch (idx) {
      case 0:
        align = "start";
      case out.headers.length:
        align = "end";
    }
    return {
      key: hdr.name,
      title: i18n.global.t(`model.${props.modelName}.field.${hdr.name}`),
      type: hdr.type,
      align: align,
      sortable: out.orderBy.includes(hdr.name),
    };
  });
  if (props.actions) {
    headers.push({
      key: "__actions",
      title: i18n.global.t(`model.__base.field.__actions`),
      type: BackendDataType.ACTIONS,
      align: "center",
      sortable: false,
    });
  }
  return headers;
}

const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(10);
const orderBy = ref([] as SortItem[]);
const filterBy = ref([] as FilterItem[]);
const items = ref([] as Record<string, any>[]);
const selectItems = ref([] as Record<string, any>[]);
const tableHeaders = ref([] as Array<TableHeader>);
const searchHeaders = ref({} as Record<string, FilterByOption>);

const pageSizeOptions = [
  { title: "10", value: 10 },
  { title: "20", value: 20 },
  { title: "50", value: 50 },
  { title: "100", value: 100 },
];

const orderByOptions = computed(() => {
  return orderBy.value.map((s) => ({
    name: s.key,
    order: s.order === "asc" || s.order === true ? 1 : -1,
  }));
});

const filterByOptions = computed(() => {
  return filterBy.value.reduce((prev, cur) => {
    prev[cur.field.value] = {
      action: cur.action.value,
      value: cur.value.value,
    };
    return prev;
  }, {} as Record<string, FilterByItem>);
});

onBeforeMount(async () => {
  await getSearchOptionsResponse(props.optionsRequest(), async (rsp) => {
    searchHeaders.value = extractSearchHeaders(rsp.data);
    tableHeaders.value = extractTableHeaders(rsp.data);
  });
});

onMounted(tableDataRefresh);
watch(page, tableDataRefresh);
watch(pageSize, tableDataRefresh);
watch(orderBy, tableDataRefresh);

async function tableDataRefresh() {
  try {
    loading.value = true;
    await getSearchResponse<Record<string, any>>(
      props.listRequest({
        greedy: false,
        page: page.value,
        pageSize: pageSize.value,
        orderBy: orderByOptions.value,
        filterBy: filterByOptions.value,
      }),
      async (rsp) => {
        total.value = rsp.data.total;
        items.value = rsp.data.items;
      }
    );
  } finally {
    loading.value = false;
  }
}

const props = defineProps({
  title: {
    type: String,
    required: true,
  },
  optionsRequest: {
    type: Function as PropType<() => AxiosPromise>,
    required: true,
  },
  listRequest: {
    type: Function as PropType<(req: SearchIn) => AxiosPromise>,
    required: true,
  },
  actions: {
    type: Function as PropType<(item: Record<string, any>) => ItemActions>,
  },
  modelName: {
    type: String,
    required: true,
  },
});

defineExpose({
  refresh: tableDataRefresh,
});
</script>
