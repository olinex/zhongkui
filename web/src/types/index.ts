export interface NumberRange {
  min: number;
  max: number;
  precision: number | null;
}

export interface BaseModelOut {
  id: number;
  createdAt: string;
  updatedAt: string;
}

export interface NormalModelOut extends BaseModelOut {
  deletedAt: string | null;
}

export enum HttpHeader {
  ACCEPT = "Accept",
  ACCEPT_LANGUAGE = "Accept-Language",
  CONTENT_LANGUAGE = "Content-Language",
  AUTHORIZATION = "Authorization",
  CONTENT_TYPE = "Content-Type",
  NAMESPACE = "X-Namespace",
}

export enum BackendDataType {
  ACTIONS = "actions",
  UUID = "datatypes.BinUUID",
  HEX = "hex",
  STRING = "string",
  INT = "int",
  UINT = "uint",
  INT8 = "int8",
  UINT8 = "uint8",
  INT16 = "int16",
  UINT16 = "uint16",
  INT32 = "int32",
  UINT32 = "uint32",
  INT64 = "int64",
  UINT64 = "uint64",
  FLOAT32 = "float32",
  FLOAT64 = "float64",
  BOOLEAN = "bool",
  BINARY_BYTE = "binByte",
  DECIMAL_BYTE = "decByte",
  JSON = "json",
  DATETIME = "time.Time",
  DATE = "serializer.DateOnly",
  TIME = "serializer.TimeOnly",
  DURATION = "time.Duration",
}

export enum SearchAction {
  EQUAL = "eq",
  NOT_EQUAL = "neq",
  PREFIX = "pfx",
  NOT_PREFIX = "npfx",
  IGNORE_CASE_PREFIX = "ipfx",
  IGNORE_CASE_NOT_PREFIX = "inpfx",
  SUFFIX = "sfx",
  NOT_SUFFIX = "nsfx",
  IGNORE_CASE_SUFFIX = "isfx",
  IGNORE_CASE_NOT_SUFFIX = "insfx",
  CONTAIN = "ctn",
  NOT_CONTAIN = "nctn",
  IGNORE_CASE_CONTAIN = "ictn",
  IGNORE_CASE_NOT_CONTAIN = "inctn",
  NIL = "nil",
  NOT_NIL = "nnil",
  LESS = "lt",
  LESS_EQUAL = "lte",
  GREATER = "gt",
  GREATER_EQUAL = "gte",
}

export enum SupportLanguage {
  English = "en",
  ZhHans = "zh-Hans",
}
