<template>
  <v-menu v-model="menu" transition="scale-transition" :close-on-content-click="false">
    <template #activator="{ props: activatorProps }">
      <!-- 密度由主题层统一控制（VBtn density="compact"）。 -->
      <v-btn
        v-bind="activatorProps"
        icon="mdi-view-column-outline"
        variant="text"
        :title="$t('action.columns')"
        :aria-label="$t('action.columns')"
      />
    </template>
    <v-card class="zk-panel">
      <v-card-title class="zk-section-title px-4 pt-3 pb-1">
        {{ $t("action.columns") }}
      </v-card-title>
      <!-- 不再就地限高：VMenu 内容由 Vuetify 按可视区裁剪，.v-card 自身 overflow:auto 滚动 -->
      <v-card-text class="px-0 py-0">
        <v-list>
          <v-list-item
            v-for="hdr in displayHeaders"
            :key="hdr.key"
            draggable="true"
            :class="{
              'drag-source': dragKey === hdr.key,
              'zk-drag-over': dragOverKey === hdr.key,
            }"
            @dragstart="onDragStart(hdr.key)"
            @dragend="onDragEnd"
            @dragover.prevent="onDragOver(hdr.key)"
            @drop.prevent="onDrop(hdr.key)"
            @click="toggle(hdr.key)"
          >
            <template #prepend>
              <!-- checkbox 仅作为可见性指示器（只读展示），点击事件不拦截，
                   统一由列表项的行点击 toggle 驱动显隐，避免出现“勾选无反应”的死区 -->
              <v-checkbox-btn
                :model-value="draftVisible.includes(hdr.key)"
                density="compact"
                hide-details
                readonly
              />
            </template>
            <v-list-item-title class="text-body-medium">{{ hdr.title }}</v-list-item-title>
            <!-- 列补充说明：链路槽位等语义随链路类型变化的列在此给出摘要 -->
            <v-list-item-subtitle v-if="hdr.description" class="text-body-small on-surface-variant">
              {{ hdr.description }}
            </v-list-item-subtitle>
            <template #append>
              <v-icon icon="mdi-drag" size="small" class="on-surface-variant drag-handle" />
            </template>
          </v-list-item>
        </v-list>
      </v-card-text>
      <v-divider />
      <v-card-actions class="px-3">
        <v-btn :text="$t('action.selectAll')" size="small" variant="text" @click="selectAll" />
        <v-btn
          :text="$t('action.systemDefault')"
          size="small"
          variant="text"
          @click="resetToDefault"
        />
        <v-spacer />
        <v-btn :text="$t('action.cancel')" size="small" variant="text" @click="cancel" />
        <v-btn
          :text="$t('action.confirm')"
          size="small"
          variant="flat"
          color="primary"
          :disabled="!dirty"
          @click="confirm"
        />
      </v-card-actions>
    </v-card>
  </v-menu>
</template>

<script setup lang="ts" name="column-visibility-menu">
// ColumnVisibilityMenu 列设置菜单
// 勾选/拖拽/全选/系统默认均先落入本地草稿（draft），仅点击“确认”才通过
// update:modelValue / update:order 一次性提交给父级，避免逐个勾选触发频繁的
// 配置持久化与同步请求；“取消”或关闭菜单则丢弃草稿
import type { PropType } from "vue";
import type { TableHeader } from "../@types/table";

const props = defineProps({
  // 全部可选列定义
  headers: {
    type: Object as PropType<Array<TableHeader>>,
    required: true,
  },
  // 当前可见列
  modelValue: {
    type: Object as PropType<Array<string>>,
    required: true,
  },
  // 当前列顺序
  order: {
    type: Object as PropType<Array<string>>,
    required: true,
  },
});

// emits 草稿确认时一次性提交列可见性与列顺序。
const emits = defineEmits<{
  (e: "update:modelValue", value: Array<string>): void;
  (e: "update:order", value: Array<string>): void;
}>();

// 菜单打开状态：仅在打开瞬间以父级配置初始化草稿
const menu = ref(false);
// 草稿：可见列子集与列顺序，确认前不触达父级
const draftVisible = ref<Array<string>>([]);
const draftOrder = ref<Array<string>>([]);

watch(menu, (open) => {
  if (open) {
    draftVisible.value = [...props.modelValue];
    draftOrder.value = [...props.order];
  }
});

// 菜单展示顺序：按草稿顺序排列全部可切换列（拖拽即时反馈，确认后才提交）
const displayHeaders = computed(() => {
  const orderIndex = new Map(draftOrder.value.map((key, index) => [key, index]));
  return [...props.headers].sort(
    (a, b) =>
      (orderIndex.get(a.key) ?? Number.POSITIVE_INFINITY) -
      (orderIndex.get(b.key) ?? Number.POSITIVE_INFINITY),
  );
});

// 拖拽排序状态：dragKey 为正在拖拽的列，dragOverKey 为当前悬停目标（用于指示器）
const dragKey = ref<string | null>(null);
const dragOverKey = ref<string | null>(null);

const onDragStart = (key: string): void => {
  dragKey.value = key;
};

const onDragEnd = (): void => {
  dragKey.value = null;
  dragOverKey.value = null;
};

const onDragOver = (key: string): void => {
  dragOverKey.value = key;
};

// 落点：将拖拽列移动到目标列当前位置（仅在草稿内调整顺序）
const onDrop = (key: string): void => {
  const dragged = dragKey.value;
  onDragEnd();
  if (dragged === null || dragged === key) {
    return;
  }
  const order = [...draftOrder.value];
  const from = order.indexOf(dragged);
  const to = order.indexOf(key);
  if (from < 0 || to < 0) {
    return;
  }
  const next = [...order];
  next.splice(from, 1);
  next.splice(to, 0, dragged);
  draftOrder.value = next;
};

// 切换单列显示状态（仅在草稿内调整可见性）
const toggle = (key: string): void => {
  if (draftVisible.value.includes(key)) {
    draftVisible.value = draftVisible.value.filter((k) => k !== key);
  } else {
    draftVisible.value = [...draftVisible.value, key];
  }
};

// 全部显示（等价于恢复默认）
const selectAll = (): void => {
  draftVisible.value = props.headers.map((hdr) => hdr.key);
};

// 恢复系统默认配置：列顺序回到表头定义顺序，仅展示 defaultVisible 标记的列（缺省可见）
const resetToDefault = (): void => {
  draftOrder.value = props.headers.map((hdr) => hdr.key);
  draftVisible.value = props.headers
    .filter((hdr) => hdr.defaultVisible !== false)
    .map((hdr) => hdr.key);
};

// 是否存在未确认的改动：可见性或顺序任一变化时，确认按钮才可点击
const dirty = computed(() => {
  return (
    draftVisible.value.join(",") !== props.modelValue.join(",") ||
    draftOrder.value.join(",") !== props.order.join(",")
  );
});

// 确认：仅当草稿确有变化时一次性提交给父级，并关闭菜单
const confirm = (): void => {
  menu.value = false;
  if (draftVisible.value.join(",") !== props.modelValue.join(",")) {
    emits("update:modelValue", [...draftVisible.value]);
  }
  if (draftOrder.value.join(",") !== props.order.join(",")) {
    emits("update:order", [...draftOrder.value]);
  }
};

// 取消：丢弃草稿，直接关闭菜单（父级状态保持不变）
const cancel = (): void => {
  menu.value = false;
};
</script>

<style scoped>
/* 仅保留布局 / 状态定位声明；容器外观（描边、圆角、底色）走 .zk-panel */

/* 拖拽把手：提示该行可拖拽排序 */
.drag-handle {
  cursor: grab;
}

/* 正在拖拽的列半透明显示 */
.drag-source {
  opacity: 0.4;
}
</style>
