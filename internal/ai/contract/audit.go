package contract

// AuditLevel 是工具调用的审核重要级标识：由评估子系统在挂起前对本次全部待执行调用分类产出，
// 取值 must / suggest / auto / none，依次对应「必须审核 / 建议审核 / 可自动审核 / 无需审核」。
// 与会话级审核阀门（ReviewValve）合成后决定调用是否需人工审核。
type AuditLevel string

// 审核重要级常量。
const (
	// AuditLevelMust 必须审核
	AuditLevelMust AuditLevel = "must"
	// AuditLevelSuggest 建议审核
	AuditLevelSuggest AuditLevel = "suggest"
	// AuditLevelAuto 可自动审核
	AuditLevelAuto AuditLevel = "auto"
	// AuditLevelNone 无需审核
	AuditLevelNone AuditLevel = "none"
)

// AuditLevels 是审核重要级的取值空间，声明顺序即枚举类条目顺序。
var AuditLevels = []AuditLevel{
	AuditLevelMust,
	AuditLevelSuggest,
	AuditLevelAuto,
	AuditLevelNone,
}

// ToolExecOwner 标识一次挂起工具调用的执行归属：client 由前端在用户自身凭据下执行，backend 由
// 后端在恢复时自行执行（builtin / MCP 工具，仅在审核重要级为 must 时挂起等审批）。
type ToolExecOwner string

// 执行归属常量。
const (
	// ToolExecOwnerClient 前端执行（既有 client 工具语义）
	ToolExecOwnerClient ToolExecOwner = "client"
	// ToolExecOwnerBackend 后端执行（builtin / MCP 工具，经 must 级审批后由后端执行）
	ToolExecOwnerBackend ToolExecOwner = "backend"
)

// PendingCall 是一条挂起等待人工审批 / 前端执行的工具调用：既作为挂起现场（agent_runs）中
// pending_calls 的 JSON 形状，也作为交互载荷与接口出参的调用描述，全模块唯一权威定义。
type PendingCall struct {
	// CallID 与模型 tool call id 一致
	CallID string `json:"callID"`
	// ToolName 待执行的工具名
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）
	Arguments string `json:"arguments"`
	// ExecOwner 执行归属（client / backend）
	ExecOwner ToolExecOwner `json:"execOwner"`
	// AuditLevel 本条调用的审核重要级（挂起前批量评估产出，随运行现场落库留痕）
	AuditLevel AuditLevel `json:"auditLevel"`
	// ApprovalRequired 由审核重要级与会话阀门合成得出的最终结论：true 表示需人工审核
	ApprovalRequired bool `json:"approvalRequired"`
	// Round 该次调用所属的轮次（从 1 起，挂起前由工具循环写入）；恢复续跑时据此让阶段事件沿用
	// 同一轮次，使同一轮内并发发起的调用在前端仍聚为一组
	Round int `json:"round,omitempty"`
	// InterruptID 是 eino ADK 为该次挂起分配的 interrupt 地址（ResumeParams.Targets 的键）。
	// 仅用于挂起现场与续跑映射，不直接下发给前端。
	InterruptID string `json:"interruptID,omitempty"`
}

// ToPendingCalls 把挂起现场中的待执行调用转为回传列表：审核重要级与执行归属原样透传；
// 审批合成结论未随现场持久化，问卷类调用由前端按工具名识别、豁免审批，其余调用保守置为
// 「需人工审批」，避免回退后未经用户决策即自动执行。
func ToPendingCalls(pending []PendingCall) []PendingCall {
	calls := make([]PendingCall, 0, len(pending))
	for _, call := range pending {
		calls = append(calls, PendingCall{
			CallID:           call.CallID,
			ToolName:         call.ToolName,
			Arguments:        call.Arguments,
			AuditLevel:       call.AuditLevel,
			ExecOwner:        call.ExecOwner,
			ApprovalRequired: !IsQuestionnaireTool(call.ToolName),
			InterruptID:      call.InterruptID,
		})
	}
	return calls
}
