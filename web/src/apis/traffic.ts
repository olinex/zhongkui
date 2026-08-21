import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type { SearchIn } from "@/components/@types/table";
import { HTTP_TRANSPORT_MODEL_KEY } from "@/utils/rego";
import type { PlaintextCryptoIn, PlaintextS3In, TrafficType } from "./@types/traffic";

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
