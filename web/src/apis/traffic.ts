import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import { BackendDataType } from "@/types";
import type { SearchIn } from "@/components/@types/table";
import { HTTP_TRANSPORT_MODEL_KEY } from "@/utils/rego";
import type {
  PlaintextCryptoIn,
  PlaintextS3In,
  TrafficDetailIn,
  TrafficType,
} from "./@types/traffic";

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

// TrafficExtraFieldTypes 流量模型非 STRING 字段渲染类型（按字段 json 名，缺省 STRING）：
// 仅覆盖需要特殊渲染的非常用字段，供列表/详情/样例文档按列表页方式（AdaptiveCell）统一渲染；
// 类型依据 internal/models/network 下三个模型（PacketIPv4TCP / PacketIPv6TCP / HttpTransport）的 JSON tag 逐一核对
export const TrafficExtraFieldTypes: Record<string, BackendDataType> = {
  // 布尔标志：捕获状态与 TCP 扩展标志
  truncated: BackendDataType.BOOLEAN,
  bitEce: BackendDataType.BOOLEAN,
  bitCwr: BackendDataType.BOOLEAN,
  bitNs: BackendDataType.BOOLEAN,
  // 复合结构：JSON 预览
  tags: BackendDataType.JSON,
  options: BackendDataType.JSON,
  queries: BackendDataType.JSON,
  layerStack: BackendDataType.JSON,
  ipv4Options: BackendDataType.JSON,
  ipv6HopByHop: BackendDataType.JSON,
  // 时间戳
  clientStartTimestamp: BackendDataType.DATETIME,
  clientEndTimestamp: BackendDataType.DATETIME,
  serverStartTimestamp: BackendDataType.DATETIME,
  serverEndTimestamp: BackendDataType.DATETIME,
  // 字节数与负载偏移（uint32 / uint64）
  captureLength: BackendDataType.UINT32,
  clientConsumed: BackendDataType.UINT64,
  clientStartPayloadOffset: BackendDataType.UINT64,
  clientEndPayloadOffset: BackendDataType.UINT64,
  serverConsumed: BackendDataType.UINT64,
  serverStartPayloadOffset: BackendDataType.UINT64,
  serverEndPayloadOffset: BackendDataType.UINT64,
  // TCP 数值字段
  dataOffset: BackendDataType.UINT8,
  checksum: BackendDataType.UINT16,
  urgent: BackendDataType.UINT16,
  // IPv4 数值字段
  ipv4Version: BackendDataType.UINT8,
  ipv4Ihl: BackendDataType.UINT8,
  ipv4Tos: BackendDataType.UINT8,
  ipv4ID: BackendDataType.UINT16,
  ipv4Flags: BackendDataType.UINT8,
  ipv4FragOffset: BackendDataType.UINT16,
  ipv4Checksum: BackendDataType.UINT16,
  // IPv6 数值字段
  ipv6Version: BackendDataType.UINT8,
  ipv6TrafficClass: BackendDataType.UINT8,
  ipv6FlowLabel: BackendDataType.UINT32,
  ipv6LastHeader: BackendDataType.UINT8,
  // 链路层数值字段
  ethType: BackendDataType.UINT16,
  ethLength: BackendDataType.UINT16,
  // HTTP 传输数值字段
  clientIsn: BackendDataType.UINT32,
  serverIsn: BackendDataType.UINT32,
  streamIndex: BackendDataType.UINT32,
  ipVersion: BackendDataType.UINT8,
  fpTcpMtu: BackendDataType.UINT16,
  // 指纹置信度（float64，0-1 区间）
  fpOsQuality: BackendDataType.FLOAT64,
  fpTcpClientOsQuality: BackendDataType.FLOAT64,
  fpTcpServerOsQuality: BackendDataType.FLOAT64,
  fpHttpBrowserQuality: BackendDataType.FLOAT64,
  fpHttpServerQuality: BackendDataType.FLOAT64,
};

// TrafficFieldTypes 各类型流量模型字段渲染类型（含通用/特有字段，缺省 STRING）：
// 供样例文档、Rego 试运行等无法从表头推导类型的场景，按列表页方式（AdaptiveCell）渲染字段值
export const TrafficFieldTypes: Record<TrafficType, Record<string, BackendDataType>> = {
  "ipv4-tcp": {
    ...TrafficExtraFieldTypes,
    timestamp: BackendDataType.DATETIME,
    createdAt: BackendDataType.DATETIME,
    sessionID: BackendDataType.UUID,
    tcpStreamID: BackendDataType.HEX,
    srcPort: BackendDataType.UINT16,
    dstPort: BackendDataType.UINT16,
    seq: BackendDataType.UINT32,
    ack: BackendDataType.UINT32,
    bitFin: BackendDataType.BOOLEAN,
    bitSyn: BackendDataType.BOOLEAN,
    bitRst: BackendDataType.BOOLEAN,
    bitPsh: BackendDataType.BOOLEAN,
    bitAck: BackendDataType.BOOLEAN,
    bitUrg: BackendDataType.BOOLEAN,
    window: BackendDataType.UINT16,
    ipv4Ttl: BackendDataType.UINT8,
    ipv4IpProtocol: BackendDataType.UINT8,
    ipv4Length: BackendDataType.UINT16,
    fpMtu: BackendDataType.UINT16,
  },
  "ipv6-tcp": {
    ...TrafficExtraFieldTypes,
    timestamp: BackendDataType.DATETIME,
    createdAt: BackendDataType.DATETIME,
    sessionID: BackendDataType.UUID,
    tcpStreamID: BackendDataType.HEX,
    srcPort: BackendDataType.UINT16,
    dstPort: BackendDataType.UINT16,
    seq: BackendDataType.UINT32,
    ack: BackendDataType.UINT32,
    bitFin: BackendDataType.BOOLEAN,
    bitSyn: BackendDataType.BOOLEAN,
    bitRst: BackendDataType.BOOLEAN,
    bitPsh: BackendDataType.BOOLEAN,
    bitAck: BackendDataType.BOOLEAN,
    bitUrg: BackendDataType.BOOLEAN,
    window: BackendDataType.UINT16,
    ipv6HopLimit: BackendDataType.UINT8,
    ipv6NextHeader: BackendDataType.UINT8,
    ipv6Length: BackendDataType.UINT16,
    fpMtu: BackendDataType.UINT16,
  },
  "http-transport": {
    ...TrafficExtraFieldTypes,
    timestamp: BackendDataType.DATETIME,
    createdAt: BackendDataType.DATETIME,
    sessionID: BackendDataType.UUID,
    traceID: BackendDataType.UUID,
    clientPort: BackendDataType.UINT16,
    serverPort: BackendDataType.UINT16,
    reqBodyLength: BackendDataType.DECIMAL_BYTE,
    rspBodyLength: BackendDataType.DECIMAL_BYTE,
  },
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
