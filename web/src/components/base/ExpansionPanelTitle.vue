<template>
  <!-- 展开面板标题区基线：图标头像 + 加粗标题 + 说明；全站 v-expansion-panels 共用，
       用法处不得自写标题结构（见 AGENTS.md「展开面板族」） -->
  <v-expansion-panel-title>
    <div class="panel-title">
      <v-avatar :color="color" variant="tonal" size="40" rounded>
        <v-icon size="small" :class="{ 'mdi-spin': spin }">{{ icon }}</v-icon>
      </v-avatar>
      <div class="panel-title-text">
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
withDefaults(
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
  }>(),
  { color: "primary", subtitle: "", spin: false },
);
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
</style>
