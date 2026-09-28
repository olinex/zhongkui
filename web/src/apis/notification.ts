import { HttpStatusCode } from "axios";
import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type {
  NotificationChannelCreateIn,
  NotificationChannelSearchIn,
  NotificationChannelUpdateIn,
  NotificationGroupCreateIn,
  NotificationGroupSearchIn,
  NotificationGroupUpdateIn,
  NotificationRecordSearchIn,
  NotificationTargetCreateIn,
  NotificationTargetSearchIn,
  NotificationTargetUpdateIn,
  NotificationTemplateCreateIn,
  NotificationTemplateDryRunIn,
  NotificationTemplateRestoreDefaultIn,
  NotificationTemplateRenderTestIn,
  NotificationTemplateUpdateIn,
  NotificationTemplateValidateIn,
} from "./@types/notification";
import type {
  FilterByOption,
  HeaderOption,
  SearchIn,
  SearchOptionsOut,
} from "@/components/@types/table";

// 工具
import { EnumClass, enumOptionsOf, ensureEnum } from "@/utils/enums";
import { BackendDataType, SearchAction } from "@/types";

// ---- 通知渠道类型 ----
export function notificationChannelTypesGet(): AxiosPromise {
  return request.post(
    "/notification/channel/types",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 通知渠道（命名空间化，归属当前请求命名空间）----
export function notificationChannelListGet(data: NotificationChannelSearchIn): AxiosPromise {
  return request.post("/notification/channel/list", data, {
    headers: getJwtHeader(),
  });
}

export function notificationChannelListOptions(): AxiosPromise {
  return request.options("/notification/channel/list", {
    headers: getJwtHeader(),
  });
}

export function notificationChannelCreate(data: NotificationChannelCreateIn): AxiosPromise {
  return request.post("/notification/channel/create", data, {
    headers: getJwtHeader(),
  });
}

export function notificationChannelDetailGet(id: number): AxiosPromise {
  return request.post(
    `/notification/channel/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationChannelUpdate(
  id: number,
  data: NotificationChannelUpdateIn,
): AxiosPromise {
  return request.post(`/notification/channel/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function notificationChannelBatchDelete(ids: number[]): AxiosPromise {
  return request.post(
    "/notification/channel/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationChannelBatchSetEnabled(enabled: boolean, ids: number[]): AxiosPromise {
  return request.post(
    `/notification/channel/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 通知目标（命名空间化，归属当前请求命名空间）----
export function notificationTargetListGet(data: NotificationTargetSearchIn): AxiosPromise {
  return request.post("/notification/target/list", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTargetListOptions(): AxiosPromise {
  return request.options("/notification/target/list", {
    headers: getJwtHeader(),
  });
}

export function notificationTargetCreate(data: NotificationTargetCreateIn): AxiosPromise {
  return request.post("/notification/target/create", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTargetDetailGet(id: number): AxiosPromise {
  return request.post(
    `/notification/target/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationTargetUpdate(
  id: number,
  data: NotificationTargetUpdateIn,
): AxiosPromise {
  return request.post(`/notification/target/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function notificationTargetBatchDelete(ids: number[]): AxiosPromise {
  return request.post(
    "/notification/target/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationTargetBatchSetEnabled(enabled: boolean, ids: number[]): AxiosPromise {
  return request.post(
    `/notification/target/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationTargetTestSend(id: number): AxiosPromise {
  return request.post(
    `/notification/target/${id}/test-send`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 通知分组（命名空间化，归属当前请求命名空间）----
export function notificationGroupListGet(data: NotificationGroupSearchIn): AxiosPromise {
  return request.post("/notification/group/list", data, {
    headers: getJwtHeader(),
  });
}

export function notificationGroupListOptions(): AxiosPromise {
  return request.options("/notification/group/list", {
    headers: getJwtHeader(),
  });
}

export function notificationGroupDetailGet(id: number): AxiosPromise {
  return request.post(
    `/notification/group/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationGroupCreate(data: NotificationGroupCreateIn): AxiosPromise {
  return request.post("/notification/group/create", data, {
    headers: getJwtHeader(),
  });
}

export function notificationGroupUpdate(id: number, data: NotificationGroupUpdateIn): AxiosPromise {
  return request.post(`/notification/group/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function notificationGroupBatchDelete(ids: number[]): AxiosPromise {
  return request.post(
    "/notification/group/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function notificationGroupSetDefault(id: number): AxiosPromise {
  return request.post(
    `/notification/group/${id}/set-default`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 通知记录（命名空间化，归属当前请求命名空间）----
export function notificationRecordListGet(data: NotificationRecordSearchIn): AxiosPromise {
  return request.post("/notification/record/list", data, {
    headers: getJwtHeader(),
  });
}

export function notificationRecordListOptions(): AxiosPromise {
  return request.options("/notification/record/list", {
    headers: getJwtHeader(),
  });
}

export function notificationRecordDetailGet(id: number): AxiosPromise {
  return request.post(
    `/notification/record/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 通知模板（命名空间化，归属当前请求命名空间）----
// notificationTemplateEventsMetaGet 通知模板事件元数据（events/meta）：
// 仅编辑器语言提示与变量补全所需，不承载枚举值集合；
// 事件枚举值经统一枚举接口 notificationEvent 类获取（出参 NotificationTemplateEventMetaOut）。
export function notificationTemplateEventsMetaGet(): AxiosPromise {
  return request.post(
    "/notification/template/events/meta",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// search-table 以通用 SearchIn（filterBy 对象）提交过滤条件；后端模板列表接口已接入
// 统一搜索基座（BuildSearchQuery / ListMetaGinHandler），直接透传即可。
export function notificationTemplateListGet(data: SearchIn): AxiosPromise {
  return request.post("/notification/template/list", data, {
    headers: getJwtHeader(),
  });
}

// 后端 /template/list OPTIONS 虽已接入 ListMetaGinHandler（提供静态搜索选项），但模板的
// event/channelType/language 为枚举（来自统一枚举注册表），静态选项无法覆盖存量未收录的
// 渠道/事件。此处仍前端本地组装搜索选项（表格列 + 可过滤字段）：先确保相关枚举类按当前语言
// 缓存（filterBy 携带 enumType，SearchTable 渲染期经枚举 store 解析可选项），语言筛选另附
// 通配 "*" 选项；source 与 event 不带显式 options，由 SearchTable 按 enumType 自动解析。
export async function notificationTemplateListOptions(): Promise<AxiosPromise> {
  // 事件/渠道/语言/来源枚举类确保缓存（store 内部在途去重，未命中下拉由 SearchTable 兜底）
  await Promise.all([
    ensureEnum(EnumClass.notificationEvent),
    ensureEnum(EnumClass.channelType),
    ensureEnum(EnumClass.language),
    ensureEnum(EnumClass.notificationSource),
  ]);
  const filterBy: FilterByOption[] = [
    {
      name: "event",
      title: "",
      type: BackendDataType.ENUM,
      enumType: "notificationEvent",
      actions: [SearchAction.EQUAL],
    },
    {
      name: "channelType",
      title: "",
      type: BackendDataType.ENUM,
      enumType: "channelType",
      actions: [SearchAction.EQUAL],
      // 通配 "*"（全部渠道）+ 统一枚举注册表渠道类型选项（含占位符渠道，模板需覆盖全部渠道）
      options: [{ value: "*", title: "option.channel.*" }, ...enumOptionsOf(EnumClass.channelType)],
    },
    {
      name: "language",
      title: "",
      type: BackendDataType.ENUM,
      enumType: "language",
      actions: [SearchAction.EQUAL],
      options: [{ value: "*", title: "option.language.*" }, ...enumOptionsOf(EnumClass.language)],
    },
    {
      name: "source",
      title: "",
      type: BackendDataType.ENUM,
      enumType: "notificationSource",
      actions: [SearchAction.EQUAL],
    },
  ];
  const headers: HeaderOption[] = [
    { name: "id", type: BackendDataType.UINT },
    { name: "namespaceName", type: BackendDataType.STRING },
    { name: "event", type: BackendDataType.ENUM, enumType: "notificationEvent" },
    { name: "channelType", type: BackendDataType.ENUM, enumType: "channelType" },
    { name: "language", type: BackendDataType.ENUM, enumType: "language" },
    { name: "source", type: BackendDataType.ENUM, enumType: "notificationSource" },
    // 标题/正文为长度不固定的长文本列，默认隐藏（可在列设置中按需开启）
    { name: "title", type: BackendDataType.STRING, defaultVisible: false },
    { name: "content", type: BackendDataType.STRING, defaultVisible: false },
    { name: "description", type: BackendDataType.STRING },
  ];
  const options: SearchOptionsOut = { orderBy: [], filterBy, headers };
  // 模拟真实后端 OPTIONS 响应信封 { code, data }，与 getResponse/SearchTable 的解析约定一致
  return {
    status: HttpStatusCode.Ok,
    statusText: "OK",
    headers: {},
    data: { code: HttpStatusCode.Ok, data: options },
  } as unknown as AxiosPromise;
}

export function notificationTemplateCreate(data: NotificationTemplateCreateIn): AxiosPromise {
  return request.post("/notification/template/create", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTemplateUpdate(data: NotificationTemplateUpdateIn): AxiosPromise {
  return request.post("/notification/template/update", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTemplateRestoreDefault(
  data: NotificationTemplateRestoreDefaultIn,
): AxiosPromise {
  return request.post("/notification/template/restore-default", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTemplateRenderTest(
  data: NotificationTemplateRenderTestIn,
): AxiosPromise {
  return request.post("/notification/template/render-test", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTemplateValidate(data: NotificationTemplateValidateIn): AxiosPromise {
  return request.post("/notification/template/validate", data, {
    headers: getJwtHeader(),
  });
}

export function notificationTemplateDryRun(data: NotificationTemplateDryRunIn): AxiosPromise {
  return request.post("/notification/template/dry-run", data, {
    headers: getJwtHeader(),
  });
}
