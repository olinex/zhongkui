<template>
  <v-card flat>
    <v-card-title class="px-4 pt-3 pb-2 text-subtitle-1 font-weight-bold">
      {{ title }}
    </v-card-title>
    <v-card-text class="px-4 pt-0 pb-2">
      <expression-field v-model="filterBy" :headers="searchHeaders" />
    </v-card-text>
    <!-- 操作按钮行：独占一行、右对齐、紧贴列表上方；批量模式就地切换为批量操作（v-btn-group），不额外插入一行，避免列表垂直位移 -->
    <div class="px-4 pb-2 d-flex justify-end align-center ga-2">
      <template v-if="batchActions && selectItems.length > 0">
        <span class="text-body-2 text-no-wrap">{{
          $t("notice.selectedCount", { count: selectItems.length })
        }}</span>
        <v-btn-group variant="tonal" color="primary" density="compact" divided>
          <v-btn
            v-for="(action, key) in batchActions(selectItems)"
            :key="key"
            @click="action(selectItems)"
          >
            {{ $t(`action.${key}`) }}
          </v-btn>
        </v-btn-group>
        <v-btn density="compact" variant="text" @click="clearSelection">
          {{ $t("action.clearSelection") }}
        </v-btn>
      </template>
      <template v-else>
        <action-button-group
          :refresh="tableDataRefresh"
          :create="create"
          :loading="loading"
          collapsible
        >
          <column-visibility-menu
            v-model="visibleKeys"
            v-model:order="columnOrder"
            :headers="allToggleable"
          />
        </action-button-group>
      </template>
    </div>
    <v-container fluid class="px-4 pt-0 pb-4">
      <v-data-table-server
        v-model="selectItems"
        v-model:page="page"
        v-model:items-per-page="pageSize"
        v-model:sort-by="orderBy"
        :items="items"
        :items-length="total"
        :items-per-page-options="pageSizeOptions"
        :headers="selectTableHeaders"
        :loading="loading"
        select-strategy="page"
        :show-select="!!batchActions"
        return-object
      >
        <template #loading>
          <v-skeleton-loader :type="`table-row@${total || pageSize}`" />
        </template>
        <template #item="{ index, item, internalItem, isSelected, toggleSelect }">
          <tr class="text-no-wrap">
            <td
              v-if="batchActions"
              class="v-data-table-column--fixed v-data-table-column--last-fixed"
            >
              <v-checkbox-btn
                :model-value="isSelected(internalItem)"
                color="primary"
                density="compact"
                @update:model-value="toggleSelect(internalItem, index)"
              ></v-checkbox-btn>
            </td>
            <td
              v-for="hdr in visibleHeaders"
              :key="hdr.key"
              :class="
                hdr.fixed === 'end'
                  ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                  : undefined
              "
            >
              <!-- 允许页面按列名提供 cell-<key> 具名插槽覆盖默认渲染（如 model 列显示可读名称） -->
              <slot :name="`cell-${hdr.key}`" :item="item" :value="item[hdr.key]">
                <adaptive-cell
                  :header="hdr"
                  :item="item"
                  :actions="actions"
                  :refresh="tableDataRefresh"
                />
              </slot>
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
import { filterOptionTitle } from "@/utils/options";
import { useColumnVisibility } from "@/utils/column";
import type { FilterItem } from "@/components/forms/ExpressionField.vue";
import type { SortItem } from "vuetify/lib/components/VDataTable/composables/sort.mjs";
import type { BaseHeaders, ErrorResponse, Status, SuccessResponse } from "@/apis/@types/base";
import type {
  Align,
  BatchActions,
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
  success: (rsp: SuccessResponse<SearchOut<Item>>, headers: BaseHeaders) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>,
): Promise<Status> {
  return await getResponse<SearchOut<Item>>(request, success, failed, noResponse, noRequest, final);
}

async function getSearchOptionsResponse(
  request: AxiosPromise,
  success: (rsp: SuccessResponse<SearchOptionsOut>, headers: BaseHeaders) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>,
): Promise<Status> {
  return await getResponse<SearchOptionsOut>(
    request,
    success,
    failed,
    noResponse,
    noRequest,
    final,
  );
}

function extractSearchHeaders(out: SearchOptionsOut): Record<string, FilterByOption> {
  return out.filterBy.reduce(
    (prev, cur) => {
      prev[cur.name] = {
        name: cur.name,
        title: i18n.global.t(`model.${props.modelName}.field.${cur.name}`),
        actions: cur.actions,
        type: cur.type,
        // 枚举筛选项 title 为后端返回的枚举值本身，此处按 model.<Name>.option.<title>
        // 统一解析为可读展示名（未配置翻译时回退为枚举值，如 HTTP 方法）
        options: cur.options?.map((option) => ({
          ...option,
          title: filterOptionTitle(props.modelName, option.title),
        })),
      };
      return prev;
    },
    {} as Record<string, FilterByOption>,
  );
}

function extractTableHeaders(out: SearchOptionsOut): Array<TableHeader> {
  const headers: Array<TableHeader> = out.headers.map((hdr, idx) => {
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
      defaultVisible: hdr.defaultVisible,
    };
  });
  if (props.actions) {
    headers.push({
      key: "__actions",
      title: i18n.global.t(`model.__base.field.__actions`),
      type: BackendDataType.ACTIONS,
      align: "center",
      sortable: false,
      fixed: "end",
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
// Server response items are dynamic.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const items = ref([] as Record<string, any>[]);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const selectItems = ref([] as Record<string, any>[]);
const tableHeaders = ref([] as Array<TableHeader>);
const searchHeaders = ref({} as Record<string, FilterByOption>);

// selectHeader 定义最左侧多选列的配置，fixed: "start" 使其冻结在表格左边缘。
const selectHeader: TableHeader = {
  key: "data-table-select",
  title: "",
  type: BackendDataType.ACTIONS,
  fixed: "start",
  sortable: false,
};

// selectTableHeaders 将多选列置于字段列之前，供 v-data-table-server 渲染表头。
// 仅当存在批量操作（batchActions）时才插入多选列，与 #item 模板中的复选框单元格
// （v-if="batchActions"）保持一致，避免无批量操作页面表头多出一列导致列错位。
const selectTableHeaders = computed(() =>
  props.batchActions ? [selectHeader, ...visibleHeaders.value] : visibleHeaders.value,
);

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
  return filterBy.value.reduce(
    (prev, cur) => {
      prev[cur.field.value] = {
        action: cur.action.value,
        value: cur.value.value,
      };
      return prev;
    },
    {} as Record<string, FilterByItem>,
  );
});

// 筛选查询快照：序列化的实际查询载荷（action + value），用于识别筛选条件的“实际变更”，
// 从而忽略仅重开/重确认但仍产生等价模型值（如引用变化、Date 实例不一致）的无效变更。
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

// 筛选条件发生实际变更时立即刷新：排序（orderBy）与每页个数（pageSize）保持不变，
// 仅将页码重置为 1。仅当查询载荷真正变化时（比对 lastFilterBy 快照）才触发，
// 避免仅重开/重确认筛选菜单产生的等价变更导致无谓刷新。
watch(
  () => JSON.stringify(filterByOptions.value),
  (next, prev) => {
    if (next === prev) {
      return;
    }
    const wasFirstPage = page.value === 1;
    page.value = 1;
    if (wasFirstPage) {
      // 页码本就为 1（不会触发 watch(page)），需手动刷新；否则由 watch(page) 触发
      tableDataRefresh();
    }
  },
);

async function tableDataRefresh() {
  try {
    loading.value = true;
    // Server response items are dynamic.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
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
      },
    );
  } finally {
    loading.value = false;
  }
}

const props = defineProps({
  title: {
    type: String,
    default: "",
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
    // Row payload is dynamic.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Function as PropType<(item: Record<string, any>) => ItemActions>,
    default: undefined,
  },
  batchActions: {
    // Selected rows payload is dynamic.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Function as PropType<(items: Record<string, any>[]) => BatchActions>,
    default: undefined,
  },
  create: {
    type: Function as PropType<() => void>,
    default: undefined,
  },
  modelName: {
    type: String,
    required: true,
  },
  storageKey: {
    type: String,
    default: undefined,
  },
});

// 列显示配置：固定列始终展示，其余列按用户勾选显隐，并按 storageKey（缺省为 modelName）
// 持久化到 localStorage；表头加载完成后自动应用已保存配置
const { allToggleable, columnOrder, visibleKeys, visibleHeaders } = useColumnVisibility(
  props.storageKey ?? `model:${props.modelName}`,
  tableHeaders,
);

// clearSelection 清空勾选（批量操作完成后复位勾选状态）
const clearSelection = (): void => {
  selectItems.value = [];
};

defineExpose({
  refresh: tableDataRefresh,
  clearSelection,
  selectItems,
});
</script>
