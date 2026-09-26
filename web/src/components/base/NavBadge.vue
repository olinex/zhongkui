<template>
  <!-- 无标识时不渲染任何元素：调用方直接绑定解析结果即可，空插槽不会留下空容器参与布局 -->
  <!-- 限定 tooltip 宽度（Vuetify 以行内样式落到 .v-overlay__content，故只能用 max-width 属性、
       不能用 CSS 类覆盖）：Vuetify 的位置策略在首选方向放不下时会自动翻到对侧，而标识紧贴
       抽屉右侧、左侧可用空间约 133px，宽度超过该值即被翻到右侧，使同类标识方向不一致 -->
  <v-tooltip v-if="kind" :text="tooltip" location="left" :max-width="TOOLTIP_MAX_WIDTH">
    <template #activator="{ props: tooltipProps }">
      <v-icon
        v-bind="tooltipProps"
        :icon="APPEARANCE[kind].icon"
        size="small"
        :color="APPEARANCE[kind].color"
      />
    </template>
  </v-tooltip>
</template>

<script setup lang="ts" name="nav-badge">
import type { NavBadgeKind } from "@/navigation";
import i18n from "@/utils/i18n";

const props = defineProps<{
  // kind 标识种类；缺省时不渲染任何标识
  kind?: NavBadgeKind;
}>();

// TOOLTIP_MAX_WIDTH tooltip 最大宽度（px）：标识紧贴抽屉右侧，其左侧可用空间（抽屉宽度减去
// tooltip 与图标的偏移及 Vuetify 的边界内缩）约 133px，取 132 使 tooltip 恒定朝左展开。
// 该值也是文案长度的预算：可容纳约 6~7 个汉字，故文案只表达「是什么」，
// 不承载后端下发的具体不可用原因（技术细节长且随实现变化，交由说明页承载）
const TOOLTIP_MAX_WIDTH = 132;

// 标识的图标与语义色成对定义：锁定是不可用**状态**，用 warning；命名空间是**归属**而非状态，
// 用 primary（语义色只表达状态，禁止用于表达归属关系）
const APPEARANCE = {
  lock: { icon: "mdi-lock-outline", color: "warning" },
  namespace: { icon: "mdi-alpha-n-box-outline", color: "primary" },
} as const;

// tooltip 标识说明：一律取短文案（见 TOOLTIP_MAX_WIDTH），保证单行展示
const tooltip = computed(() =>
  i18n.global.t(props.kind === "lock" ? "navbar.featureUnavailable" : "navbar.namespaceScoped"),
);
</script>
