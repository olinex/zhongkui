import * as yup from "yup";

import i18n from "@/utils/i18n";

// LLM 模型端点的表单校验 schema。
// 除 apiKey 外的单行文本字段统一在 schema 层做 trim 规范化；apiKey 为凭据，
// 可合法包含首尾空白，禁止 trim。

// jsonObjectMessage / jsonValueMessage 生成 JSON 校验失败时的本地化文案：
// yup 内置类型的提示由全局 setLocale 本地化，自定义 test 需自行经 i18n 渲染，
// path 传入的已是标签键（yup 会优先取 schema 的 label），故二次 t() 得到字段名。
const jsonObjectMessage = (params: { path: string }): string =>
  i18n.global.t("model.llmModel.validation.jsonObject", {
    path: i18n.global.t(params.path),
  });

const jsonValueMessage = (params: { path: string }): string =>
  i18n.global.t("model.llmModel.validation.jsonValue", {
    path: i18n.global.t(params.path),
  });

// isJsonObject / isJsonValue 与后端 llm.ParseExtraBody / llm.ParseJSONValue 对齐：
// 空白串视为未填写（放行）；extraBody 须为 JSON 对象（null / 数组 / 标量皆非法）；
// 思考开关取值须为任意合法 JSON 值。
const isBlank = (value: string | undefined): boolean => (value ?? "").trim() === "";

const isJsonObject = (value: string | undefined): boolean => {
  if (isBlank(value)) {
    return true;
  }
  try {
    const parsed: unknown = JSON.parse(value as string);
    return parsed !== null && typeof parsed === "object" && !Array.isArray(parsed);
  } catch {
    return false;
  }
};

const isJsonValue = (value: string | undefined): boolean => {
  if (isBlank(value)) {
    return true;
  }
  try {
    JSON.parse(value as string);
    return true;
  } catch {
    return false;
  }
};

const name = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(1)
  .max(64)
  .label("model.llmModel.field.name");

const kind = yup.string().nonNullable().required().label("model.llmModel.field.kind");

const baseURL = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(1)
  .max(512)
  .label("model.llmModel.field.baseURL");

// apiKey 创建时必填；更新时留空表示沿用原凭据，故仅做非空校验
const apiKeyCreate = yup
  .string()
  .nonNullable()
  .required()
  .min(1)
  .label("model.llmModel.field.apiKey");
const apiKeyUpdate = yup.string().nonNullable().label("model.llmModel.field.apiKey");

const model = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(1)
  .max(128)
  .label("model.llmModel.field.model");

const timeoutSeconds = yup
  .number()
  .nonNullable()
  .required()
  .integer()
  .min(1)
  .label("model.llmModel.field.timeoutSeconds");

const temperature = yup
  .number()
  .nonNullable()
  .required()
  .min(0)
  .max(2)
  .label("model.llmModel.field.temperature");

const maxTokens = yup
  .number()
  .nonNullable()
  .required()
  .integer()
  .min(1)
  .label("model.llmModel.field.maxTokens");

// contextTokens 为上下文窗口容量：chat 类模型必填正整数（agent 据此计算占用比例），
// embedding 类不涉及、留 0。创建表单含 kind 字段，按其取值分支校验
const contextTokens = yup
  .number()
  .nonNullable()
  .integer()
  .label("model.llmModel.field.contextTokens")
  .when("kind", {
    is: (k: string) => k === "chat",
    then: (s) => s.required().min(1),
    otherwise: (s) => s.min(0),
  });

// 类型不可变，更新表单不含 kind，故无法按类型分支：统一要求非负整数（chat 的正整数约束由后端兜底）
const contextTokensUpdate = yup
  .number()
  .nonNullable()
  .required()
  .integer()
  .min(0)
  .label("model.llmModel.field.contextTokens");

const enabled = yup.bool().nonNullable().required().label("model.llmModel.field.enabled");

const description = yup.string().nonNullable().max(512).label("model.llmModel.field.description");

// extraBody 为通用额外请求字段（JSON 对象文本），可选；非空时须为合法 JSON 对象
const extraBody = yup
  .string()
  .nonNullable()
  .test("jsonObject", jsonObjectMessage, isJsonObject)
  .label("model.llmModel.field.extraBody");

// 思考开关三项：thinkingField 为字段名（可选，最长 64）；开/关取值可选，
// 非空时须为合法 JSON 值；三者齐备方由后端启用思考开关
const thinkingField = yup
  .string()
  .trim()
  .nonNullable()
  .max(64)
  .label("model.llmModel.field.thinkingField");
const thinkingOnValue = yup
  .string()
  .nonNullable()
  .test("jsonValue", jsonValueMessage, isJsonValue)
  .label("model.llmModel.field.thinkingOnValue");
const thinkingOffValue = yup
  .string()
  .nonNullable()
  .test("jsonValue", jsonValueMessage, isJsonValue)
  .label("model.llmModel.field.thinkingOffValue");

// 注册 LLM 模型表单 schema（kind 必填且创建后不可更改）
export const createLlmModelSchema = () =>
  yup.object({
    name,
    kind,
    baseURL,
    apiKey: apiKeyCreate,
    model,
    timeoutSeconds,
    temperature,
    maxTokens,
    contextTokens,
    enabled,
    description,
    extraBody,
    thinkingField,
    thinkingOnValue,
    thinkingOffValue,
  });

// 更新 LLM 模型表单 schema（类型不可变，apiKey 留空沿用原凭据）
export const updateLlmModelSchema = () =>
  yup.object({
    name,
    baseURL,
    apiKey: apiKeyUpdate,
    model,
    timeoutSeconds,
    temperature,
    maxTokens,
    contextTokens: contextTokensUpdate,
    enabled,
    description,
    extraBody,
    thinkingField,
    thinkingOnValue,
    thinkingOffValue,
  });

// MCP 服务器端点的表单校验 schema。
// 创建与更新同一套规则：headers 均为可选（创建留空表示无请求头，更新留空表示沿用原凭据），
// 且 headers 承载凭据、可合法包含首尾空白，故禁止 trim；name / url 为单行文本，统一 trim 规范化。
const mcpServerName = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(1)
  .max(64)
  .label("model.mcpServer.field.name");

const mcpServerTransport = yup
  .string()
  .nonNullable()
  .required()
  .label("model.mcpServer.field.transport");

const mcpServerURL = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(1)
  .max(512)
  .label("model.mcpServer.field.url");

const mcpServerHeaders = yup.string().nonNullable().label("model.mcpServer.field.headers");

const mcpServerTimeoutSeconds = yup
  .number()
  .nonNullable()
  .required()
  .integer()
  .min(1)
  .label("model.mcpServer.field.timeoutSeconds");

const mcpServerEnabled = yup.bool().nonNullable().required().label("model.mcpServer.field.enabled");

const mcpServerDescription = yup
  .string()
  .nonNullable()
  .max(512)
  .label("model.mcpServer.field.description");

// MCP 服务器表单 schema（创建与更新共用）
export const mcpServerSchema = () =>
  yup.object({
    name: mcpServerName,
    transport: mcpServerTransport,
    url: mcpServerURL,
    headers: mcpServerHeaders,
    timeoutSeconds: mcpServerTimeoutSeconds,
    enabled: mcpServerEnabled,
    description: mcpServerDescription,
  });
