/**
 * src/utils/chartTheme.ts
 *
 * ECharts theme helpers driven by the Vuetify theme so every chart follows
 * the active light/dark theme (colors, text, grid and tooltip styling).
 */

import { computed } from "vue";
import type { ComputedRef } from "vue";
import { useTheme } from "vuetify";

/** Semantic color keys used to build the default series palette. */
const PALETTE_KEYS = ["primary", "secondary", "success", "warning", "error", "info"] as const;

/** Maximum number of colors a palette should contain. */
const PALETTE_SIZE = 8;

/**
 * Resolved chart theme snapshot. `useChartTheme()` returns this as a reactive
 * `ComputedRef` so charts re-render when the Vuetify theme changes.
 */
export interface ChartTheme {
  /** Current Vuetify theme name (e.g. "dark"). */
  name: string;
  /** Raw Vuetify theme colors keyed by semantic name. */
  colors: Record<string, string>;
  /** Series color ramp derived from the theme semantic colors. */
  palette: string[];
  /** Primary text color (titles, axis labels, tooltips). */
  textColor: string;
  /** Secondary text color (legend, dimmed labels). */
  subTextColor: string;
  /** Grid / axis line color (on-surface at low opacity). */
  gridColor: string;
  /** Tooltip background color. */
  tooltipBg: string;
  /** Tooltip border color. */
  tooltipBorder: string;
}

/**
 * Reactive chart theme. Watching the theme name keeps the returned values in
 * sync when the user toggles light/dark.
 */
export function useChartTheme(): ComputedRef<ChartTheme> {
  const theme = useTheme();

  return computed<ChartTheme>(() => {
    const colors = theme.global.current.value.colors;
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

/** Parse a 3- or 6-digit hex color into its RGB channels. */
function parseHex(hex: string): [number, number, number] {
  const value = hex.replace("#", "").trim();
  const digits = value.length === 3 ? value.split("").map((c) => c + c).join("") : value;
  const int = /^[0-9a-fA-F]{6}$/.test(digits) ? parseInt(digits, 16) : 0x000000;
  return [(int >> 16) & 0xff, (int >> 8) & 0xff, int & 0xff];
}

/** Mix `base` into `hex` by `ratio` (0 = pure hex, 1 = pure base). */
function mixHex(hex: string, base: string, ratio: number): string {
  const [r1, g1, b1] = parseHex(hex);
  const [r2, g2, b2] = parseHex(base);
  const channel = (a: number, b: number): string =>
    Math.round(a + (b - a) * ratio)
      .toString(16)
      .padStart(2, "0");
  return `#${channel(r1, r2)}${channel(g1, g2)}${channel(b1, b2)}`;
}

/** Convert a hex color into an rgba() string with the given alpha. */
function hexToRgba(hex: string, alpha: number): string {
  const [r, g, b] = parseHex(hex);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

/** Build a deduplicated series palette, padded with primary shades. */
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

/** True for plain objects (arrays and other values are replaced on merge). */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Lightweight deep merge: plain objects merge recursively, arrays and other
 * values are replaced by the override. Page-level ECharts options always win.
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

/** Axis color defaults shared by xAxis and yAxis. */
function axisDefaults(theme: ChartTheme): Record<string, unknown> {
  return {
    axisLine: { lineStyle: { color: theme.gridColor } },
    axisLabel: { color: theme.subTextColor },
    splitLine: { lineStyle: { color: theme.gridColor } },
  };
}

/**
 * Merge theme defaults into an ECharts option. Explicit values in `option`
 * always win (deep merged, arrays replaced). `grid` is intentionally left to
 * the page defaults.
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
    xAxis: axisDefaults(theme),
    yAxis: axisDefaults(theme),
  };
  return deepMerge(themeDefaults, option) as T;
}
