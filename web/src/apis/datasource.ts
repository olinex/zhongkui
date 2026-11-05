import type { AxiosPromise } from "axios";
import { HttpStatusCode } from "axios";
import { getJwtHeader, request } from "./base";

import type { FilterByOption, HeaderOption, SearchOptionsOut } from "@/components/@types/table";
import type {
  DatasourceCreateIn,
  DatasourceIntrospectIn,
  DatasourceSearchIn,
  DatasourceTestIn,
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
// buildCreateForm 当 sqlite_file 携带 .db 文件时以 multipart/form-data 提交（Streaming 直通后端
// S3 加密上传）；未携带文件时仍为 JSON 体。
function buildCreateForm(
  data: DatasourceCreateIn,
  file: File | null,
): FormData | DatasourceCreateIn {
  if (!file) {
    return data;
  }
  const form = new FormData();
  form.append("name", data.name);
  form.append("type", data.type);
  form.append("enabled", String(data.enabled));
  form.append("config", data.config);
  if (data.description) {
    form.append("description", data.description);
  }
  form.append("file", file);
  return form;
}

export function datasourceListGet(data: DatasourceSearchIn): AxiosPromise {
  return request.post("/datasource/list", data, {
    headers: getJwtHeader(),
  });
}

// 本地组装列表搜索选项（与后端 OPTIONS 契约一致），确保展示 name / config（密文）等列表列，
// 并让 type 过滤以枚举筛选框呈现（经 enumType 指定的 datasourceType 枚举类解析可选项）。
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
      enumType: "datasourceType",
      actions: [SearchAction.EQUAL],
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
    { name: "type", type: BackendDataType.ENUM, enumType: "datasourceType" },
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

export function datasourceCreate(data: DatasourceCreateIn, file: File | null = null): AxiosPromise {
  return request.post("/datasource/create", buildCreateForm(data, file), {
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

export function datasourceUpdate(
  id: number,
  data: DatasourceUpdateIn,
  file: File | null = null,
): AxiosPromise {
  // 更新时仅当传入了新的 .db 文件才走 multipart（覆盖同一 S3 对象），否则为 JSON 体
  const payload: FormData | DatasourceUpdateIn = file
    ? (() => {
        const form = new FormData();
        form.append("enabled", String(data.enabled));
        form.append("config", data.config);
        if (data.description) {
          form.append("description", data.description);
        }
        form.append("file", file);
        return form;
      })()
    : data;
  return request.post(`/datasource/${id}/update`, payload, {
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

// ---- 预检（测试连接 / 自省表结构）----
// datasourceTest 测试数据源联通性：config 为配置明文 JSON 文本，出参 data.success 表明连接结果
export function datasourceTest(data: DatasourceTestIn): AxiosPromise {
  return request.post("/datasource/test", data, { headers: getJwtHeader() });
}

// datasourceIntrospect 自省数据源表结构：sqlite_file 类型以 multipart/form-data 提交（附 .db 文件），
// 其余类型为 JSON 体。出参 data.columns 为解析出的列定义。
export function datasourceIntrospect(
  data: DatasourceIntrospectIn,
  file: File | null = null,
): AxiosPromise {
  const payload: FormData | DatasourceIntrospectIn = file
    ? (() => {
        const form = new FormData();
        form.append("type", data.type);
        form.append("config", data.config);
        form.append("file", file);
        return form;
      })()
    : data;
  return request.post("/datasource/introspect", payload, { headers: getJwtHeader() });
}
