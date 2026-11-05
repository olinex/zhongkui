package contract

// ApprovalRequest 是后端审批中间件和 api_invoke 客户端工具
// 产生的类型化中断负载。它通过 eino schema/gob
// 注册，以便在检查点/恢复中保留。
type ApprovalRequest struct {
	// CallID 是模型的工具调用 ID。
	CallID string `json:"callID"`
	// ToolName 是被调用的工具。
	ToolName string `json:"toolName"`
	// Arguments 是规范的 JSON 参数字符串。
	Arguments string `json:"arguments"`
	// AuditLevel 是评估出的审核级别。
	AuditLevel AuditLevel `json:"auditLevel"`
	// ApprovalRequired 是最终的人工审核决定。
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 是 client 或 backend。
	ExecOwner ToolExecOwner `json:"execOwner"`
	// Round 是工具调用轮次。
	Round int `json:"round,omitempty"`
}

// QuestionnaireRequest 是 ask_user 客户端工具
// 产生的类型化中断负载。
type QuestionnaireRequest struct {
	// CallID 是模型的工具调用 ID。
	CallID string `json:"callID"`
	// Questions 是校验后的问卷负载。
	Questions []QuestionnaireQuestion `json:"questions"`
	// Round 是工具调用轮次。
	Round int `json:"round,omitempty"`
	// Arguments 是原始 JSON 参数字符串。
	Arguments string `json:"arguments,omitempty"`
}

// ToolResumeResult 是通过 ResumeParams.Targets 传给挂起工具的类型化
// 恢复数据。
type ToolResumeResult struct {
	// CallID 是模型的工具调用 ID（用于客户端关联）。
	CallID string `json:"callID,omitempty"`
	// Approved 表示用户是否批准执行。
	Approved bool `json:"approved"`
	// Result 是客户端执行结果文本。
	Result string `json:"result,omitempty"`
	// Error 是客户端执行错误文本。
	Error string `json:"error,omitempty"`
	// NoResult 表示客户端未返回任何结果。
	NoResult bool `json:"noResult,omitempty"`
	// Reason 是可选的拒绝原因。
	Reason string `json:"reason,omitempty"`
}
