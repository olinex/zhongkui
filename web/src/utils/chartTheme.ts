/**
 * src/utils/chartTheme.ts
 *
 * 由 Vuetify 主题驱动的 ECharts 主题辅助函数，使每个图表都跟随
 * 当前明/暗主题（颜色、文字、网格与提示框样式）。
 */

import { computed } from "vue";
import type { ComputedRef } from "vue";
import { useTheme } from "vuetify";

/** 用于构建默认系列调色板的语义颜色键。 */
const PALETTE_KEYS = ["primary", "secondary", "success", "warning", "error", "info"] as const;

/** 调色板应包含的最大颜色数。 */
const PALETTE_SIZE = 8;

/**
 * 解析后的图表主题快照。`useChartTheme()` 以响应式 `ComputedRef`
 * 返回它，使图表在 Vuetify 主题变化时重新渲染。
 */
export interface ChartTheme {
  /** 当前 Vuetify 主题名（如 "dark"）。 */
  name: string;
  /** 按语义名索引的原始 Vuetify 主题颜色。 */
  colors: Record<string, string>;
  /** 由主题语义色派生的系列颜色梯度。 */
  palette: string[];
  /** 主文字色（标题、轴标签、提示框）。 */
  textColor: string;
  /** 次级文字色（图例、弱化标签）。 */
  subTextColor: string;
  /** 网格/轴线颜色（on-surface 低透明度）。 */
  gridColor: string;
  /** 提示框背景色。 */
  tooltipBg: string;
  /** 提示框边框色。 */
  tooltipBorder: string;
}

/**
 * 响应式图表主题。监听主题名可在用户切换明/暗时保持返回值同步。
 */
export function useChartTheme(): ComputedRef<ChartTheme> {
  const theme = useTheme();

  return computed<ChartTheme>(() => {
    // Vuetify 4 的主题颜色值类型放宽为 Color（string | number | RGB | HSV ...），
    // 本项目主题均由十六进制字符串定义，此处按字符串表收窄以适配图表取色
    const colors = theme.global.current.value.colors as unknown as Record<string, string>;
    const textColor = colors["on-surface"] || colors["on-background"] || "#E6E8EC";
    const subTextColor = colors["on-surface-variant"] || textColor;
    return {
      name: theme.global.name.value,
      colors,
      palette: buildPalette(colors),
      textColor,
      subTextColor,
      gridColor: hexToRgba(textColor, 0.12),
      tooltipBg: colors["surface"] || "#151A21",
      tooltipBorder: colors["surface-variant"] || "#1D242E",
    };
  });
}

/** 把 3 位或 6 位十六进制颜色解析为 RGB 通道。 */
function parseHex(hex: string): [number, number, number] {
  const value = hex.replace("#", "").trim();
  const digits =
    value.length === 3
      ? value
          .split("")
          .map((c) => c + c)
          .join("")
      : value;
  const int = /^[0-9a-fA-F]{6}$/.test(digits) ? parseInt(digits, 16) : 0x000000;
  return [(int >> 16) & 0xff, (int >> 8) & 0xff, int & 0xff];
}

/** 按 ratio 把 base 混入 hex（0 = 纯 hex，1 = 纯 base）。 */
function mixHex(hex: string, base: string, ratio: number): string {
  const [r1, g1, b1] = parseHex(hex);
  const [r2, g2, b2] = parseHex(base);
  const channel = (a: number, b: number): string =>
    Math.round(a + (b - a) * ratio)
      .toString(16)
      .padStart(2, "0");
  return `#${channel(r1, r2)}${channel(g1, g2)}${channel(b1, b2)}`;
}

/** 把十六进制颜色转为带指定 alpha 的 rgba() 字符串。 */
function hexToRgba(hex: string, alpha: number): string {
  const [r, g, b] = parseHex(hex);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

/** 构建去重的系列调色板，并用主色调的明暗变体补齐。 */
function buildPalette(colors: Record<string, string>): string[] {
  const palette: string[] = [];
  for (const key of PALETTE_KEYS) {
    const color = colors[key];
    if (color && !palette.includes(color)) {
      palette.push(color);
    }
  }
  const primary = colors["primary"];
  if (primary) {
    const shades = [
      mixHex(primary, "#FFFFFF", 0.7),
      mixHex(primary, "#FFFFFF", 0.45),
      mixHex(primary, "#000000", 0.2),
      mixHex(primary, "#000000", 0.35),
    ];
    for (const shade of shades) {
      if (palette.length >= PALETTE_SIZE) {
        break;
      }
      if (!palette.includes(shade)) {
        palette.push(shade);
      }
    }
  }
  return palette;
}

/** 判断是否为纯对象（数组及其他值在合并时被替换）。 */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * 轻量深合并：纯对象递归合并，数组及其他值由覆盖值替换。
 * 页面级 ECharts 选项始终优先。
 */
function deepMerge(base: unknown, override: unknown): unknown {
  if (Array.isArray(base) || Array.isArray(override)) {
    return override === undefined ? base : override;
  }
  if (isPlainObject(base) && isPlainObject(override)) {
    const result: Record<string, unknown> = { ...base };
    for (const key of Object.keys(override)) {
      const baseValue = base[key];
      const overrideValue = override[key];
      if (overrideValue === undefined) {
        continue;
      }
      result[key] =
        isPlainObject(baseValue) && isPlainObject(overrideValue)
          ? deepMerge(baseValue, overrideValue)
          : overrideValue;
    }
    return result;
  }
  return override === undefined ? base : override;
}

/** xAxis 与 yAxis 共用的轴颜色默认值。 */
function axisDefaults(theme: ChartTheme): Record<string, unknown> {
  return {
    axisLine: { lineStyle: { color: theme.gridColor } },
    axisLabel: { color: theme.subTextColor },
    splitLine: { lineStyle: { color: theme.gridColor } },
  };
}

/**
 * 将主题默认值合并进 ECharts 选项。`option` 中的显式值始终优先
 * （深合并，数组被替换）。`grid` 有意留给页面默认值；坐标轴主题默认值
 * 仅注入到自带坐标轴的图表（折线/柱状），饼图等无坐标轴图表不注入，
 * 避免凭空引入 grid 组件触发未注册报错。
 */
export function withChartTheme<T extends object>(option: T, theme: ChartTheme): T {
  const themeDefaults: Record<string, unknown> = {
    color: theme.palette,
    textStyle: { color: theme.textColor },
    title: { textStyle: { color: theme.textColor } },
    legend: { textStyle: { color: theme.subTextColor } },
    tooltip: {
      backgroundColor: theme.tooltipBg,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.textColor },
    },
  };
  if (isPlainObject(option) && ("xAxis" in option || "yAxis" in option)) {
    themeDefaults.xAxis = axisDefaults(theme);
    themeDefaults.yAxis = axisDefaults(theme);
  }
  return deepMerge(themeDefaults, option) as T;
}
