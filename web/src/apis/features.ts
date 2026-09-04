// 全局功能开关（feature flags）接口。
// Global feature-flag API.
import type { AxiosPromise } from "axios";

import { request } from "./base";

// featuresListGet 获取全部功能开关（key + enabled），供前端决定是否展示相关组件。
export function featuresListGet(): AxiosPromise {
  return request.post("/features/list");
}

// 复导出类型，方便消费方按需 import。
export type { FeaturesListGetOut, FeatureOut } from "./@types/features";
