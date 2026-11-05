<template>
  <!-- 工具调用审批面板：一批多个调用按「一项一步」呈现，与问卷面板同构（标题行兼步骤条、
       窗口逐项展示、页脚翻项），避免多张审批卡片纵向堆叠挤占消息区。
       等待态下可在页脚逐项批准 / 拒绝；历史批次以记录态回显（决策徽章 + 结果），仍可逐项回看 -->
  <div v-if="calls.length > 0" class="assistant-approval-steps zk-panel">
    <!-- 展开区滚动落在文本块自身（.assistant-approval-block），故挂 --inner-scroll 修饰类：
         步骤窗口不再整体滚动，参数 / 结果的标题行与开关始终留在原位 -->
    <v-stepper
      v-model="current"
      editable
      flat
      class="zk-compact-stepper zk-compact-stepper--inner-scroll"
    >
      <div class="d-flex align-center pa-2">
        <v-icon icon="mdi-shield-key-outline" size="small" class="me-2" />
        <span class="zk-label-caps">{{ $t("page.aiAssistant.approvalTitle") }}</span>
        <span v-if="awaiting" class="zk-hint ms-2">
          {{ $t("page.aiAssistant.approvalWaiting") }}
        </span>
        <v-stepper-header v-if="multi">
          <template v-for="(c, ci) in calls" :key="ci">
            <v-divider v-if="ci > 0" />
            <v-stepper-item :value="ci + 1" :complete="isDecided(ci) && current !== ci + 1">
              <template #icon="{ hasCompleted }">
                <v-icon v-if="hasCompleted" icon="mdi-check" />
                <span v-else>{{ ci + 1 }}</span>
              </template>
            </v-stepper-item>
          </template>
        </v-stepper-header>
        <v-spacer v-else />
        <slot name="action" />
      </div>
      <v-stepper-window v-model="current">
        <v-stepper-window-item v-for="(c, ci) in calls" :key="ci" :value="ci + 1">
          <div class="d-flex flex-column ga-2">
            <div class="d-flex align-center">
              <span class="zk-mono-emphasis">{{ c.toolName }}</span>
              <!-- 审核重要级徽章（label 经枚举类 auditLevel 按当前语言渲染） -->
              <v-chip v-if="c.auditLevel" size="x-small" color="info" variant="tonal" class="ms-2">
                {{ enumLabelOf(EnumClass.auditLevel, c.auditLevel) }}
              </v-chip>
              <v-spacer />
              <!-- 免审批调用：自动批准标识，不展示批准 / 拒绝按钮 -->
              <v-chip
                v-if="c.approvalRequired === false"
                size="x-small"
                color="info"
                variant="tonal"
                class="me-2"
              >
                {{ $t("page.aiAssistant.autoApproved") }}
              </v-chip>
              <v-chip v-if="c.decision === true" size="x-small" color="success">
                {{ $t("page.aiAssistant.approved") }}
              </v-chip>
              <v-chip v-else-if="c.decision === false" size="x-small" color="error">
                {{ $t("page.aiAssistant.rejected") }}
              </v-chip>
            </div>
            <!-- 后端执行的调用：前端仅审批，工具由服务端执行，用户无需（也无法）本地执行 -->
            <div v-if="c.execOwner === 'backend'" class="zk-hint">
              {{ $t("page.aiAssistant.approvalBackendNote") }}
            </div>
            <template v-if="c.arguments">
              <!-- 调用参数：待决策时始终展开供审阅；已决策（含历史记录）后默认收起，点击标题行 /
                   箭头再展开，避免审批完成后参数 JSON 长期占用对话空间 -->
              <div
                class="zk-hint d-flex align-center"
                :class="{ 'assistant-approval-section-toggle': !canDecide(c) }"
                :role="!canDecide(c) ? 'button' : undefined"
                :tabindex="!canDecide(c) ? 0 : undefined"
                :aria-expanded="!canDecide(c) ? paramsExpandedOf(ci) : undefined"
                @click="!canDecide(c) && toggleParams(ci)"
                @keydown.enter.prevent="!canDecide(c) && toggleParams(ci)"
                @keydown.space.prevent="!canDecide(c) && toggleParams(ci)"
              >
                <span>{{ $t("page.aiAssistant.approvalParams") }}</span>
                <v-spacer />
                <v-icon
                  v-if="!canDecide(c)"
                  :icon="paramsExpandedOf(ci) ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  size="x-small"
                />
              </div>
              <div
                v-if="canDecide(c) || paramsExpandedOf(ci)"
                class="zk-code-preview font-mono assistant-approval-block"
              >
                {{ c.arguments }}
              </div>
            </template>
            <!-- 调用错误：与参数 / 结果同构，默认收起；错误正文可能带多行 JSON，长文本不撑高面板 -->
            <template v-if="c.error">
              <div
                class="zk-error-text d-flex align-center assistant-approval-section-toggle"
                role="button"
                tabindex="0"
                :aria-expanded="errorExpandedOf(ci)"
                @click="toggleError(ci)"
                @keydown.enter.prevent="toggleError(ci)"
                @keydown.space.prevent="toggleError(ci)"
              >
                <span>{{ $t("page.aiAssistant.approvalError") }}</span>
                <v-spacer />
                <v-icon
                  :icon="errorExpandedOf(ci) ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  size="x-small"
                />
              </div>
              <div
                v-if="errorExpandedOf(ci)"
                class="zk-code-preview font-mono assistant-approval-block"
              >
                {{ c.error }}
              </div>
            </template>
            <!-- 调用结果：审批完成后默认收起（结果 JSON 可能很长），点击标题行 / 箭头展开 -->
            <template v-else-if="c.result">
              <div
                class="zk-hint d-flex align-center assistant-approval-section-toggle"
                role="button"
                tabindex="0"
                :aria-expanded="resultExpandedOf(ci)"
                @click="toggleResult(ci)"
                @keydown.enter.prevent="toggleResult(ci)"
                @keydown.space.prevent="toggleResult(ci)"
              >
                <span>{{ $t("page.aiAssistant.approvalResult") }}</span>
                <v-spacer />
                <v-icon
                  :icon="resultExpandedOf(ci) ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  size="x-small"
                />
              </div>
              <div
                v-if="resultExpandedOf(ci)"
                class="zk-code-preview font-mono assistant-approval-block"
              >
                {{ c.result }}
              </div>
            </template>
          </div>
        </v-stepper-window-item>
      </v-stepper-window>
    </v-stepper>
    <!-- 页脚：决策按钮固定在滚动窗口之外、自成一行——调用参数再长也无需滚到最后即可批准 / 拒绝；
         说明与翻项按钮的节奏与问卷面板一致 -->
    <div v-if="multi || awaiting" class="zk-hairline-top pa-2">
      <div v-if="awaiting" class="zk-hint">
        {{ $t("page.aiAssistant.approvalHint") }}
      </div>
      <!-- 决策行：按当前步骤取目标调用，与参数区的滚动完全分离 -->
      <div v-if="canDecideCurrent" class="d-flex align-center ga-2" :class="{ 'mt-1': awaiting }">
        <v-btn
          size="small"
          variant="tonal"
          color="primary"
          :loading="currentExecuting"
          :disabled="currentExecuting"
          @click="decide(currentCallID, 'approve')"
        >
          {{ $t("page.aiAssistant.approve") }}
        </v-btn>
        <v-btn
          size="small"
          variant="text"
          :disabled="currentExecuting"
          @click="decide(currentCallID, 'reject')"
        >
          {{ $t("page.aiAssistant.reject") }}
        </v-btn>
      </div>
      <!-- 翻项行：多步时提供不产生决策的浏览入口 -->
      <div
        v-if="multi"
        class="d-flex align-center ga-2"
        :class="{ 'mt-1': awaiting || canDecideCurrent }"
      >
        <v-spacer />
        <v-btn v-if="!isFirst" size="small" variant="text" @click="goPrev">
          {{ $t("page.aiAssistant.approvalPrev") }}
        </v-btn>
        <v-btn v-if="!isLast" size="small" variant="tonal" @click="goNext">
          {{ $t("page.aiAssistant.approvalNext") }}
        </v-btn>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" name="assistant-approval-steps">
import type { AgentApprovalCallRecord } from "@/apis/@types/ai";
import { EnumClass, enumLabelOf } from "@/utils/enums";

const props = withDefaults(
  defineProps<{
    /** 本批审批的调用清单（顺序即步骤顺序）。 */
    calls: AgentApprovalCallRecord[];
    /** 本批是否仍处于等待审批态：为 true 时未决策调用可批准 / 拒绝。 */
    interactive?: boolean;
    /** 正在前端执行的调用 ID：按钮据此显示 loading 并禁止重复点击。 */
    executingCallIDs?: string[];
  }>(),
  { interactive: false, executingCallIDs: () => [] },
);

const emits = defineEmits<{
  approve: [callID: string];
  reject: [callID: string];
}>();

// 当前项（1 起，与步骤条取值同源）；多项时才呈现步骤条
const current = ref(1);
const multi = computed(() => props.calls.length > 1);
const isFirst = computed(() => current.value <= 1);
const isLast = computed(() => current.value >= props.calls.length);

// awaiting 本批是否仍有待人工决策的调用：据此展示「等待审批」与动作按钮；免审批与已决策项不算
const awaiting = computed(
  () =>
    props.interactive &&
    props.calls.some((c) => c.approvalRequired !== false && c.decision === undefined),
);

// isDecided 判定某项是否已有结论（免审批视为已自动批准），供步骤条对勾徽标使用
const isDecided = (ci: number): boolean => {
  const call = props.calls[ci];
  return call.approvalRequired === false || call.decision !== undefined;
};

// isExecuting 该项是否正在前端执行（api_invoke）
const isExecuting = (call: AgentApprovalCallRecord): boolean =>
  props.executingCallIDs.includes(call.callID);

// canDecide 该项是否可批准 / 拒绝：仅等待态、需人工审批且尚未决策
const canDecide = (call: AgentApprovalCallRecord): boolean =>
  props.interactive && call.approvalRequired !== false && call.decision === undefined;

// 页脚决策行按当前步骤取目标调用：决策按钮固定在滚动窗口外，参数再长也不会被挤到需滚动的位置
const currentCall = computed<AgentApprovalCallRecord | undefined>(
  () => props.calls[current.value - 1],
);

const canDecideCurrent = computed<boolean>(() => {
  const call = currentCall.value;
  return call !== undefined && canDecide(call);
});

const currentCallID = computed<string>(() => currentCall.value?.callID ?? "");

const currentExecuting = computed<boolean>(() => {
  const call = currentCall.value;
  return call !== undefined && isExecuting(call);
});

// expandedParamSeqs 已展开调用参数的步骤序号集合：已决策项默认收起（步骤序号在组件内稳定，
// 直接按下标记忆），待决策项不受此集合影响、始终展开
const expandedParamSeqs = ref(new Set<number>());

const paramsExpandedOf = (ci: number): boolean => expandedParamSeqs.value.has(ci);

const toggleParams = (ci: number): void => {
  if (expandedParamSeqs.value.has(ci)) {
    expandedParamSeqs.value.delete(ci);
    return;
  }
  expandedParamSeqs.value.add(ci);
};

// expandedErrorSeqs 已展开调用错误的步骤序号集合：错误正文可能含多行 JSON，默认收起
const expandedErrorSeqs = ref(new Set<number>());

const errorExpandedOf = (ci: number): boolean => expandedErrorSeqs.value.has(ci);

const toggleError = (ci: number): void => {
  if (expandedErrorSeqs.value.has(ci)) {
    expandedErrorSeqs.value.delete(ci);
    return;
  }
  expandedErrorSeqs.value.add(ci);
};

// expandedResultSeqs 已展开调用结果的步骤序号集合：结果 JSON 可能很长，默认全部收起
const expandedResultSeqs = ref(new Set<number>());

const resultExpandedOf = (ci: number): boolean => expandedResultSeqs.value.has(ci);

const toggleResult = (ci: number): void => {
  if (expandedResultSeqs.value.has(ci)) {
    expandedResultSeqs.value.delete(ci);
    return;
  }
  expandedResultSeqs.value.add(ci);
};

// decide 记录当前项决策后自动进入下一项：逐项审批时无需再手动点「下一项」；末项不再前进。
// 决策本身由父组件写入 pendingCalls / 审批块，异步执行（api_invoke）的 loading 不阻塞翻项。
const decide = (callID: string, decision: "approve" | "reject"): void => {
  if (decision === "approve") {
    emits("approve", callID);
  } else {
    emits("reject", callID);
  }
  if (!isLast.value) current.value += 1;
};

// 相邻项之间移动：首 / 末项各自隐藏对应按钮，此处再作一次边界保护
const goPrev = (): void => {
  if (!isFirst.value) current.value -= 1;
};

const goNext = (): void => {
  if (!isLast.value) current.value += 1;
};
</script>

<style scoped>
/* 布局定位层：描边 / 圆角 / 底色由 .zk-panel 承载，步骤条尺度复用
   .zk-compact-stepper（头部并入标题行、窗口承载滚动），本块只保留布局。 */

/* 审批面板：宽度撑满父容器（位置由父组件决定），高度由内部步骤窗口封顶 */
.assistant-approval-steps {
  display: flex;
  flex-direction: column;
  width: 100%;
  overflow: hidden;
}

/* 已决策项的「调用参数 / 调用结果」标题行兼展开开关 */
.assistant-approval-section-toggle {
  cursor: pointer;
}

/* 调用参数 / 调用结果 JSON 视区：限高滚动，长响应不撑高气泡，也不让步骤窗口整体滚动。
   本类由本组件模板使用，父组件（AssistantDrawer）的 scoped 样式无法命中，故声明在此 */
.assistant-approval-block {
  max-height: var(--zk-inline-preview-max-height);
  overflow: auto;
}
</style>
