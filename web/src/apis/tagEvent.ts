import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";

import type { SearchIn } from "@/components/@types/table";
import type {
  FlexIDs,
  TagEventRecordHitRecordsIn,
  TagEventRuleCreateIn,
  TagEventRuleDryRunIn,
  TagEventRuleUpdateIn,
  TagEventRuleValidateIn,
  TagRuleCreateIn,
  TagRuleDryRunIn,
  TagRuleModelIn,
  TagRuleSampleCreateIn,
  TagRuleSampleUpdateIn,
  TagRuleUpdateIn,
  TagRuleValidateIn,
} from "./@types/admin";

// ---- 标签识别规则（命名空间化，归属当前请求命名空间）----
export function tagRuleListGet(data: SearchIn): AxiosPromise {
  return request.post("/tag-event/tag-rule/list", data, { headers: getJwtHeader() });
}

export function tagRuleListOptions(): AxiosPromise {
  return request.options("/tag-event/tag-rule/list", { headers: getJwtHeader() });
}

export function tagRuleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/tag-event/tag-rule/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleCreate(data: TagRuleCreateIn): AxiosPromise {
  return request.post("/tag-event/tag-rule/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleUpdate(id: number, data: TagRuleUpdateIn): AxiosPromise {
  return request.post(`/tag-event/tag-rule/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/tag-event/tag-rule/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleSetEnabled(ids: FlexIDs, enabled: boolean): AxiosPromise {
  return request.post(
    `/tag-event/tag-rule/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleValidate(data: TagRuleValidateIn): AxiosPromise {
  return request.post("/tag-event/tag-rule/validate", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleModelMetadata(data: TagRuleModelIn): AxiosPromise {
  return request.post("/tag-event/tag-rule/model/metadata", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleModelSamples(data: TagRuleModelIn): AxiosPromise {
  return request.post("/tag-event/tag-rule/model/samples", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleDryRun(data: TagRuleDryRunIn): AxiosPromise {
  return request.post("/tag-event/tag-rule/single/dry-run", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleFunctions(): AxiosPromise {
  return request.post(
    "/tag-event/tag-rule/functions",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleSqliteMeta(): AxiosPromise {
  return request.post(
    "/tag-event/tag-rule/sqlite/meta",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleVersionList(id: number): AxiosPromise {
  return request.post(
    `/tag-event/tag-rule/${id}/version/list`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleVersionRollback(id: number, version: number): AxiosPromise {
  return request.post(`/tag-event/tag-rule/${id}/version/${version}/rollback`, null, {
    headers: getJwtHeader(),
  });
}

// ---- 标签规则样例（命名空间化，归属当前请求命名空间）----
export function tagRuleSampleCreate(data: TagRuleSampleCreateIn): AxiosPromise {
  return request.post("/tag-event/tag-rule-sample/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleListGet(data: SearchIn): AxiosPromise {
  return request.post("/tag-event/tag-rule-sample/list", data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleListOptions(): AxiosPromise {
  return request.options("/tag-event/tag-rule-sample/list", {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/tag-event/tag-rule-sample/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagRuleSampleUpdate(id: number, data: TagRuleSampleUpdateIn): AxiosPromise {
  return request.post(`/tag-event/tag-rule-sample/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagRuleSampleDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/tag-event/tag-rule-sample/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 标签事件规则（命名空间化，归属当前请求命名空间）----
export function tagEventRuleListGet(data: SearchIn): AxiosPromise {
  return request.post("/tag-event/tag-event-rule/list", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleListOptions(): AxiosPromise {
  return request.options("/tag-event/tag-event-rule/list", {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDetailGet(id: number): AxiosPromise {
  return request.post(
    `/tag-event/tag-event-rule/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleCreate(data: TagEventRuleCreateIn): AxiosPromise {
  return request.post("/tag-event/tag-event-rule/create", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleUpdate(id: number, data: TagEventRuleUpdateIn): AxiosPromise {
  return request.post(`/tag-event/tag-event-rule/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDelete(ids: FlexIDs): AxiosPromise {
  return request.post(
    "/tag-event/tag-event-rule/delete",
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleSetEnabled(ids: FlexIDs, enabled: boolean): AxiosPromise {
  return request.post(
    `/tag-event/tag-event-rule/${enabled ? "enable" : "disable"}`,
    { ids },
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleValidate(data: TagEventRuleValidateIn): AxiosPromise {
  return request.post("/tag-event/tag-event-rule/validate", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleDryRun(data: TagEventRuleDryRunIn): AxiosPromise {
  return request.post("/tag-event/tag-event-rule/dry-run", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRuleVersionList(id: number): AxiosPromise {
  return request.post(
    `/tag-event/tag-event-rule/${id}/version/list`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function tagEventRuleVersionRollback(id: number, version: number): AxiosPromise {
  return request.post(`/tag-event/tag-event-rule/${id}/version/${version}/rollback`, null, {
    headers: getJwtHeader(),
  });
}

// ---- 标签事件记录（命名空间化，归属当前请求命名空间）----
export function tagEventRecordListGet(data: SearchIn): AxiosPromise {
  return request.post("/tag-event/record/list", data, {
    headers: getJwtHeader(),
  });
}

export function tagEventRecordListOptions(): AxiosPromise {
  return request.options("/tag-event/record/list", {
    headers: getJwtHeader(),
  });
}

export function tagEventRecordDetailGet(id: number): AxiosPromise {
  return request.post(
    `/tag-event/record/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// tagEventRecordHitRecordsGet 命中条件原始数据反查：前端从详情透传命中明细的
// 模型/原始记录 ID 列表与入库时间范围，后端按 created_at 分区预筛查询明细行
export function tagEventRecordHitRecordsGet(data: TagEventRecordHitRecordsIn): AxiosPromise {
  return request.post("/tag-event/record/hit/records", data, {
    headers: getJwtHeader(),
  });
}
