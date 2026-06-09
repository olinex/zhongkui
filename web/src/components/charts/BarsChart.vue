<template>
  <v-chart :style="{ height, width }" :option="option" />
</template>

<script setup lang="ts" name="bars-chart">
import { use } from "echarts/core";
import { BarChart } from "echarts/charts";
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
} from "echarts/components";
import VChart, { THEME_KEY } from "vue-echarts";
import { CanvasRenderer } from "echarts/renderers";
import { type BarsChartOption } from "../@types/charts";
import { type PropType, provide } from "vue";
import { useTheme } from "vuetify";

const theme = useTheme();

use([
  BarChart,
  CanvasRenderer,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
]);

provide(THEME_KEY, theme.current.value.dark ? "dark" : "primary");

const props = defineProps({
  option: {
    type: Object as PropType<BarsChartOption>,
    required: true,
  },
  height: {
    type: [Number, String],
    default: "400px",
  },
  width: {
    type: [Number, String],
  },
});
</script>
