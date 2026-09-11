import { BackendDataType, type NumberRange, SupportLanguage } from "@/types";
import i18n from "@/utils/i18n";

// Types
import type { ApiMeta, EnumOption } from "@/components/@types/meta";

// 语言名称 i18n 键前缀：option.language.<语言键>（含 "*" 通配与具体语言键，如 en / zh-Hans）
const LANGUAGE_NAME_KEY_PREFIX = "option.language";

// SUPPORT_LANGUAGES 平台支持的语言选项，titleKey 为 i18n 键（渲染时经 $t 翻译），
// 作为全局语言切换（AppHeader）与通知目标语言下拉框（target.vue）的唯一事实来源。
// 新增语言时须同步补充 locale 文件中的 option.language.<键> 翻译。
export const SUPPORT_LANGUAGES: { titleKey: string; value: SupportLanguage }[] = [
  {
    titleKey: `${LANGUAGE_NAME_KEY_PREFIX}.${SupportLanguage.English}`,
    value: SupportLanguage.English,
  },
  {
    titleKey: `${LANGUAGE_NAME_KEY_PREFIX}.${SupportLanguage.ZhHans}`,
    value: SupportLanguage.ZhHans,
  },
];

// languageNameKey 返回任意语言键（含 "*" 通配符）对应的 i18n 键；
// 未知语言键经 $t 翻译时未命中，回退为键本身，避免显示异常。
export const languageNameKey = (language: string): string =>
  `${LANGUAGE_NAME_KEY_PREFIX}.${language}`;

// 枚举展示名 i18n 键前缀：model.<modelName>.option.<value>，与字段标题
// model.<modelName>.field.<name> 平行；value 为枚举值本身（如 pending）。
const ENUM_OPTION_KEY_PREFIX = "model";

// enumMetaOptions 由 meta 自省返回的可选枚举值条目 [{key, value}] 构建 v-select 选项：
// - value 取条目原 value（提交/存储值，多数与 key 相同）；
// - title 经 enumValueTitle(modelName, key) 翻译展示（key 不可直接展示，须经 i18n 翻译），
//   未配置翻译时回退为 key 本身。
// 与 enumOptions（静态枚举值集合）、enumValueOptions（后端动态枚举条目）并列，
// 统一元信息自省枚举下拉的构造入口。
export const enumMetaOptions = (
  modelName: string,
  options: EnumOption[] | undefined,
): Array<{ title: string; value: string }> =>
  (options ?? []).map((o) => ({
    title: enumValueTitle(modelName, o.key),
    value: o.value,
  }));

// enumFieldOptionsFromMeta 从列表接口自省响应（MetaOut）的 bodyOutput 结构体中，
// 按字段 JSON 名取可选枚举值，作为"字段 → 枚举集合"单源绑定的前端查询入口；
// 未注册枚举或字段不存在时返回 undefined，前端据此回退为普通文本筛选项。
export const enumFieldOptionsFromMeta = (
  meta: ApiMeta | undefined,
  field: string,
): EnumOption[] | undefined =>
  meta?.POST?.bodyOutput?.fields.find((f) => f.name === field)?.enumOptions;

// enumValueTitle 统一枚举展示名入口（表格单元格 / 表单下拉 / 筛选下拉共用）：
// 以 model.<modelName>.option.<value> 定位翻译文本；未配置翻译时回退为枚举值本身
// （如 HTTP 方法 GET / 版本 HTTP/1.1 等无需翻译的低基数字段，或后端动态注册的新值）。
export const enumValueTitle = (
  modelName: string,
  value: string | number | undefined | null,
): string => {
  if (value === undefined || value === null || value === "") return "";
  const key = `${ENUM_OPTION_KEY_PREFIX}.${modelName}.option.${value}`;
  return i18n.global.te(key) ? i18n.global.t(key) : String(value);
};

// 由前端静态枚举值集合构建 v-select 选项：title 与 value 一致均为枚举值，
// 渲染时经 enumValueTitle 翻译（渲染期翻译，语言切换即时刷新）。
export const enumOptions = <T extends string>(
  values: readonly T[],
): Array<{ title: string; value: T }> => values.map((value) => ({ title: value, value }));

// enumValueOptions 由后端动态枚举条目数组构建 v-select 选项：
// - value 取 item[valueKey]（后端枚举值本身，供 v-select 提交与筛选匹配）；
// - title 默认经 enumValueTitle(modelName, value) 翻译展示名（未配置翻译时回退为枚举值本身），
//   即“i18n option 键”类动态枚举（渠道类型 / 数据源类型 / 模板事件等）；
// - 传入 titleKey 时 title 取 item[titleKey]（后端直接返回的可读标签，如名称字段），
//   即“引用型下拉”类动态枚举。
// 与 enumOptions（静态枚举值集合）对应，统一动态枚举下拉的构造入口。
export const enumValueOptions = <T extends Record<string, unknown>>(
  modelName: string,
  items: T[],
  valueKey: keyof T = "value",
  titleKey?: keyof T,
): Array<{ title: string; value: unknown }> =>
  items.map((item) => {
    const value = item[valueKey] as string | number;
    const title = titleKey ? String(item[titleKey]) : enumValueTitle(modelName, value);
    return { title, value };
  });

// LanguageSelectOption 语言下拉框可选项：title 为已翻译文案，value 为语言键。
export interface LanguageSelectOption {
  title: string;
  value: string;
}

// languageSelectOptions 将语言键列表映射为 v-select 可选项（含 "*" 通配翻译），
// 供通知模板等动态语言下拉框复用；缺省时枚举平台支持的全部语言。
export const languageSelectOptions = (
  languages: string[] = SUPPORT_LANGUAGES.map((l) => l.value),
): LanguageSelectOption[] =>
  languages.map((language) => ({
    title: i18n.global.t(languageNameKey(language)),
    value: language,
  }));

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
  [BackendDataType.HEX]: null,
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
  [BackendDataType.ENUM]: null,
};

export function isEnumValue<T extends object>(enumObj: T, value: string | number): boolean {
  return Object.values(enumObj).includes(value);
}
