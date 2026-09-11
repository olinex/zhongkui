import type { AxiosPromise } from "axios";
import { HttpStatusCode } from "axios";
import { getJwtHeader, request } from "./base";
import { DynamicEnums, dynamicEnumOptionKey } from "@/constants/dynamicEnums";

import type { FilterByOption, HeaderOption, SearchOptionsOut } from "@/components/@types/table";
import type {
  DatasourceCreateIn,
  DatasourceSearchIn,
  DatasourceUpdateIn,
} from "./@types/datasource";
import { BackendDataType, SearchAction } from "@/types";

// ---- 第三方数据源类型枚举 ----
export function datasourceTypesGet(): AxiosPromise {
  return request.post(
    "/datasource/types",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 第三方数据源（命名空间化，归属当前请求命名空间）----
export function datasourceListGet(data: DatasourceSearchIn): AxiosPromise {
  return request.post("/datasource/list", data, {
    headers: getJwtHeader(),
  });
}

// 本地组装列表搜索选项（与后端 OPTIONS 契约一致），确保展示 name / config（密文）等列表列，
// 并让 type 过滤以枚举筛选框呈现（option 渲染走 i18n）。
export function datasourceListOptions(): AxiosPromise {
  const filterBy: FilterByOption[] = [
    {
      name: "name",
      title: "",
      type: BackendDataType.STRING,
      actions: [SearchAction.EQUAL, SearchAction.CONTAIN],
    },
    {
      name: "type",
      title: "",
      type: BackendDataType.ENUM,
      actions: [SearchAction.EQUAL],
      options: [
        { value: "redis", title: dynamicEnumOptionKey(DynamicEnums.datasourceType, "redis") },
        { value: "mysql", title: dynamicEnumOptionKey(DynamicEnums.datasourceType, "mysql") },
      ],
    },
    {
      name: "enabled",
      title: "",
      type: BackendDataType.BOOLEAN,
      actions: [SearchAction.EQUAL],
    },
  ];
  const headers: HeaderOption[] = [
    { name: "id", type: BackendDataType.UINT },
    { name: "createdAt", type: BackendDataType.DATETIME },
    { name: "updatedAt", type: BackendDataType.DATETIME },
    { name: "name", type: BackendDataType.STRING },
    { name: "type", type: BackendDataType.ENUM },
    { name: "enabled", type: BackendDataType.BOOLEAN },
    // config 为密文，列表默认隐藏（可在列设置中开启）
    { name: "config", type: BackendDataType.STRING, defaultVisible: false },
    { name: "description", type: BackendDataType.STRING },
  ];
  const options: SearchOptionsOut = { orderBy: [], filterBy, headers };
  // 与真实后端 OPTIONS 响应信封 { code, data } 对齐，供 getResponse/SearchTable 解析
  return {
    status: HttpStatusCode.Ok,
    statusText: "OK",
    headers: {},
    data: { code: HttpStatusCode.Ok, data: options },
  } as unknown as AxiosPromise;
}

export function datasourceCreate(data: DatasourceCreateIn): AxiosPromise {
  return request.post("/datasource/create", data, {
    headers: getJwtHeader(),
  });
}

export function datasourceDetailGet(id: number): AxiosPromise {
  return request.post(
    `/datasource/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function datasourceUpdate(id: number, data: DatasourceUpdateIn): AxiosPromise {
  return request.post(`/datasource/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function datasourceBatchDelete(ids: number[]): AxiosPromise {
  return request.post(
    "/datasource/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function datasourceBatchSetEnabled(enabled: boolean, ids: number[]): AxiosPromise {
  return request.post(
    `/datasource/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}
