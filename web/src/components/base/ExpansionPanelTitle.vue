<template>
  <!-- 展开面板标题区基线：图标头像 + 加粗标题 + 说明；全站 v-expansion-panels 共用，
       用法处不得自写标题结构（见 AGENTS.md「展开面板族」）。
       size="sm" 为「紧凑变体」：标题与说明并为单行、头像缩至 24px，供默认收起、
       仅作收纳入口的折叠块（如 AI 助手的思考过程）压缩收起态占用高度 -->
  <v-expansion-panel-title :class="{ 'zk-expansion-title--sm': size === 'sm' }">
    <div class="panel-title" :class="{ 'panel-title--sm': size === 'sm' }">
      <v-avatar :color="color" variant="tonal" :size="avatarSize" rounded>
        <v-icon :size="iconSize" :class="{ 'mdi-spin': spin }">{{ icon }}</v-icon>
      </v-avatar>
      <div class="panel-title-text" :class="{ 'panel-title-text--sm': size === 'sm' }">
        <div class="zk-section-title">{{ title }}</div>
        <div v-if="subtitle" class="zk-hint">{{ subtitle }}</div>
      </div>
      <!-- 行内元信息（计数 chip 等）：紧随文字块，不顶到最右 -->
      <slot />
    </div>

    <!-- actions 槽会顶替 Vuetify 的默认展开图标，故由本组件统一补回 chevron，
         避免各使用方各自手写、图标与展开语义出现分叉 -->
    <template #actions="{ expanded }">
      <slot name="actions" />
      <v-icon class="ms-1" :icon="expanded ? '$collapse' : '$expand'" />
    </template>
  </v-expansion-panel-title>
</template>

<script setup lang="ts" name="expansion-panel-title">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    /** 标题区头像内的 mdi-* 图标 */
    icon: string;
    /** 加粗主标题 */
    title: string;
    /** 标题下方的说明文本；缺省（如分组未登记翻译）时不渲染说明行 */
    subtitle?: string;
    /** 头像色：默认 primary，仅状态类面板（成功 / 失败）改走语义色 */
    color?: string;
    /** 标题区图标是否旋转（运行中 / 加载态；复用 @mdi/font 的 .mdi-spin 动画） */
    spin?: boolean;
    /** 尺寸变体：md 为全站基线（头像 40px + 标题 / 说明上下两行），sm 为紧凑单行 */
    size?: "md" | "sm";
  }>(),
  { color: "primary", subtitle: "", spin: false, size: "md" },
);

// 紧凑变体收小头像与图标：24px 头像配 x-small 图标，与 40px 头像配 small 图标同比例
const avatarSize = computed(() => (props.size === "sm" ? 24 : 40));
const iconSize = computed(() => (props.size === "sm" ? "x-small" : "small"));
</script>

<style scoped>
/* 仅布局定位：字号 / 字重 / 颜色由外壳层的排版类承担 */
.panel-title {
  display: flex;
  align-items: center;
  gap: var(--zk-space-3);
}

.panel-title-text {
  display: flex;
  flex-direction: column;
}

/* 紧凑变体：头像与文字块、标题与说明各自贴上，说明随标题同一基线排列 */
/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.panel-title--sm {
  gap: var(--zk-space-2);
}

/* stylelint-disable-next-line selector-class-pattern -- BEM 修饰符，与同组件其它修饰符同风格 */
.panel-title-text--sm {
  flex-direction: row;
  align-items: baseline;
  gap: var(--zk-space-2);
}
</style>
