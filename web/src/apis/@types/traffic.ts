// TrafficType 流量审计支持的数据类型（对应后端 /api/v1/traffic/{type} 路由段）
export type TrafficType = "ipv4-tcp" | "ipv6-tcp" | "http-transport";

// TrafficListItem 流量列表项（列表接口直接返回全部字段）
// 表头默认列出常用审计字段，其余字段（密文/S3 对象键/指纹细项等）经索引签名访问，
// 供行详情对话框直接展示，无需再发起详情请求
export interface TrafficListItem {
  namespace: string;
  timestamp: string;
  createdAt: string;
  sessionId: string;
  tcpStreamId?: string | null;
  traceId?: string | null;
  capture?: string | null;
  version?: string | null;
  // ipv4-tcp / ipv6-tcp 网络层字段
  ipv4Src?: string | null;
  ipv4Dst?: string | null;
  ipv6Src?: string | null;
  ipv6Dst?: string | null;
  srcPort?: number | null;
  dstPort?: number | null;
  // ipv4-tcp / ipv6-tcp TCP 头部审计字段
  seq?: number | null;
  ack?: number | null;
  bitFin?: boolean | null;
  bitSyn?: boolean | null;
  bitRst?: boolean | null;
  bitPsh?: boolean | null;
  bitAck?: boolean | null;
  bitUrg?: boolean | null;
  window?: number | null;
  // ipv4-tcp 网络层审计字段
  ipv4Ttl?: number | null;
  ipv4IpProtocol?: number | null;
  ipv4Length?: number | null;
  // ipv6-tcp 网络层审计字段
  ipv6NextHeader?: number | null;
  ipv6HopLimit?: number | null;
  ipv6Length?: number | null;
  // ipv4-tcp / ipv6-tcp TCP 被动指纹
  fpOs?: string | null;
  fpSig?: string | null;
  fpMtu?: number | null;
  // http-transport 字段
  method?: string | null;
  host?: string | null;
  path?: string | null;
  clientIp?: string | null;
  serverIp?: string | null;
  clientPort?: number | null;
  serverPort?: number | null;
  httpVersion?: string | null;
  reqBodyLength?: number | null;
  rspBodyLength?: number | null;
  fpTcpClientOs?: string | null;
  fpTcpServerOs?: string | null;
  fpHttpBrowser?: string | null;
  fpHttpServer?: string | null;
  fpDeviceGuess?: string | null;
  // 其余全字段，键为模型字段 json 名
  [key: string]: unknown;
}

// TrafficListOut 流量列表响应
export interface TrafficListOut {
  total: number;
  items: TrafficListItem[];
}

// NetworkStackListItem HTTP 传输关联网络堆栈列表项
// 对应后端 network.NetworkStack 模型（POST /traffic/http-transport/stack/list 返回），
// 一包可能展开为多条记录（链路/网络/传输各一层），payloadStart/EndOffset 为该帧所在
// TCP 包载荷在所属方向字节流中的 [start, end) 区间，用于与 HttpTransport.AppTailer
// 的 Client/Server*PayloadOffset 相交定位请求/响应包
export interface NetworkStackListItem {
  namespace: string;
  timestamp: string;
  createdAt: string;
  sessionId: string;
  traceId: string;
  capture?: string | null;
  version?: string | null;
  cli2Srv: boolean;
  topProto?: number | null;
  proto?: number | null;
  seq?: number | null;
  level?: number | null;
  headers?: string | null;
  payloadStartOffset?: number | null;
  payloadEndOffset?: number | null;
  // 其余全字段（BaseTotalOut 的分页计数等），键为模型字段 json 名
  [key: string]: unknown;
}

// NetworkStackListOut 网络堆栈列表响应
export interface NetworkStackListOut {
  total: number;
  items: NetworkStackListItem[];
}

// PlaintextCryptoIn 加密字段明文查看请求
// model 为后端模型注册名（如 "ipv4-tcp"、"http-transport"，与 TrafficType 一致），
// field 为字段 json 名，二者与后端权限点 object（plaintext:crypto:{model}:{field}）对应
export interface PlaintextCryptoIn {
  model: string;
  field: string;
  ciphertext: string;
}

// PlaintextS3In S3 body 明文查看请求
// model/field 与后端权限点 object（plaintext:s3:{model}:{field}）对应
export interface PlaintextS3In {
  model: string;
  field: string;
  object_key: string;
}

// TrafficDetailIn 流量行详情查询入参（按 namespace + id 获取单条全字段记录）
export interface TrafficDetailIn {
  namespace: string;
  id: string;
}
