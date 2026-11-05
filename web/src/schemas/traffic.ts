import { DateTime } from "luxon";

import * as yup from "yup";

// 流量列表筛选：timestamp（采集时间）/ createdAt（入库时间）两个时间范围均必填，
// 每个范围为 [起始, 截止] 二元组（缺失时后端返回 400）
const timestampRange = yup
  .mixed<[DateTime, DateTime]>()
  .nonNullable()
  .required()
  .label("model.__base.field.timestampRange");
const createdAtRange = yup
  .mixed<[DateTime, DateTime]>()
  .nonNullable()
  .required()
  .label("model.__base.field.createdAtRange");

export const trafficFilterSchema = yup.object({
  timestampRange,
  createdAtRange,
});
