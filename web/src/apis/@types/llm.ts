// 第三方
import type { FlexIDs } from "./admin";
import type { SearchIn } from "@/components/@types/table";

// ---- LLM 模型注册表（/admin/llm/model）----
// LlmModelItem LLM 模型端点列表项；apiKey 为 base64 密文（列表出参即密文，
// 仅用于展示加密标识与直传明文接口解密，明文不回显）
export interface LlmModelItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  kind: string;
  baseURL: string;
  apiKey: string;
  model: string;
  timeoutSeconds: number;
  temperature: number;
  maxTokens: number;
  contextTokens: number;
  dimensions: number;
  enabled: boolean;
  description: string;
  // extraBody 高级请求配置：每次请求自动合并的 JSON 对象（原始 JSON 文本，空表示不附加）；
  // thinkingField 思考开关的请求字段名，与 thinkingOnValue / thinkingOffValue 三者齐备时该模型支持思考开关
  extraBody: string;
  thinkingField: string;
  thinkingOnValue: string;
  thinkingOffValue: string;
}

// LlmModelDetailOut LLM 模型详情出参；字段与列表项一致（apiKey 同为密文，
// 编辑时留空表示沿用原凭据）
export type LlmModelDetailOut = LlmModelItem;

// LlmModelListOut LLM 模型列表出参
export interface LlmModelListOut {
  items: LlmModelItem[];
  total: number;
}

// LlmModelCreateIn 注册 LLM 模型端点入参；kind 创建后不可更改；
// contextTokens 为上下文窗口容量，chat 类模型必填正整数，embedding 类留 0；
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
  contextTokens: number;
  enabled: boolean;
  description?: string;
  // extraBody 高级请求配置（JSON 对象文本，空表示不附加）；thinkingField / thinkingOnValue /
  // thinkingOffValue 为思考开关字段名与开/关取值，三者齐备时该模型支持思考开关
  extraBody?: string;
  thinkingField?: string;
  thinkingOnValue?: string;
  thinkingOffValue?: string;
}

// LlmModelUpdateIn 更新 LLM 模型端点入参；不含 kind（类型不可变），
// apiKey 留空表示沿用原凭据；contextTokens 为上下文窗口容量，chat 类模型必填正整数；
// dimensions 为数据库 schema 属性，由后端探测写回，不接受前端提交
export interface LlmModelUpdateIn {
  name: string;
  baseURL: string;
  apiKey: string;
  model: string;
  timeoutSeconds: number;
  temperature: number;
  maxTokens: number;
  contextTokens: number;
  enabled: boolean;
  description?: string;
  // extraBody 高级请求配置（JSON 对象文本，空表示不附加）；thinkingField / thinkingOnValue /
  // thinkingOffValue 为思考开关字段名与开/关取值，三者齐备时该模型支持思考开关
  extraBody?: string;
  thinkingField?: string;
  thinkingOnValue?: string;
  thinkingOffValue?: string;
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
  modelID: number;
  modelName: string;
  kind: string;
  requiredKind: string;
  providerModel: string;
  enabled: boolean;
  dimensions: number;
  bound: boolean;
  available: boolean;
}

// LlmBindingListOut 档位绑定列表出参；disabledReason 为 AI 不可用时的原因（按请求语言返回）；
// indexDimensions 为知识库向量索引（knowledge_chunks.embedding 列）当前维度，索引列未就绪时为 0，
// 供运维在提交 embedding 档位绑定前预知目标维度
export interface LlmBindingListOut {
  slots: LlmSlotBindingOut[];
  enabled: boolean;
  disabledReason: string;
  indexDimensions: number;
}

// LlmBindingSetIn 绑定档位入参
export interface LlmBindingSetIn {
  modelID: number;
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

// ---- MCP 服务器注册表（/admin/llm/mcp）----
// McpServerItem MCP 服务器列表项；transport 取值由后端枚举类 mcpTransport 下发（streamable_http / sse）
export interface McpServerItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  transport: string;
  url: string;
  timeoutSeconds: number;
  enabled: boolean;
  description: string;
}

// McpServerDetailOut MCP 服务器详情出参；headers 为密文（编辑留空表示沿用原凭据）；
// 列表字段投影白名单含 headers，故列表项亦为该类型
export interface McpServerDetailOut extends McpServerItem {
  headers: string;
}

// McpServerListOut MCP 服务器列表出参
export interface McpServerListOut {
  items: McpServerDetailOut[];
  total: number;
}

// McpServerCreateIn 注册 MCP 服务器入参；headers 为 JSON 文本形式的请求头（可选）
export interface McpServerCreateIn {
  name: string;
  transport: string;
  url: string;
  headers?: string;
  timeoutSeconds: number;
  enabled: boolean;
  description?: string;
}

// McpServerUpdateIn 更新 MCP 服务器入参；headers 留空表示沿用原凭据
export interface McpServerUpdateIn {
  name: string;
  transport: string;
  url: string;
  headers: string;
  timeoutSeconds: number;
  enabled: boolean;
  description?: string;
}

// McpServerSearchIn MCP 服务器搜索入参
// 继承通用 SearchIn（page/pageSize/orderBy/filterBy 等），附加特有过滤字段；
// 过滤字段命名与后端 search.toml 的 McpServerListGet 一致
export interface McpServerSearchIn extends SearchIn {
  search?: string;
  name?: string;
  transport?: string;
  enabled?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

// McpServerTestIn 连通性测试入参；id 大于 0 时按已注册服务器探活，其余字段被忽略；
// 否则按提交的完整服务器配置探活（不落库）
export interface McpServerTestIn {
  id?: number;
  transport?: string;
  url?: string;
  headers?: string;
  timeoutSeconds?: number;
}

// McpServerTestOut 连通性测试出参；toolNames 为探活时探测到的远端工具名清单
export interface McpServerTestOut {
  ok: boolean;
  toolCount: number;
  toolNames: string[];
}

// McpServerSyncOut 同步远端工具清单出参
export interface McpServerSyncOut {
  created: number;
  updated: number;
  removed: number;
}

// ---- 工具注册表（/admin/llm/tool）----
// LlmToolItem 工具列表项；source 取值由后端枚举类 llmToolSource 下发（builtin / mcp），
// mcp 来源工具附所属服务器名与远端工具名
export interface LlmToolItem {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  source: string;
  mcpServerName: string;
  remoteName: string;
  enabled: boolean;
  description: string;
}

// LlmToolListOut 工具列表出参
export interface LlmToolListOut {
  items: LlmToolItem[];
  total: number;
}

// LlmToolSearchIn 工具搜索入参
// 继承通用 SearchIn（page/pageSize/orderBy/filterBy 等），附加特有过滤字段；
// 过滤字段命名与后端 search.toml 的 LlmToolListGet 一致
export interface LlmToolSearchIn extends SearchIn {
  search?: string;
  name?: string;
  source?: string;
  enabled?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

// LlmToolSetEnabledIn 批量设置工具启用态入参
export interface LlmToolSetEnabledIn {
  ids: FlexIDs;
  enabled: boolean;
}
