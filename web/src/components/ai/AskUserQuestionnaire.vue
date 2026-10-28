<template>
  <!-- 主动提问问卷面板：一轮内可能合并出多道题，逐题展示题干与候选答案（多题时标题行右侧带
       可点击的序号步骤条）；面板按触发轮次内联于消息流中（见 AssistantDrawer 的 assistantBlocksOf），
       会话阻塞直到提交或取消（输入区同时被整体禁用）。位置与显隐由父组件决定（本组件不自带遮罩），
       仅承载问卷自身的交互与归一化 -->
  <div class="assistant-questionnaire zk-panel">
    <!-- 标题行兼步骤条：标题靠左、序号徽标右对齐，两者共用一行——步骤条不再单独占一条高度。
         滚动与高度上限由窗口承载（外层面板需裁剪溢出）。内边距取 8px（--zk-space-2）与窗口
         外边距同一档，标题图标 / 题干 / 候选项高亮带的左边缘因此都在 8px 上对齐 -->
    <v-stepper v-model="current" editable flat class="zk-compact-stepper">
      <div class="d-flex align-center pa-2">
        <v-icon icon="mdi-comment-question-outline" size="small" class="me-2" />
        <span class="zk-label-caps">{{ $t("page.aiAssistant.questionnaireTitle") }}</span>
        <!-- 步骤条即题间切换入口：步骤可点击（Vuetify 的 editable 才把点击接到选中上，否则按钮
             是 disabled）；单题时不呈现——无进度也无处可切。徽标内容自绘：editable 会把当前题
             徽标换成编辑铅笔，问卷场景里「第几题」比「可编辑」更该被看到，故当前题仍显序号 -->
        <v-stepper-header v-if="multi">
          <template v-for="(q, qi) in payload.questions" :key="q.id">
            <!-- 步骤间连线：步骤条最典型的视觉信号，横向细线不占高度（长度由 .zk-compact-stepper 收窄成短段） -->
            <v-divider v-if="qi > 0" />
            <v-stepper-item :value="qi + 1" :complete="isAnswered(qi) && current !== qi + 1">
              <template #icon="{ hasCompleted }">
                <v-icon v-if="hasCompleted" icon="mdi-check" />
                <span v-else>{{ qi + 1 }}</span>
              </template>
            </v-stepper-item>
          </template>
        </v-stepper-header>
      </div>
      <v-stepper-window v-model="current">
        <v-stepper-window-item v-for="(q, qi) in payload.questions" :key="q.id" :value="qi + 1">
          <div class="d-flex flex-column ga-2">
            <div class="text-body-medium font-weight-medium">{{ q.question }}</div>
            <div class="d-flex flex-column ga-1" role="radiogroup">
              <!-- 候选项：选中态 / 悬停底由外壳类 .zk-row-selected / .zk-row-hover 提供；
                   再次点击已选项即取消选择（清空该题作答） -->
              <div
                v-for="(opt, oi) in q.options"
                :key="oi"
                class="zk-chat-option zk-row-hover py-1 px-2"
                :class="{
                  'zk-row-selected': states[qi].selected === oi,
                  'zk-chat-option--disabled': disabled,
                }"
                role="radio"
                :aria-checked="states[qi].selected === oi"
                :aria-disabled="disabled"
                :tabindex="disabled ? -1 : 0"
                @click="toggleOption(qi, oi)"
                @keydown.enter.prevent="toggleOption(qi, oi)"
                @keydown.space.prevent="toggleOption(qi, oi)"
              >
                <v-icon
                  :icon="states[qi].selected === oi ? 'mdi-radiobox-marked' : 'mdi-radiobox-blank'"
                  size="small"
                  :color="states[qi].selected === oi ? 'primary' : undefined"
                  class="me-2"
                />
                <div class="assistant-questionnaire-option-main">
                  <div class="text-body-medium">{{ opt.text }}</div>
                  <div v-if="opt.detail" class="zk-hint">{{ opt.detail }}</div>
                </div>
              </div>
              <!-- 「其他」：每题恒定附带的自由作答项，选中后展开自定义输入框 -->
              <div
                class="zk-chat-option zk-row-hover py-1 px-2"
                :class="{
                  'zk-row-selected': states[qi].selected === OTHER,
                  'zk-chat-option--disabled': disabled,
                }"
                role="radio"
                :aria-checked="states[qi].selected === OTHER"
                :aria-disabled="disabled"
                :tabindex="disabled ? -1 : 0"
                @click="toggleOther(qi)"
                @keydown.enter.prevent="toggleOther(qi)"
                @keydown.space.prevent="toggleOther(qi)"
              >
                <v-icon
                  :icon="
                    states[qi].selected === OTHER ? 'mdi-radiobox-marked' : 'mdi-radiobox-blank'
                  "
                  size="small"
                  :color="states[qi].selected === OTHER ? 'primary' : undefined"
                  class="me-2"
                />
                <div class="assistant-questionnaire-option-main text-body-medium">
                  {{ $t("page.aiAssistant.questionnaireOther") }}
                </div>
              </div>
            </div>
            <v-text-field
              v-if="states[qi].selected === OTHER"
              v-model="states[qi].custom"
              class="ps-6"
              hide-details
              :disabled="disabled"
              :placeholder="$t('page.aiAssistant.questionnaireCustomPlaceholder')"
            />
          </div>
        </v-stepper-window-item>
      </v-stepper-window>
    </v-stepper>

    <!-- 页脚两行：作答规则说明独占一行（与下方动作行留一个 4px 间距，读作说明而非按钮的一部分），
         动作行右对齐。窄抽屉下多一个「上一题」按钮也不会把说明挤成多行。
         只读态（disabled）下改用说明文本：现场不可用、无法提交作答，引导用户另起新提问 -->
    <div class="zk-hairline-top pa-2">
      <div class="zk-hint">
        {{
          disabled
            ? $t("page.aiAssistant.questionnaireUnavailable")
            : $t("page.aiAssistant.questionnaireUnanswered")
        }}
      </div>
      <div class="d-flex align-center justify-end ga-2 mt-1">
        <!-- 取消回答：退出问卷（以「拒绝」语义回传，模型收到既有拒绝文案） -->
        <v-btn size="small" variant="text" :disabled="disabled" @click="emits('cancel')">
          {{ $t("page.aiAssistant.questionnaireCancel") }}
        </v-btn>
        <v-btn v-if="!isFirst" size="small" variant="text" :disabled="disabled" @click="goPrev">
          {{ $t("page.aiAssistant.questionnairePrev") }}
        </v-btn>
        <!-- 末题才可提交：中间步骤只提供「下一题」，提交时机与进度末尾对齐 -->
        <v-btn v-if="!isLast" size="small" variant="tonal" :disabled="disabled" @click="goNext">
          {{ $t("page.aiAssistant.questionnaireNext") }}
        </v-btn>
        <v-btn
          v-else
          size="small"
          variant="tonal"
          color="primary"
          :disabled="disabled"
          @click="submit"
        >
          {{ $t("page.aiAssistant.questionnaireSubmit") }}
        </v-btn>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" name="ask-user-questionnaire">
import type {
  QuestionnaireAnswer,
  QuestionnairePayload,
  QuestionnaireQuestion,
} from "@/apis/@types/ai";

// 「其他」项的哨兵选中值：候选项用下标（≥0），OTHER 表示选中「其他」，null 表示该题未作答
const OTHER = -1;

// 单题作答状态：selected 为选中项（候选项下标 / OTHER / null），custom 为「其他」的输入文本
interface QuestionState {
  selected: number | null;
  custom: string;
}

const props = defineProps<{
  /** 问卷载荷：题目与可选的上次答案（用于预填）。 */
  payload: QuestionnairePayload;
  /** 是否降级为只读展示（禁用全部作答与提交 / 取消交互）：承载问卷的运行现场不可用时为 true。 */
  disabled?: boolean;
}>();

// 问卷作答结果与取消事件；取消无载荷，始终可用
const emits = defineEmits<{
  submit: [payload: { answers: QuestionnaireAnswer[] }];
  cancel: [];
}>();

// initState 由上次答案初始化单题作答状态：上次答案为「其他」时还原到 OTHER + 输入框，其余按候选项
// 文本回填选中态，未作答（或缺该题答案）回到未选状态
const initState = (q: QuestionnaireQuestion): QuestionState => {
  const ans = props.payload.answers?.find((a) => a.questionID === q.id);
  if (!ans || !ans.answered) return { selected: null, custom: "" };
  if (ans.custom) return { selected: OTHER, custom: ans.answer };
  const idx = q.options.findIndex((o) => o.text === ans.answer);
  // 命中候选项则回填选中；未命中（既非候选项又未标 custom）按「其他」回填，保住自由文本
  return idx >= 0 ? { selected: idx, custom: "" } : { selected: OTHER, custom: ans.answer };
};

const states = ref<QuestionState[]>(props.payload.questions.map(initState));

// 题目可能在面板打开期间继续并入：同一轮内多条 ask_user 调用逐条下发 tool_request，前端把题目
// 累积进同一张问卷，故按其数量补齐状态数组（新增题目恒追加在末尾），避免新并入的题目读到未定义状态
watch(
  () => props.payload.questions.length,
  (total) => {
    for (let i = states.value.length; i < total; i++) {
      states.value.push(initState(props.payload.questions[i]));
    }
  },
);

// 当前题（1 起，与步骤条取值同源）；题目多于一道时才呈现步骤条
const current = ref(1);
const multi = computed(() => props.payload.questions.length > 1);
const isFirst = computed(() => current.value <= 1);
const isLast = computed(() => current.value >= props.payload.questions.length);

// isAnswered 判定某题是否已作答：选候选项即已作答；选「其他」需填了文本才算（与 normalizeAnswers
// 同一判定），供步骤条对勾徽标使用
const isAnswered = (qi: number): boolean => {
  const st = states.value[qi];
  return st.selected !== null && (st.selected !== OTHER || st.custom.trim() !== "");
};

// 相邻题之间移动：首 / 末题各自隐藏对应按钮，此处再作一次边界保护
const goPrev = (): void => {
  if (!isFirst.value) current.value -= 1;
};

const goNext = (): void => {
  if (!isLast.value) current.value += 1;
};

// 切换候选项：再次点击已选项即取消选择（清空该题作答）；只读态下不接受作答
const toggleOption = (qi: number, oi: number): void => {
  if (props.disabled) return;
  const st = states.value[qi];
  st.selected = st.selected === oi ? null : oi;
};

// 切换「其他」：再次点击收起并清空该题作答；只读态下不接受作答
const toggleOther = (qi: number): void => {
  if (props.disabled) return;
  const st = states.value[qi];
  st.selected = st.selected === OTHER ? null : OTHER;
};

// 归一化作答：选候选项取其 text 且 custom=false；选「其他」且填写则 custom=true、answer 为输入
// 文本（未填写按未作答处理）；未作答项 answered=false、answer=""、custom=false
const normalizeAnswers = (): QuestionnaireAnswer[] =>
  props.payload.questions.map((q, qi) => {
    const st = states.value[qi];
    if (st.selected === null) {
      return { questionID: q.id, answered: false, answer: "", custom: false };
    }
    if (st.selected === OTHER) {
      const text = st.custom.trim();
      if (!text) return { questionID: q.id, answered: false, answer: "", custom: false };
      return { questionID: q.id, answered: true, answer: text, custom: true };
    }
    return { questionID: q.id, answered: true, answer: q.options[st.selected].text, custom: false };
  });

// submit 汇总每题作答并上报
const submit = (): void => {
  emits("submit", { answers: normalizeAnswers() });
};
</script>

<style scoped>
/* 布局定位层：描边 / 圆角 / 底色 / 悬停与选中态由 .zk-panel、.zk-row-hover、
   .zk-row-selected、.zk-chat-option 承载；本块只保留布局。 */

/* 问卷面板：宽度撑满父容器（位置由父组件决定），高度由内部滚动区封顶，不随视口伸缩 */
.assistant-questionnaire {
  display: flex;
  flex-direction: column;
  width: 100%;
  overflow: hidden;
}

/* 候选项行：「其他」与候选项共用；文字与单选图标顶端对齐 */
.zk-chat-option {
  display: flex;
  align-items: flex-start;
  gap: var(--zk-space-2);
}

/* 候选项文本块：占满行内剩余宽度，长文案可换行 */
.assistant-questionnaire-option-main {
  flex: 1 1 auto;
  min-width: 0;
}
</style>
