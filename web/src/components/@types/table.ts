import type { BackendDataType, SearchAction } from "@/types";

export type Align = "start" | "end" | "center" | undefined;
export type Fixed = boolean | "end" | "start" | undefined;
// Table row values are heterogeneous (string / number / boolean / Date / ...).
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type ItemAction = (item: Record<string, any>) => void;
export type ItemActions = Record<string, ItemAction>;
// Batch row values are heterogeneous, same as ItemAction.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type BatchAction = (items: Record<string, any>[]) => void;
export type BatchActions = Record<string, BatchAction>;

export interface TableHeader {
  key: string;
  title: string;
  type: BackendDataType;
  align?: Align;
  sortable?: boolean;
  fixed?: Fixed;
  // 默认可见性：无本地保存配置时新增列默认是否展示，缺省 true（展示）
  defaultVisible?: boolean;
}

export interface OrderByItem {
  name: string;
  order: number;
}

export interface FilterByItem {
  action: string;
  value?: string | boolean | number | null;
}

export interface SearchIn {
  greedy: boolean;
  page?: number | undefined;
  pageSize?: number | undefined;
  orderBy?: OrderByItem[] | undefined;
  filterBy?: Record<string, FilterByItem>;
}

export interface SearchOut<Item> {
  total: number;
  items: Array<Item>;
}

export interface FilterByOption {
  name: string;
  title: string;
  type: BackendDataType;
  actions: SearchAction[];
}

export interface HeaderOption {
  name: string;
  type: BackendDataType;
}

export interface SearchOptionsOut {
  orderBy: string[];
  filterBy: FilterByOption[];
  headers: HeaderOption[];
}
