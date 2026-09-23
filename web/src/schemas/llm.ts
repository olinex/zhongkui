import * as yup from "yup";

// LLM 模型端点的表单校验 schema。
// 除 apiKey 外的单行文本字段统一在 schema 层做 trim 规范化；apiKey 为凭据，
// 可合法包含首尾空白，禁止 trim。

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

const enabled = yup.bool().nonNullable().required().label("model.llmModel.field.enabled");

const description = yup.string().nonNullable().max(512).label("model.llmModel.field.description");

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
    enabled,
    description,
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
    enabled,
    description,
  });
