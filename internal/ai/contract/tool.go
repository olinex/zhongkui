package contract

// CompressEventType 是一次上下文压缩过程的事件类型：开始（compress_start）、摘要增量
// （compress_delta）与完成（compress_done）。仅内部流转使用，不注册枚举类。
type CompressEventType string

// 上下文压缩事件类型常量。
const (
	// CompressEventStart 上下文压缩开始
	CompressEventStart CompressEventType = "compress_start"
	// CompressEventDelta 上下文压缩摘要增量
	CompressEventDelta CompressEventType = "compress_delta"
	// CompressEventDone 上下文压缩完成
	CompressEventDone CompressEventType = "compress_done"
)

// ToolRequest 是一条待执行工具调用请求（对应 SSE tool_request 事件的载荷）：client 工具由前端
// 在用户自身凭据下执行，backend 工具由后端在恢复时自行执行。
type ToolRequest struct {
	// CallID 与模型 tool call id 一致，用于回传时关联
	CallID string `json:"callID"`
	// ToolName 待执行的工具名（模型给出的调用名）
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本，后端不做解释）
	Arguments string `json:"arguments"`
	// AuditLevel 本条调用的审核重要级（由挂起前的批量评估产出）
	AuditLevel AuditLevel `json:"auditLevel"`
	// ApprovalRequired 由审核重要级与会话阀门合成得出的最终结论：true 表示需人工审核
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 执行归属（client / backend）
	ExecOwner ToolExecOwner `json:"execOwner"`
	// MessageID 本次挂起所锚定的助手消息 ID（agent_messages.id）：挂起必定把本回合的助手消息行落为
	// 锚点（问卷挂起写问卷载荷、其余挂起仅置 phase=suspended），故正常挂起恒非零，前端据此在实时会话
	// 内即刻把挂起定位到具体助手消息行（「重新回答」须按它截断其后消息）。仅运行期共享状态缺失的旁路
	// 调用为 0（省略，此时无处可锚）
	MessageID uint `json:"messageID,omitempty"`
}

// ToolRequestReporter 上报一条待执行的工具调用请求；返回错误（写失败/客户端断开）时挂起中止。
type ToolRequestReporter func(ToolRequest) error

// ToolResult 是前端回传的一次客户端工具调用执行结果（对应恢复请求的载荷），按 CallID 与挂起时
// 下发的 tool_request 关联。
type ToolResult struct {
	// CallID 与 tool_request 下发的 callID 一致
	CallID string `json:"callID"`
	// Approved 用户是否批准执行该调用
	Approved bool `json:"approved"`
	// Result 执行结果文本（批准且执行成功时有效）
	Result string `json:"result,omitempty"`
	// Error 执行错误文本（前端执行失败时有效）
	Error string `json:"error,omitempty"`
}

// CompressEvent 是一次上下文压缩过程的事件载荷：Type 决定发布的运行事件类型
// （compress_start / compress_delta / compress_done），其余字段按事件类型择一有值。
type CompressEvent struct {
	// Type 事件类型（compress_start/compress_delta/compress_done），不进入 JSON 载荷
	Type CompressEventType `json:"-"`
	// Reason 压缩原因（auto / manual），start/done 事件携带
	Reason string `json:"reason,omitempty"`
	// Delta 摘要生成的增量文本（仅 compress_delta）
	Delta string `json:"delta,omitempty"`
	// Tokens 压缩前会话占用 token（仅 compress_start）
	Tokens int `json:"tokens,omitempty"`
	// Limit 本次有效档位的上下文容量（仅 compress_start）
	Limit int `json:"limit,omitempty"`
	// Threshold 触发压缩的占用比例阈值（仅 compress_start）
	Threshold float64 `json:"threshold,omitempty"`
	// TokensBefore 压缩前占用 token（仅 compress_done）
	TokensBefore int `json:"tokensBefore,omitempty"`
	// TokensAfter 压缩后占用 token 的即时估算（仅 compress_done）：压缩前占用 − 被折叠内容占用 +
	// 新摘要占用，优先取摘要模型用量算出的估值，用量缺失或其估值不可用时回落为按字节折算的估值
	// （见 estimatedCompactedTokens），亦即压缩完成即刻总能给出一个可展示的值——两种口径都不可用时
	// 才缺省（前端回落为仅展示压缩前占用与折叠条数）。该值仅作即时展示并与检查点同时落库，待在途
	// 或后续生成由供应商测得的真实 prompt tokens 到达后覆盖为精确值。
	TokensAfter int `json:"tokensAfter,omitempty"`
	// FoldedCount 本次折叠的消息条数（仅 compress_done）
	FoldedCount int `json:"foldedCount,omitempty"`
	// Summary 压缩完成后的摘要全文（仅 compress_done）：随终态一并下发，供前端在完成后收起并支持
	// 展开查看，不必依赖增量重新拼接
	Summary string `json:"summary,omitempty"`
	// SummaryUptoSeq 压缩后滚动摘要游标（仅 compress_done）
	SummaryUptoSeq uint `json:"summaryUptoSeq,omitempty"`
}

// CompressReporter 上报一次上下文压缩事件；返回错误（写失败/客户端断开）时压缩过程应中止。
type CompressReporter func(CompressEvent) error
