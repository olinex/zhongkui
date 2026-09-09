import * as yup from "yup";

// 本地数据源名校验：合法 SQL 标识符（^[A-Za-z_][A-Za-z0-9_]*$），与后端
// vtab.ValidateLocalName 保持一致（正则缺失的细节如不含 __、不以 _ 结尾由后端兜底），
// 从而保证裸名 vtable {name} 是可解析的表名。
const fieldName = yup
  .string()
  .trim()
  .nonNullable()
  .required()
  .min(1)
  .max(128)
  .matches(/^[A-Za-z_][A-Za-z0-9_]*$/u)
  .label("model.datasource.field.name");

const type = yup.string().nonNullable().required().label("model.datasource.field.type");

const enabled = yup.bool().nonNullable().required().label("model.datasource.field.enabled");

const description = yup
  .string()
  .nonNullable()
  .max(256)
  .label("model.datasource.field.description");

// 创建第三方数据源表单 schema（config 为连接配置 JSON 文本，由页面按类型动态拼装并单独校验）
export const createDatasourceSchema = () =>
  yup.object({
    name: fieldName,
    type,
    enabled,
    description,
  });

// 更新第三方数据源表单 schema（名称与类型不变，仅启用/描述，config 由页面动态拼装）
export const updateDatasourceSchema = () =>
  yup.object({
    enabled,
    description,
  });