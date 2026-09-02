import * as yup from "yup";

// 定时任务类型名称（唯一标识）
const typeName = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .label("model.scheduler.field.typeName");
// 任务所属队列名称
const queue = yup.string().trim().nonNullable().required().label("model.scheduler.field.queue");
// 调度模式二选一：cron 表达式或游标触发间隔，互斥由提交逻辑兜底校验
const cronSpec = yup.string().trim().label("model.scheduler.field.cronSpec");
const interval = yup.string().trim().label("model.scheduler.field.interval");
// 执行超时与最大重试次数
const timeout = yup.string().trim().label("model.scheduler.field.timeout");
const maxRetry = yup.number().integer().min(0).nullable().label("model.scheduler.field.maxRetry");

export const schedulerTaskSchema = yup.object({
  typeName,
  cronSpec,
  interval,
  queue,
  timeout,
  maxRetry,
  retention: yup.string().trim().label("model.scheduler.field.retention"),
  lookback: yup.string().trim().label("model.scheduler.field.lookback"),
  enabled: yup.bool().nullable().label("model.scheduler.field.enabled"),
  payload: yup.mixed().nullable().label("model.scheduler.field.payload"),
  backlogEnabled: yup.bool().nullable().label("model.scheduler.field.backlogEnabled"),
  backlogWindow: yup.string().trim().label("model.scheduler.field.backlogWindow"),
  backlogLimit: yup
    .number()
    .integer()
    .min(0)
    .nullable()
    .label("model.scheduler.field.backlogLimit"),
});
