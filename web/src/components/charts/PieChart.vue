<template>
  <v-chart :style="{ height, width }" :option="option" />
</template>

<script setup lang="ts" name="pie-chart">
import { use } from "echarts/core";
import { PieChart } from "echarts/charts";
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
} from "echarts/components";
import VChart from "vue-echarts";
import { CanvasRenderer } from "echarts/renderers";
import type { PropType } from "vue";
import type { PiesChartOption } from "../@types/charts";

use([
  CanvasRenderer,
  PieChart,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
]);

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
  },
});
</script>
