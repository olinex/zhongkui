import * as yup from "yup";

// 与后端 TagRuleCreateIn 约束一致（alphanumspecial=_-）：
// 仅含 Unicode 字母数字及 `_`/`-`，且至少包含一个字母或数字（不允许仅有特殊符号），长度 1~128
const name = yup
  .string()
  .nonNullable()
  .required()
  .matches(/^(?=.*[\p{L}\p{N}])[\p{L}\p{N}_-]+$/u)
  .min(1)
  .max(128)
  .label("model.tagRule.field.name");
const model = yup.string().nonNullable().required().max(255).label("model.tagRule.field.model");
const description = yup.string().nonNullable().max(512).label("model.tagRule.field.description");
const enabled = yup.bool().nonNullable().required().label("model.tagRule.field.enabled");

// 注意：rego 不纳入 yup schema——它由 RegoEditor 单独校验（createRegoValid/editRegoValid），
// 且未通过 defineField 绑定到表单，若加入 required 会导致表单 meta.valid 恒为 false，
// 确认按钮永远不可用。
export const createTagRuleSchema = yup.object({
  name,
  model,
  description,
  enabled,
});

// 更新仅允许修改 description/rego/enabled，名称与模型不可变
export const updateTagRuleSchema = yup.object({
  description,
  enabled,
});
