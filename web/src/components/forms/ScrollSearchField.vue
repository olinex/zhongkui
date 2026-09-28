<template>
  <v-text-field
    :model-value="modelValue"
    :label="$t(`model.__base.field.search`)"
    prepend-inner-icon="mdi-magnify"
    hide-details
    single-line
    rounded
    @update:model-value="emits('update:modelValue', $event)"
  >
    <template #append-inner>
      <v-menu
        v-model="open"
        location="bottom end"
        transition="scale-transition"
        :close-on-content-click="false"
      >
        <template #activator="{ props: activatorProps }">
          <v-icon v-bind="activatorProps" color="primary" @click="more">mdi-more</v-icon>
        </template>
        <v-card>
          <v-card-text>
            <!-- 自有滚动容器：仅在真正滚动到底时按下拉加载下一页，避免无限触发请求 -->
            <div ref="scrollEl" class="pt-0 scroll-list">
              <slot name="items" :items="items" :close="() => (open = false)" />
              <div ref="sentinelEl" class="d-flex align-center justify-center pa-2">
                <span v-if="loading" class="text-body-2 text-on-surface-variant">{{
                  $t("summary.dataLoading")
                }}</span>
                <span
                  v-else-if="!hasMore && items.length > 0"
                  class="text-body-2 text-on-surface-variant"
                  >{{ $t("summary.endOfData") }}</span
                >
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-menu>
    </template>
  </v-text-field>
</template>

<script setup lang="ts" name="scroll-search-field">
import { getResponse } from "@/apis/base";
import { SearchAction } from "@/types";
import type { AxiosPromise } from "axios";
import type { PropType } from "vue";
import type { FilterByItem, SearchIn, SearchOut } from "../@types/table";
import { useAppStore } from "@/stores/app";

const appStore = useAppStore();
const open = ref(false);
const page = ref(1);
const scrollEl = ref<HTMLElement | null>(null);
const sentinelEl = ref<HTMLElement | null>(null);
const loading = ref(false);
const hasMore = ref(true);
// 服务端返回的条目为动态结构。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const items = ref([] as Array<Record<string, any>>);

let observer: IntersectionObserver | null = null;

const more = () => {
  page.value = 1;
  items.value = [];
  // 重置 hasMore：否则首次加载拉满后 hasMore 变 false，再次打开因守卫直接返回而无数据
  hasMore.value = true;
  open.value = true;
  setupScroll();
};

// 首屏加载 + 兜底：菜单打开时初始化一次
watch(open, (val) => {
  if (val) setupScroll();
});

// 仅在哨兵进入滚动容器可视区时加载下一页
const loadMore = () => {
  if (loading.value || !hasMore.value) {
    return;
  }
  loading.value = true;
  // 服务端返回的条目为动态结构。
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  getResponse<SearchOut<Record<string, any>>>(
    props.searchRequest({
      greedy: true,
      page: page.value,
      pageSize: props.pageSize,
      orderBy: [{ name: props.orderByField, order: 1 }],
      filterBy: Object.entries(props.searchHeaders).reduce(
        (prev, [field, action]) => {
          // 搜索关键字去除首尾空白，避免误输入空白导致查询无结果
          const keyword = props.modelValue.trim();
          if (keyword === "") {
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
      loading.value = false;
      if (rsp.data.items.length === 0) {
        hasMore.value = false;
        return;
      }
      items.value = items.value.concat(rsp.data.items);
      page.value++;
      if (items.value.length >= rsp.data.total) {
        hasMore.value = false;
      }
    },
    async (rsp) => {
      loading.value = false;
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// 重建观察器：以滚动容器为 root，哨兵相交即触发加载
const setupScroll = () => {
  observer?.disconnect();
  observer = null;
  if (!open.value) {
    return;
  }
  nextTick(() => {
    if (!scrollEl.value || !sentinelEl.value) {
      return;
    }
    observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) {
          loadMore();
        }
      },
      { root: scrollEl.value, rootMargin: "100px" },
    );
    observer.observe(sentinelEl.value);
    loadMore();
  });
};

const props = defineProps({
  modelValue: {
    type: String,
    required: true,
  },
  orderByField: {
    type: String,
    default: "id",
  },
  pageSize: {
    type: Number,
    default: 10,
  },
  searchHeaders: {
    type: Object as PropType<Record<string, SearchAction>>,
    required: true,
  },
  searchRequest: {
    type: Function as PropType<(req: SearchIn) => AxiosPromise>,
    required: true,
  },
});

const emits = defineEmits(["update:modelValue"]);
</script>

<style scoped>
/* 下拉候选列表滚动容器：限制最大高度，超出内部滚动（IntersectionObserver 以此容器为 root） */
.scroll-list {
  max-height: var(--zk-dropdown-max-height);
  overflow-y: auto;
}
</style>
