import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type { FlexIDs } from "./@types/admin";
import type {
  LlmBindingSetIn,
  LlmModelCreateIn,
  LlmModelSearchIn,
  LlmModelTestIn,
  LlmModelUpdateIn,
  LlmParamResetIn,
  LlmParamSetIn,
} from "./@types/llm";

// ---- LLM 模型注册表（/api/v1/admin/llm/model）----
export function llmModelListGet(data: LlmModelSearchIn): AxiosPromise {
  return request.post("/admin/llm/model/list", data, {
    headers: getJwtHeader(),
  });
}

// llmModelListOptions 模型列表自省元信息：表头、筛选字段与枚举类名由后端 search 定义派生
export function llmModelListOptions(): AxiosPromise {
  return request.options("/admin/llm/model/list", {
    headers: getJwtHeader(),
  });
}

export function llmModelCreate(data: LlmModelCreateIn): AxiosPromise {
  return request.post("/admin/llm/model/create", data, {
    headers: getJwtHeader(),
  });
}

export function llmModelDetailGet(id: number): AxiosPromise {
  return request.post(
    `/admin/llm/model/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function llmModelUpdate(id: number, data: LlmModelUpdateIn): AxiosPromise {
  return request.post(`/admin/llm/model/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function llmModelBatchDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/admin/llm/model/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

// llmModelTest 测试模型端点连通性：探活最长可达注册的超时秒数（默认 60s），
// 故显式覆盖 axios 实例的全局短超时，避免客户端提前判失败
export function llmModelTest(data: LlmModelTestIn): AxiosPromise {
  return request.post("/admin/llm/model/test", data, {
    headers: getJwtHeader(),
    timeout: 60000,
  });
}

// ---- 档位绑定（/api/v1/admin/llm/binding）----
export function llmBindingListGet(): AxiosPromise {
  return request.post(
    "/admin/llm/binding/list",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function llmBindingSet(slot: string, data: LlmBindingSetIn): AxiosPromise {
  return request.post(`/admin/llm/binding/${slot}/set`, data, {
    headers: getJwtHeader(),
  });
}

export function llmBindingUnbind(slot: string): AxiosPromise {
  return request.post(
    `/admin/llm/binding/${slot}/unbind`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- Agent 参数覆盖（/api/v1/admin/llm/param）----
export function llmParamListGet(): AxiosPromise {
  return request.post(
    "/admin/llm/param/list",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function llmParamSet(data: LlmParamSetIn): AxiosPromise {
  return request.post("/admin/llm/param/set", data, {
    headers: getJwtHeader(),
  });
}

export function llmParamReset(data: LlmParamResetIn): AxiosPromise {
  return request.post("/admin/llm/param/reset", data, {
    headers: getJwtHeader(),
  });
}