<template>
  <div class="assistant-question-rail">
    <!-- location 必须显式写出纵向对齐轴：Vuetify 的 parseAnchor 对省略对齐轴的
         inline 定位（如 'end'）补 top，面板会贴着轨的顶边；写 'end center' 才与圆点簇同轴居中 -->
    <v-menu
      v-model="open"
      location="end center"
      open-on-hover
      :open-delay="OPEN_DELAY"
      :close-delay="CLOSE_DELAY"
      :max-width="panelMaxWidth"
    >
      <template #activator="{ props: menuProps }">
        <!-- 收起态：一条竖轨 + 每个提问一个圆点，只表达「有几个提问」；
             悬停 / 键盘聚焦激活器即展开面板（open-on-hover 下 open-on-focus 默认开启） -->
        <div
          v-bind="menuProps"
          class="rail-track"
          :aria-label="$t('page.aiAssistant.questionRail')"
        >
          <div class="rail-dots">
            <!-- 按钮承担命中区（撑满轨宽，见 scoped），视觉圆点由内层 .zk-question-mark 承载：
                 8px 的圆点作为按钮本体时，点偏一点就落到轨的空白处，点击会「毫无反应」 -->
            <button
              v-for="q in questions"
              :key="q.key"
              type="button"
              class="zk-question-dot"
              :class="{ 'is-active': q.key === activeKey }"
              :title="q.text"
              :aria-label="q.text"
              @click="pick(q.key)"
            >
              <span class="zk-question-mark" />
            </button>
          </div>
          <!-- 特殊按钮：直达底部。与提问圆点同规格——同为 8×16 命中区、8px 标记、零额外间距，
               故与圆点同节奏；靠「箭头图标 vs 实心圆点」的形状差异区分动作。已到底部时它无
               跳转目标，故隐藏。click.stop：它是菜单激活器的后代，不拦截会连带打开提问面板 -->
          <button
            v-if="!atBottom"
            type="button"
            class="zk-rail-bottom"
            :title="$t('action.scrollToBottom')"
            :aria-label="$t('action.scrollToBottom')"
            @click.stop="pickBottom"
          >
            <v-icon icon="mdi-arrow-down" size="x-small" />
          </button>
        </div>
      </template>
      <!-- 浮层须用 v-sheet 承载：VMenu 只对直接子级的 .v-card / .v-sheet / .v-list 上底色与阴影 -->
      <v-sheet class="zk-panel rail-panel">
        <v-list class="rail-list">
          <v-list-item
            v-for="(q, idx) in questions"
            :key="q.key"
            class="zk-row-hover"
            :class="{ 'zk-row-selected': q.key === activeKey, 'zk-hairline-top': idx > 0 }"
            @click="pick(q.key)"
          >
            <v-list-item-title>{{ q.text }}</v-list-item-title>
          </v-list-item>
        </v-list>
      </v-sheet>
    </v-menu>
  </div>
</template>

<script setup lang="ts" name="assistant-question-rail">
// 展开 / 收起延迟：展开略快于 --zk-transition-fast，达到「即悬即开」；
// 收起留 300ms 缓冲，让指针从轨移向右侧面板的途中不误收（同 AgentParamPanel 先例）
const OPEN_DELAY = 120;
const CLOSE_DELAY = 300;

// RailQuestion 轨上单个提问：key 为对应消息的稳定键（点击后回传给调用方定位），
// text 为归一化空白后的提问正文（同时作为圆点的悬浮提示与无障碍名称）
interface RailQuestion {
  key: number;
  text: string;
}

withDefaults(
  defineProps<{
    /** 提问列表，按消息顺序排列。 */
    questions: RailQuestion[];
    /** 当前视口内的提问键：圆点与列表行以选中态高亮，无匹配时为 undefined。 */
    activeKey?: number;
    /** 消息区是否已滚到底部：已到底部时隐藏「直达底部」按钮（无跳转目标）。 */
    atBottom?: boolean;
    /** 展开面板最大宽度（px）：由调用方按抽屉宽度收敛，避免浮层越过抽屉右缘触发翻边。 */
    panelMaxWidth?: number;
  }>(),
  { activeKey: undefined, atBottom: true, panelMaxWidth: 300 },
);

const emits = defineEmits<{
  select: [key: number];
  bottom: [];
}>();

// open 与 v-menu 双向绑定：点击后显式收起，避免面板盖住刚跳转到的这一轮
const open = ref(false);

// pick 选中某个提问：先收起面板再上报，跳转由调用方完成
const pick = (key: number): void => {
  open.value = false;
  emits("select", key);
};

// pickBottom 直达底部：同样先收起面板，跳转由调用方完成
const pickBottom = (): void => {
  open.value = false;
  emits("bottom");
};
</script>

<style scoped>
/* 轨列：与抽屉左缘的拖拽手柄同宽（--zk-space-2）、同高（inset-block: 0，延伸到抽屉底部）。
   inset-inline-start 与全抽屉 8px 的内边距（头部 pa-2 等）取齐；左缘拖拽手柄已改为贴到
   抽屉外侧（placement="outside"），故不再需要为它的命中条让位。
   绝对定位的包含块是 .assistant-drawer（position: relative），故不参与消息区布局、
   不随正文滚动；消息区左内边距由 .assistant-messages 的 ps-6 让出（见 AssistantDrawer） */
.assistant-question-rail {
  position: absolute;
  inset-block: 0;
  inset-inline-start: var(--zk-space-2);
  width: var(--zk-space-2);
}

/* 轨：整轨高度（即抽屉整高），圆点簇（含其下方的「直达底部」按钮）垂直居中——用户明确要求
   「居中对齐以减少鼠标移动」，故不做沿整轨等分。safe 关键字保证提问数过多、内容高过轨时
   退化为顶对齐，而非被裁掉。裁剪下沉到 .rail-dots：圆点簇突界时先裁圆点、保住按钮 */
.rail-track {
  display: flex;
  flex-direction: column;
  justify-content: safe center;
  height: 100%;
}

/* 圆点簇：只含提问圆点，高度即内容高；提问过多（高过轨）时由自身 overflow: hidden 裁掉露头，
   不侵占其下「直达底部」按钮的位置——提问最多时反倒最需要它 */
.rail-dots {
  flex: 0 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 「直达底部」特殊按钮：尺寸与节奏完全对齐圆点按钮——同为撑满轨宽、固定高 --zk-space-4
   的命中区且不留额外间距，故与圆点的视觉间距一致；外观（颜色 / 悬停 / 图标尺寸）登记在
   外壳层 .zk-rail-bottom */
.zk-rail-bottom {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: var(--zk-space-4);
}

/* 圆点按钮：固定高（--zk-space-4 = 16px）、宽度撑满轨宽——每个按钮即该圆点的命中区，
   相邻按钮首尾相接，簇内无死区；簇上下的轨留白处无按钮，点击不响应。
   视觉圆点（8px）居中在按钮内（外观见外壳层 .zk-question-dot / .zk-question-mark） */
.zk-question-dot {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: var(--zk-space-4);
}

/* 视觉圆点尺寸：外观（底色 / 圆角 / 过渡）在 .zk-question-dot / .zk-question-mark 外壳类中定义 */
.zk-question-mark {
  width: var(--zk-space-2);
  height: var(--zk-space-2);
}

/* 展开面板：长列表内部滚动，高度取下拉令牌 */
.rail-panel {
  overflow: hidden;
}

.rail-list {
  max-height: var(--zk-dropdown-max-height);
  overflow-y: auto;
}
</style>
