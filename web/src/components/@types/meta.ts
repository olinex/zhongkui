// 接口元信息（OPTIONS）类型定义：与后端 internal/manager/views/meta.go 的
// FieldMeta / StructMeta / MethodMeta / ApiMeta / ListMeta 序列化形状一一对应，
// 供前端表单/表格通用渲染消费。
import type { SearchOptionsOut } from "./table";

// FieldMeta 接口结构体单个字段元信息。
// 可选枚举值字段（enum=true）以下拉展示，条目经 enumType 指定的枚举类名，
// 通过统一枚举接口（/admin/enum/get）按类名获取；label 由 key 经 enum.<类名>.<key> 翻译。
export interface FieldMeta {
  name: string;
  type: string;
  binding?: string;
  kind: string;
  required: boolean;
  defaultValue?: unknown;
  enum?: boolean;
  enumType?: string;
}

// StructMeta 单个接口结构体元信息。
export interface StructMeta {
  name: string;
  fields: FieldMeta[];
}

// StructSlot 结构体在某个 HTTP 方法下的四类槽位之一。
export type StructSlot = "headerInput" | "headerOutput" | "bodyInput" | "bodyOutput";

// MethodMeta 某个 HTTP 方法对应的接口元信息；未指定结构体的部分为 null。
export interface MethodMeta {
  headerInput?: StructMeta | null;
  headerOutput?: StructMeta | null;
  bodyInput?: StructMeta | null;
  bodyOutput?: StructMeta | null;
}

// ApiMeta 一个接口路径下所有 HTTP 方法的元信息，key 为 HTTP 方法名（如 GET、POST）。
export type ApiMeta = Record<string, MethodMeta>;

// MetaOut 列表接口元信息响应：在搜索选项与表格列元数据基础上，扩展各方法的
// 结构体元信息（meta），前端一次 OPTIONS 请求即可同时获取搜索控件与枚举可选项。
export interface MetaOut extends SearchOptionsOut {
  meta: ApiMeta;
}
