import { BackendDataType, type NumberRange, SupportLanguage } from "@/types";
import i18n from "@/utils/i18n";

// 工具
import { EnumClass, enumLabelOf } from "./enums";

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

// LanguageSelectOption 语言下拉框可选项：title 为已翻译文案，value 为语言键。
export interface LanguageSelectOption {
  title: string;
  value: string;
}

// languageSelectOptions 将语言键列表映射为 v-select 可选项（含 "*" 通配翻译），
// 供通知模板等动态语言下拉框复用；缺省时枚举平台支持的全部语言。
// 具体语言名称经统一枚举接口 language 类按 value 解析（label 已按当前语言翻译），
// "*" 通配走前端 option.language.* 键。
export const languageSelectOptions = (
  languages: string[] = SUPPORT_LANGUAGES.map((l) => l.value),
): LanguageSelectOption[] =>
  languages.map((language) => ({
    title:
      language === "*"
        ? i18n.global.t(`${LANGUAGE_NAME_KEY_PREFIX}.*`)
        : enumLabelOf(EnumClass.language, language),
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
