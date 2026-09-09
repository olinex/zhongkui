import { HttpStatusCode } from "axios";
import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type {
  NotificationChannelCreateIn,
  NotificationChannelSearchIn,
  NotificationChannelTypeItem,
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
  NotificationTemplateEventItemOut,
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
import { BackendDataType, SearchAction, SupportLanguage } from "@/types";

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
export function notificationTemplateEventsGet(): AxiosPromise {
  return request.post(
    "/notification/template/events",
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
// event/channelType/language 为动态枚举（来自事件与渠道类型注册表），静态选项无法覆盖
// 存量未收录的渠道/事件。此处仍前端本地组装搜索选项（表格列 + 可过滤字段）以保证筛选
// 跟随注册表实时变化：先拉取事件与渠道类型注册表，将过滤字段构建为下拉选项
// （title 为 i18n 键，渲染时经翻译展示）。
export function notificationTemplateListOptions(): AxiosPromise {
  // 事件/渠道类型枚举拉取失败时回退为空列表，保证表格列与其余过滤字段仍可用（下拉退化为空选项）
  const eventsPromise = notificationTemplateEventsGet()
    .then((rsp) => (rsp.data?.data?.events ?? []) as NotificationTemplateEventItemOut[])
    .catch(() => []);
  const channelTypesPromise = notificationChannelTypesGet()
    .then((rsp) => (rsp.data?.data?.types ?? []) as NotificationChannelTypeItem[])
    .catch(() => []);
  return Promise.all([eventsPromise, channelTypesPromise]).then(([events, channelTypes]) => {
    // 语言可选项：通配 "*" + 平台支持语言 + 事件声明语言（均经 option.language.<键> 翻译）
    const languages = [
      ...new Set(
        ["*", ...Object.values(SupportLanguage), ...events.flatMap((e) => e.languages)].filter(
          (v): v is string => !!v,
        ),
      ),
    ];
    // 事件过滤可选项：按展示名（i18n 键）升序保证确定顺序（事件键兜底），不依赖后端返回顺序
    const eventFilterOptions = events
      .map((e) => ({
        value: e.event,
        title: `model.notificationTemplate.type.event.${e.event}`,
      }))
      .sort((a, b) =>
        a.title === b.title ? a.value.localeCompare(b.value) : a.title.localeCompare(b.title),
      );
    const filterBy: FilterByOption[] = [
      {
        name: "event",
        title: "",
        type: BackendDataType.ENUM,
        actions: [SearchAction.EQUAL],
        options: eventFilterOptions,
      },
      {
        name: "channelType",
        title: "",
        type: BackendDataType.ENUM,
        actions: [SearchAction.EQUAL],
        // 通配 "*"（全部渠道）+ 渠道类型注册表（含占位符渠道，模板需覆盖全部渠道）
        options: [
          { value: "*", title: "option.channel.*" },
          ...channelTypes.map((t) => ({
            value: t.type,
            title: `model.notificationChannel.type.${t.type}`,
          })),
        ],
      },
      {
        name: "language",
        title: "",
        type: BackendDataType.ENUM,
        actions: [SearchAction.EQUAL],
        options: languages.map((language) => ({
          value: language,
          title: `option.language.${language}`,
        })),
      },
      {
        name: "source",
        title: "",
        type: BackendDataType.ENUM,
        actions: [SearchAction.EQUAL],
        options: [
          { value: "default", title: "model.notificationTemplate.type.source.default" },
          { value: "override", title: "model.notificationTemplate.type.source.override" },
        ],
      },
    ];
    const headers: HeaderOption[] = [
      { name: "id", type: BackendDataType.UINT },
      { name: "namespaceName", type: BackendDataType.STRING },
      { name: "event", type: BackendDataType.STRING },
      { name: "channelType", type: BackendDataType.STRING },
      { name: "language", type: BackendDataType.STRING },
      { name: "source", type: BackendDataType.STRING },
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
    };
  }) as unknown as AxiosPromise;
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
