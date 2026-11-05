<template>
  <app-dialog
    v-model="opened"
    :title="$t('page.periodicReportPlan.viewSession')"
    :subtitle="subtitle"
    icon="mdi-clipboard-text-clock-outline"
    size="xl"
    :loading="loading"
  >
    <div class="d-flex flex-column ga-4">
      <v-alert type="info" variant="tonal" density="compact">
        {{ $t("page.periodicReportPlan.sessionReadonlyHint") }}
      </v-alert>

      <div v-if="!loading && messages.length === 0" class="zk-hint">
        {{ $t("page.periodicReportPlan.sessionEmpty") }}
      </div>

      <section v-for="message in messages" :key="message.id" class="d-flex flex-column ga-2">
        <div class="d-flex align-center ga-2">
          <v-icon
            :icon="message.role === 'user' ? 'mdi-account-outline' : 'mdi-robot-outline'"
            :color="message.role === 'user' ? undefined : 'primary'"
            size="small"
          />
          <span class="zk-label-caps">
            {{
              message.role === "user"
                ? $t("page.periodicReportPlan.sessionUserLabel")
                : $t("page.periodicReportPlan.sessionAssistantLabel")
            }}
          </span>
          <span class="zk-hint">
            {{ localeDatetimeFormat(DateTime.fromISO(message.createdAt)) }}
          </span>
        </div>

        <assistant-markdown v-if="message.role === 'user'" :content="message.content" />

        <template v-else>
          <div
            v-for="block in message.blocks ?? []"
            :key="block.seq"
            class="d-flex flex-column ga-2"
          >
            <v-expansion-panels
              v-if="block.kind === 'reasoning'"
              variant="accordion"
              flat
              class="report-session-panel"
            >
              <v-expansion-panel>
                <v-expansion-panel-title class="zk-label-caps">
                  {{ $t("page.aiAssistant.thinking") }}
                </v-expansion-panel-title>
                <v-expansion-panel-text>
                  <pre class="report-session-pre">{{ reasoningText(block) }}</pre>
                </v-expansion-panel-text>
              </v-expansion-panel>
            </v-expansion-panels>

            <assistant-markdown
              v-else-if="block.kind === 'content'"
              :content="contentText(block)"
            />

            <div v-else-if="block.kind === 'tool_calls'" class="d-flex flex-column ga-2">
              <span class="zk-label-caps">
                {{ $t("page.periodicReportPlan.sessionToolCalls") }}
              </span>
              <div
                v-for="call in toolCallsOf(block)"
                :key="call.callID"
                class="zk-panel--subtle d-flex flex-column ga-1 pa-2"
              >
                <div class="d-flex align-center ga-2 flex-wrap">
                  <code class="zk-section-title">{{ call.toolName }}</code>
                  <v-chip size="x-small" variant="tonal">{{ toolStatusLabel(call.status) }}</v-chip>
                  <span v-if="call.error" class="zk-error-text">{{ call.error }}</span>
                </div>
                <div class="zk-hint">{{ $t("page.periodicReportPlan.sessionArguments") }}</div>
                <pre class="report-session-pre">{{ prettyJSON(call.arguments) }}</pre>
                <template v-if="call.result">
                  <div class="zk-hint">{{ $t("page.periodicReportPlan.sessionResult") }}</div>
                  <pre class="report-session-pre">{{ prettyJSON(call.result) }}</pre>
                </template>
              </div>
            </div>

            <div v-else-if="block.kind === 'approval'" class="d-flex flex-column ga-2">
              <span class="zk-label-caps">{{ $t("page.aiAssistant.approvalTitle") }}</span>
              <div
                v-for="call in approvalCallsOf(block)"
                :key="call.callID"
                class="zk-panel--subtle d-flex flex-column ga-1 pa-2"
              >
                <div class="d-flex align-center ga-2 flex-wrap">
                  <code class="zk-section-title">{{ call.toolName }}</code>
                  <v-chip size="x-small" variant="tonal">{{ decisionLabel(call.decision) }}</v-chip>
                </div>
                <pre class="report-session-pre">{{ prettyJSON(call.arguments) }}</pre>
                <template v-if="call.error">
                  <span class="zk-error-text">{{ call.error }}</span>
                </template>
                <template v-else-if="call.result">
                  <div class="zk-hint">{{ $t("page.periodicReportPlan.sessionResult") }}</div>
                  <pre class="report-session-pre">{{ prettyJSON(call.result) }}</pre>
                </template>
              </div>
            </div>

            <div v-else-if="block.kind === 'questionnaire'" class="d-flex flex-column ga-2">
              <span class="zk-label-caps">{{
                $t("page.periodicReportPlan.sessionQuestionnaire")
              }}</span>
              <div
                v-for="question in questionnaireQuestionsOf(block)"
                :key="question.id"
                class="zk-panel--subtle d-flex flex-column ga-1 pa-2"
              >
                <span class="text-body-medium">{{ question.question }}</span>
                <span class="zk-hint">{{ answerOf(block, question.id) }}</span>
              </div>
            </div>

            <div v-else-if="block.kind === 'next_steps'" class="d-flex flex-column ga-2">
              <span class="zk-label-caps">{{
                $t("page.periodicReportPlan.sessionNextSteps")
              }}</span>
              <div v-for="(step, idx) in nextStepsOf(block)" :key="idx" class="pl-2">
                <span class="text-body-medium">{{ step.title || step.text }}</span>
                <span v-if="step.reason" class="zk-hint"> — {{ step.reason }}</span>
              </div>
            </div>
          </div>

          <assistant-markdown
            v-if="!(message.blocks?.length ?? 0) && message.content"
            :content="message.content"
          />
        </template>
      </section>
    </div>

    <template #footer>
      <v-btn :text="$t('action.close')" variant="text" @click="opened = false" />
    </template>
  </app-dialog>
</template>

<script setup lang="ts" name="report-session-viewer">
import { DateTime } from "luxon";

import { assistantRenderStream } from "@/apis/ai";
import type { AssistantEventUnion, AssistantMessageSnapshot } from "@/apis/@types/assistant";
import type {
  AgentMessageBlockOut,
  AgentReasoningBlockPayload,
  AgentContentBlockPayload,
  AgentToolCallsBlockPayload,
  AgentToolCallRecord,
  AgentApprovalBlockPayload,
  AgentApprovalCallRecord,
  AgentQuestionnaireBlockPayload,
  QuestionnaireQuestion,
  AgentNextStepsBlockPayload,
  AgentNextStep,
} from "@/apis/@types/ai";
import { useAppStore } from "@/stores/app";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

const props = defineProps<{
  // modelValue 对话框开关，配合 v-model 使用
  modelValue: boolean;
  // sessionId 本次周期报告运行的独立会话 ID
  sessionId: number;
  // subtitle 对话框副标题（通常为运行记录标题）
  subtitle?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const appStore = useAppStore();

// opened 对话框开关的双向绑定
const opened = computed<boolean>({
  get: () => props.modelValue,
  set: (value) => emit("update:modelValue", value),
});

// messages 历史快照中的消息（用户提问 + 助手回答及其内容块），仅只读展示
const messages = ref<AssistantMessageSnapshot[]>([]);
// loading 是否正在加载历史快照
const loading = ref(false);
// abort 当前 render 流的取消函数
let abort: (() => void) | null = null;

// release 取消当前渲染流，避免对话框关闭后仍保留连接
const release = (): void => {
  if (abort) {
    abort();
    abort = null;
  }
};

// load 通过 /render 读取会话历史快照：运行已结束后仅有 snapshot 事件，无需处理实时块事件
const load = (): void => {
  release();
  messages.value = [];
  if (!props.modelValue || props.sessionId <= 0) {
    loading.value = false;
    return;
  }
  loading.value = true;
  abort = assistantRenderStream(props.sessionId, {
    onEvent: (event: AssistantEventUnion) => {
      switch (event.type) {
        case "snapshot.begin":
          messages.value = [];
          return;
        case "snapshot.message":
          messages.value.push(event.payload.message);
          return;
        case "snapshot.end":
          loading.value = false;
          return;
        default:
          return;
      }
    },
    onError: (text) => {
      loading.value = false;
      appStore.notify({ text, timeout: 3000, color: "error" });
    },
    onComplete: () => {
      loading.value = false;
    },
  });
};

watch(
  () => [props.modelValue, props.sessionId] as const,
  ([visible]) => {
    if (visible) {
      load();
      return;
    }
    release();
  },
  { immediate: true },
);

onUnmounted(release);

// prettyJSON 把工具参数 / 结果格式化为缩进 JSON；无法解析时原样展示
const prettyJSON = (raw?: string): string => {
  if (!raw) {
    return "";
  }
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
};

// reasoningText 读取 reasoning 块的终态文本
const reasoningText = (block: AgentMessageBlockOut): string =>
  (block.payload as AgentReasoningBlockPayload)?.text ?? "";

// contentText 读取 content 块的终态正文
const contentText = (block: AgentMessageBlockOut): string =>
  (block.payload as AgentContentBlockPayload)?.text ?? "";

// toolCallsOf 读取工具调用组的调用记录
const toolCallsOf = (block: AgentMessageBlockOut): AgentToolCallRecord[] =>
  (block.payload as AgentToolCallsBlockPayload)?.calls ?? [];

// approvalCallsOf 读取审批块的调用记录
const approvalCallsOf = (block: AgentMessageBlockOut): AgentApprovalCallRecord[] =>
  (block.payload as AgentApprovalBlockPayload)?.calls ?? [];

// questionnaireQuestionsOf 读取问卷块的题目
const questionnaireQuestionsOf = (block: AgentMessageBlockOut): QuestionnaireQuestion[] =>
  (block.payload as AgentQuestionnaireBlockPayload)?.questions ?? [];

// nextStepsOf 读取推荐下一步
const nextStepsOf = (block: AgentMessageBlockOut): AgentNextStep[] =>
  (block.payload as AgentNextStepsBlockPayload)?.steps ?? [];

// answerOf 读取指定题目的作答文本；未作答时展示占位文案
const answerOf = (block: AgentMessageBlockOut, questionID: string): string => {
  const payload = block.payload as AgentQuestionnaireBlockPayload;
  const answer = payload?.answers?.find((item) => item.questionID === questionID);
  if (!answer || !answer.answered) {
    return i18n.global.t("page.aiAssistant.questionnaireNoAnswer");
  }
  return answer.answer;
};

// toolStatusLabel 工具调用状态的只读文案
const toolStatusLabel = (status: AgentToolCallRecord["status"]): string => {
  switch (status) {
    case "done":
      return i18n.global.t("page.periodicReportPlan.sessionToolDone");
    case "failed":
      return i18n.global.t("page.periodicReportPlan.sessionToolFailed");
    default:
      return i18n.global.t("page.periodicReportPlan.sessionToolRunning");
  }
};

// decisionLabel 审批决策的只读文案
const decisionLabel = (decision?: boolean): string => {
  if (decision === true) {
    return i18n.global.t("page.periodicReportPlan.sessionApproved");
  }
  if (decision === false) {
    return i18n.global.t("page.periodicReportPlan.sessionDeclined");
  }
  return i18n.global.t("page.periodicReportPlan.sessionPendingDecision");
};
</script>

<style scoped>
.report-session-panel {
  background: transparent;
}

.report-session-pre {
  margin: 0;
  padding: 8px;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--zk-font-mono, monospace);
  font-size: 12px;
  line-height: 1.5;
  border-radius: 4px;
  background: rgba(var(--v-theme-on-surface), 0.04);
}
</style>
