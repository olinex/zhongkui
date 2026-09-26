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

// 知识库文档索引摘要（KnowledgeDocumentChunksOut.summary）
// 全部字段由后端在读取时从文档与其分块聚合派生（含 overlap 造成的重复计数），不落库。
export interface KnowledgeDocumentIndexSummary {
  chunkCount: number;
  contentChars: number;
  chunkCharsTotal: number;
  chunkCharsAvg: number;
  chunkCharsMin: number;
  chunkCharsMax: number;
  headingPathCount: number;
  // 含代码块或表格的分块数（超长也不切分的原子块）
  atomicChunkCount: number;
  // 生效参数 chunking.size / chunking.overlap
  chunkSize: number;
  chunkOverlap: number;
  indexed: boolean;
  // 索引摘要与当前正文不一致（索引落后于正文）
  stale: boolean;
  indexedAt: string;
  hash: string;
  // embedding 档位绑定的供应商模型名（未绑定时为空）
  embeddingModel: string;
  // embedding 列的实际向量维度（不可读时为 0）
  embeddingDimensions: number;
}

// 单个知识分块明细（kinds 取值集合与展示文案由后端枚举类 chunkUnitKind 下发，
// 见 utils/enums.ts 的 EnumClass，前端不声明取值字面量）
export interface KnowledgeDocumentChunk {
  index: number;
  // 所属标题路径，空表示该块不在任何标题之下
  headingPath: string;
  content: string;
  chars: number;
  lines: number;
  kinds: string[];
}

// 知识库文档分块明细（文档级索引摘要 + 逐块明细）
export interface KnowledgeDocumentChunksOut {
  summary: KnowledgeDocumentIndexSummary;
  chunks: KnowledgeDocumentChunk[];
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

// 评估阶段标识与阶段状态：取值集合与展示文案一律由后端枚举类 evalStage / evalStageStatus
// 下发（见 utils/enums.ts 的 EnumClass），前端不声明取值字面量，此处仅为类型别名。工具调用
// 阶段（toolCall / toolResult）与知识库检索试运行的阶段同属后端的 EvalStage 一个枚举类。
export type EvalStage = string;
export type EvalStageStatus = string;

// SSE stage 事件负载（Agent 生成流）：status 非空为阶段状态事件（start/done/failed）；
// tool 为本轮事件的工具名（仅 toolCall / toolResult 阶段有值），error 仅 failed 有值
export interface AgentStageEvent {
  stage: EvalStage;
  status?: EvalStageStatus;
  delta?: string;
  elapsedMs: number;
  tool?: string;
  error?: string;
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
// 改进建议归类的取值集合与展示文案由后端枚举类 diagnosisCategory 下发（EnumClass.diagnosisCategory）
export type DiagnosisCategory = string;
// 单条改进建议：detail 为后端按请求语言输出的自由文本
export interface DiagnosisSuggestion {
  category: DiagnosisCategory;
  detail: string;
}
export interface DryRunEvaluation {
  answer: string;
  metrics: RetrievalDryRunMetrics;
  chunkJudgments: ChunkJudgment[];
  claims: FaithfulnessClaim[];
  reverseQuestions: string[];
  suggestions: DiagnosisSuggestion[];
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

// 单次调用 token 用量
export interface DryRunCallUsage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
}

// 单次调用元信息（模型名由后端汇总时填充）
export interface DryRunCallMeta {
  stage: EvalStage;
  model: string;
  elapsedMs: number;
  usage: DryRunCallUsage;
}

// SSE stage 事件负载：status 非空为阶段状态事件，delta 非空为该阶段的增量输出事件
export interface DryRunStageEvent {
  stage: EvalStage;
  status?: EvalStageStatus;
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
