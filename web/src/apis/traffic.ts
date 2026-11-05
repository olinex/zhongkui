import type { AxiosPromise } from "axios";
import { getJwtHeader, getResponse, request } from "./base";

import { BackendDataType } from "@/types";
import type { FieldMeta, MetaOut } from "@/components/@types/meta";
import type { SearchIn } from "@/components/@types/table";
import { fieldRenderTypeOfMeta } from "@/utils/meta";
import { HTTP_TRANSPORT_MODEL_KEY } from "@/utils/rego";
import type {
  PlaintextCryptoIn,
  PlaintextS3In,
  TrafficDetailIn,
  TrafficType,
} from "./@types/traffic";

// trafficFieldMetasCache 流量类型 → 列表 OPTIONS 自省字段元信息（按 JSON 字段名索引）缓存
const trafficFieldMetasCache = new Map<TrafficType, Promise<Record<string, FieldMeta>>>();

// trafficFieldMetasOf 取该流量类型的列表字段元信息：来源为 OPTIONS 的 meta.bodyOutput，
// 字段展示语义（kind）与数据类型均由后端 Out 结构体声明，前端不再维护重复字段清单；
// 自省失败时不缓存并回退空对象，下次调用重新拉取。
export function trafficFieldMetasOf(type: TrafficType): Promise<Record<string, FieldMeta>> {
  const cached = trafficFieldMetasCache.get(type);
  if (cached) {
    return cached;
  }
  const pending = new Promise<Record<string, FieldMeta>>((resolve) => {
    const failed = async (): Promise<void> => {
      trafficFieldMetasCache.delete(type);
      resolve({});
    };
    void getResponse<MetaOut>(
      trafficListOptions(type),
      async (rsp) => {
        const metas: Record<string, FieldMeta> = {};
        for (const field of rsp.data.meta?.POST?.bodyOutput?.fields ?? []) {
          metas[field.name] = field;
        }
        resolve(metas);
      },
      failed,
      failed,
      failed,
    );
  });
  trafficFieldMetasCache.set(type, pending);
  return pending;
}

// trafficKindKeysOf 按展示语义 kind（crypto / s3）取字段名集合
async function trafficKindKeysOf(type: TrafficType, kind: string): Promise<Set<string>> {
  const metas = await trafficFieldMetasOf(type);
  return new Set(
    Object.values(metas)
      .filter((field) => field.kind === kind)
      .map((field) => field.name),
  );
}

// trafficCryptoKeysOf 加密字段集合（Out 字段 form:"kind=crypto" 声明）
export const trafficCryptoKeysOf = (type: TrafficType): Promise<Set<string>> =>
  trafficKindKeysOf(type, "crypto");

// trafficS3KeysOf S3 对象字段集合（Out 字段 form:"kind=s3" 声明）
export const trafficS3KeysOf = (type: TrafficType): Promise<Set<string>> =>
  trafficKindKeysOf(type, "s3");

// trafficFieldTypesOf 字段渲染类型映射：由 OPTIONS 自省推导，替代前端硬编码字段类型表
export async function trafficFieldTypesOf(
  type: TrafficType,
): Promise<Record<string, BackendDataType>> {
  const metas = await trafficFieldMetasOf(type);
  return Object.fromEntries(
    Object.entries(metas).map(([name, meta]) => [name, fieldRenderTypeOfMeta(meta)]),
  );
}

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

// trafficDetailGet 流量行详情查询（按 namespace + id 获取单条全字段记录），
// 列表仅返回投影字段，行详情对话框/收藏样例/堆栈下钻经此接口补齐全字段
export function trafficDetailGet(type: TrafficType, data: TrafficDetailIn): AxiosPromise {
  return request.post(`/traffic/${type}/detail`, data, { headers: getJwtHeader() });
}

// trafficListOptions 流量列表元信息（可过滤字段/可排序字段），
// 供高级筛选条件编辑器（ExpressionField）动态渲染各类型可筛选字段与动作
export function trafficListOptions(type: TrafficType): AxiosPromise {
  return request.options(`/traffic/${type}/list`, { headers: getJwtHeader() });
}

// trafficStackListGet HTTP 传输关联网络堆栈列表（分页 + 排序 + 过滤）
// 以 HttpTransport 的 traceID + 时间范围查询组成该 TCP 流的全部网络堆栈记录；
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
