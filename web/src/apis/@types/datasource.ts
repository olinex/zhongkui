// Third-party
import type { SearchIn } from "@/components/@types/table";

// ---- 第三方数据源类型 ----
// DatasourceTypeField 数据源类型配置字段描述（对应后端 FieldMeta，动态渲染表单用）
// 字段展示文案由前端依据数据源类型与字段名拼接 i18n key 自行翻译，后端不返回。
export interface DatasourceTypeField {
  name: string;
  kind: string;
  type?: string;
  binding?: string;
  required: boolean;
  defaultValue?: unknown;
}

// DatasourceTypeItem 数据源类型条目（redis / mysql）
export interface DatasourceTypeItem {
  type: string;
  name: string;
  config: DatasourceTypeField[];
}

// DatasourceTypesOut 数据源类型列表出参
export interface DatasourceTypesOut {
  types: DatasourceTypeItem[];
}

// ---- 第三方数据源 ----
// DatasourceItem 第三方数据源项（config 为加密后密文，默认不展示明文）
export interface DatasourceItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  type: string;
  enabled: boolean;
  config: string;
  description: string;
}

// DatasourceListOut 第三方数据源列表出参
export interface DatasourceListOut {
  items: DatasourceItem[];
  total: number;
}

// DatasourceCreateIn 创建第三方数据源入参
// config 为连接配置的 JSON 文本，结构随数据源类型不同而不同（提交前由表单拼装）
export interface DatasourceCreateIn {
  name: string;
  type: string;
  enabled: boolean;
  config: string;
  description?: string;
}

// DatasourceUpdateIn 更新第三方数据源入参（名称与类型不可变，不进编辑表单）
export interface DatasourceUpdateIn {
  enabled: boolean;
  config: string;
  description?: string;
}

// DatasourceSearchIn 第三方数据源搜索入参
// 继承通用 SearchIn（page/pageSize/orderBy/filterBy 等），附加特有过滤字段
// 过滤字段命名与后端 search.toml 的 DatasourceListGet 一致
export interface DatasourceSearchIn extends SearchIn {
  search?: string;
  name?: string;
  type?: string;
  enabled?: boolean;
  createdAt?: string;
  updatedAt?: string;
}