import type { ComposeOption } from "echarts/core";
import type {
  LineSeriesOption,
  PieSeriesOption,
  BarSeriesOption,
} from "echarts/charts";
import type {
  TitleComponentOption,
  TooltipComponentOption,
  LegendComponentOption,
} from "echarts/components";
import type { XAXisComponentOption, YAXisComponentOption } from "echarts";

export type LineChartOption = ComposeOption<
  | TitleComponentOption
  | TooltipComponentOption
  | LegendComponentOption
  | XAXisComponentOption
  | YAXisComponentOption
  | LineSeriesOption
>;

export type PiesChartOption = ComposeOption<
  | TitleComponentOption
  | TooltipComponentOption
  | LegendComponentOption
  | PieSeriesOption
>;

export type BarsChartOption = ComposeOption<
  | TitleComponentOption
  | TooltipComponentOption
  | LegendComponentOption
  | XAXisComponentOption
  | YAXisComponentOption
  | BarSeriesOption
>;
