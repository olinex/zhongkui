<template>
  <v-chart :style="{ height, width }" :option="mergedOption" />
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
import VChart from "vue-echarts";
import { CanvasRenderer } from "echarts/renderers";
import { type BarsChartOption } from "../@types/charts";
import { type PropType } from "vue";
import { useChartTheme, withChartTheme } from "@/utils/chartTheme";

use([BarChart, CanvasRenderer, TitleComponent, TooltipComponent, LegendComponent, GridComponent]);

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
    default: "100%",
  },
});

const chartTheme = useChartTheme();
// Merge theme defaults so the chart follows light/dark switching immediately.
const mergedOption = computed(() => withChartTheme(props.option, chartTheme.value));
</script>
