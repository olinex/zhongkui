import * as yup from "yup";

const name = yup
  .string()
  .nonNullable()
  .required()
  .min(3)
  .max(32)
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
