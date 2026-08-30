import { BackendDataType, type NumberRange, SupportLanguage } from "@/types";
import i18n from "@/utils/i18n";

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

// 枚举筛选项 i18n 键前缀：model.<modelName>.option.<title>，与字段标题
// model.<modelName>.field.<name> 平行；title 为后端返回的枚举值本身（如 pending）。
const FILTER_OPTION_KEY_PREFIX = "model";

// filterOptionTitle 返回枚举筛选项的展示名：
// 以 model.<modelName>.option.<title> 定位翻译文本；未配置翻译时回退为枚举值本身
// （如 HTTP 方法 GET / 版本 HTTP/1.1 等无需翻译的低基数字段）。
export const filterOptionTitle = (modelName: string, title: string): string => {
  const key = `${FILTER_OPTION_KEY_PREFIX}.${modelName}.option.${title}`;
  return i18n.global.te(key) ? i18n.global.t(key) : title;
};

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
