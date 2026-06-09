<template>
  <v-chart :style="{ height, width }" :option="option" />
</template>

<script setup lang="ts" name="line-chart">
import { use } from "echarts/core";
import { LineChart } from "echarts/charts";
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
} from "echarts/components";
import VChart, { THEME_KEY } from "vue-echarts";
import { CanvasRenderer } from "echarts/renderers";
import type { LineChartOption } from "../@types/charts";
import type { PropType } from "vue";
import { useTheme } from "vuetify/lib/composables/theme.mjs";

const theme = useTheme();

use([
  LineChart,
  CanvasRenderer,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
]);

provide(THEME_KEY, theme.current.value.dark ? "dark" : "primary");

const props = defineProps({
  option: {
    type: Object as PropType<LineChartOption>,
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
