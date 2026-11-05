// AI 助手 SSE 协议类型：与后端 internal/ai/assistant 的 JSON 契约一一对应。
//
// 服务端按 data: <json> 逐条下发事件；实时流与历史快照共用同一套内容块模型
// （AgentMessageBlockOut）。协议口径见 docs/curated/explanation/ai-assistant-sse-schema.md。
import type {
  AgentApprovalCallRecord,
  AgentMessageItem,
  AgentNextStep,
  AgentToolCallRecord,
  QuestionnaireAnswer,
  QuestionnaireQuestion,
  RetrievalResult,
} from "./ai";

// ASSISTANT_EVENT_VERSION 当前协议版本；破坏性变更时递增。
export const ASSISTANT_EVENT_VERSION = 1 as const;

// AssistantEventType 事件类型标识：与后端 assistant.EventType 一一对应。
export type AssistantEventType =
  | "snapshot.begin"
  | "snapshot.message"
  | "snapshot.end"
  | "resume.begin"
  | "resume.reset"
  | "turn.started"
  | "turn.preempted"
  | "message.started"
  | "message.persisted"
  | "block.created"
  | "block.delta"
  | "block.updated"
  | "tool.call.updated"
  | "message.citations"
  | "message.compress"
  | "interaction.waiting"
  | "interaction.resumed"
  | "turn.completed"
  | "turn.error";

// AssistantEvent 单条 SSE 事件信封：data 行的 JSON 形状。
export interface AssistantEvent<T = unknown> {
  // 协议版本
  v: typeof ASSISTANT_EVENT_VERSION;
  // 实时游标：同一 runID 内从 1 递增；历史快照内为回放序号
  seq: number;
  // 事件类型
  type: AssistantEventType;
  // 会话 ID
  sessionID: number;
  // 实时运行 ID；历史快照不携带
  runID?: string;
  // 事件所属助手消息 ID；尚未落库时缺省
  messageID?: number;
  // 服务端时间（RFC3339）
  ts: string;
  // 事件载荷：按 type 解释
  payload: T;
}

// AssistantBlock 一块内容：kind 与 payload 一一对应。
export type AssistantBlock =
  | { kind: "reasoning"; payload: { text: string } }
  | { kind: "content"; payload: { text: string } }
  | { kind: "tool_calls"; payload: { calls: AgentToolCallRecord[] } }
  | {
      kind: "approval";
      payload: { calls: AgentApprovalCallRecord[]; callIDs?: string[] };
    }
  | {
      kind: "questionnaire";
      payload: {
        questions: QuestionnaireQuestion[];
        answers?: QuestionnaireAnswer[];
        awaiting: boolean;
        interruptIDs?: string[];
      };
    }
  | { kind: "next_steps"; payload: { steps: AgentNextStep[] } };

// AssistantBlockCreatedPayload 新建块事件的载荷。
export type AssistantBlockCreatedPayload = { blockSeq: number } & AssistantBlock;

// AssistantBlockUpdatedPayload 块收敛 / 原位更新事件的载荷。
export type AssistantBlockUpdatedPayload = { blockSeq: number } & AssistantBlock;

// AssistantBlockDeltaPayload 思考 / 正文增量事件的载荷。
export interface AssistantBlockDeltaPayload {
  // 目标块的序号
  blockSeq: number;
  // 块类型（仅 reasoning / content）
  kind: "reasoning" | "content";
  // 本次追加的增量文本
  text: string;
}

// AssistantToolCallUpdatedPayload 单条工具调用状态变化事件的载荷。
export interface AssistantToolCallUpdatedPayload {
  // 该调用所属 tool_calls 块的序号
  blockSeq: number;
  // 更新后的调用记录
  call: AgentToolCallRecord;
}

// AssistantUsage 一轮助手消息的用量合计。
export interface AssistantUsage {
  // 提示 token 累计
  promptTokens: number;
  // 补全 token 累计
  completionTokens: number;
  // 合计 token
  totalTokens: number;
  // 本轮总耗时（毫秒）
  elapsedMs: number;
}

// AssistantCompressState 上下文压缩的展示态。
export interface AssistantCompressState {
  // 压缩阶段
  phase: "running" | "done" | "failed";
  // 摘要增量文本（仅 running）
  draft?: string;
  // 压缩完成后的摘要全文（仅 done）：历史回放随检查点带回，供收起态点击展开查看
  summary?: string;
  // 推进原因
  reason: "auto" | "manual" | "rolling";
  // 推进前会话上下文占用 token
  tokensBefore?: number;
  // 本次折叠进摘要的消息条数
  foldedCount?: number;
  // 推进后上下文占用的即时估算 token
  tokensAfter?: number;
}

// AssistantMessageSnapshot 历史快照中的一条消息：在现有会话消息基础上补充用户轮次序号。
export interface AssistantMessageSnapshot extends AgentMessageItem {
  // 用户消息在会话内的序号（从 1 起），供回退 / 分支定位
  userMessageIndex?: number;
}

// AssistantSnapshotBeginPayload 历史快照开始事件的载荷。
export interface AssistantSnapshotBeginPayload {
  // 快照模式，当前固定为 history
  mode: "history";
}

// AssistantSnapshotMessagePayload 历史快照消息事件的载荷。
export interface AssistantSnapshotMessagePayload {
  // 消息终态
  message: AssistantMessageSnapshot;
}

// AssistantSnapshotEndPayload 历史快照结束事件的载荷。
export type AssistantSnapshotEndPayload = Record<string, never>;

// AssistantResumeBeginPayload 断线续播开始事件的载荷。
export interface AssistantResumeBeginPayload {
  // 正在续播的运行 ID
  runID: string;
  // 客户端最后看到的游标
  fromSeq: number;
}

// AssistantResumeResetPayload 续播游标失效事件的载荷。
export interface AssistantResumeResetPayload {
  // 失效原因
  reason: "gap" | "expired";
}

// AssistantUserMessageSnapshot 一次新提问的用户消息事实。
export interface AssistantUserMessageSnapshot {
  // 用户消息主键
  messageID: number;
  // 提问正文
  content: string;
  // 创建时间（RFC3339）
  createdAt: string;
  // 该提问在会话内的序号（从 1 起）
  userIndex: number;
  // 本轮选用的技能 ID，未选技能时缺省
  skillID?: number;
  // 本轮选用的技能名称快照，未选技能时缺省
  skillName?: string;
}

// AssistantTurnStartedPayload 回合开始事件的载荷。
export interface AssistantTurnStartedPayload {
  // 本次运行 ID
  runID: string;
  // 是否为审批 / 问卷提交后的续跑
  resumed: boolean;
  // 续跑或已完成回合的助手消息 ID
  assistantMessageID?: number;
  // 新提问的用户消息事实；续跑时缺省
  userMessage?: AssistantUserMessageSnapshot;
}

// AssistantTurnPreemptedPayload 回合被抢占事件的载荷。
export interface AssistantTurnPreemptedPayload {
  // 抢占该回合的新运行 ID
  byRunID: string;
}

// AssistantMessageStartedPayload 助手消息开始事件的载荷。
export interface AssistantMessageStartedPayload {
  // 消息角色，当前固定为 assistant
  role: "assistant";
  // 助手消息 ID；尚未落库时缺省
  messageID?: number;
}

// AssistantMessagePersistedPayload 助手消息落库事件的载荷。
export interface AssistantMessagePersistedPayload {
  // 已落库的助手消息主键
  messageID: number;
}

// AssistantMessageCitationsPayload 检索引用就绪事件的载荷。
export interface AssistantMessageCitationsPayload {
  // 检索引用列表
  citations: RetrievalResult[];
}

// AssistantMessageCompressPayload 压缩状态事件的载荷。
export interface AssistantMessageCompressPayload {
  // 压缩展示态
  compress: AssistantCompressState;
}

// AssistantInteractionWaitingPayload 进入待审批 / 待作答事件的载荷。
export interface AssistantInteractionWaitingPayload {
  // 等待子状态
  substate: "approval" | "answer";
  // 本轮挂起的根中断 ID
  interruptIDs: string[];
  // 触发等待的块序号
  blockSeq?: number;
}

// AssistantInteractionResumedPayload 交互提交后续跑事件的载荷。
export interface AssistantInteractionResumedPayload {
  // 已消费的根中断 ID
  interruptIDs: string[];
}

// AssistantTurnCompletedPayload 回合终态事件的载荷。
export interface AssistantTurnCompletedPayload {
  // 回合终态阶段
  phase: "done" | "failed" | "cancelled";
  // 落库后的助手消息 ID
  messageID?: number;
  // 本轮用量合计
  usage?: AssistantUsage;
}

// AssistantTurnErrorPayload 请求或生成失败事件的载荷。
export interface AssistantTurnErrorPayload {
  // 本地化错误文案
  message: string;
  // 可选错误码
  code?: string;
}

// AssistantEventPayloadMap 事件类型到载荷类型的映射。
export interface AssistantEventPayloadMap {
  "snapshot.begin": AssistantSnapshotBeginPayload;
  "snapshot.message": AssistantSnapshotMessagePayload;
  "snapshot.end": AssistantSnapshotEndPayload;
  "resume.begin": AssistantResumeBeginPayload;
  "resume.reset": AssistantResumeResetPayload;
  "turn.started": AssistantTurnStartedPayload;
  "turn.preempted": AssistantTurnPreemptedPayload;
  "message.started": AssistantMessageStartedPayload;
  "message.persisted": AssistantMessagePersistedPayload;
  "block.created": AssistantBlockCreatedPayload;
  "block.delta": AssistantBlockDeltaPayload;
  "block.updated": AssistantBlockUpdatedPayload;
  "tool.call.updated": AssistantToolCallUpdatedPayload;
  "message.citations": AssistantMessageCitationsPayload;
  "message.compress": AssistantMessageCompressPayload;
  "interaction.waiting": AssistantInteractionWaitingPayload;
  "interaction.resumed": AssistantInteractionResumedPayload;
  "turn.completed": AssistantTurnCompletedPayload;
  "turn.error": AssistantTurnErrorPayload;
}

// AssistantEventOf 指定类型的事件：type 与 payload 同时收窄。
export type AssistantEventOf<T extends AssistantEventType> = AssistantEvent<
  AssistantEventPayloadMap[T]
> & { type: T };

// AssistantEventUnion 全部事件的联合类型，供解析器穷举分派。
export type AssistantEventUnion = {
  [K in AssistantEventType]: AssistantEventOf<K>;
}[AssistantEventType];
