import * as yup from "yup";

// 与后端 TagRuleCreateIn 约束一致：name 支持字母数字及 `_`/`-`，1~128
const name = yup
  .string()
  .nonNullable()
  .required()
  .matches(/^[a-zA-Z0-9_-]+$/)
  .min(1)
  .max(128)
  .label("model.tagRule.field.name");
const model = yup.string().nonNullable().required().max(255).label("model.tagRule.field.model");
const description = yup.string().nonNullable().max(512).label("model.tagRule.field.description");
const rego = yup.string().nonNullable().required().label("model.tagRule.field.rego");
const enabled = yup.bool().nonNullable().required().label("model.tagRule.field.enabled");

export const createTagRuleSchema = yup.object({
  name,
  model,
  description,
  rego,
  enabled,
});

// 更新仅允许修改 description/rego/enabled，名称与模型不可变
export const updateTagRuleSchema = yup.object({
  description,
  rego,
  enabled,
});
