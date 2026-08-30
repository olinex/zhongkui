import { DateTime } from "luxon";
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

// durationToCompact 将毫秒数格式化为紧凑的 Go 风格时长字符串，
// 省略零值低位单位（如 "2m0s" → "2m"、"3h0m0s" → "3h"）。
const durationToCompact = (millis: number): string => {
  const total = Math.round(millis);
  if (total === 0) {
    return "0s";
  }
  const sign = total < 0 ? "-" : "";
  const abs = Math.abs(total);
  if (abs < 1000) {
    return `${sign}${abs}ms`;
  }
  const h = Math.floor(abs / 3600000);
  const m = Math.floor((abs % 3600000) / 60000);
  const s = (abs % 60000) / 1000;
  let out = "";
  if (h > 0) {
    out += `${h}h`;
  }
  if (m > 0) {
    out += `${m}m`;
  }
  if (s > 0) {
    out += `${Number.isInteger(s) ? s : +s.toFixed(3)}s`;
  }
  return sign + out;
};

// localDurationFormat 将时长格式化为便于人类阅读的紧凑格式（如 "2m"、"5s"、"3h"），
// 与后端 serializer.Duration 的 JSON 契约对齐：
// 字符串输入为 Go time.Duration 文本（如 "5s"、"1m30s"），数字输入为毫秒。
export const localDurationFormat = (duration: string | number): string => {
  const millis = typeof duration === "number" ? duration : durationToMillis(duration);
  if (Number.isNaN(millis)) {
    return String(duration);
  }
  return durationToCompact(millis);
};

export default i18n;
