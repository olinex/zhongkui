<template>
  <!-- 可拖拽边框手柄：贴父容器指定边，父容器须 position: relative。
       常驻握把短条的朝向随可调轴向切换——左右边框为竖条（可拖宽）、
       上下边框为横条（可拖高），让用户静态即可分辨能拖宽还是能拖高 -->
  <div
    class="resize-handle"
    :class="[`is-${side}`, { 'is-resizing': resizing }]"
    role="separator"
    :aria-orientation="isWidthResize ? 'vertical' : 'horizontal'"
    :title="label"
    :aria-label="label"
    @pointerdown="onPointerDown"
  />
</template>

<script setup lang="ts" name="resize-handle">
import i18n from "@/utils/i18n";

// ResizeSide 可拖拽边：left / right 调宽，top / bottom 调高
type ResizeSide = "left" | "right" | "top" | "bottom";

const props = defineProps<{
  /** 当前尺寸（px），双向绑定；组件据拖拽位移实时回写收敛后的值。 */
  modelValue: number;
  /** 可拖拽边：left / right 调宽，top / bottom 调高。 */
  side: ResizeSide;
  /** 尺寸下限（px）。 */
  min: number;
  /** 尺寸上限（px）。 */
  max: number;
}>();

const emits = defineEmits<{
  "update:modelValue": [value: number];
}>();

// isWidthResize 该边调的是宽度（左右边框）还是高度（上下边框）
const isWidthResize = computed<boolean>(() => props.side === "left" || props.side === "right");

// label 手柄说明：按轴向取通用文案（可拖宽 / 可拖高），同时作为悬浮提示
const label = computed(() =>
  i18n.global.t(isWidthResize.value ? "action.resizeWidth" : "action.resizeHeight"),
);

// resizing 是否正在拖拽；startPos 起点指针坐标；startSize 起点尺寸
const resizing = ref(false);
let startPos = 0;
let startSize = 0;

// onPointerDown 开始拖拽：以起点尺寸为基准，拖离该边的对侧即为增大
const onPointerDown = (e: PointerEvent): void => {
  resizing.value = true;
  startPos = isWidthResize.value ? e.clientX : e.clientY;
  startSize = props.modelValue;
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp);
  window.addEventListener("blur", onPointerUp);
};

// onPointerMove 拖拽中：按指针位移换算尺寸（left / top 向左 / 上为增，right / bottom 向右 / 下为增），
// 并收敛到 [min, max]；位置监听挂在 window 上，指针移出元素后仍可继续拖拽
const onPointerMove = (e: PointerEvent): void => {
  if (!resizing.value) {
    return;
  }
  const current = isWidthResize.value ? e.clientX : e.clientY;
  const sign = props.side === "left" || props.side === "top" ? -1 : 1;
  const next = startSize + (current - startPos) * sign;
  emits("update:modelValue", Math.min(props.max, Math.max(props.min, Math.round(next))));
};

// onPointerUp 结束拖拽：移除监听（窗口失焦时同样结束，避免残留「拖拽中」）
const onPointerUp = (): void => {
  if (!resizing.value) {
    return;
  }
  resizing.value = false;
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", onPointerUp);
  window.removeEventListener("blur", onPointerUp);
};

onUnmounted(onPointerUp);
</script>

<style scoped>
/* 命中区：贴父容器指定边，仅占 8px 不影响内容可点区域。起点自带 user-select: none——
   浏览器只在可选中的落点上开启选区，手柄落点不可选中即可避免拖动掠过内容时产生选区 */
.resize-handle {
  position: absolute;
  z-index: 1;
  user-select: none;
  touch-action: none;
}

/* 左右边框 —— 可拖宽：命中区为贴边竖条，光标为 col-resize */
.resize-handle.is-left,
.resize-handle.is-right {
  inset-block: 0;
  width: var(--zk-space-2);
  cursor: col-resize;
}

.resize-handle.is-left {
  inset-inline-start: 0;
}

.resize-handle.is-right {
  inset-inline-end: 0;
}

/* 上下边框 —— 可拖高：命中区为贴边横条，光标为 row-resize */
.resize-handle.is-top,
.resize-handle.is-bottom {
  inset-inline: 0;
  height: var(--zk-space-2);
  cursor: row-resize;
}

.resize-handle.is-top {
  inset-block-start: 0;
}

.resize-handle.is-bottom {
  inset-block-end: 0;
}

/* 握把短条：居边框正中，常态低透明度常驻，悬停 / 拖拽时转 primary。
   朝向即轴向——竖条表达可拖宽，横条表达可拖高 */
.resize-handle::before {
  content: "";
  position: absolute;
  inset: 0;
  margin: auto;
  border-radius: var(--zk-radius-pill);
  background: rgb(var(--v-theme-on-surface), var(--zk-resize-grip-opacity));
  transition: background var(--zk-transition-fast);
}

.resize-handle.is-left::before,
.resize-handle.is-right::before {
  width: var(--zk-resize-grip-thickness);
  height: var(--zk-resize-grip-length);
}

.resize-handle.is-top::before,
.resize-handle.is-bottom::before {
  width: var(--zk-resize-grip-length);
  height: var(--zk-resize-grip-thickness);
}

.resize-handle:hover::before,
.resize-handle.is-resizing::before {
  background: rgb(var(--v-theme-primary));
}
</style>
