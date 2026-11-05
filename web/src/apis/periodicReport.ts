import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";
import type {
  PeriodicReportFeedListIn,
  PeriodicReportPlanCreateIn,
  PeriodicReportPlanListIn,
  PeriodicReportPlanUpdateIn,
  PeriodicReportRunListIn,
  PeriodicReportWatchListIn,
  PeriodicReportWatchSetIn,
} from "./@types/periodicReport";

// ---- 报告技能选项 ----
export function periodicReportSkillOptionsGet(): AxiosPromise {
  return request.post(
    "/ai/periodic-report/skill/options",
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 报告计划 ----
export function periodicReportPlanListGet(data: PeriodicReportPlanListIn): AxiosPromise {
  return request.post("/ai/periodic-report/plan/list", data, {
    headers: getJwtHeader(),
  });
}

// 报告计划列表 OPTIONS：下发表头/筛选项/排序字段与字段投影白名单，供通用列表页自省
export function periodicReportPlanListOptions(): AxiosPromise {
  return request.options("/ai/periodic-report/plan/list", {
    headers: getJwtHeader(),
  });
}

export function periodicReportPlanCreate(data: PeriodicReportPlanCreateIn): AxiosPromise {
  return request.post("/ai/periodic-report/plan/create", data, {
    headers: getJwtHeader(),
  });
}

export function periodicReportPlanUpdate(id: number, data: PeriodicReportPlanUpdateIn): AxiosPromise {
  return request.post(`/ai/periodic-report/plan/${id}/update`, data, {
    headers: getJwtHeader(),
  });
}

export function periodicReportPlanToggle(id: number, enabled: boolean): AxiosPromise {
  return request.post(
    `/ai/periodic-report/plan/${id}/toggle`,
    { enabled },
    {
      headers: getJwtHeader(),
    },
  );
}

export function periodicReportPlanDelete(id: number): AxiosPromise {
  return request.post(
    `/ai/periodic-report/plan/${id}/delete`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function periodicReportPlanRun(id: number): AxiosPromise {
  return request.post(
    `/ai/periodic-report/plan/${id}/run`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 报告运行记录 ----
export function periodicReportRunListGet(data: PeriodicReportRunListIn): AxiosPromise {
  return request.post("/ai/periodic-report/run/list", data, {
    headers: getJwtHeader(),
  });
}

export function periodicReportRunDetailGet(id: number): AxiosPromise {
  return request.post(
    `/ai/periodic-report/run/${id}/get`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

export function periodicReportRunDelete(id: number): AxiosPromise {
  return request.post(
    `/ai/periodic-report/run/${id}/delete`,
    {},
    {
      headers: getJwtHeader(),
    },
  );
}

// ---- 关注计划 ----
export function periodicReportWatchListGet(data: PeriodicReportWatchListIn): AxiosPromise {
  return request.post("/ai/periodic-report/watch/list", data, {
    headers: getJwtHeader(),
  });
}

export function periodicReportWatchSet(data: PeriodicReportWatchSetIn): AxiosPromise {
  return request.post("/ai/periodic-report/watch/set", data, {
    headers: getJwtHeader(),
  });
}

// ---- 报告信息流 ----
export function periodicReportFeedListGet(data: PeriodicReportFeedListIn): AxiosPromise {
  return request.post("/ai/periodic-report/feed/list", data, {
    headers: getJwtHeader(),
  });
}

// periodicReportEnvelopeContent 按统一信封契约从运行记录正文提取 Markdown：后端存的是
// {"content":"<Markdown 报告正文>","draft":{...}}，展示时取 content；历史纯 Markdown
// 记录（解析失败）原样展示，保证旧数据仍可查看。
export function periodicReportEnvelopeContent(raw: string): string {
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed !== null && typeof parsed === "object" && "content" in parsed) {
      const content = (parsed as { content?: unknown }).content;
      if (typeof content === "string") {
        return content;
      }
    }
  } catch {
    // 非 JSON：按历史 Markdown 正文原样返回
  }
  return raw;
}
