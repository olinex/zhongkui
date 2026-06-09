import * as yup from "yup";

const namespace = yup
  .string()
  .nonNullable()
  .required()
  .min(3)
  .max(32)
  .label("model.capturer.field.namespace");
const name = yup
  .string()
  .nonNullable()
  .required()
  .min(3)
  .max(32)
  .label("model.capturer.field.name");
const version = yup
  .string()
  .nonNullable()
  .required()
  .min(3)
  .max(32)
  .label("model.capturer.field.version");
const device = yup
  .string()
  .nonNullable()
  .required()
  .min(2)
  .max(32)
  .label("model.capturer.field.device");
const bpfExpr = yup
  .string()
  .nonNullable()
  .required()
  .min(3)
  .max(512)
  .label("model.capturer.field.bpfExpr");
const snapLen = yup
  .number()
  .nonNullable()
  .required()
  .min(1)
  .max(65535)
  .label("model.capturer.field.snapLen");
const timeout = yup
  .number()
  .nonNullable()
  .required()
  .min(1)
  .max(24 * 60 * 60)
  .label("model.capturer.field.timeout");
const promisc = yup
  .bool()
  .nonNullable()
  .required()
  .label("model.capturer.field.promisc");
const enabled = yup
  .bool()
  .nonNullable()
  .required()
  .label("model.capturer.field.enabled");

export const updateCapturerSchema = yup.object({
  device,
  bpfExpr,
  snapLen,
  timeout,
  promisc,
  enabled,
});

export const createCapturerSchema = yup.object({
  namespace,
  name,
  version,
  device,
  bpfExpr,
  snapLen,
  timeout,
  promisc,
  enabled,
});
