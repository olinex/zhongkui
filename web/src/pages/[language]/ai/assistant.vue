<template>
  <main-layout current-nav="ai">
    <template #body>
      <v-row class="assistant-row" no-gutters>
        <!-- 左栏：会话列表 -->
        <v-col cols="12" md="3" class="h-100">
          <v-card class="d-flex flex-column session-card h-100">
            <v-card-title class="d-flex align-center px-3">
              <span class="flex-grow-1 text-subtitle-1 font-weight-bold">
                {{ $t("model.aiSession.name") }}
              </span>
              <v-btn
                icon="mdi-plus"
                variant="tonal"
                color="primary"
                size="small"
                :disabled="!aiEnabled"
                :title="$t('page.aiAssistant.newSession')"
                :aria-label="$t('page.aiAssistant.newSession')"
                @click="openCreateSessionDialog"
              />
            </v-card-title>
            <v-divider />
            <div class="session-list flex-grow-1 overflow-y-auto">
              <v-list nav>
                <v-list-item
                  v-for="s in sessions"
                  :key="s.id"
                  :active="activeSessionId === s.id"
                  color="primary"
                  variant="tonal"
                  class="mx-2 my-1 rounded-lg"
                  @click="selectSession(s.id)"
                >
                  <v-list-item-title class="text-body-2 text-on-surface">
                    {{ s.title }}
                  </v-list-item-title>
                  <v-list-item-subtitle class="text-caption text-on-surface-variant">
                    {{ localeDatetimeFormat(DateTime.fromISO(s.updatedAt)) }}
                  </v-list-item-subtitle>
                </v-list-item>
              </v-list>
            </div>
          </v-card>
        </v-col>

        <!-- 右栏：对话区 -->
        <v-col cols="12" md="9" class="h-100">
          <v-card class="d-flex flex-column chat-card h-100">
            <v-card-title class="d-flex align-center px-4">
              <span class="text-truncate flex-grow-1 text-subtitle-1 font-weight-bold">
                {{ currentSessionTitle }}
              </span>
              <v-btn
                icon="mdi-pencil-outline"
                variant="tonal"
                size="small"
                :disabled="!activeSessionId"
                :title="$t('page.aiAssistant.renameSession')"
                :aria-label="$t('page.aiAssistant.renameSession')"
                @click="openRenameSessionDialog"
              />
              <v-btn
                icon="mdi-delete-outline"
                variant="tonal"
                color="error"
                size="small"
                class="ms-2"
                :disabled="!activeSessionId"
                :title="$t('page.aiAssistant.deleteSession')"
                :aria-label="$t('page.aiAssistant.deleteSession')"
                @click="deleteConfirmRef?.open({})"
              />
            </v-card-title>
            <v-divider />
            <!-- 消息区 -->
            <div ref="messagesRef" class="chat-messages flex-grow-1 overflow-y-auto pa-4">
              <div
                v-if="!activeSessionId"
                class="chat-empty d-flex flex-column align-center justify-center h-100"
              >
                <v-icon icon="mdi-robot-outline" size="56" class="text-on-surface-variant" />
                <div class="text-caption text-on-surface-variant mt-3">
                  {{ $t("page.aiAssistant.hintSelectSession") }}
                </div>
              </div>
              <template v-else>
                <div
                  v-for="m in messages"
                  :key="m.key"
                  class="d-flex mb-4"
                  :class="m.role === 'user' ? 'justify-end' : 'justify-start'"
                >
                  <div
                    class="message-bubble zk-chat-bubble"
                    :class="
                      m.role === 'user'
                        ? 'bg-primary text-on-primary zk-chat-bubble--user'
                        : 'bg-surface-variant text-on-surface zk-chat-bubble--assistant'
                    "
                  >
                    <div class="d-flex align-center mb-1">
                      <span class="text-caption text-on-surface-variant">
                        {{
                          $t(
                            m.role === "user"
                              ? "page.aiAssistant.me"
                              : "page.aiAssistant.assistant",
                          )
                        }}
                      </span>
                      <v-spacer />
                      <span v-if="m.streaming" class="text-caption text-on-surface-variant">
                        {{ $t("page.aiAssistant.generating") }}
                      </span>
                      <v-chip v-else-if="m.valid === true" size="x-small" color="success">
                        {{ $t("page.aiAssistant.validationPassed") }}
                      </v-chip>
                      <v-chip v-else-if="m.valid === false" size="x-small" color="error">
                        {{ $t("page.aiAssistant.validationFailed") }}
                      </v-chip>
                    </div>
                    <!-- 工具调用阶段：与检索试运行的阶段列表同源展示（图标 + 阶段名 + 工具名 + 耗时/失败原因） -->
                    <div v-if="m.stages && m.stages.length > 0" class="stage-list">
                      <div v-for="s in m.stages" :key="s.key" class="stage-row">
                        <v-icon
                          :icon="stageIconOf(s, m.streaming === true)"
                          :color="stageColorOf(s, m.streaming === true)"
                          size="small"
                          :class="{ 'mdi-spin': m.streaming === true && s.status === 'start' }"
                        />
                        <span class="stage-name text-body-2 text-on-surface">
                          {{ stageLabelOf(s.stage) }}
                        </span>
                        <span v-if="s.tool" class="text-caption text-on-surface-variant font-mono">
                          {{ s.tool }}
                        </span>
                        <v-spacer />
                        <span
                          v-if="s.elapsedMs > 0 || s.error"
                          class="text-caption text-on-surface-variant"
                        >
                          {{ stageMetaTextOf(s) }}
                        </span>
                      </div>
                    </div>
                    <div v-if="m.content" class="message-content text-body-2">{{ m.content }}</div>
                    <div
                      v-if="m.validationDetail"
                      class="text-caption text-on-surface-variant mt-1"
                    >
                      {{ m.validationDetail }}
                    </div>
                    <template v-if="citationsOf(m).length > 0">
                      <v-divider class="my-2" />
                      <div class="text-caption text-on-surface-variant mb-1">
                        {{ $t("page.aiAssistant.citations") }}
                      </div>
                      <v-list class="citation-list bg-transparent">
                        <v-list-item v-for="(c, idx) in citationsOf(m)" :key="idx">
                          <v-list-item-title class="text-body-2 text-on-surface">
                            {{ c.title }}
                          </v-list-item-title>
                          <v-list-item-subtitle class="text-caption text-on-surface-variant">
                            {{ c.content }}
                          </v-list-item-subtitle>
                        </v-list-item>
                      </v-list>
                    </template>
                    <template v-if="hasDraft(m)">
                      <v-divider class="my-2" />
                      <div class="d-flex align-center mb-1">
                        <span class="text-caption text-on-surface-variant">
                          {{ $t("page.aiAssistant.draft") }}
                        </span>
                        <v-spacer />
                        <v-btn variant="text" size="x-small" color="primary" @click="copyDraft(m)">
                          {{ $t("page.aiAssistant.copyDraft") }}
                        </v-btn>
                      </div>
                      <v-sheet rounded="md" class="draft-sheet">
                        <VueJsonPretty :data="m.draft" />
                      </v-sheet>
                    </template>
                  </div>
                </div>
              </template>
            </div>
            <v-divider />
            <!-- 输入区 -->
            <div class="chat-composer pa-4">
              <v-alert v-if="aiStatus && !aiStatus.enabled" type="warning" class="mb-2">
                {{ aiStatus.reason || $t("page.aiAssistant.disabled") }}
              </v-alert>
              <v-form>
                <v-select
                  v-model="slot"
                  :items="slotOptions"
                  :label="$t('model.aiSession.field.slot')"
                  :hint="$t('page.aiAssistant.slotHint')"
                  persistent-hint
                  class="mb-2"
                />
                <v-textarea
                  v-model="query"
                  :label="$t('model.aiSession.field.query')"
                  rows="2"
                  auto-grow
                  hide-details
                  @keydown.enter.exact.prevent="!streaming && activeSessionId && sendQuery()"
                />
                <v-select
                  v-model="docIDs"
                  :items="docOptions"
                  item-title="title"
                  item-value="value"
                  :label="$t('model.aiSession.field.docIDs')"
                  multiple
                  clearable
                  chips
                  class="mt-2"
                />
                <v-textarea
                  v-model="businessData"
                  :label="$t('model.aiSession.field.businessData')"
                  rows="2"
                  auto-grow
                  class="mt-2"
                  :hint="$t('page.aiAssistant.businessDataHint')"
                  persistent-hint
                />
                <v-textarea
                  v-model="regoRule"
                  :label="$t('model.aiSession.field.regoRule')"
                  rows="1"
                  auto-grow
                  class="mt-2 font-mono"
                />
                <div class="d-flex align-center justify-space-between mt-2">
                  <v-btn variant="text" size="small" color="primary" @click="openTagRulePage">
                    {{ $t("page.aiAssistant.openTagRulePage") }}
                  </v-btn>
                  <div>
                    <v-btn v-if="streaming" color="error" variant="tonal" @click="stopGenerating">
                      {{ $t("page.aiAssistant.stop") }}
                    </v-btn>
                    <v-btn
                      v-else
                      color="primary"
                      :disabled="!activeSessionId || !aiEnabled"
                      @click="sendQuery"
                    >
                      {{ $t("page.aiAssistant.generate") }}
                    </v-btn>
                  </div>
                </div>
              </v-form>
            </div>
          </v-card>
        </v-col>
      </v-row>

      <!-- 新建会话对话框 -->
      <temp-editable-dialog
        ref="createSessionDialogRef"
        :title="$t('model.aiSession.name')"
        :confirm-disabled="!createSessionForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-plus"
        @confirm="createSessionAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="createSessionTitle"
                  v-bind="createSessionTitleProps"
                  :label="$t('model.aiSession.field.title')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 重命名会话对话框 -->
      <temp-editable-dialog
        ref="renameSessionDialogRef"
        :title="$t('page.aiAssistant.renameSession')"
        :confirm-disabled="!renameSessionForm.meta.value.valid"
        force-editable
        prepend-icon="mdi-pencil-outline"
        @confirm="renameSessionAction"
      >
        <template #body>
          <v-form>
            <v-row dense>
              <v-col cols="12">
                <v-text-field
                  v-model="renameSessionTitle"
                  v-bind="renameSessionTitleProps"
                  :label="$t('model.aiSession.field.title')"
                />
              </v-col>
            </v-row>
          </v-form>
        </template>
      </temp-editable-dialog>

      <!-- 删除会话确认对话框 -->
      <confirm-dialog
        ref="deleteConfirmRef"
        :title="$t('page.aiAssistant.deleteSession')"
        prepend-icon="mdi-delete-outline"
        @confirm="deleteSessionAction"
      >
        <template #body>
          <confirm-alert
            :action="$t('action.delete')"
            :text="$t('page.aiAssistant.confirmDeleteSession')"
          />
        </template>
      </confirm-dialog>
    </template>
  </main-layout>
</template>

<route lang="yaml">
name: AI Assistant
meta:
  title: title.aiAssistant
  requiresAuth: true
  requiresPerm: true
</route>

<script setup lang="ts" name="ai-assistant">
import { DateTime } from "luxon";
import * as yup from "yup";
import { useForm } from "vee-validate";
import VueJsonPretty from "vue-json-pretty";
import "vue-json-pretty/lib/styles.css";

import {
  agentMessageListGet,
  agentSessionCreate,
  agentSessionDelete,
  agentSessionListGet,
  agentSessionRename,
  agentStatusGet,
  agentStreamGenerate,
  knowledgeDocumentListGet,
} from "@/apis/ai";
import type {
  AgentMessageItem,
  AgentSessionItem,
  AgentStageEvent,
  AgentStatusOut,
  EvalStage,
  EvalStageStatus,
  KnowledgeDocumentItem,
  RetrievalResult,
} from "@/apis/@types/ai";
import { getResponse, getSimpleResponse } from "@/apis/base";
import type { SearchOut } from "@/components/@types/table";
import { useAppStore } from "@/stores/app";
import { copyToClipboard } from "@/utils/clipboard";
import { EnumClass, enumLabelOf, ensureEnum } from "@/utils/enums";
import { vuetifyCfg } from "@/utils/form";
import i18n, { localeDatetimeFormat } from "@/utils/i18n";

const appStore = useAppStore();
const router = useRouter();
const route = useRoute();

// ---- 会话表单校验 schema ----
const sessionSchema = yup.object({
  title: yup
    .string()
    .trim()
    .nonNullable()
    .required()
    .min(1)
    .max(255)
    .label("model.aiSession.field.title"),
});

const createSessionForm = useForm({ validationSchema: sessionSchema });
const renameSessionForm = useForm({ validationSchema: sessionSchema });

const [createSessionTitle, createSessionTitleProps] = createSessionForm.defineField(
  "title",
  vuetifyCfg,
);
const [renameSessionTitle, renameSessionTitleProps] = renameSessionForm.defineField(
  "title",
  vuetifyCfg,
);

const createSessionDialogRef = useTemplateRef("createSessionDialogRef");
const renameSessionDialogRef = useTemplateRef("renameSessionDialogRef");
const deleteConfirmRef = useTemplateRef("deleteConfirmRef");
const messagesRef = useTemplateRef("messagesRef");

// ---- 会话与消息状态 ----
const sessions = ref<AgentSessionItem[]>([]);
const activeSessionId = ref<number | null>(null);

// AgentStageState 工具调用阶段视图模型：一次生成中每个「阶段 + 工具名」组合一行，
// 后续同键状态事件仅更新该行（与检索试运行的阶段列表同构）
interface AgentStageState {
  key: string;
  stage: EvalStage;
  tool: string;
  status?: EvalStageStatus;
  elapsedMs: number;
  error?: string;
}

// ChatMessage 会话消息视图模型：会话历史消息与流式占位消息统一形态。
// draft/citations 由 done 事件回填；valid/validationDetail 仅在有校验规则时才有意义；
// stages 为本次生成实时累积的工具调用阶段（历史消息不携带，后端不持久化阶段）
interface ChatMessage {
  key: number;
  role: "user" | "assistant" | "system";
  content: string;
  draft?: Record<string, unknown>;
  citations?: RetrievalResult[];
  valid?: boolean;
  validationDetail?: string;
  streaming?: boolean;
  stages?: AgentStageState[];
}

let messageKey = 0;
const nextKey = (): number => ++messageKey;

const messages = ref<ChatMessage[]>([]);

// ---- AI 状态与档位 ----
const aiStatus = ref<AgentStatusOut | null>(null);
// slot 当前档位：初值留空，由状态接口下发的后端默认档位（参数 agent.defaultSlot）填充，
// 前端不声明档位字面量，避免与后端配置漂移
const slot = ref("");

// aiEnabled AI 能力是否可用；状态未加载完成时不禁用操作，由后端错误兜底
const aiEnabled = computed((): boolean => (aiStatus.value ? aiStatus.value.enabled : true));

// slotOptions 档位下拉选项：仅展示对话档位，不可用档位禁选
const slotOptions = computed(() => {
  const chatSlots = (aiStatus.value?.slots ?? []).filter((s) => s.kind === "chat");
  return chatSlots.map((s) => ({
    title: s.modelName
      ? `${enumLabelOf(EnumClass.llmSlot, s.slot)} · ${s.modelName}`
      : enumLabelOf(EnumClass.llmSlot, s.slot),
    value: s.slot,
    props: { disabled: !s.available },
  }));
});

// loadStatus 拉取 AI 状态：驱动档位下拉选项与未启用原因展示
const loadStatus = async (): Promise<void> => {
  await getResponse<AgentStatusOut>(
    agentStatusGet(),
    async (rsp) => {
      aiStatus.value = rsp.data;
      // 未选择档位时取后端下发的默认档位（参数 agent.defaultSlot）
      if (!slot.value) {
        slot.value = rsp.data.defaultSlot;
      }
      // 当前选中档位不可用时回落到首个可用对话档位
      const available = (rsp.data.slots ?? []).filter((s) => s.kind === "chat" && s.available);
      const current = (rsp.data.slots ?? []).find((s) => s.slot === slot.value);
      if (available.length > 0 && current?.available !== true) {
        slot.value = available[0].slot;
      }
    },
    async () => {
      aiStatus.value = null;
    },
  );
};

// asRecord 将未知载荷归一化为 JSON 对象（null/数组/非对象返回 undefined）
const asRecord = (v: unknown): Record<string, unknown> | undefined =>
  v !== null && v !== undefined && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : undefined;

const currentSessionTitle = computed((): string => {
  const s = sessions.value.find((x) => x.id === activeSessionId.value);
  return s?.title ?? "";
});

// citationsOf 归一化引用列表（后端消息可能省略该字段）
const citationsOf = (m: ChatMessage): RetrievalResult[] => m.citations ?? [];

// hasDraft 判断消息是否携带非空结构化草稿（JSON 对象）
const hasDraft = (m: ChatMessage): boolean =>
  m.draft !== undefined && Object.keys(m.draft).length > 0;

// copyDraft 复制结构化草稿 JSON 文本到剪贴板
const copyDraft = async (m: ChatMessage): Promise<void> => {
  await copyToClipboard(JSON.stringify(m.draft, null, 2));
};

// ---- 工具调用阶段 ----
// stageLabelOf 阶段展示名（经枚举注册表 evalStage 按当前语言渲染 label，与检索试运行同源）
const stageLabelOf = (stage: EvalStage): string => enumLabelOf(EnumClass.evalStage, stage);

// stageIconOf 阶段状态图标：运行中旋转、完成打勾、失败告警；流已结束仍在进行的阶段展示为已中止
const stageIconOf = (stage: AgentStageState, running: boolean): string => {
  if (stage.status === "done") {
    return "mdi-check-circle-outline";
  }
  if (stage.status === "failed") {
    return "mdi-alert-circle-outline";
  }
  return running ? "mdi-loading" : "mdi-stop-circle-outline";
};

// stageColorOf 阶段状态语义色
const stageColorOf = (stage: AgentStageState, running: boolean): string => {
  if (stage.status === "done") {
    return "success";
  }
  if (stage.status === "failed") {
    return "warning";
  }
  return running ? "primary" : "warning";
};

// stageMetaTextOf 阶段元信息：耗时与失败原因
const stageMetaTextOf = (stage: AgentStageState): string => {
  const parts: string[] = [];
  if (stage.elapsedMs > 0) {
    parts.push(`${stage.elapsedMs} ms`);
  }
  if (stage.error) {
    parts.push(`${i18n.global.t("page.aiAssistant.stageFailed")}: ${stage.error}`);
  }
  return parts.join(" · ");
};

// applyAgentStage 应用一次工具调用阶段事件：按「阶段 + 工具名」归并到同一行
// （同一工具的多轮调用各自成行），后续同键事件仅更新状态/耗时/失败原因
const applyAgentStage = (key: number, ev: AgentStageEvent): void => {
  const msg = messages.value.find((m) => m.key === key);
  if (!msg) {
    return;
  }
  const stages = (msg.stages ??= []);
  const stageKey = `${ev.stage}:${ev.tool ?? ""}`;
  const target = stages.find((s) => s.key === stageKey);
  if (target) {
    target.status = ev.status ?? target.status;
    if (ev.elapsedMs > 0) {
      target.elapsedMs = ev.elapsedMs;
    }
    if (ev.error) {
      target.error = ev.error;
    }
  } else {
    stages.push({
      key: stageKey,
      stage: ev.stage,
      tool: ev.tool ?? "",
      status: ev.status,
      elapsedMs: ev.elapsedMs,
      error: ev.error,
    });
  }
  scrollToBottom();
};

// ---- 会话加载 ----
const loadSessions = async (): Promise<void> => {
  await getResponse<AgentSessionItem[]>(
    agentSessionListGet(),
    async (rsp) => {
      sessions.value = rsp.data ?? [];
      // 当前选中会话若已被归档/删除则清空选中态
      if (
        activeSessionId.value !== null &&
        !sessions.value.some((s) => s.id === activeSessionId.value)
      ) {
        activeSessionId.value = null;
        messages.value = [];
      }
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

const loadMessages = async (id: number): Promise<void> => {
  await getResponse<AgentMessageItem[]>(
    agentMessageListGet(id),
    async (rsp) => {
      messages.value = (rsp.data ?? []).map((m) => ({
        key: nextKey(),
        role: m.role as ChatMessage["role"],
        content: m.content ?? "",
        draft: asRecord(m.draft),
        citations: m.citations,
      }));
      scrollToBottom();
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

const selectSession = async (id: number): Promise<void> => {
  stopGenerating();
  activeSessionId.value = id;
  messages.value = [];
  // 会话记录了缺省档位时同步到档位下拉；未记录时保持当前选择
  const target = sessions.value.find((s) => s.id === id);
  if (target && target.slot) {
    slot.value = target.slot;
  }
  await loadMessages(id);
};

const scrollToBottom = (): void => {
  void nextTick(() => {
    const el = messagesRef.value;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  });
};

// ---- 会话增删改 ----
const openCreateSessionDialog = (): void => {
  createSessionForm.resetField("title", { value: "" });
  createSessionDialogRef.value?.open({});
};

const createSessionAction = createSessionForm.handleSubmit(async (values) => {
  await getResponse<AgentSessionItem>(
    agentSessionCreate({
      // 会话标题为单行文本，提交前去除首尾空白
      title: (values.title as string).trim(),
      // 会话缺省档位随当前档位下拉传入，作为后续生成的回退档位
      slot: slot.value || undefined,
    }),
    async (rsp) => {
      sessions.value = [rsp.data, ...sessions.value];
      await selectSession(rsp.data.id);
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
});

const openRenameSessionDialog = (): void => {
  if (activeSessionId.value === null) {
    return;
  }
  renameSessionForm.resetField("title", { value: currentSessionTitle.value });
  renameSessionDialogRef.value?.open({});
};

const renameSessionAction = renameSessionForm.handleSubmit(async (values) => {
  if (activeSessionId.value === null) {
    return;
  }
  await getSimpleResponse(
    agentSessionRename(activeSessionId.value, {
      id: activeSessionId.value,
      title: (values.title as string).trim(),
    }),
    async (rsp) => {
      await loadSessions();
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
});

const deleteSessionAction = async (): Promise<void> => {
  if (activeSessionId.value === null) {
    return;
  }
  const id = activeSessionId.value;
  stopGenerating();
  await getSimpleResponse(
    agentSessionDelete(id),
    async (rsp) => {
      sessions.value = sessions.value.filter((s) => s.id !== id);
      if (activeSessionId.value === id) {
        activeSessionId.value = null;
        messages.value = [];
      }
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "success",
      });
    },
    async (rsp) => {
      appStore.notify({
        text: rsp.summary,
        timeout: 3000,
        color: "error",
      });
    },
  );
};

// ---- 知识库文档选项（检索限定下拉）----

const docOptions = ref<{ title: string; value: number }[]>([]);

const loadDocOptions = async (): Promise<void> => {
  await getResponse<SearchOut<KnowledgeDocumentItem>>(
    knowledgeDocumentListGet({ greedy: false, page: 1, pageSize: 100 }),
    async (rsp) => {
      docOptions.value = (rsp.data.items ?? []).map((d) => ({
        title: d.title,
        value: d.id,
      }));
    },
    async () => {
      docOptions.value = [];
    },
  );
};

// ---- 流式问答 ----
const query = ref("");
const docIDs = ref<number[]>([]);
const businessData = ref("");
const regoRule = ref("");
const streaming = ref(false);
let abortStream: (() => void) | null = null;

// sendQuery 发起一次流式问答：先校验输入，再推送用户消息与流式占位消息，
// SSE 增量经 onDelta 累积，done 事件回填内容/引用/草稿与校验结果。
const sendQuery = async (): Promise<void> => {
  if (streaming.value || activeSessionId.value === null) {
    return;
  }
  const q = query.value.trim();
  if (!q) {
    appStore.notify({
      text: i18n.global.t("page.aiAssistant.emptyQuery"),
      timeout: 3000,
      color: "error",
    });
    return;
  }
  let parsedBusiness: Record<string, unknown> | undefined;
  if (businessData.value.trim()) {
    try {
      parsedBusiness = JSON.parse(businessData.value) as Record<string, unknown>;
    } catch {
      appStore.notify({
        text: i18n.global.t("page.aiAssistant.invalidJson"),
        timeout: 3000,
        color: "error",
      });
      return;
    }
  }
  const sessionID = activeSessionId.value;
  const assistantKey = nextKey();
  const hasRule = regoRule.value.trim().length > 0;
  messages.value.push({ key: nextKey(), role: "user", content: q });
  messages.value.push({ key: assistantKey, role: "assistant", content: "", streaming: true });
  query.value = "";
  scrollToBottom();

  streaming.value = true;
  abortStream = agentStreamGenerate(
    sessionID,
    {
      query: q,
      docIDs: docIDs.value.length > 0 ? docIDs.value : undefined,
      businessData: parsedBusiness,
      regoRule: regoRule.value.trim() || undefined,
      // 本次生成的显式档位（为空时由后端回退到会话档位与默认档位参数）
      slot: slot.value || undefined,
    },
    {
      onDelta: (delta: string) => {
        const msg = messages.value.find((m) => m.key === assistantKey);
        if (msg) {
          msg.content += delta;
          scrollToBottom();
        }
      },
      // 工具调用阶段事件：工具调用轮不产生答案增量，仅实时累积阶段行（含工具名）
      onStage: (ev: AgentStageEvent) => {
        applyAgentStage(assistantKey, ev);
      },
      onDone: (out) => {
        const msg = messages.value.find((m) => m.key === assistantKey);
        if (msg) {
          msg.content = out.content ?? msg.content;
          msg.draft = asRecord(out.draft);
          msg.citations = out.citations;
          if (hasRule) {
            msg.valid = out.valid;
            msg.validationDetail = out.validationDetail;
          }
          msg.streaming = false;
          scrollToBottom();
        }
        streaming.value = false;
        abortStream = null;
        // 会话活跃时间变化，刷新列表以维持最近活跃在前的排序
        void loadSessions();
      },
      onError: () => {
        const msg = messages.value.find((m) => m.key === assistantKey);
        if (msg) {
          msg.streaming = false;
          if (!msg.content) {
            msg.content = i18n.global.t("page.aiAssistant.error");
          }
        }
        streaming.value = false;
        abortStream = null;
        appStore.notify({
          text: i18n.global.t("page.aiAssistant.error"),
          timeout: 3000,
          color: "error",
        });
      },
    },
  );
};

const stopGenerating = (): void => {
  if (abortStream) {
    abortStream();
    abortStream = null;
  }
  if (streaming.value) {
    streaming.value = false;
    // 中止后残留的流式占位消息停止闪烁，保留已生成的增量内容
    const last = messages.value[messages.value.length - 1];
    if (last && last.streaming) {
      last.streaming = false;
    }
  }
};

// ---- 前往标签规则页 ----
const openTagRulePage = (): void => {
  router.push({ name: "Tag Rule List", params: { language: route.params.language } });
};

onMounted(() => {
  void ensureEnum(EnumClass.llmSlot);
  // 工具调用阶段名经统一枚举注册表渲染 label（与检索试运行共用 evalStage）
  void ensureEnum(EnumClass.evalStage);
  void loadStatus();
  void loadSessions();
  void loadDocOptions();
});

// 页面卸载时中止在途流式请求，避免组件销毁后继续写入
onUnmounted(() => {
  stopGenerating();
});
</script>

<style scoped>
/* 双栏等高布局：减去顶栏（64px）、页面内边距与面包屑占位，最小高度保证窄屏可用 */
.assistant-row {
  height: calc(100vh - 180px);
  min-height: calc(var(--zk-editor-min-height) * 3);
}

.chat-messages {
  scroll-behavior: smooth;
}

/* 气泡：宽度上限保留原 85% 比例（无对应令牌）；圆角由 .zk-chat-bubble* 外壳类承载 */
.message-bubble {
  max-width: calc(100% * 0.85);
  padding: var(--zk-space-2) var(--zk-space-3);
}

/* 正文保留换行与空格（模型输出 Markdown 原文，v1 阶段按纯文本展示） */
.message-content {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.6;
}

/* 工具调用阶段列表：与检索试运行的阶段行同构（气泡内不可点击，故不设选中态与光标） */
.stage-list {
  margin-top: var(--zk-space-1);
}

.stage-row {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
  padding: var(--zk-space-1) 0;
}

.stage-name {
  flex: 0 0 auto;
}

/* 引用正文两行截断，避免单条引用撑爆气泡 */
.citation-list :deep(.v-list-item-subtitle) {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 草稿 JSON 视区：表面色衬底与气泡（surface-variant）区分，保证可读性 */
.draft-sheet {
  max-height: calc(var(--zk-editor-min-height) * 1.5);
  overflow: auto;
  padding: var(--zk-space-2) var(--zk-space-3);
}
</style>
