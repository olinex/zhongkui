<template>
  <div class="d-flex align-center ga-2">
    <!-- 常驻按钮：收起时也保持展示（查询 / 刷新 / 创建） -->
    <v-btn
      v-if="query"
      :loading="loading"
      icon="mdi-magnify"
      color="primary"
      variant="text"
      :title="$t('action.query')"
      :aria-label="$t('action.query')"
      @click="query"
    />
    <v-btn
      v-if="refresh"
      :disabled="loading"
      icon="mdi-refresh"
      variant="text"
      :title="$t('action.refresh')"
      :aria-label="$t('action.refresh')"
      @click="refresh"
    />
    <v-btn
      v-if="create"
      icon="mdi-plus"
      color="primary"
      variant="text"
      :title="$t('action.create')"
      :aria-label="$t('action.create')"
      @click="create"
    />
    <!-- 常驻次级按钮（alwaysShow）：与查询/刷新/创建同级，收起时也保持展示 -->
    <v-btn
      v-for="(action, index) in alwaysShowActions"
      :key="index"
      :icon="action.icon"
      :color="action.color"
      variant="text"
      :title="action.title"
      :aria-label="action.title"
      @click="action.onClick"
    >
      <!-- 图标经默认插槽渲染（而非 icon prop 直接渲染），以便挂旋转类；同时因插槽存在，
           VBtn 不会再经 icon prop 额外渲染一次图标 -->
      <v-icon :class="{ 'mdi-spin': action.spin }" :icon="action.icon" />
    </v-btn>
    <!-- 次级按钮：仅可收起时随收起状态隐藏，否则始终展示 -->
    <template v-if="!collapsible || !collapse">
      <v-btn
        v-for="(action, index) in collapsibleActions"
        :key="index"
        :icon="action.icon"
        :color="action.color"
        variant="text"
        :title="action.title"
        :aria-label="action.title"
        @click="action.onClick"
      >
        <v-icon :class="{ 'mdi-spin': action.spin }" :icon="action.icon" />
      </v-btn>
      <!-- 次级插槽内容与 actions 同级：收起时一并隐藏，可放入自定义按钮/菜单组件 -->
      <slot />
    </template>
    <v-btn
      v-if="collapsible"
      :icon="collapse ? 'mdi-chevron-right' : 'mdi-chevron-left'"
      variant="text"
      :title="$t(collapse ? 'action.expand' : 'action.collapse')"
      :aria-label="$t(collapse ? 'action.expand' : 'action.collapse')"
      @click="collapse = !collapse"
    />
  </div>
</template>

<script setup lang="ts" name="action-button-group">
// ActionButtonGroup 表单页操作按钮组
// - 默认以 div 容器 + 图标按钮展示，hover 时经 :title tooltip 展示按钮名称
// - 按钮轮廓与背景容器（v-sheet / v-card）一致：variant="text"
// - collapsible 时收起仅保留常驻按钮（刷新 / 创建），次级按钮隐藏

// 次级操作按钮定义（收起时隐藏，alwaysShow 的除外）
export interface ActionButton {
  /** 图标名（mdi-*） */
  icon: string;
  /** 按钮名称（已翻译），hover 时经 tooltip 展示 */
  title: string;
  /** 图标颜色（可选） */
  color?: string;
  /** 旋转动效：true 时图标持续旋转（复用 @mdi/font 的 .mdi-spin），用于表达任务进行中（可选）。
      不可改用 VBtn 的 loading —— 会落 `.v-btn--loading{pointer-events:none}`，按钮将无法点击 */
  spin?: boolean;
  /** 常驻标志：true 时收起状态也保持展示（可选） */
  alwaysShow?: boolean;
  /** 点击回调 */
  onClick: () => void;
}

const props = withDefaults(
  defineProps<{
    /** 查询操作，显式查询模式下发起查询；收起时始终展示（不传则不渲染） */
    query?: () => void;
    /** 刷新操作，收起时始终展示（不传则不渲染） */
    refresh?: () => void;
    /** 创建操作，收起时始终展示（不传则不渲染） */
    create?: () => void;
    /** 次级操作，收起时隐藏（alwaysShow 的除外） */
    actions?: ActionButton[];
    /** 是否可收起（展示收起/展开切换按钮） */
    collapsible?: boolean;
    /** 加载状态，禁用刷新按钮 */
    loading?: boolean;
  }>(),
  {
    query: undefined,
    refresh: undefined,
    create: undefined,
    actions: () => [],
    collapsible: false,
    loading: false,
  },
);

// 常驻次级按钮（alwaysShow）：收起时也保持展示
const alwaysShowActions = computed(() => props.actions.filter((a) => a.alwaysShow));
// 普通次级按钮：收起时隐藏
const collapsibleActions = computed(() => props.actions.filter((a) => !a.alwaysShow));

// 收起状态：默认收起，仅展示刷新/创建等常驻按钮
const collapse = ref(true);
</script>
