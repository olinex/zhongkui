package contract

// InteractionKind 标识一条持久化交互（agent_interactions.kind）的类型：审批（approval）、问卷
// （questionnaire）与推荐的下一步（next_steps）。取值与 agent_interactions.kind 列一致。
type InteractionKind string

const (
	// InteractionApproval 审批交互：模型请求调用平台原生接口，等待用户批准 / 拒绝。
	InteractionApproval InteractionKind = "approval"
	// InteractionQuestionnaire 问卷交互：模型通过 ask_user 主动提问，等待用户作答。
	InteractionQuestionnaire InteractionKind = "questionnaire"
	// InteractionNextSteps 推荐的下一步：模型在回答末尾给出可继续追问的方向。
	InteractionNextSteps InteractionKind = "next_steps"
)

// InteractionStatus 是持久化交互（agent_interactions.status）的生命周期状态：待处理（pending）、
// 已作答 / 已决策（answered）与已取消（cancelled）。取值与 agent_interactions.status 列一致。
type InteractionStatus string

const (
	// InteractionPending 待处理：仍在等待用户决策或作答。
	InteractionPending InteractionStatus = "pending"
	// InteractionAnswered 已作答 / 已决策：用户已提交答案或审批决定。
	InteractionAnswered InteractionStatus = "answered"
	// InteractionCancelled 已取消：用户退出了该次交互（以拒绝语义回传），不再回放。
	InteractionCancelled InteractionStatus = "cancelled"
)

// Interaction 是持久化到 agent_interactions 的一条业务交互。Payload 是可 JSON 序列化的业务结构
// （审批调用、问卷问题 / 答案、下一步）。
type Interaction struct {
	// Kind 交互类型（审批 / 问卷 / 下一步）。
	Kind InteractionKind `json:"kind"`
	// Status 交互的生命周期状态。
	Status InteractionStatus `json:"status"`
	// Payload 交互的业务负载（可 JSON 序列化）。
	Payload any `json:"payload"`
	// MessageID 交互所属的助手消息主键（agent_interactions.message_id）：历史回放据此把交互卡片
	// 挂到对应消息之后；0 表示无绑定，历史回放退化为按原始顺序追加在末尾。
	MessageID uint `json:"messageID,omitempty"`
}

// ApprovalPayload 是审批交互的持久化负载。
type ApprovalPayload struct {
	// Calls 待用户审批的工具调用列表。
	Calls []PendingCall `json:"calls"`
	// Decisions 用户已提交的审批决定。
	Decisions []ApprovalDecision `json:"decisions,omitempty"`
}

// NextStepsPayload 是下一步的持久化负载。
type NextStepsPayload struct {
	// Steps 推荐的下一步列表。
	Steps []NextStep `json:"steps"`
}

// ApprovalDecision 是客户端提交的一条审批决定。
type ApprovalDecision struct {
	// CallID 被审批的工具调用 ID。
	CallID string `json:"callID"`
	// InterruptID 该调用对应的中断 ID（可选）。
	InterruptID string `json:"interruptId,omitempty"`
	// Approved 是否批准执行。
	Approved bool `json:"approved"`
	// Reason 决策原因（可选）。
	Reason string `json:"reason,omitempty"`
	// Result 批准后回填的执行结果（可选）。
	Result string `json:"result,omitempty"`
	// Error 拒绝或执行失败时的错误信息（可选）。
	Error string `json:"error,omitempty"`
}

// ActionRequest 是 POST /agent/:sessionID/action 的请求体。
type ActionRequest struct {
	// Action 动作名，取值见下方 Action* 常量。
	Action string `json:"action"`
	// InterruptID 单个中断 ID（可选）。
	InterruptID string `json:"interruptId,omitempty"`
	// InterruptIDs 多个中断 ID（可选）。
	InterruptIDs []string `json:"interruptIds,omitempty"`
	// Decisions 审批决定列表。
	Decisions []ApprovalDecision `json:"decisions,omitempty"`
	// Answers 问卷答案列表。
	Answers []QuestionnaireAnswer `json:"answers,omitempty"`
	// Text 文本内容（如 chat.send 的消息正文）。
	Text string `json:"text,omitempty"`
}

// 服务端与 Web 客户端共用的操作名。
const (
	ActionApprovalDecide      = "approval.decide"
	ActionQuestionnaireSubmit = "questionnaire.submit"
	ActionChatSend            = "chat.send"
	// ActionMessageCopy 前端把上下文中的文本复制到剪贴板（不经过服务端动作接口）
	ActionMessageCopy = "message.copy"
	// ActionMessageRollback 前端从指定用户轮次回退（截断该轮及其之后的消息）
	ActionMessageRollback = "message.rollback"
	// ActionMessageBranch 前端以指定用户轮次为结尾创建会话分支
	ActionMessageBranch = "message.branch"
	// ActionMessageCompact 前端把上下文中的助手消息作为压缩边界发起上下文压缩
	ActionMessageCompact = "message.compact"
)
