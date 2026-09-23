// Third-party
import type { SearchIn } from "@/components/@types/table";

// ---- LLM 模型注册表（/admin/llm/model）----
// LlmModelItem LLM 模型端点列表项；列表字段投影白名单不含 apiKey，故不含凭据字段
export interface LlmModelItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  kind: string;
  baseURL: string;
  model: string;
  timeoutSeconds: number;
  temperature: number;
  maxTokens: number;
  dimensions: number;
  enabled: boolean;
  description: string;
}

// LlmModelDetailOut LLM 模型详情出参；apiKey 为密文（明文不回显，编辑留空表示沿用原凭据）
export interface LlmModelDetailOut extends LlmModelItem {
  apiKey: string;
}

// LlmModelListOut LLM 模型列表出参
export interface LlmModelListOut {
  items: LlmModelItem[];
  total: number;
}

// LlmModelCreateIn 注册 LLM 模型端点入参；kind 创建后不可更改；
// dimensions 为数据库 schema 属性，由后端探测写回，不接受前端提交
export interface LlmModelCreateIn {
  name: string;
  kind: string;
  baseURL: string;
  apiKey: string;
  model: string;
  timeoutSeconds: number;
  temperature: number;
  maxTokens: number;
  enabled: boolean;
  description?: string;
}

// LlmModelUpdateIn 更新 LLM 模型端点入参；不含 kind（类型不可变），
// apiKey 留空表示沿用原凭据；dimensions 为数据库 schema 属性，由后端探测写回，不接受前端提交
export interface LlmModelUpdateIn {
  name: string;
  baseURL: string;
  apiKey: string;
  model: string;
  timeoutSeconds: number;
  temperature: number;
  maxTokens: number;
  enabled: boolean;
  description?: string;
}

// LlmModelSearchIn LLM 模型搜索入参
// 继承通用 SearchIn（page/pageSize/orderBy/filterBy 等），附加特有过滤字段；
// 过滤字段命名与后端 search.toml 的 LlmModelListGet 一致
export interface LlmModelSearchIn extends SearchIn {
  search?: string;
  name?: string;
  kind?: string;
  enabled?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

// LlmModelTestIn 连通性测试入参；id 大于 0 时按已注册模型探活，其余字段被忽略；
// 否则按提交的完整端点配置探活
export interface LlmModelTestIn {
  id?: number;
  kind?: string;
  baseURL?: string;
  apiKey?: string;
  model?: string;
  timeoutSeconds?: number;
  temperature?: number;
  maxTokens?: number;
}

// LlmModelTestOut 连通性测试出参
export interface LlmModelTestOut {
  success: boolean;
  message: string;
}

// ---- 档位绑定（/admin/llm/binding）----
// LlmSlotBindingOut 单个档位的绑定摘要与可用性；requiredKind 为该档位要求的模型类型
export interface LlmSlotBindingOut {
  slot: string;
  modelId: number;
  modelName: string;
  kind: string;
  requiredKind: string;
  providerModel: string;
  enabled: boolean;
  dimensions: number;
  bound: boolean;
  available: boolean;
}

// LlmBindingListOut 档位绑定列表出参；disabledReason 为 AI 不可用时的原因（按请求语言返回）
export interface LlmBindingListOut {
  slots: LlmSlotBindingOut[];
  enabled: boolean;
  disabledReason: string;
}

// LlmBindingSetIn 绑定档位入参
export interface LlmBindingSetIn {
  modelId: number;
}

// ---- Agent 参数（/admin/llm/param）----
// LlmParamItemOut 单个 Agent 参数的元信息、出厂值、当前生效值与偏离态；
// default 为库内出厂值（由数据库迁移注入，任何接口路径都不可写），value 为库内当前生效值，
// overridden 表示二者不同；label / comment 由后端按请求语言本地化后返回，前端直接展示；
// options 为后端注册表声明的封闭取值清单（仅 string 类型参数可能非空，非空时渲染下拉框，
// 清单内为库内存储的原始字面量、不做本地化）；enumType 为清单所属的枚举类名（与 options
// 同时非空），选项文案据此经枚举 store 的 label（enum.<类名>.<key>）渲染，与全站枚举展示同源
export interface LlmParamItemOut {
  key: string;
  group: string;
  kind: string;
  default: unknown;
  value: unknown;
  overridden: boolean;
  label: string;
  comment: string;
  options: string[] | null;
  enumType: string;
}

// LlmParamListOut Agent 参数列表出参（按注册表声明顺序）
export interface LlmParamListOut {
  items: LlmParamItemOut[];
}

// LlmParamSetItemIn 单条参数写入入参；value 为 null 或空串表示把该键恢复为出厂值
export interface LlmParamSetItemIn {
  key: string;
  value: unknown;
}

// LlmParamSetIn 批量写入 Agent 参数入参
export interface LlmParamSetIn {
  items: LlmParamSetItemIn[];
}

// LlmParamResetIn 恢复 Agent 参数出厂值入参；group 优先（恢复该分组全部参数），两者皆空被拒绝
export interface LlmParamResetIn {
  keys?: string[];
  group?: string;
}
