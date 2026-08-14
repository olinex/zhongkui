import * as yup from "yup";

// use field
const username = yup
  .string()
  .nonNullable()
  .required()
  .min(6)
  .max(32)
  .label("model.user.field.username");
const password = yup
  .string()
  .nonNullable()
  .required()
  .min(8)
  .max(64)
  .label("model.user.field.password");
const nickleName = yup
  .string()
  .nonNullable()
  .required()
  .min(6)
  .max(32)
  .label("model.user.field.nickleName");
const email = yup
  .string()
  .nonNullable()
  .required()
  .min(10)
  .max(128)
  .email()
  .label("model.user.field.email");

// access key field
const enabled = yup.bool().nonNullable().required().label("model.accessKey.field.enabled");
const description = yup
  .string()
  .nonNullable()
  .required()
  .min(6)
  .max(256)
  .label("model.accessKey.field.description");
const expiredAt = yup.date().default(null).nullable().label("model.accessKey.field.description");

export const loginSchema = yup.object({
  username,
  password,
});

export const resetPasswordSchema = yup.object({
  oldPassword: password,
  newPassword: password,
  newPasswordAgain: password.equals([yup.ref("newPassword")]),
});

export const changeProfileSchema = yup.object({
  nickleName,
  email,
});

export const accessKeySchema = yup.object({
  enabled,
  description,
  expiredAt,
});
