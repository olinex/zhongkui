import type { Ref } from "vue";
import { computed, ref, toValue, watch } from "vue";

import type { TableHeader } from "@/components/@types/table";
import { useConfigSyncStore } from "@/stores/configSync";

// 表格列配置在 localStorage 中的存储键前缀
const COLUMN_STORAGE_PREFIX = "table-columns:";

// 行操作列（__actions）约定：该列专用于摆放行级按钮，统一水平居中并固定置尾。
// 在共享列工具统一覆写，页面 / 表格无需（也不应）就地声明其 align / fixed，
// 避免各列表页（SearchTable 与原生 v-data-table）对齐不一致。
const ACTIONS_COLUMN_KEY = "__actions";

const normalizeActionsHeader = (headers: Array<TableHeader>): Array<TableHeader> => {
  return headers.map((hdr) =>
    hdr.key === ACTIONS_COLUMN_KEY ? { ...hdr, align: "center", fixed: "end" } : hdr,
  );
};

// 持久化的列配置：order 为全量可切换列顺序，visible 为可见列子集（顺序遵循 order）
interface ColumnConfig {
  order: string[];
  visible: string[];
}

// 过滤出字符串 key 数组，忽略脏数据
const toKeys = (value: unknown): string[] => {
  return Array.isArray(value) ? value.filter((key): key is string => typeof key === "string") : [];
};

// 从 localStorage 读取列配置（order 为全量可切换列顺序，visible 为可见列子集）
const loadColumnConfig = (storageKey: string): ColumnConfig => {
  try {
    const raw = localStorage.getItem(`${COLUMN_STORAGE_PREFIX}${storageKey}`);
    if (raw === null) {
      return { order: [], visible: [] };
    }
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      const cfg = parsed as Partial<ColumnConfig>;
      return { order: toKeys(cfg.order), visible: toKeys(cfg.visible) };
    }
    return { order: [], visible: [] };
  } catch {
    return { order: [], visible: [] };
  }
};

// 将列配置写入 localStorage
const saveColumnConfig = (storageKey: string, config: ColumnConfig): void => {
  localStorage.setItem(`${COLUMN_STORAGE_PREFIX}${storageKey}`, JSON.stringify(config));
};

/**
 * useColumnVisibility 表格列可见性与排序组合式函数
 * - 依据表头派生可切换列、可见列与展示顺序，并把用户配置持久化到 localStorage（按 storageKey 区分）
 * - 固定列（fixed 非空，如操作列）不参与切换与排序，始终置尾展示
 * - 新增字段按表头 defaultVisible 决定默认是否展示（缺省可见）并追加在末尾；已移除字段的旧配置自动忽略
 * @param storageKey 配置存储键（按模型/表格区分，可传字符串、ref 或 getter）
 * @param headers 表格全部表头
 */
export function useColumnVisibility(
  storageKey: string | Ref<string> | (() => string),
  headers: Ref<Array<TableHeader>>,
) {
  // 全量可切换列（排除固定列），顺序跟随表头定义
  const allToggleable = computed(() => {
    return normalizeActionsHeader(headers.value).filter((hdr) => !hdr.fixed);
  });

  // 用户自定义的列顺序（全量可切换列，持久化）；隐藏列也保留位置，重新勾选后回到原位
  const columnOrder = ref<Array<string>>([]);
  // 可见列 key 子集（持久化），顺序遵循 columnOrder
  const visibleKeys = ref<Array<string>>([]);

  // 表头就绪时合并已保存配置：保留仍存在的列顺序，新增列追加末尾且按 defaultVisible 决定默认展示
  watch(
    allToggleable,
    (hdrs) => {
      const keys = hdrs.map((hdr) => hdr.key);
      const keySet = new Set(keys);
      const saved = loadColumnConfig(toValue(storageKey));
      const savedOrder = saved.order.filter((key) => keySet.has(key));
      const orderSet = new Set(savedOrder);
      const newKeys = keys.filter((key) => !orderSet.has(key));
      columnOrder.value = [...savedOrder, ...newKeys];
      const savedVisible = saved.visible.filter((key) => keySet.has(key));
      // 新增列中仅 defaultVisible !== false 的列默认可见（如列设置中的全量字段默认隐藏）
      const newVisible = newKeys.filter((key) => {
        const hdr = hdrs.find((h) => h.key === key);
        return hdr ? hdr.defaultVisible !== false : true;
      });
      visibleKeys.value = [...savedVisible, ...newVisible];
    },
    { immediate: true },
  );

  // 顺序或可见性变化即持久化，并通知配置同步（开启同步时防抖推送至服务端）
  watch([columnOrder, visibleKeys], () => {
    saveColumnConfig(toValue(storageKey), {
      order: columnOrder.value,
      visible: visibleKeys.value,
    });
    useConfigSyncStore().notifyLocalChange();
  });

  // 可见表头：按用户顺序输出可见列，固定列（如操作列）始终置尾
  const visibleHeaders = computed(() => {
    const normalized = normalizeActionsHeader(headers.value);
    const byKey = new Map(normalized.map((hdr) => [hdr.key, hdr]));
    const visibleSet = new Set(visibleKeys.value);
    const ordered = columnOrder.value
      .map((key) => byKey.get(key))
      .filter((hdr): hdr is TableHeader => hdr !== undefined && visibleSet.has(hdr.key));
    const fixed = normalized.filter((hdr) => hdr.fixed);
    return [...ordered, ...fixed];
  });

  return {
    allToggleable,
    columnOrder,
    visibleKeys,
    visibleHeaders,
  };
}
