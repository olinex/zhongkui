import { BackendDataType, SearchAction } from "@/types";

export interface Option {
  title: string;
  // Search option values are heterogeneous (string / number / boolean / enum).
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  value: any;
}

export interface SearchItem {
  field: { value: string; title: string };
  action: { value: string; title: string };
  // Form field values are heterogeneous (string / number / boolean / Date).
  value: {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    value: any;
    title: string;
  };
}

export enum SearchInputType {
  STRING = "string",
  NUMBER = "number",
  DATETIME = "datetime",
  DATE = "date",
  TIME = "time",
  BOOLEAN = "boolean",
  EMPTY = "empty",
  UNKNOWN = "unknown",
}

export function fromBackendDataType(btype: BackendDataType, action: SearchAction): SearchInputType {
  switch (action) {
    case SearchAction.NIL:
    case SearchAction.NOT_NIL:
      return SearchInputType.EMPTY;
    case SearchAction.EQUAL:
    case SearchAction.NOT_EQUAL:
      switch (btype) {
        case BackendDataType.INT:
        case BackendDataType.UINT:
        case BackendDataType.INT8:
        case BackendDataType.UINT8:
        case BackendDataType.INT16:
        case BackendDataType.UINT16:
        case BackendDataType.INT32:
        case BackendDataType.UINT32:
        case BackendDataType.INT64:
        case BackendDataType.UINT64:
        case BackendDataType.FLOAT32:
        case BackendDataType.FLOAT64:
          return SearchInputType.NUMBER;
        case BackendDataType.BOOLEAN:
          return SearchInputType.BOOLEAN;
        case BackendDataType.DATETIME:
          return SearchInputType.DATETIME;
        case BackendDataType.DATE:
          return SearchInputType.DATE;
        case BackendDataType.TIME:
          return SearchInputType.TIME;
        case BackendDataType.STRING:
          return SearchInputType.STRING;
        default:
          return SearchInputType.UNKNOWN;
      }
    case SearchAction.LESS:
    case SearchAction.LESS_EQUAL:
    case SearchAction.GREATER:
    case SearchAction.GREATER_EQUAL:
      switch (btype) {
        case BackendDataType.INT:
        case BackendDataType.UINT:
        case BackendDataType.INT8:
        case BackendDataType.UINT8:
        case BackendDataType.INT16:
        case BackendDataType.UINT16:
        case BackendDataType.INT32:
        case BackendDataType.UINT32:
        case BackendDataType.INT64:
        case BackendDataType.UINT64:
        case BackendDataType.FLOAT32:
        case BackendDataType.FLOAT64:
          return SearchInputType.NUMBER;
        case BackendDataType.DATETIME:
          return SearchInputType.DATETIME;
        case BackendDataType.DATE:
          return SearchInputType.DATE;
        case BackendDataType.TIME:
          return SearchInputType.TIME;
        default:
          return SearchInputType.UNKNOWN;
      }
    case SearchAction.PREFIX:
    case SearchAction.NOT_PREFIX:
    case SearchAction.IGNORE_CASE_PREFIX:
    case SearchAction.IGNORE_CASE_NOT_PREFIX:
    case SearchAction.SUFFIX:
    case SearchAction.NOT_SUFFIX:
    case SearchAction.IGNORE_CASE_SUFFIX:
    case SearchAction.IGNORE_CASE_NOT_SUFFIX:
    case SearchAction.CONTAIN:
    case SearchAction.NOT_CONTAIN:
    case SearchAction.IGNORE_CASE_CONTAIN:
    case SearchAction.IGNORE_CASE_NOT_CONTAIN:
      switch (btype) {
        case BackendDataType.STRING:
          return SearchInputType.STRING;
        default:
          return SearchInputType.UNKNOWN;
      }
    default:
      return SearchInputType.UNKNOWN;
  }
}
