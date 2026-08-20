import { BackendDataType, type NumberRange, SupportLanguage } from "@/types";

export const SUPPORT_LANGUAGES: { title: string; value: SupportLanguage }[] = [
  { title: "English", value: SupportLanguage.English },
  { title: "简体中文", value: SupportLanguage.ZhHans },
];

export const DataTypeRanges: Record<BackendDataType, NumberRange | null> = {
  [BackendDataType.INT]: {
    min: -(2 ** 31),
    max: 2 ** 31 - 1,
    precision: 0,
  },
  [BackendDataType.UINT]: {
    min: 0,
    max: 2 ** 32 - 1,
    precision: 0,
  },
  [BackendDataType.INT8]: {
    min: -(2 ** 7),
    max: 2 ** 7 - 1,
    precision: 0,
  },
  [BackendDataType.UINT8]: {
    min: 0,
    max: 2 ** 8 - 1,
    precision: 0,
  },
  [BackendDataType.INT16]: {
    min: -(2 ** 15),
    max: 2 ** 15 - 1,
    precision: 0,
  },
  [BackendDataType.UINT16]: {
    min: 0,
    max: 2 ** 16 - 1,
    precision: 0,
  },
  [BackendDataType.INT32]: {
    min: -(2 ** 31),
    max: 2 ** 31 - 1,
    precision: 0,
  },
  [BackendDataType.UINT32]: {
    min: 0,
    max: 2 ** 32 - 1,
    precision: 0,
  },
  [BackendDataType.INT64]: {
    min: -(2 ** 63),
    max: 2 ** 63 - 1,
    precision: 0,
  },
  [BackendDataType.UINT64]: {
    min: 0,
    max: 2 ** 64 - 1,
    precision: 0,
  },
  [BackendDataType.FLOAT32]: {
    min: -(2 ** 128),
    max: 2 ** 128,
    precision: null,
  },
  [BackendDataType.FLOAT64]: {
    min: -(2 ** 1024),
    max: 2 ** 1024,
    precision: null,
  },
  [BackendDataType.UUID]: null,
  [BackendDataType.BOOLEAN]: null,
  [BackendDataType.BINARY_BYTE]: null,
  [BackendDataType.DECIMAL_BYTE]: null,
  [BackendDataType.STRING]: null,
  [BackendDataType.DATETIME]: null,
  [BackendDataType.DATE]: null,
  [BackendDataType.TIME]: null,
  [BackendDataType.DURATION]: null,
  [BackendDataType.ACTIONS]: null,
  [BackendDataType.JSON]: null,
};

export function isEnumValue<T extends object>(enumObj: T, value: string | number): boolean {
  return Object.values(enumObj).includes(value);
}
