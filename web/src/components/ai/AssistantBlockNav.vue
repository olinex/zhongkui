<template>
  <!-- 块间快速跳读：消息区右缘的最小图标控件——上一块 / 当前第几块（共几块）/ 下一块，与提问轨
       「直达底部」同口径：透明底、on-surface 强色、悬停转主色，仅用 icon 表达上下移动。 -->
  <div class="assistant-block-nav" role="group" :aria-label="$t('page.aiAssistant.blockNav')">
    <v-tooltip location="left" :text="$t('page.aiAssistant.blockPrev')">
      <template #activator="{ props }">
        <v-btn
          v-bind="props"
          class="zk-chat-nav-btn"
          icon="mdi-arrow-up"
          size="x-small"
          variant="text"
          :disabled="!canPrev"
          :aria-label="$t('page.aiAssistant.blockPrev')"
          @click="emit('prev')"
        />
      </template>
    </v-tooltip>
    <!-- 当前位置 / 总块数：夹在上下箭头之间，读作「第 3 块 / 共 12 块」。
         两端均补零到两位（如 09/11），避免 9/11 ↔ 10/11 切换时宽度跳动导致指针点偏 -->
    <span class="zk-chat-nav-count zk-hint" aria-hidden="true">
      {{ padCount(current + 1) }}/{{ padCount(total) }}
    </span>
    <v-tooltip location="left" :text="$t('page.aiAssistant.blockNext')">
      <template #activator="{ props }">
        <v-btn
          v-bind="props"
          class="zk-chat-nav-btn"
          icon="mdi-arrow-down"
          size="x-small"
          variant="text"
          :disabled="!canNext"
          :aria-label="$t('page.aiAssistant.blockNext')"
          @click="emit('next')"
        />
      </template>
    </v-tooltip>
  </div>
</template>

<script setup lang="ts" name="assistant-block-nav">
withDefaults(
  defineProps<{
    /** 当前块下标（从 0 起），用于置灰两端按钮 */
    current?: number;
    /** 块总数 */
    total?: number;
    /** 是否还有向上的目标：首块中段（首块开头仍在视口上方）也算可向上 */
    canPrev?: boolean;
    /** 是否还有向下的目标：末块之后还有下一轮回答时也算可向下 */
    canNext?: boolean;
  }>(),
  { current: 0, total: 0, canPrev: true, canNext: true },
);

const emit = defineEmits<{
  /** 上一块 */
  prev: [];
  /** 下一块 */
  next: [];
}>();

// padCount 块计数补零到两位：一位数 ↔ 两位数切换时组件宽度保持不变；超过 99 块时按实际位数
// 展示（此时宽度变化可接受）
const padCount = (n: number): string => (n > 99 ? String(n) : String(n).padStart(2, "0"));
</script>

<style scoped>
/* 布局定位层：颜色 / 悬停变色 / 圆角 / 底色由 .zk-chat-nav-btn / .zk-chat-nav-count 承载。 */

.assistant-block-nav {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--zk-space-1);
}

/* 计数用等宽数字，避免 9/10 → 10/10 时宽度跳动（tabular-nums 在外壳类中统一声明） */
.zk-chat-nav-count {
  white-space: nowrap;
}
</style>
