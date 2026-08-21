import * as yup from "yup";

// 与后端 TagRuleSampleUpdateIn 约束一致：name 支持字母数字及 `_`/`-`，1~128
const name = yup
  .string()
  .nonNullable()
  .required()
  .matches(/^[a-zA-Z0-9_-]+$/)
  .min(1)
  .max(128)
  .label("model.tagRuleSample.field.name");
const description = yup.string().nonNullable().max(512).label("model.tagRuleSample.field.description");

// 更新仅允许修改 name/description，model 与 input 不可变
export const updateTagRuleSampleSchema = yup.object({
  name,
  description,
});
