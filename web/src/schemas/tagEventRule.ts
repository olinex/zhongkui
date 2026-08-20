import * as yup from "yup";

// 与后端 TagEventRuleConditionIn 约束一致（window 在前端以分钟数输入）
export const tagEventConditionSchema = () =>
  yup.object({
    name: yup
      .string()
      .nonNullable()
      .required()
      .min(1)
      .max(255)
      .label("model.tagEventCondition.field.name"),
    model: yup.string().nonNullable().max(255).label("model.tagEventCondition.field.model"),
    dimension: yup.string().nonNullable().max(128).label("model.tagEventCondition.field.dimension"),
    item: yup.string().nonNullable().max(255).label("model.tagEventCondition.field.item"),
    window: yup.number().nonNullable().positive().label("model.tagEventCondition.field.window"),
    threshold: yup
      .number()
      .nonNullable()
      .positive()
      .label("model.tagEventCondition.field.threshold"),
    expression: yup
      .string()
      .nonNullable()
      .required()
      .max(1024)
      .label("model.tagEventCondition.field.expression"),
    enabled: yup.bool().nonNullable().required().label("model.tagEventCondition.field.enabled"),
    description: yup
      .string()
      .nonNullable()
      .max(256)
      .label("model.tagEventCondition.field.description"),
  });

const name = yup
  .string()
  .nonNullable()
  .required()
  .min(1)
  .max(255)
  .label("model.tagEventRule.field.name");
const description = yup
  .string()
  .nonNullable()
  .max(256)
  .label("model.tagEventRule.field.description");
const matchAll = yup.bool().nonNullable().required().label("model.tagEventRule.field.matchAll");
const enabled = yup.bool().nonNullable().required().label("model.tagEventRule.field.enabled");

// 条件集合至少一条，后端 conditions 亦要求 min=1
export const createTagEventRuleSchema = yup.object({
  name,
  description,
  matchAll,
  enabled,
  conditions: yup
    .array()
    .of(tagEventConditionSchema())
    .min(1)
    .label("model.tagEventRule.field.conditions"),
});

export const updateTagEventRuleSchema = yup.object({
  description,
  matchAll,
  enabled,
  conditions: yup
    .array()
    .of(tagEventConditionSchema())
    .min(1)
    .label("model.tagEventRule.field.conditions"),
});
