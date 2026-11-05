package assistant

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// Block 是助手消息内的一块内容：Seq 为消息内单调序号（从 1 起），Kind 为块类型，
// Payload 按 Kind 解释（见各 Block*Payload 结构）。
type Block struct {
	// Seq 块在所属消息内的单调序号（从 1 起），即真实产生顺序。
	Seq int `json:"seq"`
	// Kind 块类型。
	Kind BlockKind `json:"kind"`
	// Payload 块的明文 JSON 载荷，形状按 Kind 区分。
	Payload any `json:"payload"`
}

// ReasoningBlockPayload 是一段连续思考文本。
type ReasoningBlockPayload struct {
	// Text 思考文本；实时流中由 block.delta 追加，收敛事件携带终态全文。
	Text string `json:"text"`
}

// ContentBlockPayload 是一段连续正文文本。
type ContentBlockPayload struct {
	// Text 正文文本；实时流中由 block.delta 追加，收敛事件携带终态全文。
	Text string `json:"text"`
}

// ToolCallRecord 是一条工具调用从发起到结果的完整事实。
type ToolCallRecord struct {
	// CallID 本次调用的唯一标识（与模型 tool call id 一致）。
	CallID string `json:"callID"`
	// ToolName 工具名。
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）。
	Arguments string `json:"arguments"`
	// AuditLevel 本条调用的审核重要级。
	AuditLevel contract.AuditLevel `json:"auditLevel"`
	// ExecOwner 执行归属：client 由前端执行，backend 由后端执行。
	ExecOwner contract.ToolExecOwner `json:"execOwner"`
	// Round 该次调用所属的工具轮次（从 1 起）；同一轮并发调用在前端聚为一组。
	Round int `json:"round,omitempty"`
	// Status 调用状态（start / done / failed）。
	Status ToolCallStatus `json:"status"`
	// ElapsedMs 调用耗时（毫秒，仅 done / failed 有值）。
	ElapsedMs int64 `json:"elapsedMs"`
	// Error 调用失败信息（仅 failed 有值）。
	Error string `json:"error,omitempty"`
	// Result 调用结果文本（仅 done 有值）。
	Result string `json:"result,omitempty"`
}

// ToolCallsBlockPayload 是同一轮发起的工具调用组。
type ToolCallsBlockPayload struct {
	// Calls 本轮调用记录；创建时仅首条，其后由 tool.call.updated 按 CallID 增量更新。
	Calls []ToolCallRecord `json:"calls"`
}

// ApprovalCallRecord 是一轮待审批调用的单条记录。
type ApprovalCallRecord struct {
	// CallID 本次调用的唯一标识。
	CallID string `json:"callID"`
	// ToolName 工具名。
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）。
	Arguments string `json:"arguments"`
	// AuditLevel 本條调用的审核重要级。
	AuditLevel contract.AuditLevel `json:"auditLevel"`
	// ApprovalRequired 合成后的最终结论：true 表示需人工审核。
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 执行归属：client 由前端执行，backend 由后端执行。
	ExecOwner contract.ToolExecOwner `json:"execOwner"`
	// Decision 用户决策：缺省表示尚无最终决策，true 批准、false 拒绝。
	Decision *bool `json:"decision,omitempty"`
	// Result 调用结果文本（仅已批准并执行完成时有值）。
	Result string `json:"result,omitempty"`
	// Error 调用失败信息（仅执行失败时有值）。
	Error string `json:"error,omitempty"`
}

// ApprovalBlockPayload 是一轮待审批调用清单。
type ApprovalBlockPayload struct {
	// Calls 本轮待审批调用。
	Calls []ApprovalCallRecord `json:"calls"`
	// CallIDs 卡片上的全部调用 ID（含已决策行），供「重新审批」回退定位。
	CallIDs []string `json:"callIDs,omitempty"`
}

// QuestionnaireBlockPayload 是一轮 ask_user 问卷：题目、作答与是否仍待回答。
type QuestionnaireBlockPayload struct {
	// Questions 本轮题目。
	Questions []contract.QuestionnaireQuestion `json:"questions"`
	// Answers 本轮用户作答；未作答或取消时缺省。
	Answers []contract.QuestionnaireAnswer `json:"answers,omitempty"`
	// Awaiting 本轮是否仍待用户作答。
	Awaiting bool `json:"awaiting"`
	// InterruptIDs 本轮 ask_user 调用的根中断 ID，供「重新回答」回退定位。
	InterruptIDs []string `json:"interruptIDs,omitempty"`
}

// NextStepsBlockPayload 是回答末尾的推荐下一步。
type NextStepsBlockPayload struct {
	// Steps 推荐项，顺序即推荐程度（首条最推荐）。
	Steps []contract.NextStep `json:"steps"`
}

// Usage 是一轮助手消息的用量合计。
type Usage struct {
	// PromptTokens 提示 token 累计。
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 补全 token 累计。
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 合计 token。
	TotalTokens int `json:"totalTokens"`
	// ElapsedMs 本轮总耗时（毫秒）。
	ElapsedMs int64 `json:"elapsedMs"`
}

// CompressState 是上下文压缩的展示态。
type CompressState struct {
	// Phase 压缩阶段（running / done / failed）。
	Phase CompressPhase `json:"phase"`
	// Draft 摘要增量文本（仅 running 有值）。
	Draft string `json:"draft,omitempty"`
	// Summary 压缩完成后的摘要全文（仅 done 有值）：实时取 compress_done，历史回放取压缩检查点，
	// 供前端在折叠态下点击展开查看。
	Summary string `json:"summary,omitempty"`
	// Reason 推进原因（auto / manual / rolling）。
	Reason string `json:"reason,omitempty"`
	// TokensBefore 推进前会话上下文占用 token。
	TokensBefore int `json:"tokensBefore,omitempty"`
	// FoldedCount 本次折叠进摘要的消息条数。
	FoldedCount int `json:"foldedCount,omitempty"`
	// TokensAfter 推进后上下文占用的即时估算 token。
	TokensAfter int `json:"tokensAfter,omitempty"`
}

// PendingCallSnapshot 是历史快照中的一条挂起调用：在 PendingCall 基础上补充锚点消息 ID。
type PendingCallSnapshot struct {
	contract.PendingCall
	// MessageID 该挂起所锚定的助手消息 ID。
	MessageID uint `json:"messageID,omitempty"`
}

// MessageSnapshot 是历史快照中的一条消息：字段与现有会话消息出参一致，并附带终态块与元信息。
type MessageSnapshot struct {
	// ID 消息主键。
	ID uint `json:"id"`
	// Role 消息角色（user / assistant / system）。
	Role string `json:"role"`
	// Content 消息正文：仅作纯文本兜底与复制 / 提问导航用。
	Content string `json:"content"`
	// CreatedAt 创建时间（RFC3339）。
	CreatedAt string `json:"createdAt"`
	// PromptTokens 助手消息的提示 token 累计；用户消息为 0。
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 助手消息的补全 token 累计；用户消息为 0。
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 助手消息的合计 token；用户消息为 0。
	TotalTokens int `json:"totalTokens"`
	// ElapsedMs 助手消息的总耗时（毫秒）；用户消息为 0。
	ElapsedMs int64 `json:"elapsedMs"`
	// Phase 助手消息所属回合的终态阶段；用户消息缺省。
	Phase modelsai.AgentRunPhase `json:"phase,omitempty"`
	// UserMessageIndex 用户消息在会话内的序号（从 1 起），供回退 / 分支定位。
	UserMessageIndex int `json:"userMessageIndex,omitempty"`
	// SkillID 本条用户消息所在轮次选用的技能 ID，0 表示未选技能；仅用户消息有值。
	SkillID uint `json:"skillID,omitempty"`
	// SkillName 本条用户消息所在轮次选用的技能名称快照，空表示未选技能；仅用户消息有值。
	SkillName string `json:"skillName,omitempty"`
	// Citations 检索引用。
	Citations []retrieval.RetrievalResult `json:"citations,omitempty"`
	// Compress 该消息锚定的历史压缩记录。
	Compress *CompressState `json:"compress,omitempty"`
	// Blocks 该助手消息的有序内容块（按 Seq 升序）；用户消息缺省。
	Blocks []Block `json:"blocks,omitempty"`
	// PendingCalls 该助手消息仍待用户输入的挂起调用清单。
	PendingCalls []PendingCallSnapshot `json:"pendingCalls,omitempty"`
}
