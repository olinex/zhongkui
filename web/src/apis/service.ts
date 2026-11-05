// 服务状态（service status）接口。
import type { AxiosPromise } from "axios";

import { getJwtHeader, request } from "./base";

// serviceListGet 读取 etcd 服务目录，返回全部运行中服务实例的状态。
export function serviceListGet(): AxiosPromise {
  return request.post("/admin/service/list", {}, { headers: getJwtHeader() });
}

// 复导出类型，方便消费方按需 import。
export type { ServiceItemOut, ServiceListOut } from "./@types/service";
