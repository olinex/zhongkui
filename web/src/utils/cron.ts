/**
 * Cron 表达式工具：解析、校验、人类可读描述、下次触发时间预览与引导式构造。
 *
 * 解析口径对齐后端 `internal/pkg/validator/cron.go`（`robfig/cron` 的 `ParseStandard`）：
 * 支持 5 段标准 cron（分 时 日 月 周）与 `@daily` 等描述符（含 `@every <duration>`），
 * 允许 `TZ=` / `CRON_TZ=` 前缀；不支持带秒的 6 段表达式。
 */

import { DateTime } from "luxon";

import i18n from "@/utils/i18n";

// CronFieldName 五段 cron 字段名（分 / 时 / 日 / 月 / 周）。
export type CronFieldName = "minute" | "hour" | "dayOfMonth" | "month" | "dayOfWeek";

// CronFrequency 引导模式支持的执行频率。
export type CronFrequency = "minute" | "hourly" | "daily" | "weekly" | "monthly" | "yearly";

// CronMode 字段的取值模式：
//   descriptor 只接受宏描述符（`@hourly` / `@daily` / `@weekly` / `@monthly` / `@yearly` / `@every <duration>`）；
//   cron 只接受 5 段标准 cron（拒绝上述宏描述符）；
//   both 两种写法都接受。
export type CronMode = "descriptor" | "cron" | "both";

// DAILY_DESCRIPTOR 每天执行的描述符，作为 descriptor 模式的默认取值。
export const DAILY_DESCRIPTOR = "@daily";

// CronBuilderState 引导模式的表单状态：频率 + 该频率下可选的时刻与日期。
export interface CronBuilderState {
  frequency: CronFrequency;
  minute: number;
  hour: number;
  daysOfWeek: number[];
  dayOfMonth: number;
  month: number;
}

// ParsedCronSpec 解析后的 cron 计划：各字段允许的取值集合与通配标记。
// 通配标记对应 `robfig/cron` 的 starBit，决定「日」与「周」同时受限时的匹配口径。
export interface ParsedCronSpec {
  minutes: number[];
  hours: number[];
  daysOfMonth: number[];
  months: number[];
  daysOfWeek: number[];
  minuteWildcard: boolean;
  hourWildcard: boolean;
  dayOfMonthWildcard: boolean;
  monthWildcard: boolean;
  dayOfWeekWildcard: boolean;
  // zone 为 `TZ=` / `CRON_TZ=` 前缀声明的时区，用于预览下次触发时间。
  zone?: string;
  // everyMillis 为 `@every <duration>` 的间隔毫秒数。
  everyMillis?: number;
}

// CronSpecParseResult 解析结果：成功时携带解析结构，失败时携带已本地化的原因。
export interface CronSpecParseResult {
  ok: boolean;
  spec?: ParsedCronSpec;
  error?: string;
}

// CronBounds 单个字段的取值边界（含端点）与允许的名称（月 / 周支持英文缩写）。
interface CronBounds {
  min: number;
  max: number;
  names?: Record<string, number>;
}

// CronFieldError 字段级解析错误：仅在内部分发，最终由 renderFieldError 本地化。
class CronFieldError extends Error {
  constructor(
    readonly kind: "syntax" | "range" | "order",
    readonly min?: number,
    readonly max?: number,
  ) {
    super(kind);
    this.name = "CronFieldError";
  }
}

const FIELD_ORDER: CronFieldName[] = ["minute", "hour", "dayOfMonth", "month", "dayOfWeek"];

const MONTH_NAMES: Record<string, number> = {
  jan: 1,
  feb: 2,
  mar: 3,
  apr: 4,
  may: 5,
  jun: 6,
  jul: 7,
  aug: 8,
  sep: 9,
  oct: 10,
  nov: 11,
  dec: 12,
};

const WEEKDAY_NAMES: Record<string, number> = {
  sun: 0,
  mon: 1,
  tue: 2,
  wed: 3,
  thu: 4,
  fri: 5,
  sat: 6,
};

const WEEKDAY_KEYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;

// BOUNDS 各字段的取值边界，与 robfig/cron 的 bounds 一致。
const BOUNDS: Record<CronFieldName, CronBounds> = {
  minute: { min: 0, max: 59 },
  hour: { min: 0, max: 23 },
  dayOfMonth: { min: 1, max: 31 },
  month: { min: 1, max: 12, names: MONTH_NAMES },
  dayOfWeek: { min: 0, max: 6, names: WEEKDAY_NAMES },
};

// TZ_PREFIX_RE 匹配 `TZ=` / `CRON_TZ=` 时区前缀。
const TZ_PREFIX_RE = /^(?:TZ|CRON_TZ)=(\S+)\s+/;

const NEXT_RUN_LIMIT_YEARS = 12;

// rangeValues 生成 [min, max] 的整数数组。
const rangeValues = (min: number, max: number): number[] =>
  Array.from({ length: max - min + 1 }, (_, index) => min + index);

const t = (key: string, params?: Record<string, unknown>): string =>
  i18n.global.t(key, params ?? {});

// parseIntOrName 解析数字或英文名称，与 robfig/cron 的 parseIntOrName 对齐。
const parseIntOrName = (token: string, bounds: CronBounds): number => {
  const named = bounds.names?.[token.toLowerCase()];
  if (named !== undefined) {
    return named;
  }
  if (!/^\d+$/.test(token)) {
    throw new CronFieldError("syntax");
  }
  return Number(token);
};

// getRange 解析单个区间表达式（`*` / `5` / `1-5` / `*/3` / `5/10` 等），
// 与 robfig/cron 的 getRange 行为对齐（含 `N/step` 表示 `N-max/step`）。
const getRange = (expr: string, bounds: CronBounds): { values: number[]; wildcard: boolean } => {
  const rangeAndStep = expr.split("/");
  if (rangeAndStep.length > 2) {
    throw new CronFieldError("syntax");
  }
  const [range, stepToken] = rangeAndStep;
  const lowAndHigh = range.split("-");
  if (lowAndHigh.length > 2) {
    throw new CronFieldError("syntax");
  }

  const isStar = lowAndHigh[0] === "*" || lowAndHigh[0] === "?";
  const single = lowAndHigh.length === 1;
  const start = isStar ? bounds.min : parseIntOrName(lowAndHigh[0], bounds);
  let end: number;
  if (isStar) {
    end = single ? bounds.max : parseIntOrName(lowAndHigh[1], bounds);
  } else {
    end = single ? start : parseIntOrName(lowAndHigh[1], bounds);
  }

  let step = 1;
  if (stepToken !== undefined) {
    if (!/^\d+$/.test(stepToken)) {
      throw new CronFieldError("syntax");
    }
    step = Number(stepToken);
    if (step === 0) {
      throw new CronFieldError("syntax");
    }
    // 单值 + 步长表示「从该值到上界按步长取值」。
    if (single) {
      end = bounds.max;
    }
  }

  if (start < bounds.min || end > bounds.max) {
    throw new CronFieldError("range", bounds.min, bounds.max);
  }
  if (start > end) {
    throw new CronFieldError("order");
  }

  const values: number[] = [];
  for (let value = start; value <= end; value += step) {
    values.push(value);
  }
  // 步长大于 1 时 robfig/cron 会清掉 starBit，与「通配」语义区分。
  return { values, wildcard: isStar && step === 1 };
};

// parseField 解析逗号分隔的字段表达式，返回去重升序的取值集合与通配标记。
const parseField = (expr: string, name: CronFieldName): { values: number[]; wildcard: boolean } => {
  const bounds = BOUNDS[name];
  const pieces = expr.split(",").filter((piece) => piece !== "");
  if (pieces.length === 0) {
    throw new CronFieldError("syntax");
  }
  const values = new Set<number>();
  let wildcard = false;
  for (const piece of pieces) {
    const parsed = getRange(piece, bounds);
    parsed.values.forEach((value) => values.add(value));
    wildcard = wildcard || parsed.wildcard;
  }
  return { values: [...values].sort((left, right) => left - right), wildcard };
};

// renderFieldError 把字段级错误渲染为本地化文案。
const renderFieldError = (issue: CronFieldError, name: CronFieldName): string => {
  const field = t(`cron.field.${name}`);
  if (issue.kind === "range") {
    return t("cron.error.range", { field, min: issue.min, max: issue.max });
  }
  if (issue.kind === "order") {
    return t("cron.error.order", { field });
  }
  return t("cron.error.syntax", { field });
};

// allSpec 构造全通配的计划结构（作为描述符展开的基底）。
const allSpec = (zone?: string): ParsedCronSpec => ({
  minutes: rangeValues(0, 59),
  hours: rangeValues(0, 23),
  daysOfMonth: rangeValues(1, 31),
  months: rangeValues(1, 12),
  daysOfWeek: rangeValues(0, 6),
  minuteWildcard: true,
  hourWildcard: true,
  dayOfMonthWildcard: true,
  monthWildcard: true,
  dayOfWeekWildcard: true,
  zone,
});

// DURATION_UNIT_MILLIS Go 时长单位对应的毫秒数。
const DURATION_UNIT_MILLIS: Record<string, number> = {
  ns: 0.000001,
  us: 0.001,
  µs: 0.001,
  μs: 0.001,
  ms: 1,
  s: 1000,
  m: 60000,
  h: 3600000,
};

// parseEveryMillis 解析 `@every` 后的 Go 时长文本（如 `1h30m`、`90s`），非法时返回 null。
const parseEveryMillis = (raw: string): number | null => {
  if (raw === "0") {
    return 0;
  }
  const pattern = /(\d+(?:\.\d+)?)(ns|us|µs|μs|ms|s|m|h)/g;
  let total = 0;
  let index = 0;
  let match = pattern.exec(raw);
  while (match !== null) {
    if (match.index !== index) {
      return null;
    }
    total += Number(match[1]) * DURATION_UNIT_MILLIS[match[2]];
    index = pattern.lastIndex;
    match = pattern.exec(raw);
  }
  if (index === 0 || index !== raw.length) {
    return null;
  }
  return total;
};

// parseDescriptor 解析 `@daily` 等描述符，展开为与五段表达式同构的计划结构。
const parseDescriptor = (descriptor: string, zone?: string): CronSpecParseResult => {
  const base = allSpec(zone);
  switch (descriptor) {
    case "@yearly":
    case "@annually":
      return {
        ok: true,
        spec: {
          ...base,
          minutes: [0],
          hours: [0],
          daysOfMonth: [1],
          months: [1],
          minuteWildcard: false,
          hourWildcard: false,
          dayOfMonthWildcard: false,
          monthWildcard: false,
        },
      };
    case "@monthly":
      return {
        ok: true,
        spec: { ...base, minutes: [0], hours: [0], daysOfMonth: [1], dayOfMonthWildcard: false },
      };
    case "@weekly":
      return {
        ok: true,
        spec: { ...base, minutes: [0], hours: [0], daysOfWeek: [0], dayOfWeekWildcard: false },
      };
    case "@daily":
    case "@midnight":
      return { ok: true, spec: { ...base, minutes: [0], hours: [0] } };
    case "@hourly":
      return { ok: true, spec: { ...base, minutes: [0], minuteWildcard: false } };
    default:
      break;
  }

  const everyPrefix = "@every ";
  if (descriptor.startsWith(everyPrefix)) {
    const millis = parseEveryMillis(descriptor.slice(everyPrefix.length).trim());
    if (millis !== null && millis > 0) {
      return { ok: true, spec: { ...base, everyMillis: millis } };
    }
  }
  return { ok: false, error: t("cron.error.descriptor", { token: descriptor }) };
};

// parseCronSpec 解析 cron 表达式；失败时返回已本地化的错误原因。
export const parseCronSpec = (raw: string): CronSpecParseResult => {
  let spec = raw.trim();
  if (spec === "") {
    return { ok: false, error: t("cron.error.empty") };
  }

  let zone: string | undefined;
  const zoneMatch = TZ_PREFIX_RE.exec(spec);
  if (zoneMatch !== null) {
    zone = zoneMatch[1];
    spec = spec.slice(zoneMatch[0].length).trim();
    if (spec === "") {
      return { ok: false, error: t("cron.error.empty") };
    }
  }

  if (spec.startsWith("@")) {
    return parseDescriptor(spec, zone);
  }

  const fields = spec.split(/\s+/);
  if (fields.length !== 5) {
    return { ok: false, error: t("cron.error.fieldCount") };
  }

  const parsed: Array<{ values: number[]; wildcard: boolean }> = [];
  for (let index = 0; index < FIELD_ORDER.length; index += 1) {
    const name = FIELD_ORDER[index];
    try {
      parsed.push(parseField(fields[index], name));
    } catch (error) {
      if (!(error instanceof CronFieldError)) {
        throw error;
      }
      return { ok: false, error: renderFieldError(error, name) };
    }
  }

  return {
    ok: true,
    spec: {
      minutes: parsed[0].values,
      hours: parsed[1].values,
      daysOfMonth: parsed[2].values,
      months: parsed[3].values,
      daysOfWeek: parsed[4].values,
      minuteWildcard: parsed[0].wildcard,
      hourWildcard: parsed[1].wildcard,
      dayOfMonthWildcard: parsed[2].wildcard,
      monthWildcard: parsed[3].wildcard,
      dayOfWeekWildcard: parsed[4].wildcard,
      zone,
    },
  };
};

// stripZonePrefix 去掉 `TZ=` / `CRON_TZ=` 前缀，返回用于模式判定的表达式主体。
const stripZonePrefix = (spec: string): string => {
  const zoneMatch = TZ_PREFIX_RE.exec(spec);
  if (zoneMatch === null) {
    return spec;
  }
  return spec.slice(zoneMatch[0].length).trim();
};

// parseCronSpecForMode 按字段模式解析：在通用解析之外再施加 descriptor / cron 的取值限制。
export const parseCronSpecForMode = (raw: string, mode: CronMode = "both"): CronSpecParseResult => {
  const spec = raw.trim();
  if (spec === "") {
    return { ok: false, error: t("cron.error.empty") };
  }
  const body = stripZonePrefix(spec);
  if (mode === "descriptor" && !body.startsWith("@")) {
    return { ok: false, error: t("cron.error.descriptorOnly") };
  }
  if (mode === "cron" && body.startsWith("@")) {
    return { ok: false, error: t("cron.error.cronOnly") };
  }
  return parseCronSpec(spec);
};

// isCronSpecValid 判断表达式是否可提交：空白视为未填写（由 required 规则兜底），其余须符合模式。
export const isCronSpecValid = (value: string | undefined, mode: CronMode = "both"): boolean => {
  const spec = (value ?? "").trim();
  return spec === "" || parseCronSpecForMode(spec, mode).ok;
};

// singleValue 取值集合只有一个元素时返回该值，否则返回 null。
const singleValue = (values: number[]): number | null => (values.length === 1 ? values[0] : null);

// pad2 补零到两位，用于 HH:mm 展示。
const pad2 = (value: number): string => String(value).padStart(2, "0");

// weekdayText 把星期取值渲染为本地化名称列表。
const weekdayText = (values: number[]): string =>
  values.map((value) => t(`cron.weekday.${WEEKDAY_KEYS[value] ?? "sun"}`)).join(", ");

// describeCronSpec 生成人类可读的执行计划描述；无法解析时返回空串。
export const describeCronSpec = (raw: string): string => {
  const result = parseCronSpec(raw);
  if (!result.ok || !result.spec) {
    return "";
  }
  const spec = result.spec;

  if (spec.everyMillis !== undefined) {
    return t("cron.describe.every", {
      duration: raw.trim().replace(/^@every\s+/i, ""),
    });
  }

  const minute = singleValue(spec.minutes);
  const hour = singleValue(spec.hours);
  const time = minute !== null && hour !== null ? `${pad2(hour)}:${pad2(minute)}` : "";
  const dateWildcard = spec.dayOfMonthWildcard && spec.monthWildcard && spec.dayOfWeekWildcard;

  if (spec.minuteWildcard && spec.hourWildcard && dateWildcard) {
    return t("cron.describe.everyMinute");
  }
  if (minute !== null && spec.hourWildcard && dateWildcard) {
    return t("cron.describe.hourlyAt", { minute });
  }
  if (minute !== null && hour !== null && dateWildcard) {
    return t("cron.describe.dailyAt", { time });
  }
  if (minute !== null && hour !== null && spec.dayOfMonthWildcard && spec.monthWildcard) {
    return t("cron.describe.weeklyAt", {
      weekdays: weekdayText(spec.daysOfWeek),
      time,
    });
  }

  const day = singleValue(spec.daysOfMonth);
  const month = singleValue(spec.months);
  if (
    minute !== null &&
    hour !== null &&
    day !== null &&
    spec.monthWildcard &&
    spec.dayOfWeekWildcard
  ) {
    return t("cron.describe.monthlyAt", { day, time });
  }
  if (
    minute !== null &&
    hour !== null &&
    day !== null &&
    month !== null &&
    spec.dayOfWeekWildcard
  ) {
    return t("cron.describe.yearlyAt", { month, day, time });
  }
  return t("cron.describe.custom");
};

// nextCronTriggers 计算严格晚于 from 的前 count 个触发时间；无法解析或找不到时返回空数组。
export const nextCronTriggers = (
  raw: string,
  count = 3,
  from: DateTime = DateTime.now(),
): DateTime[] => {
  const result = parseCronSpec(raw);
  if (!result.ok || !result.spec) {
    return [];
  }
  const spec = result.spec;
  const zoned = spec.zone ? from.setZone(spec.zone) : from;
  const base = zoned.isValid ? zoned : from;

  if (spec.everyMillis !== undefined) {
    const triggers: DateTime[] = [];
    let next = base.plus({ milliseconds: spec.everyMillis });
    for (let index = 0; index < count; index += 1) {
      triggers.push(next);
      next = next.plus({ milliseconds: spec.everyMillis });
    }
    return triggers;
  }

  const triggers: DateTime[] = [];
  let day = base.startOf("day");
  const limit = day.plus({ years: NEXT_RUN_LIMIT_YEARS });
  while (day < limit && triggers.length < count) {
    if (spec.months.includes(day.month)) {
      const dayOfMonthMatch = spec.daysOfMonth.includes(day.day);
      // Luxon 的 weekday 以周一为 1、周日为 7，cron 以周日为 0。
      const dayOfWeekMatch = spec.daysOfWeek.includes(day.weekday % 7);
      // 与 robfig/cron 的 dayMatches 对齐：两侧有一侧通配时取「与」，两侧均受限时取「或」。
      const dayMatch =
        spec.dayOfMonthWildcard || spec.dayOfWeekWildcard
          ? dayOfMonthMatch && dayOfWeekMatch
          : dayOfMonthMatch || dayOfWeekMatch;
      if (dayMatch) {
        for (const hour of spec.hours) {
          for (const minute of spec.minutes) {
            const candidate = day.set({ hour, minute, second: 0, millisecond: 0 });
            if (candidate > base) {
              triggers.push(candidate);
            }
            if (triggers.length >= count) {
              break;
            }
          }
          if (triggers.length >= count) {
            break;
          }
        }
      }
    }
    day = day.plus({ days: 1 });
  }
  return triggers;
};

// buildCronSpec 由引导模式状态生成 5 段 cron 表达式；星期为空时返回空串（不构成合法表达式）。
export const buildCronSpec = (state: CronBuilderState): string => {
  switch (state.frequency) {
    case "minute":
      return "* * * * *";
    case "hourly":
      return `${state.minute} * * * *`;
    case "daily":
      return `${state.minute} ${state.hour} * * *`;
    case "weekly": {
      if (state.daysOfWeek.length === 0) {
        return "";
      }
      const days = [...state.daysOfWeek].sort((left, right) => left - right);
      return `${state.minute} ${state.hour} * * ${days.join(",")}`;
    }
    case "monthly":
      return `${state.minute} ${state.hour} ${state.dayOfMonth} * *`;
    case "yearly":
      return `${state.minute} ${state.hour} ${state.dayOfMonth} ${state.month} *`;
    default:
      return "";
  }
};

// createBuilderState 返回引导模式的默认状态（每天 09:00）。
export const createBuilderState = (): CronBuilderState => ({
  frequency: "daily",
  minute: 0,
  hour: 9,
  daysOfWeek: [1],
  dayOfMonth: 1,
  month: 1,
});

// parseCronBuilder 尝试把已有表达式回填到引导模式；不属于引导模式可表达的形状时返回 null。
export const parseCronBuilder = (raw: string): CronBuilderState | null => {
  const result = parseCronSpec(raw);
  if (!result.ok || !result.spec || result.spec.everyMillis !== undefined) {
    return null;
  }
  const spec = result.spec;
  const minute = singleValue(spec.minutes);
  const hour = singleValue(spec.hours);
  const day = singleValue(spec.daysOfMonth);
  const month = singleValue(spec.months);
  const defaults = createBuilderState();

  if (
    spec.minuteWildcard &&
    spec.hourWildcard &&
    spec.dayOfMonthWildcard &&
    spec.monthWildcard &&
    spec.dayOfWeekWildcard
  ) {
    return { ...defaults, frequency: "minute" };
  }
  if (
    minute !== null &&
    spec.hourWildcard &&
    spec.dayOfMonthWildcard &&
    spec.monthWildcard &&
    spec.dayOfWeekWildcard
  ) {
    return { ...defaults, frequency: "hourly", minute };
  }
  if (
    minute !== null &&
    hour !== null &&
    spec.dayOfMonthWildcard &&
    spec.monthWildcard &&
    spec.dayOfWeekWildcard
  ) {
    return { ...defaults, frequency: "daily", minute, hour };
  }
  if (minute !== null && hour !== null && spec.dayOfMonthWildcard && spec.monthWildcard) {
    return { ...defaults, frequency: "weekly", minute, hour, daysOfWeek: spec.daysOfWeek };
  }
  if (
    minute !== null &&
    hour !== null &&
    day !== null &&
    spec.monthWildcard &&
    spec.dayOfWeekWildcard
  ) {
    return { ...defaults, frequency: "monthly", minute, hour, dayOfMonth: day };
  }
  if (
    minute !== null &&
    hour !== null &&
    day !== null &&
    month !== null &&
    spec.dayOfWeekWildcard
  ) {
    return {
      ...defaults,
      frequency: "yearly",
      minute,
      hour,
      dayOfMonth: day,
      month,
    };
  }
  return null;
};
