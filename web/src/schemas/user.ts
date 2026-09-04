import * as yup from "yup";

// use field
const username = yup
  .string()
  // 单行文本统一去除首尾空白，避免用户误输入不可见空白字符导致登录失败
  .trim()
  .nonNullable()
  .required()
  .min(6)
  .max(32)
  .label("model.user.field.username");
const password = yup
  .string()
  // 密码不做 trim：首尾空白是密码的合法字符，trim 会改变凭据本身
  .nonNullable()
  .required()
  .min(8)
  .max(64)
  .label("model.user.field.password");
const nickleName = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(6)
  .max(32)
  .label("model.user.field.nickleName");
const email = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(10)
  .max(128)
  .email()
  .label("model.user.field.email");
// 重置令牌，由邮件链接携带，必填且满足长度下限
const token = yup.string().trim().nonNullable().required().min(16).label("model.user.field.token");

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

export const forgotPasswordSchema = yup.object({
  username,
  email,
});

export const forgotResetPasswordSchema = yup.object({
  newPassword: password,
  newPasswordAgain: password.equals([yup.ref("newPassword")]),
});

export const resetPasswordByTokenSchema = yup.object({
  token,
  newPassword: password,
  newPasswordAgain: password.equals([yup.ref("newPassword")]),
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

export const createUserSchema = yup.object({
  username,
  password,
  passwordAgain: password.equals([yup.ref("password")]),
  nickleName,
  email,
});

export const adminResetPasswordSchema = yup.object({
  newPassword: password,
  newPasswordAgain: password.equals([yup.ref("newPassword")]),
});

export const accessKeySchema = yup.object({
  enabled,
  description,
  expiredAt,
});
