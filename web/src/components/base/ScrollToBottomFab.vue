<template>
  <!-- positionStyle 为运行时拖拽坐标（持久化到 localStorage），非静态样式，故保留 :style 绑定 -->
  <div v-if="visible" class="scroll-fab" :style="positionStyle" @pointerdown="onPointerDown">
    <v-fab
      :title="$t('action.scrollToBottom')"
      :aria-label="$t('action.scrollToBottom')"
      icon="mdi-arrow-down"
      color="primary"
      @click="onFabClick"
    />
  </div>
</template>

<script setup lang="ts" name="scroll-to-bottom-fab">
import { localManager } from "@/stores/local";

// FAB 直径（v-fab 默认尺寸 56px），用于把拖拽位置约束在视口内
const FAB_SIZE = 56;
// 默认贴靠右下角时距视口边距
const MARGIN = 24;
// 拖拽位移超过该阈值判定为「拖动」而非「点击」，避免拖完误触发跳转
const DRAG_THRESHOLD = 4;
// 距离页面底部小于该像素视为已到底，隐藏悬浮按钮
const BOTTOM_EPSILON = 8;

// 页面内容是否超出视口高度（出现纵向滚动条）
const isScrollable = ref(false);
// 是否已滚动到页面底部
const isAtBottom = ref(false);

// 拖动后的按钮左上角坐标（px）；未拖动过为 (-1,-1)，此时按右下角贴靠定位
const position = ref(localManager.getScrollFabPos() ?? { left: -1, top: -1 });
const positionStyle = computed(() => {
  if (position.value.left >= 0 && position.value.top >= 0) {
    return { left: `${position.value.left}px`, top: `${position.value.top}px` };
  }
  return { right: `${MARGIN}px`, bottom: `${MARGIN}px` };
});

// 仅当页面可滚动且尚未到底时展示
const visible = computed(() => isScrollable.value && !isAtBottom.value);

const dragging = ref(false);
// 本次拖拽是否已产生位移（超过阈值）；用于点击时区分"拖动"与"单击"
const dragMoved = ref(false);
// 拖拽结束后抑制紧随其后的 click（浏览器会在 pointerup 后派发 click），
// 避免拖完误触发跳转；在 onFabClick 中消费后复位
const suppressClick = ref(false);
const dragStart = reactive({ pointerX: 0, pointerY: 0, left: 0, top: 0 });

// 刷新可滚动/到底状态，并把拖动位置约束回视口内（视口缩放或路由切换后）
const update = () => {
  const doc = document.documentElement;
  isScrollable.value = doc.scrollHeight > window.innerHeight + 1;
  isAtBottom.value = window.scrollY + window.innerHeight >= doc.scrollHeight - BOTTOM_EPSILON;
  if (position.value.left >= 0 || position.value.top >= 0) {
    position.value.left = Math.min(Math.max(position.value.left, 0), window.innerWidth - FAB_SIZE);
    position.value.top = Math.min(Math.max(position.value.top, 0), window.innerHeight - FAB_SIZE);
  }
};

// 视口尺寸变化：按约定把按钮复位回右下角贴靠——丢弃已拖动的坐标并清除持久化，
// 使按钮重新显示时直接落在右下角；随后刷新可滚动/到底状态。
// 仅响应 window resize（视口尺寸），不含内容高度变化（ResizeObserver 仍走 update），
// 避免数据加载导致内容变高时误清用户拖动的位置
const onResize = () => {
  position.value = { left: -1, top: -1 };
  localManager.clearScrollFabPos();
  update();
};

// 直达页面底部（平滑滚动）
const scrollToBottom = () => {
  window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "smooth" });
};

// 开始拖拽：未拖动过（right/bottom 定位）时先换算为 left/top 坐标以便增量移动。
// 额外监听 window blur 与指针移出浏览器边界（document.mouseout 且无 relatedTarget），
// 指针离开页面后即时结束拖拽、原地固定，避免残留"拖动中"导致鼠标移回时继续位移
const onPointerDown = (e: PointerEvent) => {
  if (position.value.left < 0 || position.value.top < 0) {
    position.value.left = window.innerWidth - FAB_SIZE - MARGIN;
    position.value.top = window.innerHeight - FAB_SIZE - MARGIN;
  }
  dragging.value = true;
  dragMoved.value = false;
  dragStart.pointerX = e.clientX;
  dragStart.pointerY = e.clientY;
  dragStart.left = position.value.left;
  dragStart.top = position.value.top;
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp);
  window.addEventListener("blur", onDragLeave);
  document.addEventListener("mouseout", onMouseOutDocument);
};

const onPointerMove = (e: PointerEvent) => {
  if (!dragging.value) return;
  const dx = e.clientX - dragStart.pointerX;
  const dy = e.clientY - dragStart.pointerY;
  if (!dragMoved.value && (Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD)) {
    dragMoved.value = true;
  }
  position.value.left = Math.min(Math.max(dragStart.left + dx, 0), window.innerWidth - FAB_SIZE);
  position.value.top = Math.min(Math.max(dragStart.top + dy, 0), window.innerHeight - FAB_SIZE);
};

const onPointerUp = () => finalizeDrag();
// 窗口失焦（于点住鼠标时切到其它应用）也结束拖拽
const onDragLeave = () => finalizeDrag();
// 指针移出浏览器文档边界（relatedTarget 为空即离开整个页面）时原地固定
const onMouseOutDocument = (e: MouseEvent) => {
  if (e.relatedTarget === null) {
    finalizeDrag();
  }
};

// 结束拖拽：移除全部监听；产生了位移才算真正拖动并持久化新位置，
// 同时抑制紧随其后（或浏览器在边界外补发的）click，避免误触发跳转
const finalizeDrag = () => {
  if (!dragging.value) return;
  dragging.value = false;
  window.removeEventListener("pointermove", onPointerMove);
  window.removeEventListener("pointerup", onPointerUp);
  window.removeEventListener("blur", onDragLeave);
  document.removeEventListener("mouseout", onMouseOutDocument);
  if (dragMoved.value) {
    dragMoved.value = false;
    suppressClick.value = true;
    localManager.setScrollFabPos({ left: position.value.left, top: position.value.top });
  }
};

// 跳转页面底部（平滑滚动）；若刚结束一次拖动则忽略随后的 click
const onFabClick = () => {
  if (suppressClick.value) {
    suppressClick.value = false;
    return;
  }
  scrollToBottom();
};

let resizeObserver: ResizeObserver | undefined;
onMounted(() => {
  update();
  window.addEventListener("scroll", update, { passive: true });
  window.addEventListener("resize", onResize);
  // 表格/内容高度变化（如调整每页数目后行数骤增）时刷新可见性：
  // scroll 事件只在滚动时触发，内容变高但用户未滚动时需由尺寸观察兜底
  resizeObserver = new ResizeObserver(update);
  resizeObserver.observe(document.documentElement);
});
onUnmounted(() => {
  window.removeEventListener("scroll", update);
  window.removeEventListener("resize", onResize);
  resizeObserver?.disconnect();
});
</script>

<style scoped>
/* 全局悬浮：右下角贴靠，可被拖动；拖动时禁止选中文本与触摸滚动干扰 */
.scroll-fab {
  position: fixed;
  z-index: 1000;
  user-select: none;
  touch-action: none;
}
</style>
