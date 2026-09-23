<template>
  <!-- height / width 为公开 prop（默认 400px / 100%），由调用方按图表场景传入，
       属运行时计算值，故保留 :style 绑定 -->
  <v-chart :style="{ height, width }" :option="mergedOption" />
</template>

<script setup lang="ts" name="pie-chart">
import { use } from "echarts/core";
import { PieChart } from "echarts/charts";
import { TitleComponent, TooltipComponent, LegendComponent } from "echarts/components";
import VChart from "vue-echarts";
import { CanvasRenderer } from "echarts/renderers";
import type { PropType } from "vue";
import { useChartTheme, withChartTheme } from "@/utils/chartTheme";
import type { PiesChartOption } from "../@types/charts";

use([CanvasRenderer, PieChart, TitleComponent, TooltipComponent, LegendComponent]);

const props = defineProps({
  option: {
    type: Object as PropType<PiesChartOption>,
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
