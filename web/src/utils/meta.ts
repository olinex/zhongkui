// 列表接口 OPTIONS 自省元数据的前端读取工具：列展示语义（kind）、按 kind 归集的字段名
// 与单元格渲染类型统一从 FieldMeta 派生，页面与表格组件不重复维护字段清单。
import type { ApiMeta, FieldMeta } from "@/components/@types/meta";
import { BackendDataType } from "@/types";

// fieldMetaOfMeta 从列表接口自省响应（MetaOut）的 bodyOutput 中按 JSON 字段名取字段元信息；
// meta 未下发或字段不存在时返回 undefined，消费方据此回退到表头自身的声明。
export const fieldMetaOfMeta = (
  meta: ApiMeta | undefined,
  field: string,
): FieldMeta | undefined => meta?.POST?.bodyOutput?.fields.find((f) => f.name === field);

// fieldKindSetsOfMeta 按展示语义 kind 归集列表 bodyOutput 的字段名（如 crypto / s3 / json）：
// 列表列渲染与详情字段分组复用同一份 OPTIONS 自省，避免前端另维护重复的字段清单。
export const fieldKindSetsOfMeta = (meta: ApiMeta | undefined): Record<string, Set<string>> => {
  const sets: Record<string, Set<string>> = {};
  for (const field of meta?.POST?.bodyOutput?.fields ?? []) {
    if (!field.kind) {
      continue;
    }
    (sets[field.kind] ??= new Set<string>()).add(field.name);
  }
  return sets;
};

// fieldRenderTypeOfMeta 推导字段的表格渲染类型：展示语义 kind 优先，其次按 Go 类型名映射，
// 未识别类型回退字符串。字段类型名取自 OPTIONS 的 FieldMeta.type（指针已解引用）。
export const fieldRenderTypeOfMeta = (field: FieldMeta | undefined): BackendDataType => {
  if (!field) {
    return BackendDataType.STRING;
  }
  switch (field.kind) {
    case "json":
      return BackendDataType.JSON;
    case "hex":
      return BackendDataType.HEX;
    case "decByte":
      return BackendDataType.DECIMAL_BYTE;
    case "crypto":
    case "s3":
      // 加密 / S3 字段由加密单元格组件渲染，类型仅作回退
      return BackendDataType.STRING;
  }
  // 切片 / map 序列化为 JSON 数组 / 对象；字节切片按字符串处理
  if (field.type.startsWith("[]")) {
    return field.type === "[]uint8" || field.type === "[]byte"
      ? BackendDataType.STRING
      : BackendDataType.JSON;
  }
  if (field.type.startsWith("map[")) {
    return BackendDataType.JSON;
  }
  switch (field.type) {
    case "bool":
      return BackendDataType.BOOLEAN;
    case "time.Time":
      return BackendDataType.DATETIME;
    case "serializer.DateOnly":
      return BackendDataType.DATE;
    case "serializer.TimeOnly":
      return BackendDataType.TIME;
    case "time.Duration":
      return BackendDataType.DURATION;
    case "datatypes.BinUUID":
    case "serializer.HexUUID":
      return BackendDataType.UUID;
    case "datatypes.JSON":
    case "json.RawMessage":
      return BackendDataType.JSON;
    case "uint8":
    case "layers.IPProtocol":
      return BackendDataType.UINT8;
    case "uint16":
    case "layers.TCPPort":
      return BackendDataType.UINT16;
    case "uint32":
      return BackendDataType.UINT32;
    case "uint64":
      return BackendDataType.UINT64;
    case "int8":
      return BackendDataType.INT8;
    case "int16":
      return BackendDataType.INT16;
    case "int32":
      return BackendDataType.INT32;
    case "int64":
      return BackendDataType.INT64;
    case "float32":
      return BackendDataType.FLOAT32;
    case "float64":
      return BackendDataType.FLOAT64;
    default:
      return BackendDataType.STRING;
  }
};
