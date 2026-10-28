// 文档格式与处理状态：取值集合与展示文案一律由后端枚举类 documentFormat（归属
// internal/pkg/docconv）与 knowledgeDocStatus（归属 internal/models/ai）下发（见
// utils/enums.ts 的 EnumClass），前端不声明取值字面量，此处仅为类型别名。
export type DocumentFormat = string;
export type KnowledgeDocStatus = string;

// 知识库文档（KnowledgeDocument）
export interface KnowledgeDocumentItem {
  id: number;
  title: string;
  // 文档所属命名空间名，系统视图行内据此跳转详情页
  namespaceName?: string;
  // 文档来源格式（markdown/txt/pdf/docx），手动录入文档固定为 markdown
  format: DocumentFormat;
  // 文档处理状态（pending/parsing/ready/failed），手动录入文档固定为 ready
  status: KnowledgeDocStatus;
  // 原件字节数，无原件（手动录入）时为 0
  sizeBytes: number;
  chunkCount: number;
  // 被检索命中的次数（每次检索命中该文档的任一分块自增一次）
  useCount: number;
  // 最近一次被检索命中的时间，从未命中时为 null
  lastUsedAt: string | null;
  createdAt: string;
  updatedAt: string;
}
export interface KnowledgeDocumentDetail extends KnowledgeDocumentItem {
  content: string;
  hash: string;
  // 导入时的原件文件名，手动录入文档为空
  originalName: string;
  // 解析失败原因（按请求语言返回），非 failed 状态为空
  parseError: string;
}
export interface KnowledgeDocumentCreateIn {
  title: string;
  content: string;
}
export interface KnowledgeDocumentUpdateIn {
  id: number;
  title: string;
  // 正文可选：导入文档仅允许改标题（携带非空 content 会被后端拒绝），手动录入文档语义不变
  content?: string;
}
export interface KnowledgeDocumentReindexOut {
  id: number;
  chunkCount: number;
  hash: string;
  // 重新索引后的处理状态：导入文档异步重解析时为 pending，手动录入文档同步重建时为 ready
  status: KnowledgeDocStatus;
}

// 知识库文档导入出参（KnowledgeDocumentUploadOut）：客户端据 id 轮询列表、据 status 判断进度
export interface KnowledgeDocumentUploadOut {
  id: number;
  status: KnowledgeDocStatus;
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
  // 会话所属 Agent 类型（knowledge/rego），取值集合与展示文案由后端枚举类 agentKind 下发
  agentKind: string;
  // 会话级审核阀门（each/auto/none），取值集合与展示文案由后端枚举类 reviewValve 下发
  reviewValve: string;
  // 会话滚动摘要，无摘要时为空串
  summary: string;
  // 该会话当前上下文占用 token：末次真实测量（最近一条有用量记录的助手消息）与更晚的压缩后
  // 即时估算中更晚者，二者皆无时为 0（0 表示尚无测量）
  contextTokens: number;
  // 按该会话当前有效档位解析的上下文容量 token；≤0 表示该档位未配置容量
  contextLimit: number;
  // 会话交互主状态 / 子状态：由后端按既有事实推导（不落库），取值集合与展示文案由后端枚举类
  // agentInteractionState / agentInteractionSubstate 下发；前端仅透传消费，不自行推断
  interactionState: string;
  interactionSubstate: string;
  // 会话是否已锁定（有效轮次达上限）；恒等于 interactionState 为 locked
  locked: boolean;
  // 会话有效轮次（当前存在的用户提问条数，被回滚删除的不计入）与轮次上限（≤0 表示不设上限）
  userTurns: number;
  maxUserTurns: number;
  createdAt: string;
  updatedAt: string;
}
export interface AgentSessionCreateIn {
  // 首个用户问题，必填；后端据此经 LLM 生成会话主题（失败时按其截断兜底）
  query: string;
  slot?: string;
  // 会话所属 Agent 类型（knowledge/rego），可选；缺省由后端取 knowledge
  agentKind?: string;
  // 会话初始审核阀门（each/auto/none），可选；缺省由后端取会话列默认值
  reviewValve?: string;
}
export interface AgentSessionRenameIn {
  id: number;
  title: string;
}
// 更新单个会话审核阀门的请求体（reviewValve 取值由后端枚举类校验）
export interface AgentSessionValveIn {
  reviewValve: string;
}
// 会话压缩请求体（AgentSessionCompactIn）：assistantMessageID 为折叠边界（按助手回答定位）。
// 契约口径依赖后端 Task 8（按助手回答边界的上下文压缩）：边界改以助手消息 ID 表示，
// 原「第 N 轮用户提问」口径（userMessageIndex）不再用于压缩边界。
export interface AgentSessionCompactIn {
  // 折叠边界的助手消息 ID（会话消息列表出参的 id）；0 或缺省表示全量压缩（折叠全部未摘要消息）
  assistantMessageID?: number;
}
// 提示词优化请求体（AgentPromptOptimizeIn）：query 为待优化的用户诉求
export interface AgentPromptOptimizeIn {
  // 待优化的用户诉求（必填）
  query: string;
}
// 提示词优化响应体（AgentPromptOptimizeOut）：query 为优化后的提问文本
export interface AgentPromptOptimizeOut {
  // 优化后的提问文本（供前端整体替换输入框内容）
  query: string;
}

// 长期记忆（AgentMemory）
// 记忆所属 Agent 类型 / 类别 / 状态的取值集合与展示文案一律由后端枚举类
// agentKind / memoryKind / memoryStatus 下发（见 utils/enums.ts 的 EnumClass），前端不声明取值字面量。
export interface AgentMemoryItemOut {
  id: number;
  // 记忆所属 Agent 类型（knowledge/rego）
  agentKind: string;
  // 记忆类别（fact/preference/decision/artifact）
  kind: string;
  title: string;
  content: string;
  // 记忆状态（active/archived）
  status: string;
  // 被检索命中的次数
  useCount: number;
  // 最近一次被检索召回的时间，可空
  lastUsedAt: string | null;
  createdAt: string;
  updatedAt: string;
}
export interface AgentMemoryDetailOut extends AgentMemoryItemOut {
  // 所属命名空间
  namespaceName: string;
  // 来源会话 ID（记忆按会话隔离，仅在来源会话内可见）
  sessionID: number;
}
export interface AgentMemoryStatusIn {
  // 目标状态（active/archived）
  status: string;
}

// 检索引用（RetrievalResult，来自 done 事件的 citations）
export interface RetrievalResult {
  documentID: string;
  title: string;
  chunkIndex: number;
  content: string;
  score: number;
  // 来源文档所属命名空间名，用于选择读取接口/溯源跳转
  namespaceName: string;
}

// 会话消息（AgentMessageItem）
export interface AgentMessageItem {
  id: number;
  role: string; // user | assistant | system
  content: string;
  draft?: unknown; // 助手消息的结构化草稿 JSON 对象（自由形态）
  citations?: RetrievalResult[];
  createdAt: string;
  // 助手回答的用量元信息：本轮全部模型调用的 token 累计（提示 / 补全 / 合计）与总耗时（毫秒），
  // 随消息落库，故重新加载历史会话后仍可展示；用户消息与无用量数据的消息一律为 0。
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  elapsedMs: number;
  // 本条助手消息所属运行的生命周期终态阶段（done/failed/cancelled，取值来自后端枚举类
  // agentRunPhase）：failed/cancelled 表示本轮被中断，前端据此展示中断提示；用户消息与在途
  // 运行的阶段性落库行缺省
  phase?: string;
  // 该消息所锚定的历史压缩记录（压缩游标等于本条消息 seq 时由后端给出）：供刷新 / 重进会话后
  // 回显过往的压缩推进（含静默滚动 rolling）；无则为缺省
  compress?: AgentMessageCompressItem;
  // 该助手消息的有序内容块（按 seq 升序）：思考段 / 工具调用组 / 授权待审 / 主动提问 / 正文段各按
  // 其真实产生时序排列，是消息体的唯一展示来源，与流式 block 事件同源（后端 AgentMessageBlockOut）。
  // 用户消息与无块的消息不带此字段
  blocks?: AgentMessageBlockOut[];
  // 该助手消息若承载「仍待用户输入」的挂起现场（问卷待作答 / 工具待授权），后端给出该现场的待执行
  // 调用清单：回退重开只重置挂起现场、不产生在途运行，故刷新 / 重进会话时改由此处恢复可交互面板
  // （提交 / 取消续跑需原样回传其中的 callID）；其余消息不带此字段
  pendingCalls?: ToolRequestPayload[];
}

// 助手消息内容块的类型取值（后端 AgentMessageBlockKind）：不注册枚举类、无 i18n label（同
// AgentRunEventType 约定），前端不声明取值空间，此处仅为「块 → 渲染块」分派用的字面量联合类型。
export type AgentBlockKind =
  "reasoning" | "tool_calls" | "approval" | "questionnaire" | "content" | "next_steps";

// 助手消息所承载的**一块**内容块展示项（后端 AgentMessageBlockOut）：块在消息内的序号、块类型与
// 该块的明文 JSON 载荷。载荷形状与流式 block 事件的 payload 一致（按 kind 区分），刷新 / 重进会话
// 与实时会话由同一数据源驱动，故前端据同一套「块 → 渲染块」映射同时服务二者。
export interface AgentMessageBlockOut {
  // 块在所属消息内的单调序号（从 1 起），即内容的真实产生顺序；前端据此按 seq upsert
  seq: number;
  // 块类型
  kind: AgentBlockKind;
  // 该块的明文 JSON 载荷（对象）：按 kind 解释（见下）
  payload: unknown;
  // 该事件是否为块的创建事件：true 为新建（reasoning / content 的载荷仅带首个增量，其后同段增量经
  // delta / reasoning 事件追加），false 为同块的收敛 / 更新（reasoning / content 的载荷为终态全文）；
  // 仅流式事件携带，会话消息出参（已落库块）不带。
  created?: boolean;
}

// reasoning 块载荷：一段连续思考文本（开放期内增量累积）
export interface AgentReasoningBlockPayload {
  text: string;
}
// content 块载荷：一段连续正文文本（开放期内增量累积）
export interface AgentContentBlockPayload {
  text: string;
}
// tool_calls 块内一条工具调用记录（后端 ToolCallRecord）：一条调用从发起到结果的完整事实
export interface AgentToolCallRecord {
  callID: string;
  toolName: string;
  arguments: string;
  auditLevel: string;
  execOwner: "client" | "backend";
  round?: number;
  // 调用状态：start 发起 / done 完成 / failed 失败
  status: "start" | "done" | "failed";
  elapsedMs: number;
  error?: string;
  result?: string;
}
// tool_calls 块载荷：同一轮（round）发起的调用并列为一组
export interface AgentToolCallsBlockPayload {
  calls: AgentToolCallRecord[];
}
// approval 块内一条待审批调用记录（后端 ApprovalCallRecord）：调用描述 + 用户决策与执行结果
export interface AgentApprovalCallRecord {
  callID: string;
  toolName: string;
  arguments: string;
  auditLevel: string;
  approvalRequired: boolean;
  execOwner: "client" | "backend";
  // 用户决策：缺省表示尚无最终决策（仍待审批 / 只读回显），true 批准、false 拒绝
  decision?: boolean;
  result?: string;
  error?: string;
}
// approval 块载荷：本轮待审批调用清单。callIDs 为卡片上的全部工具调用 ID（含已决策行）——已决策
// 行没有可点击按钮，前端据此仍能精确定位该审批交互，供「重新审批」回退。
export interface AgentApprovalBlockPayload {
  calls: AgentApprovalCallRecord[];
  callIDs?: string[];
}
// questionnaire 块载荷：一轮 ask_user 问卷的题目与作答（同一回合内多轮各成一块，仅末轮可待回答）。
// interruptIDs 为本轮 ask_user 调用的根中断 ID——已作答卡片不再渲染提交按钮，前端据此仍能定位该
// 问卷交互，供「重新回答」回退到未作答状态。
export interface AgentQuestionnaireBlockPayload {
  questions: QuestionnaireQuestion[];
  answers?: QuestionnaireAnswer[];
  awaiting: boolean;
  interruptIDs?: string[];
}
// next_steps 块内一条推荐的下一步：title 为不超过 24 字的行动标题，reason 为不超过 64 字的推荐理由，
// text 为可直接发送的完整诉求；recommended 标记最推荐项
export interface AgentNextStep {
  title?: string;
  text: string;
  reason?: string;
  recommended?: boolean;
}
// next_steps 块载荷：回答完成后基于整体上下文给出的 1-3 条推荐项，顺序即推荐程度（首条最推荐）
export interface AgentNextStepsBlockPayload {
  steps: AgentNextStep[];
}

// 助手消息所承载的**一轮**问卷展示项（后端 AgentQuestionnaireOut）：仅用于「回退到助手待输入回合」
// 的接口出参（消息列表出参的问卷已改由 questionnaire 块承载）。
export interface AgentMessageQuestionnaire {
  // 本轮问卷题目列表（每题含候选项与可选说明）
  questions: QuestionnaireQuestion[];
  // 本轮用户作答列表（未作答或取消回答时为空）
  answers?: QuestionnaireAnswer[];
  // 本轮是否仍待用户回答
  awaiting: boolean;
  // 承载该问卷的运行现场中的待执行调用：重新回答接口出参携带（供重答后经既有 tool-result 回传续跑）
  pendingCalls?: ToolRequestPayload[];
}

// 回退到一条助手待输入回合的请求体（AgentAssistantInputRerunIn）：messageID 为该回合的助手锚点消息 id
// （问卷作答与工具授权共用一个入口）；blockSeq 指定要回退到的授权块序号（同一回合可多次挂起、各轮授权
// 各落一个 approval 块），省略时后端沿用「取末轮」口径。
export interface AgentAssistantInputRerunIn {
  messageID: number;
  // blockSeq 要回退到的授权块序号（锚点消息内 block_seq）：仅授权回合使用，省略时取末个 approval 块
  blockSeq?: number;
}

// 回退到一条助手待输入回合的出参（AgentAssistantInputRerunOut）：按回合种类给出对应载荷，并附回退后的
// 会话交互主状态（供前端立即刷新状态驱动 UI）。kind 与回退后的交互子状态同值。
export interface AgentAssistantInputRerunOut {
  // 回合种类：answer（问卷作答）/ approval（工具授权）
  kind: "answer" | "approval";
  // 回退后的会话交互主状态（与后端 StateOf 同口径）
  interactionState: string;
  // 问卷回合按轮次先后并列的各轮载荷（题目 + 上次答案 + 末轮待执行调用），仅问卷回合携带
  questionnaires?: AgentMessageQuestionnaire[];
  // 该回合的待执行调用清单，授权回合据此重建审批面板
  pendingCalls?: ToolRequestPayload[];
}

// 消息所锚定的历史压缩记录（AgentMessageCompressOut）：承载回显所需的度量字段与摘要全文
// （摘要用于压缩块收起后点击展开查看）。
export interface AgentMessageCompressItem {
  // 压缩完成后的摘要全文（仅 done）：供历史回放中的压缩块展开查看
  summary?: string;
  // 推进原因（rolling 静默滚动 / auto 自动压缩 / manual 手动压缩）
  reason: string;
  // 推进前会话上下文占用 token
  tokensBefore: number;
  // 本次折叠进摘要的消息条数
  foldedCount: number;
  // 推进后上下文占用的即时估算 token（仅 auto / manual 给出，静默滚动与估算不可用时为 0）：
  // 供刷新 / 重进会话后仍回显压缩前后对比；为 0 时仅展示压缩前占用与折叠条数
  tokensAfter: number;
}

// 截断会话消息请求体（AgentMessageTruncateIn）：从第 userMessageIndex 条用户消息（从 1 起，
// 仅按用户消息计数、按消息 seq 升序计）起，删除该条及其之后的全部消息（含助手回复）。
export interface AgentMessageTruncateIn {
  userMessageIndex: number;
}

// 会话分支请求体（AgentSessionBranchIn）：以第 userMessageIndex 条用户消息（从 1 起，
// 仅按用户消息计数、按消息 seq 升序计）所在轮次为结尾，把该轮及之前的全部消息复制到新会话。
export interface AgentSessionBranchIn {
  userMessageIndex: number;
  // 新会话标题，可选；缺省由后端沿用源会话标题
  title?: string;
}

// 启动一轮对话的请求体（AgentChatIn）：POST /agent/:sessionID/chat，
// 语义与旧的生成流请求体一致（query 必填，其余为可选上下文）。
export type AgentChatIn = AgentStreamIn;

// 重新审批 / 重新回答请求体（AgentInteractionReopenIn）：审批回退按卡片上的工具调用 ID（callIDs）
// 定位交互，问卷回退按卡片上的根中断 ID（interruptIDs）定位交互，二者至少提供其一。后端把会话回退
// 到该次交互的待处理状态（删除该交互之后的回答，不重跑整轮）。
export interface AgentInteractionReopenIn {
  callIDs?: string[];
  interruptIDs?: string[];
}

// 流式生成请求体（AgentStreamIn）
export interface AgentStreamIn {
  query: string;
  docIDs?: number[];
  businessData?: Record<string, unknown>;
  // 本次生成的显式档位（flash/standard/advanced），空表示回退到会话档位与 agent.defaultSlot
  slot?: string;
  // 思考开关（on/off）：仅当当前档位绑定模型支持思考开关时携带，空 / 缺省表示不注入
  thinking?: "on" | "off";
}

// SSE done 事件负载（后端 AgentOutput）
export interface AgentOutput {
  content?: string;
  done: boolean;
  citations?: RetrievalResult[];
  draft?: unknown;
  valid: boolean;
  validationDetail?: string;
  // 仅在 done 项上给出的用量元信息：本轮全部模型调用的 token 累计与总耗时（毫秒），
  // 与消息列表中的同名字段语义一致（前者是刚生成完的实时值，后者是回读落库值）
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  elapsedMs?: number;
  // 本回合助手消息的落库行 ID（仅 done 项，且仅当本次运行持久化了会话时给出）：前端实时会话
  // 据此把本地气泡绑定到该行，使「压缩到此」等按消息 ID 定位的操作无需先回读历史即可发起；
  // 与 tool_request 载荷携带的挂起锚点 ID 同源同义
  messageID?: number;
  // 本次思考过程全文（无思考时为空 / 缺省）
  reasoning?: string;
}

// 上下文压缩开始事件载荷（compress_start）
export interface CompressStartPayload {
  // 压缩原因（auto 自动 / manual 手动）
  reason: string;
  // 压缩前会话占用 token
  tokens: number;
  // 本次有效档位的上下文容量 token（≤0 表示未配置）
  limit: number;
  // 触发压缩的占用比例阈值
  threshold: number;
}
// 上下文压缩摘要增量事件载荷（compress_delta）：摘要生成的实时增量文本
export interface CompressDeltaPayload {
  delta: string;
}
// 上下文压缩完成事件载荷（compress_done）
export interface CompressDonePayload {
  // 压缩原因（auto 自动 / manual 手动）
  reason: string;
  // 压缩前会话占用 token（仅成功压缩时有效；字段可能因后端 omitempty 缺失）
  tokensBefore?: number;
  // 压缩后会话占用 token 的即时估算（仅成功压缩时有效；字段可能因后端 omitempty 缺失）：
  // 优先取摘要模型用量算出的估值，用量缺失时回落为按字节折算的估值；供压缩完成即刻展示与占用
  // 指示，后续生成的真实 promptTokens 到达后校准
  tokensAfter?: number;
  // 本次折叠的消息条数（0 表示未折叠，如无消息可折叠或压缩失败）
  foldedCount?: number;
  // 压缩后滚动摘要游标（仅成功压缩时有效；字段可能因 omitempty 缺失）
  summaryUptoSeq?: number;
}

// 评估阶段标识与阶段状态：取值集合与展示文案一律由后端枚举类 evalStage / evalStageStatus
// 下发（见 utils/enums.ts 的 EnumClass），前端不声明取值字面量，此处仅为类型别名。工具调用
// 阶段（toolCall / toolResult）与知识库检索试运行的阶段同属后端的 EvalStage 一个枚举类。
export type EvalStage = string;
export type EvalStageStatus = string;

// SSE stage 事件负载（Agent 生成流）：status 非空为阶段状态事件（start/done/failed）；
// tool 为本轮事件的工具名（仅 toolCall / toolResult 阶段有值），error 仅 failed 有值；
// callID 为本次工具调用的唯一标识（仅 toolCall / toolResult 阶段有值），供前端把同一次调用的
// 「发起」与「结果」归并到同一行；缺失时（如达到轮数上限的整体失败）为空；
// round 为该次调用所属轮次（从 1 起，仅 toolCall / toolResult 阶段有值），供前端把同一轮内
// 并发发起的多次调用聚为一组展示
export interface AgentStageEvent {
  stage: EvalStage;
  status?: EvalStageStatus;
  delta?: string;
  elapsedMs: number;
  tool?: string;
  error?: string;
  callID?: string;
  round?: number;
}

// 挂起事件负载（后端 ToolRequest）：模型请求调用某个工具，需经人工审批后执行。execOwner 指明
// 执行归属：client 由前端以用户自身凭据执行，再把结果回传后端续跑；
// backend 由后端在审批通过后自行执行，前端仅做批准 / 拒绝。后端不解释入参，原样透传工具名与入参文本。
// auditLevel 为后端评估产出的审核重要级（取值集合与展示文案由后端枚举类 auditLevel 下发）；
// approvalRequired 由后端按审核重要级与会话阀门合成得出，为 false 时无需人工审批。
export interface ToolRequestPayload {
  // 本次调用的唯一 ID，回传结果时必须原样带回
  callID: string;
  // 待执行的工具名（模型给出的调用名）
  toolName: string;
  // 模型给出的调用入参（原始 JSON 文本，由前端自行解释）
  arguments: string;
  // 本次调用的审核重要级（must/suggest/auto/none），经枚举注册表渲染展示名
  auditLevel: string;
  // 是否需要人工审批：由后端按审核重要级与会话阀门合成，可能为 false（自动批准）
  approvalRequired: boolean;
  // 执行归属：client 由前端执行、backend 由后端执行（前端仅审批）
  execOwner: "client" | "backend";
  // 本次挂起所锚定的助手消息 ID（服务端 agent_messages.id）：挂起一律把本回合的助手消息落为锚点行，
  // 故正常挂起均携带，前端据此把挂起定位到本地气泡；「重新回答」须按其截断其后消息并可重答。
  // 仅旁路调用（服务端未注入运行期共享状态）缺省
  messageID?: number;
  // 该调用所属的根中断 ID（快照下由此恢复待审批 / 待作答的续跑定位）
  interruptID?: string;
}

// 单条审批结果（ToolResultItemIn）：approved=true 时 result 为工具执行结果文本、执行失败
// 时改填 error；approved=false（用户拒绝）时 result 与 error 均留空。
export interface ToolResultItemIn {
  callID: string;
  approved: boolean;
  result: string;
  error: string;
}

// 审批续跑请求体（ToolResultIn）：一次请求必须回传本轮全部待执行调用，
// 后端会把运行现场一次性消费。
export interface ToolResultIn {
  results: ToolResultItemIn[];
}

// 单条审批决策（AssistantApprovalDecision）：toolName / arguments / execOwner / approvalRequired
// 为审批卡片上下文元信息，随决策一并回传（供后端续跑定位与执行归属）。
export interface AssistantApprovalDecision {
  callID: string;
  interruptId?: string;
  approved: boolean;
  reason?: string;
  /** Frontend execution result for client tools (api_invoke). */
  result?: string;
  /** Frontend execution error for client tools. */
  error?: string;
  /** Tool metadata copied from the approval card context; not sent as a decision field. */
  toolName?: string;
  arguments?: string;
  execOwner?: string;
  approvalRequired?: boolean;
}

// 单题作答（AssistantQuestionnaireAnswer）。
export interface AssistantQuestionnaireAnswer {
  questionID: string;
  answered: boolean;
  answer?: string;
  custom?: boolean;
}

// 助手动作请求体（AssistantActionRequest）：POST /ai/agent/:sessionID/action。
export interface AssistantActionRequest {
  action: string;
  interruptId?: string;
  interruptIds?: string[];
  decisions?: AssistantApprovalDecision[];
  answers?: AssistantQuestionnaireAnswer[];
  text?: string;
}

// 主动提问问卷：模型在信息不足时经 client 工具 ask_user 向用户发起一组带候选的提问，前端据此
// 渲染问卷面板并阻塞会话，直到用户提交或取消。后端不解释问卷语义，原样透传题目与（可选的）
// 上次答案，前端负责渲染与把作答归一化回传。
export interface QuestionnaireOption {
  // 候选项主文案（选中后作为该题作答文本回传）
  text: string;
  // 候选项次要说明，可为空
  detail?: string;
}
export interface QuestionnaireQuestion {
  // 题目唯一标识，作答回传时原样带回
  id: string;
  // 题干
  question: string;
  // 候选项列表（「其他」自由作答项由前端恒定追加，不在其中）
  options: QuestionnaireOption[];
}
// 单题作答：answered=false 表示该题未作答（answer 为空、custom 为假）
export interface QuestionnaireAnswer {
  questionID: string;
  answered: boolean;
  // 作答文本：选候选项时为该候选项 text，选「其他」时为自定义输入文本
  answer: string;
  // 是否为「其他」自定义作答
  custom: boolean;
}
// 问卷载荷：questions 为题目；answers 为可选的上次答案，供前端预填
export interface QuestionnairePayload {
  questions: QuestionnaireQuestion[];
  answers?: QuestionnaireAnswer[];
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
  // 该档位绑定模型是否支持思考开关（思考字段名与开 / 关取值齐备时为 true）
  thinkingToggle: boolean;
  // 该档位模型的上下文窗口 token 容量；≤0 表示该档位关闭占用百分比展示与自动压缩
  contextTokenLimit: number;
}

// 在途运行态（AgentLiveRunOut）：供刷新 / 重开抽屉后发现「在跑 / 待审批」并据 runID 续播；
// phase 取值集合与展示文案由后端枚举类 agentRunPhase 下发（前端仅透传消费）；
// 交互状态五项（interactionState/substate/locked/userTurns/maxUserTurns）与快照口径一致。
export interface AgentLiveRunOut {
  // 当前运行标识（与后端 json tag `runID` 一致）
  runID: string;
  // 运行阶段（running/suspended/done/failed/cancelled）
  phase: string;
  // 服务端已发布的最大事件序号（供前端判断续播进度）
  lastSeq: number;
  // 是否处于等待人工审批（挂起）
  awaitingApproval: boolean;
  // 本轮待审批调用（挂起时非空）
  pendingCalls: ToolRequestPayload[];
  // 该会话当前交互主状态 / 子状态（与会话详情 / 列表出参同口径，由后端唯一推导）
  interactionState: string;
  interactionSubstate: string;
  // 是否已锁定、有效轮次与轮次上限
  locked: boolean;
  userTurns: number;
  maxUserTurns: number;
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
  // 通用助手创建会话时使用的 Agent 类型，取值由后端下发
  assistantAgentKind: string;
  // 会话尚未创建（草稿态）时审核阀门下拉的初始值（后端默认阀门），前端据此取初值，
  // 不在前端声明阀门字面量
  defaultReviewValve: string;
  // 当前生效值与出厂值不同的行为参数条数（0 表示全部参数仍为出厂值）
  paramOverrideCount: number;
  // 会话轮次上限（参数 session.maxUserTurns 的生效值；≤0 表示不设上限）：供无会话上下文时展示闸门口径
  maxUserTurns: number;
  // AI 能力不可用的原因说明（按请求语言返回；可用时为空）
  reason: string;
  // 目标会话的在途运行态（无在途运行时为 null），供刷新后前端发现「在跑 / 待审批」并续播
  liveRun?: AgentLiveRunOut | null;
  // 触发自动压缩的占用比例阈值
  compactThresholdRatio: number;
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
  // 该阶段调用的对话模型名（评估阶段共用同一档位模型；检索阶段不带）
  model?: string;
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
