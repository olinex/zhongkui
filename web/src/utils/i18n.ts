import { DateTime, Duration } from "luxon";
import { createI18n } from "vue-i18n";
import { en, zhHans } from "vuetify/locale";
import { setLocale } from "yup";

import { DEFAULT_LOCALE, localManager } from "@/stores/local";
import { SupportLanguage } from "@/types";
import { durationToMillis } from "@/utils/duration";
import messages from "@intlify/unplugin-vue-i18n/messages";

const typedMessages = messages as {
  en: Record<string, unknown>;
  zhHans: Record<string, unknown>;
};

export const i18n = createI18n({
  legacy: false, // Composition API mode
  locale: localManager.getLanguage().toString(),
  fallbackLocale: DEFAULT_LOCALE.toString(),
  messages: {
    [SupportLanguage.English]: {
      ...typedMessages.en,
      $vuetify: {
        ...en,
      },
    },
    [SupportLanguage.ZhHans]: {
      ...typedMessages.zhHans,
      $vuetify: {
        ...zhHans,
      },
    },
  }, // import locale files
});

const createDefaultMessage =
  (key: string) =>
  ({ path }: { path: string }) =>
    i18n.global.t(`validation.${key}`, {
      path: i18n.global.t(path),
    });

const createOneMessage =
  (key: string, nameOne: string) =>
  <T extends { path: string }>(params: T) =>
    i18n.global.t(`validation.${key}`, {
      path: i18n.global.t(params.path),
      [nameOne]: (params as T & Record<string, unknown>)[nameOne],
    });

const createOptionMessage =
  (key: string) =>
  ({ path, values }: { path: string; values: string[] }) =>
    i18n.global.t(`validation.${key}`, {
      path: i18n.global.t(path),
      values: values.map((v: string) => i18n.global.t(`input.options.${path}.${v}`)),
    });

setLocale({
  mixed: {
    default: createDefaultMessage("mixed.default"),
    required: createDefaultMessage("mixed.required"),
    defined: createDefaultMessage("mixed.defined"),
    notNull: createDefaultMessage("mixed.notNull"),
    oneOf: createOptionMessage("mixed.oneOf"),
    notOneOf: createOptionMessage("mixed.notOneOf"),
  },
  string: {
    length: createOneMessage("string.length", "length"),
    min: createOneMessage("string.min", "min"),
    max: createOneMessage("string.max", "max"),
    matches: createOneMessage("string.matches", "regex"),
    email: createDefaultMessage("string.email"),
    url: createDefaultMessage("string.url"),
    uuid: createDefaultMessage("string.uuid"),
    datetime: createDefaultMessage("string.datetime"),
    datetime_precision: createOneMessage("string.datetime_precision", "precision"),
    datetime_offset: createDefaultMessage("string.datetime_offset"),
    trim: createDefaultMessage("string.trim"),
    lowercase: createDefaultMessage("string.lowercase"),
    uppercase: createDefaultMessage("string.uppercase"),
  },
  number: {
    min: createOneMessage("number.min", "min"),
    max: createOneMessage("number.max", "max"),
    lessThan: createOneMessage("number.lessThan", "lessThan"),
    moreThan: createOneMessage("number.moreThan", "moreThan"),
    positive: createDefaultMessage("number.positive"),
    negative: createDefaultMessage("number.negative"),
    integer: createDefaultMessage("number.integer"),
  },
  date: {
    min: createOneMessage("date.min", "min"),
    max: createOneMessage("date.max", "max"),
  },
  boolean: {
    isValue: createOneMessage("boolean.isValue", "value"),
  },
  object: {
    noUnknown: createOneMessage("object.noUnknown", "unknown"),
    exact: createOneMessage("object.exact", "properties"),
  },
  array: {
    min: createOneMessage("array.min", "min"),
    max: createOneMessage("array.max", "max"),
    length: createOneMessage("array.length", "length"),
  },
});

export const localeDateFormat = (date: DateTime): string => {
  return date.setLocale(i18n.global.locale.value).toLocaleString(DateTime.DATE_SHORT);
};

export const localeTimeFormat = (time: DateTime): string => {
  return time.setLocale(i18n.global.locale.value).toLocaleString(DateTime.TIME_24_WITH_SECONDS);
};

export const localeDatetimeFormat = (dt: DateTime): string => {
  return dt
    .setLocale(i18n.global.locale.value)
    .toLocaleString(DateTime.DATETIME_SHORT_WITH_SECONDS);
};

// localDurationFormat 将时长格式化为本地化的人类可读文本。
// 支持 Go time.Duration 字符串（如 "5s"）与纳秒整数两种输入。
export const localDurationFormat = (duration: string | number): string => {
  const millis = typeof duration === "number" ? duration / 1e6 : durationToMillis(duration);
  if (Number.isNaN(millis)) {
    return String(duration);
  }
  return Duration.fromMillis(millis, {
    locale: i18n.global.locale.value,
  }).toHuman();
};

export default i18n;
