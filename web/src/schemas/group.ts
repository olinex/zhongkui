import * as yup from "yup";

const name = yup.string().nonNullable().required().min(3).max(32).label("model.group.field.name");
const description = yup
  .string()
  .nonNullable()
  .required()
  .min(6)
  .max(256)
  .label("model.group.field.description");

export const joinGroupSchema = yup.object({
  name,
});

export const createGroupSchema = yup.object({
  name,
  description,
});

export const updateGroupSchema = yup.object({
  description,
});
