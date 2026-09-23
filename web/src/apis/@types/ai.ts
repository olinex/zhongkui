// 知识库文档（KnowledgeDocument）
export interface KnowledgeDocumentItem {
  id: number;
  title: string;
  chunkCount: number;
  createdAt: string;
  updatedAt: string;
}
export interface KnowledgeDocumentDetail extends KnowledgeDocumentItem {
  content: string;
  hash: string;
}
export interface KnowledgeDocumentCreateIn {
  title: string;
  content: string;
}
export interface KnowledgeDocumentUpdateIn {
  id: number;
  title: string;
  content: string;
}
export interface KnowledgeDocumentReindexOut {
  id: number;
  chunkCount: number;
  hash: string;
}

// AI 会话（AgentSession）
export interface AgentSessionItem {
  id: number;
  title: string;
  // 会话缺省档位（flash/standard/advanced），空表示按 agent.defaultSlot 参数解析
  slot: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}
export interface AgentSessionCreateIn {
  title: string;
  slot?: string;
}
export interface AgentSessionRenameIn {
  id: number;
  title: string;
}

// 检索引用（RetrievalResult，来自 done 事件的 citations）
export interface RetrievalResult {
  documentID: string;
  title: string;
  chunkIndex: number;
  content: string;
  score: number;
}

// 会话消息（AgentMessageItem）
export interface AgentMessageItem {
  id: number;
  role: string; // user | assistant | system
  content: string;
  draft?: unknown; // 助手消息的结构化草稿 JSON 对象（自由形态）
  citations?: RetrievalResult[];
  createdAt: string;
}

// 流式生成请求体（AgentStreamIn）
export interface AgentStreamIn {
  query: string;
  docIDs?: number[];
  businessData?: Record<string, unknown>;
  regoRule?: string;
  // 本次生成的显式档位（flash/standard/advanced），空表示回退到会话档位与 agent.defaultSlot
  slot?: string;
}

// SSE done 事件负载（后端 AgentOutput）
export interface AgentOutput {
  content?: string;
  done: boolean;
  citations?: RetrievalResult[];
  draft?: unknown;
  valid: boolean;
  validationDetail?: string;
}

// 单个档位的绑定模型与可用性（AgentSlotStatusOut）
export interface AgentSlotStatusOut {
  // 档位标识（flash/standard/advanced/embedding）
  slot: string;
  modelName: string;
  providerModel: string;
  kind: string;
  enabled: boolean;
  // 绑定是否可用（模型已启用且类型与档位匹配）
  bound: boolean;
  // 该档位当前是否可解析到已构建的客户端
  available: boolean;
}

// AI 状态（AgentStatusOut）
export interface AgentStatusOut {
  enabled: boolean;
  chatModel: string;
  embeddingModel: string;
  embeddingDimensions: number;
  dimensionMismatch: boolean;
  dimensionDetail: string;
  // 各档位绑定的模型与可用性（按 flash/standard/advanced/embedding 顺序）
  slots: AgentSlotStatusOut[];
  // 未显式指定档位且会话未记录档位时使用的默认档位（后端参数 agent.defaultSlot 的生效值），
  // 前端档位下拉据此取初值，不在前端声明档位字面量
  defaultSlot: string;
  // 当前生效值与出厂值不同的行为参数条数（0 表示全部参数仍为出厂值）
  paramOverrideCount: number;
  // AI 能力不可用的原因说明（按请求语言返回；可用时为空）
  reason: string;
}

// 检索试运行参数默认值（来自后端 [llm].rerank 配置，供前端对话框预填展示）
export interface RetrievalWeightsOut {
  vector: number;
  keyword: number;
}
export interface RetrievalOptionsOut {
  topK: number;
  candidates: number;
  fusion: string;
  weights: RetrievalWeightsOut;
}

// 检索试运行（Retrieval Dry Run）
export interface RetrievalDryRunIn {
  query: string;
  docIDs?: number[];
  topK?: number;
  candidates?: number;
  fusion?: string;
  weights?: { vector: number; keyword: number };
}
export interface RetrievalDryRunMetrics {
  contextPrecision: number;
  faithfulness: number;
  answerRelevance: number;
  contextUtilization: number;
}
export interface ChunkJudgment {
  index: number;
  relevant: boolean;
  score: number;
  reason: string;
}
export interface FaithfulnessClaim {
  statement: string;
  supported: boolean;
  chunkIndexes: number[];
}
export interface DryRunEvaluation {
  answer: string;
  metrics: RetrievalDryRunMetrics;
  chunkJudgments: ChunkJudgment[];
  claims: FaithfulnessClaim[];
  reverseQuestions: string[];
  relevantCount: number;
  totalChunks: number;
  warnings: string[];
}
export interface RetrievalDryRunOut {
  query: string;
  results: RetrievalResult[];
  evaluation?: DryRunEvaluation;
  evalError?: string;
  meta: DryRunMeta;
}

// 检索试运行阶段标识（与后端 EvalStage 严格对齐）
export type DryRunStage = "retrieve" | "answer" | "chunkRelevance" | "claims" | "reverseQuestions";
// 阶段状态：start 阶段开始，done 阶段完成，failed 阶段失败（已降级为 warning）
export type DryRunStageStatus = "start" | "done" | "failed";

// 单次调用 token 用量
export interface DryRunCallUsage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
}

// 单次调用元信息（模型名由后端汇总时填充）
export interface DryRunCallMeta {
  stage: DryRunStage;
  model: string;
  elapsedMs: number;
  usage: DryRunCallUsage;
}

// SSE stage 事件负载：status 非空为阶段状态事件，delta 非空为该阶段的增量输出事件
export interface DryRunStageEvent {
  stage: DryRunStage;
  status?: DryRunStageStatus;
  delta?: string;
  elapsedMs: number;
  usage?: DryRunCallUsage;
  error?: string;
}

// 调用元信息汇总（检索耗时、各次调用明细、总计与 token 汇总）
export interface DryRunMeta {
  retrieveElapsedMs: number;
  calls: DryRunCallMeta[];
  totalElapsedMs: number;
  usage: DryRunCallUsage;
}
