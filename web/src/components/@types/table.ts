import type { BackendDataType, SearchAction } from "@/types";
import type { Option } from "./forms";

export type Align = "start" | "end" | "center" | undefined;
export type Fixed = boolean | "end" | "start" | undefined;
// 表格行值类型异构（字符串 / 数字 / 布尔 / 日期 等）。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type ItemAction = (item: Record<string, any>) => void;
export type ItemActions = Record<string, ItemAction>;
// 批量行值与 ItemAction 同理，类型异构。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type BatchAction = (items: Record<string, any>[]) => void;
export type BatchActions = Record<string, BatchAction>;

export interface TableHeader {
  key: string;
  title: string;
  type: BackendDataType;
  // 列展示语义类型（取自 OPTIONS 的 kind，如 json）；JSON 不是数据类型，
  // 展示方式经该字段下发，单元格据此选择渲染分支
  kind?: string;
  align?: Align;
  sortable?: boolean;
  fixed?: Fixed;
  // 默认可见性：无本地保存配置时新增列默认是否展示，缺省 true（展示）
  defaultVisible?: boolean;
  // 枚举类名引用：枚举列单元格经 enumLabelOf(enumType, value) 渲染 label
  enumType?: string;
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
  // 字段投影：仅返回指定字段（后端按白名单过滤，未知字段忽略；缺省返回全量白名单字段）
  fields?: string[] | undefined;
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
  // 枚举字段的下拉可选项（title 为 i18n 键，渲染时经翻译展示）
  options?: Option[];
  // 枚举类名引用：供 SearchTable 经枚举 store 解析可选项；携带 options 时以 options 为准
  enumType?: string;
}

export interface HeaderOption {
  name: string;
  type: BackendDataType;
  // 默认可见性：无本地保存配置时新增列默认是否展示，缺省 true（展示）；
  // 长度不固定的长文本列（如通知模板标题/正文）可显式设 false 默认隐藏
  defaultVisible?: boolean;
  // 枚举类名引用：枚举列单元格经 enumLabelOf(enumType, value) 渲染 label
  enumType?: string;
}

export interface SearchOptionsOut {
  orderBy: string[];
  filterBy: FilterByOption[];
  headers: HeaderOption[];
}
