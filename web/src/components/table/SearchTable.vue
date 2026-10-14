<template>
  <v-card flat>
    <v-card-title class="px-4 pt-3 pb-2 text-body-large font-weight-bold">
      {{ title }}
    </v-card-title>
    <!-- 筛选区：高级条件编辑器常驻一行；页面可通过 extra-filters 插槽追加额外筛选字段（如流量时间窗口）。
         查询按钮统一收纳在下方操作按钮组（action-button-group）第一顺位，保持列表页行为一致 -->
    <v-card-text class="px-4 pt-0 pb-2">
      <expression-field v-model="filterBy" :headers="searchHeaders" />
      <slot name="extra-filters" />
    </v-card-text>
    <!-- 操作按钮行：独占一行、右对齐、紧贴列表上方；批量模式就地切换为批量操作（v-btn-group），不额外插入一行，避免列表垂直位移 -->
    <div class="px-4 pb-2 d-flex justify-end align-center ga-2">
      <template v-if="batchActions && selectItems.length > 0">
        <span class="text-body-medium text-no-wrap">{{
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
        <v-btn variant="text" @click="clearSelection">
          {{ $t("action.clearSelection") }}
        </v-btn>
      </template>
      <template v-else>
        <action-button-group
          :query="triggerQuery"
          :refresh="tableDataRefresh"
          :create="create"
          :actions="toolbarActions"
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
        :hover="!!rowClick"
        select-strategy="page"
        :show-select="!!batchActions"
        return-object
        @click:row="onRowClick"
      >
        <template #loading>
          <v-skeleton-loader :type="`table-row@${total || pageSize}`" />
        </template>
        <template #item="{ index, item, internalItem, isSelected, toggleSelect }">
          <tr class="text-no-wrap" :class="{ 'row-clickable': !!rowClick }">
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
              :class="[
                hdr.align ? `v-data-table-column--align-${hdr.align}` : undefined,
                hdr.fixed === 'end'
                  ? 'v-data-table-column--fixed-end v-data-table-column--first-fixed-end'
                  : undefined,
              ]"
            >
              <!-- 允许页面按列名提供 cell-<key> 具名插槽覆盖默认渲染（如 model 列显示可读名称）；
                   或经通用 cell 插槽统一接管全部单元格（如流量页加密/S3 单元格分发） -->
              <slot :name="`cell-${hdr.key}`" :item="item" :value="item[hdr.key]">
                <slot name="cell" :item="item" :header="hdr">
                  <adaptive-cell
                    :header="hdr"
                    :item="item"
                    :actions="actions"
                    :refresh="tableDataRefresh"
                  />
                </slot>
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
import { enumFieldTypeOfMeta, enumOptionsOf, ensureEnum } from "@/utils/enums";
import { useColumnVisibility } from "@/utils/column";
import type { FilterItem } from "@/components/forms/ExpressionField.vue";
import type { ActionButton } from "@/components/base/ActionButtonGroup.vue";
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
import type { MetaOut } from "@/components/@types/meta";
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
  success: (rsp: SuccessResponse<MetaOut>, headers: BaseHeaders) => Promise<void>,
  failed?: (rsp: ErrorResponse, headers: BaseHeaders) => Promise<void>,
  noResponse?: (req: XMLHttpRequest) => Promise<void>,
  noRequest?: (msg: string) => Promise<void>,
  final?: () => Promise<void>,
): Promise<Status> {
  return await getResponse<MetaOut>(request, success, failed, noResponse, noRequest, final);
}

async function extractSearchHeaders(out: MetaOut): Promise<Record<string, FilterByOption>> {
  // 枚举字段经 enumType（filterBy 显式声明，缺省回退 bodyOutput 自省）确定枚举类名，
  // 先统一 ensure 全部枚举类按当前语言缓存，再构造筛选元数据
  const enumTypes = new Set<string>();
  for (const cur of out.filterBy) {
    const enumType = cur.enumType ?? enumFieldTypeOfMeta(out.meta, cur.name);
    if (enumType) enumTypes.add(enumType);
  }
  await Promise.all([...enumTypes].map((t) => ensureEnum(t)));
  return out.filterBy.reduce(
    (prev, cur) => {
      // 枚举类名命中则为枚举下拉（options 经枚举 store 解析），未命中保持后端字段类型
      const enumType = cur.enumType ?? enumFieldTypeOfMeta(out.meta, cur.name);
      prev[cur.name] = enumType
        ? {
            ...cur,
            title: i18n.global.t(`model.${props.modelName}.field.${cur.name}`),
            type: BackendDataType.ENUM,
            // 显式 options（如通配 "*"）优先；否则经枚举 store 按类解析
            options: cur.options?.length ? cur.options : enumOptionsOf(enumType),
          }
        : {
            ...cur,
            title: i18n.global.t(`model.${props.modelName}.field.${cur.name}`),
            options: cur.options,
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
    // 枚举列 class 名：header 显式声明优先，缺省回退 bodyOutput 自省的 enumType；
    // 仅存在时写入，避免 undefined 键污染
    const enumType = hdr.enumType ?? enumFieldTypeOfMeta((out as MetaOut).meta, hdr.name);
    const header: TableHeader = {
      key: hdr.name,
      title: i18n.global.t(`model.${props.modelName}.field.${hdr.name}`),
      type: hdr.type,
      align: align,
      sortable: out.orderBy.includes(hdr.name),
      defaultVisible: hdr.defaultVisible,
    };
    if (enumType) {
      header.enumType = enumType;
    }
    return header;
  });
  if (props.actions) {
    // align / fixed 由 useColumnVisibility 依操作列约定统一覆写（居中 + 固定置尾）
    headers.push({
      key: "__actions",
      title: i18n.global.t(`model.__base.field.__actions`),
      type: BackendDataType.ACTIONS,
      sortable: false,
    });
  }
  return headers;
}

const props = defineProps({
  title: {
    type: String,
    default: "",
  },
  optionsRequest: {
    // 缺省时列表表头/筛选字段元数据由页面经 headers / filterHeaders 自定义源提供；
    // 允许异步：页面可先 await 枚举类缓存（如 notificationTemplateListOptions）再返回选项
    type: Function as PropType<() => AxiosPromise | Promise<AxiosPromise>>,
    default: undefined,
  },
  listRequest: {
    type: Function as PropType<(req: SearchIn) => AxiosPromise>,
    required: true,
  },
  actions: {
    // 行载荷为动态结构。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Function as PropType<(item: Record<string, any>) => ItemActions>,
    default: undefined,
  },
  batchActions: {
    // 选中行的载荷为动态结构。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Function as PropType<(items: Record<string, any>[]) => BatchActions>,
    default: undefined,
  },
  create: {
    type: Function as PropType<() => void>,
    default: undefined,
  },
  toolbarActions: {
    // 工具栏次级操作按钮（收起时隐藏），透传给 action-button-group
    type: Array as PropType<ActionButton[]>,
    default: () => [],
  },
  modelName: {
    // 经 OPTIONS 派生表头/筛选字段时用于 model.<modelName>.field.* 翻译；自定义源页面无需提供
    type: String,
    default: "",
  },
  storageKey: {
    type: String,
    default: undefined,
  },
  // 自定义查询触发：页面可传 vee-validate handleSubmit 等先做表单校验再发起查询；缺省直接执行默认查询
  query: {
    type: Function as PropType<() => void | Promise<void>>,
    default: undefined,
  },
  // 查询载荷过滤器：在默认 filterBy 基础上追加/改写筛选条件（如流量页注入时间窗口与命名空间）；
  // 返回 null 表示当前条件不合法（如必填时间范围缺失），跳过本次查询
  buildFilter: {
    type: Function as PropType<
      (filterBy: Record<string, FilterByItem>) => Record<string, FilterByItem> | null
    >,
    default: undefined,
  },
  // 字段投影：默认按当前可见列 + id 生成 fields 请求参数；
  // 页面可覆写以补充行操作所需的隐藏字段（如用户页 unionID、流量页 namespace/traceID）
  projectFields: {
    type: Function as PropType<(visibleKeys: string[]) => string[]>,
    default: (visible: string[]) => ["id", ...visible],
  },
  // 自定义表格表头源（覆盖 OPTIONS 派生）：数组、ref 或函数，供需要定制列类型/顺序/默认可见性的页面使用
  headers: {
    type: [Object, Function] as PropType<
      Array<TableHeader> | Ref<Array<TableHeader>> | (() => Array<TableHeader>)
    >,
    default: undefined,
  },
  // 自定义高级筛选字段元数据源（覆盖 OPTIONS 派生）：对象、ref 或函数
  filterHeaders: {
    type: [Object, Function] as PropType<
      | Record<string, FilterByOption>
      | Ref<Record<string, FilterByOption>>
      | (() => Record<string, FilterByOption>)
    >,
    default: undefined,
  },
  // 默认排序：初始生效，用户在表头调整后由组件内部接管
  defaultOrderBy: {
    type: Array as PropType<SortItem[]>,
    default: () => [],
  },
  // 行点击回调：提供后表格行可点击（hover 高亮 + pointer 光标），用于行详情下钻
  rowClick: {
    // 表格行的值类型不固定。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type: Function as PropType<(item: Record<string, any>) => void>,
    default: undefined,
  },
});

const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(10);
// 默认排序克隆自 props.defaultOrderBy，避免列表排序时原地改写传入数组
const orderBy = ref([...(props.defaultOrderBy ?? [])]);
const filterBy = ref([] as FilterItem[]);
// 服务端响应条目为动态结构。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const items = ref([] as Record<string, any>[]);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const selectItems = ref([] as Record<string, any>[]);
// OPTIONS 派生的表头/筛选字段元数据（无自定义源时使用）
const optionsTableHeaders = ref([] as Array<TableHeader>);
const optionsSearchHeaders = ref({} as Record<string, FilterByOption>);

// 表格表头：优先取页面自定义源（headers prop），否则由 OPTIONS 派生。
// 行操作列（__actions）的居中 + 置尾由 useColumnVisibility 统一覆写，此处无需处理
const tableHeaders = computed((): Array<TableHeader> => {
  return props.headers ? toValue(props.headers) : optionsTableHeaders.value;
});
// 高级筛选字段元数据：优先取页面自定义源（filterHeaders prop），否则由 OPTIONS 派生
const searchHeaders = computed((): Record<string, FilterByOption> => {
  return props.filterHeaders ? toValue(props.filterHeaders) : optionsSearchHeaders.value;
});

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

// 拉取 OPTIONS：仅当未提供自定义表头/筛选字段源时执行，结果用于派生表格表头与高级筛选字段
onBeforeMount(async () => {
  // 自定义表头/筛选字段源页面无需 OPTIONS；仅 OPTIONS 派生表头时需等待其就绪后再发起首次查询
  const optionsRequest = props.optionsRequest;
  if (!optionsRequest) {
    return;
  }
  // optionsRequest 允许返回 Promise（异步预取枚举类后返回选项）；Promise.resolve 统一归一：
  // 真 AxiosPromise（axios）原样透传，页面返回的包装 Promise 解包为响应对象后走同一解析路径。
  // getResponse 调用 success 回调时不 await（extractSearchHeaders 内部仍 await ensureEnum），
  // getSearchOptionsResponse 返回时表头可能尚未填充；故以显式 Promise 门闩等待表头真正就绪，
  // 再发起首次查询，避免可见列尚未填充使字段投影遗漏 createdAt/updatedAt 等默认可见列，
  // 导致列表返回字段不全（首屏单元格显示为未定义）
  const headersReady = new Promise<void>((resolve) => {
    void getSearchOptionsResponse(
      Promise.resolve(optionsRequest()),
      async (rsp) => {
        try {
          optionsSearchHeaders.value = await extractSearchHeaders(rsp.data);
          optionsTableHeaders.value = extractTableHeaders(rsp.data);
        } finally {
          resolve();
        }
      },
      async () => resolve(),
      async () => resolve(),
      async () => resolve(),
    );
  });
  await headersReady;
  // 表头就绪后 useColumnVisibility 的可见列派生（pre flush watch）才在本组件调度，
  // nextTick 确保可见列填充完成后再发起首次查询
  await nextTick();
  tableDataRefresh();
});

// 无 OPTIONS（页面自定义表头/筛选字段源，如流量页）时表头在挂载即就绪，按挂载即查
onMounted(() => {
  if (!props.optionsRequest) {
    tableDataRefresh();
  }
});
watch(page, tableDataRefresh);
watch(pageSize, tableDataRefresh);
watch(orderBy, tableDataRefresh);

async function tableDataRefresh() {
  // 默认查询载荷：高级条件编辑器序列化的筛选条件；自定义载荷过滤器可追加/改写
  const baseFilter = filterByOptions.value;
  let queryFilterBy: Record<string, FilterByItem> | null = baseFilter;
  if (props.buildFilter) {
    queryFilterBy = props.buildFilter(baseFilter);
  }
  // 自定义载荷过滤器返回 null 表示当前条件不合法（如必填时间范围缺失），跳过查询
  if (queryFilterBy === null) {
    return;
  }
  // 字段投影：按当前可见列 + 页面补充字段生成 fields 请求参数，列表仅返回所需字段
  const fields = props.projectFields(visibleKeys.value);
  try {
    loading.value = true;
    // 服务端响应条目为动态结构。
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    await getSearchResponse<Record<string, any>>(
      props.listRequest({
        greedy: false,
        page: page.value,
        pageSize: pageSize.value,
        orderBy: orderByOptions.value,
        filterBy: queryFilterBy,
        // 字段投影：仅查询所需字段（结果非空时附加）
        ...(fields.length > 0 ? { fields } : {}),
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

// startLoading 立即清空旧列表并进入加载态（骨架屏）：
// 数据源切换（如流量页切换类型）时表头已即时更新，但旧数据仍驻留列表，
// 在新表头下会错位/空值展示；故先清空并置加载态，加载期间仅显示骨架，
// 真实查询由调用方随后触发（refresh/query），加载态持续至数据返回
function startLoading() {
  items.value = [];
  total.value = 0;
  loading.value = true;
}

// doQuery 显式查询：筛选条件实际变更后由查询按钮触发，页码重置为 1 再刷新；
// 页码本就为 1（不会触发 watch(page)）时需手动刷新，否则由 watch(page) 触发
async function doQuery() {
  const wasFirstPage = page.value === 1;
  page.value = 1;
  if (wasFirstPage) {
    await tableDataRefresh();
  }
}

// triggerQuery 查询按钮统一入口：优先执行页面自定义查询（可含表单校验），否则默认查询
const triggerQuery = async (): Promise<void> => {
  if (props.query) {
    await props.query();
  } else {
    await doQuery();
  }
};

// onRowClick 行点击统一处理：仅在提供 rowClick 回调时下钻行详情等操作
const onRowClick = (
  _event: Event,
  // 表格行的值类型不固定。
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: { item: Record<string, any> },
): void => {
  props.rowClick?.(data.item);
};

// 列显示配置：固定列始终展示，其余列按用户勾选显隐，并按 storageKey（缺省为 modelName）
// 持久化到 localStorage；表头加载完成后自动应用已保存配置。storageKey 以 getter 传入，
// 页面切换类型等场景（如流量页 traffic:<type>）可随 prop 变化切换各自保存的列配置。
const { allToggleable, columnOrder, visibleKeys, visibleHeaders } = useColumnVisibility(
  () => props.storageKey ?? `model:${props.modelName}`,
  tableHeaders,
);

// clearSelection 清空勾选（批量操作完成后复位勾选状态）
const clearSelection = (): void => {
  selectItems.value = [];
};

defineExpose({
  refresh: tableDataRefresh,
  query: doQuery,
  clearSelection,
  startLoading,
  selectItems,
  // 当前列表行：供页面按行状态派生的行为（如知识库页对 pending/parsing 行的有界轮询）读取
  items,
  filterBy,
});
</script>

<style scoped>
/* 行可点击（rowClick）时展示 pointer 光标，提示可下钻行详情 */
.row-clickable {
  cursor: pointer;
}
</style>
