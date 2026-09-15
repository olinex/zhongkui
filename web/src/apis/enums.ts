import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

// EnumEntryOut 枚举条目：key（i18n 定位）、value（数据库存储）、label（按请求语言翻译）
export interface EnumEntryOut {
  key: string;
  value: string;
  label: string;
}

// EnumTypeOut 单个枚举类出参：类名 + 条目列表
export interface EnumTypeOut {
  type: string;
  entries: EnumEntryOut[];
}

// enumGet 按枚举类名 + 语言获取条目（/admin/enum/get）
export function enumGet(data: { type: string; lang?: string }): AxiosPromise {
  return request.post("/admin/enum/get", data, { headers: getJwtHeader() });
}

// enumList 按语言获取全部枚举类条目（/admin/enum/list）
export function enumList(data: { lang?: string }): AxiosPromise {
  return request.post("/admin/enum/list", data, { headers: getJwtHeader() });
}
