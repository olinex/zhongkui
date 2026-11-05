import * as yup from "yup";

const name = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(3)
  .max(32)
  // 与后端 alphanumspecial=_- 一致：Unicode 字母数字与 _-，
  // 保证名称可作为 URL 路径参数安全传递
  .matches(/^[\p{L}\p{N}_-]+$/u)
  .label("model.namespace.field.name");
const description = yup
  .string()
  .nonNullable()
  .required()
  .min(6)
  .max(256)
  .label("model.namespace.field.description");

export const createNamespaceSchema = yup.object({
  name,
  description,
});

export const updateNamespaceSchema = yup.object({
  description,
});
