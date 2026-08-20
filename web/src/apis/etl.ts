import type { AxiosPromise } from "axios";
import { getJwtHeader, request } from "./base";
import type { InspectorDailyStatIn, InspectorStateTaskIn, InspectorTaskListIn } from "./@types/etl";

export function inspectorServerListGet(): AxiosPromise {
  return request.post("/etl/inspector/server/list", null, {
    headers: getJwtHeader(),
  });
}

export function inspectorSchedulerListGet(): AxiosPromise {
  return request.post("/etl/inspector/scheduler/list", null, {
    headers: getJwtHeader(),
  });
}

export function inspectorQueueListGet(): AxiosPromise {
  return request.post("/etl/inspector/queue/list", null, {
    headers: getJwtHeader(),
  });
}

export function inspectorQueuePauseToggle(name: string, running: boolean): AxiosPromise {
  return request.post(`/etl/inspector/queue/${name}/${running ? "pause" : "unpause"}`, null, {
    headers: getJwtHeader(),
  });
}

export function inspectorDailyStatListGet(data: InspectorDailyStatIn): AxiosPromise {
  return request.post("/etl/inspector/queue/daily-stat/list", data, {
    headers: getJwtHeader(),
  });
}

export function inspectorTaskListGet(data: InspectorTaskListIn): AxiosPromise {
  return request.post("/etl/inspector/task/list", data, {
    headers: getJwtHeader(),
  });
}

export function inspectorStateTaskDelete(data: InspectorStateTaskIn): AxiosPromise {
  return request.post("/etl/inspector/task/state/delete", data, {
    headers: getJwtHeader(),
  });
}

export function inspectorStateTaskArchive(data: InspectorStateTaskIn): AxiosPromise {
  return request.post("/etl/inspector/task/state/archive", data, {
    headers: getJwtHeader(),
  });
}

export function inspectorStateTaskRun(data: InspectorStateTaskIn): AxiosPromise {
  return request.post("/etl/inspector/task/state/run", data, {
    headers: getJwtHeader(),
  });
}

export function inspectorTaskDelete(id: string, queue: string): AxiosPromise {
  return request.post(
    `/etl/inspector/task/${id}/delete`,
    { queue: queue },
    { headers: getJwtHeader() },
  );
}

export function inspectorTaskArchive(id: string, queue: string): AxiosPromise {
  return request.post(
    `/etl/inspector/task/${id}/archive`,
    { queue: queue },
    { headers: getJwtHeader() },
  );
}

export function inspectorTaskRun(id: string, queue: string): AxiosPromise {
  return request.post(
    `/etl/inspector/task/${id}/run`,
    { queue: queue },
    { headers: getJwtHeader() },
  );
}

export function inspectorTaskCancelProcessing(id: string): AxiosPromise {
  return request.post(`/etl/inspector/task/${id}/cancel-processing`, null, {
    headers: getJwtHeader(),
  });
}
