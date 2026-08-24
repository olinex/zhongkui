import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type { SearchIn } from "@/components/@types/table";
import { HTTP_TRANSPORT_MODEL_KEY } from "@/utils/rego";
import type { PlaintextCryptoIn, PlaintextS3In, TrafficType } from "./@types/traffic";
import type {
  FlexIDs,
  TagEventRuleCreateIn,
  TagEventRuleDryRunIn,
  TagEventRuleUpdateIn,
  TagEventRuleValidateIn,
  TagRuleCreateIn,
  TagRuleDryRunIn,
  TagRuleModelIn,
  TagRuleSampleCreateIn,
  TagRuleSampleUpdateIn,
  TagRuleUpdateIn,
  TagRuleValidateIn,
} from "./@types/admin";

// TrafficCryptoFields 各类型加密字段清单
// 对应模型 secure:"serializer:crypto" 字段，列表返回 base64 密文
export const TrafficCryptoFields: Record<TrafficType, string[]> = {
  "ipv4-tcp": ["payload"],
  "ipv6-tcp": ["payload"],
  "http-transport": [
    "reqHdrAuthorization",
    "reqHdrProxyAuthorization",
    "reqHdrCookie",
    "reqHdrSecWebsocketKey",
    "rspHdrWwwAuthenticate",
    "rspHdrProxyAuthenticate",
    "rspHdrSetCookie",
    "rspHdrSecWebsocketAccept",
  ],
};

// TrafficS3Fields 各类型 S3 body 字段清单
// 列表返回 S3 对象键，明文经 /plaintext/s3 按 object_key 读取
export const TrafficS3Fields: Record<TrafficType, string[]> = {
  "ipv4-tcp": [],
  "ipv6-tcp": [],
  "http-transport": ["reqBody", "rspBody"],
};

// TrafficModelKeys 流量类型 → 目标模型类型键（tagengine 注册名），供“收藏样例”使用；
// 仅登记 tagengine 已注册模型，未登记类型不显示收藏入口
export const TrafficModelKeys: Partial<Record<TrafficType, string>> = {
  "http-transport": HTTP_TRANSPORT_MODEL_KEY,
};

// trafficTypeFromModelKey 目标模型类型键 → 流量类型（TrafficModelKeys 的逆映射）；
// 未知类型返回 undefined，调用方据此跳过加密/S3 字段识别
export function trafficTypeFromModelKey(typeKey: string): TrafficType | undefined {
  return (Object.keys(TrafficModelKeys) as TrafficType[]).find(
    (trafficType) => TrafficModelKeys[trafficType] === typeKey,
  );
}

// trafficListGet 流量列表（分页 + 排序 + 过滤）
export function trafficListGet(type: TrafficType, data: SearchIn): AxiosPromise {
  return request.post(`/traffic/${type}/list`, data, { headers: getJwtHeader() });
}

// trafficStackListGet HTTP 传输关联网络堆栈列表（分页 + 排序 + 过滤）
// 以 HttpTransport 的 traceId + 时间范围查询组成该 TCP 流的全部网络堆栈记录；
// 前端借助 HttpTransport.AppTailer 的 payload offset 与记录的
// payloadStartOffset/payloadEndOffset 相交，高亮请求/响应对应的数据包
export function trafficStackListGet(data: SearchIn): AxiosPromise {
  return request.post("/traffic/http-transport/stack/list", data, {
    headers: getJwtHeader(),
  });
}

// plaintextCryptoGet 加密字段明文解密（全局通用接口）
// model 为模型注册名、field 为字段名，二者与后端权限点 object（plaintext:crypto:{model}:{field}）对应；
// 响应为原始字节流，不走 getResponse 的 JSON 成功回调，由调用方直接处理 ArrayBuffer
export function plaintextCryptoGet(data: PlaintextCryptoIn): AxiosPromise {
  return request.post("/plaintext/crypto", data, {
    headers: {
      ...getJwtHeader(),
      Accept: "application/octet-stream",
    },
    responseType: "arraybuffer",
  });
}

// plaintextS3Get S3 body 明文读取（全局通用接口）
// 响应为原始字节流，不走 getResponse 的 JSON 成功回调，由调用方直接处理 ArrayBuffer
export function plaintextS3Get(data: PlaintextS3In): AxiosPromise {
  return request.post("/plaintext/s3", data, {
    headers: {
      ...getJwtHeader(),
      Accept: "application/octet-stream",
    },
    responseType: "arraybuffer",
  });
}

// ---- 标签识别规则（命名空间化，归属当前请求命名空间）----
export function tagRuleListGet(data: SearchIn): AxiosPromise {
  return request.post("/traffic/tag-rule/list", data, { headers: getJwtHeader() });
}

export function tagRuleListOptions(): AxiosPromise {
  return request.options("/traffic/tag-rule/list", { headers: getJwtHeader() });
}

export function tagRuleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/traffic/tag-rule/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleCreate(data: TagRuleCreateIn): AxiosPromise {
  return request.post("/traffic/tag-rule/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleUpdate(id: number, data: TagRuleUpdateIn): AxiosPromise {
  return request.post(`/traffic/tag-rule/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/traffic/tag-rule/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleSetEnabled(ids: FlexIDs, enabled: boolean): AxiosPromise {
  return request.post(
    `/traffic/tag-rule/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleValidate(data: TagRuleValidateIn): AxiosPromise {
  return request.post("/traffic/tag-rule/validate", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleModelOptions(): AxiosPromise {
  return request.post(
    "/traffic/tag-rule/model/options",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleModelMetadata(data: TagRuleModelIn): AxiosPromise {
  return request.post("/traffic/tag-rule/model/metadata", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleModelSamples(data: TagRuleModelIn): AxiosPromise {
  return request.post("/traffic/tag-rule/model/samples", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleDryRun(data: TagRuleDryRunIn): AxiosPromise {
  return request.post("/traffic/tag-rule/single/dry-run", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleVersionList(id: number): AxiosPromise {
  return request.post(
    `/traffic/tag-rule/${id}/version/list`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleVersionRollback(id: number, version: number): AxiosPromise {
  return request.post(`/traffic/tag-rule/${id}/version/${version}/rollback`, null, {
    headers: getJwtHeader(),
  });
}

// ---- 标签规则样例（命名空间化，归属当前请求命名空间）----
export function tagRuleSampleCreate(data: TagRuleSampleCreateIn): AxiosPromise {
  return request.post("/traffic/tag-rule-sample/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleListGet(data: SearchIn): AxiosPromise {
  return request.post("/traffic/tag-rule-sample/list", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleListOptions(): AxiosPromise {
  return request.options("/traffic/tag-rule-sample/list", {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/traffic/tag-rule-sample/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleSampleUpdate(id: number, data: TagRuleSampleUpdateIn): AxiosPromise {
  return request.post(`/traffic/tag-rule-sample/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/traffic/tag-rule-sample/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 标签事件规则（命名空间化，归属当前请求命名空间）----
export function tagEventRuleListGet(data: SearchIn): AxiosPromise {
  return request.post("/traffic/tag-event-rule/list", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleListOptions(): AxiosPromise {
  return request.options("/traffic/tag-event-rule/list", {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/traffic/tag-event-rule/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleCreate(data: TagEventRuleCreateIn): AxiosPromise {
  return request.post("/traffic/tag-event-rule/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleUpdate(id: number, data: TagEventRuleUpdateIn): AxiosPromise {
  return request.post(`/traffic/tag-event-rule/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/traffic/tag-event-rule/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleSetEnabled(ids: FlexIDs, enabled: boolean): AxiosPromise {
  return request.post(
    `/traffic/tag-event-rule/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleValidate(data: TagEventRuleValidateIn): AxiosPromise {
  return request.post("/traffic/tag-event-rule/validate", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDryRun(data: TagEventRuleDryRunIn): AxiosPromise {
  return request.post("/traffic/tag-event-rule/dry-run", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleVersionList(id: number): AxiosPromise {
  return request.post(
    `/traffic/tag-event-rule/${id}/version/list`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleVersionRollback(id: number, version: number): AxiosPromise {
  return request.post(`/traffic/tag-event-rule/${id}/version/${version}/rollback`, null, {
    headers: getJwtHeader(),
  });
}
