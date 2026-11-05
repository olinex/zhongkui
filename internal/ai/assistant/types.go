package assistant

import "time"

// Version 是当前事件协议版本；破坏性变更时递增，客户端据其决定是否降级。
const Version = 1

// EventType 是 AI 助手 SSE 的事件类型标识。
type EventType string

// 事件类型常量：与协议文档「事件目录」一一对应。
const (
	// EventTypeSnapshotBegin 历史快照开始：客户端清空消息列表与运行状态。
	EventTypeSnapshotBegin EventType = "snapshot.begin"
	// EventTypeSnapshotMessage 历史快照中的一条消息（含终态块与元信息）。
	EventTypeSnapshotMessage EventType = "snapshot.message"
	// EventTypeSnapshotEnd 历史快照结束：客户端结束加载态。
	EventTypeSnapshotEnd EventType = "snapshot.end"
	// EventTypeResumeBegin 断线续播开始：服务端即将重放 seq 之后的事件。
	EventTypeResumeBegin EventType = "resume.begin"
	// EventTypeResumeReset 续播游标已失效：客户端应重新调用 /render 获取快照。
	EventTypeResumeReset EventType = "resume.reset"
	// EventTypeTurnStarted 新一轮或续跑开始。
	EventTypeTurnStarted EventType = "turn.started"
	// EventTypeTurnPreempted 旧回合被新提问抢占。
	EventTypeTurnPreempted EventType = "turn.preempted"
	// EventTypeMessageStarted 助手消息开始，前端据此创建气泡。
	EventTypeMessageStarted EventType = "message.started"
	// EventTypeMessagePersisted 助手消息已落库，前端回填消息主键。
	EventTypeMessagePersisted EventType = "message.persisted"
	// EventTypeBlockCreated 新建内容块。
	EventTypeBlockCreated EventType = "block.created"
	// EventTypeBlockDelta 思考 / 正文增量。
	EventTypeBlockDelta EventType = "block.delta"
	// EventTypeBlockUpdated 块收敛或非工具块的原位更新。
	EventTypeBlockUpdated EventType = "block.updated"
	// EventTypeToolCallUpdated 单条工具调用状态变化。
	EventTypeToolCallUpdated EventType = "tool.call.updated"
	// EventTypeMessageCitations 检索引用就绪。
	EventTypeMessageCitations EventType = "message.citations"
	// EventTypeMessageCompress 上下文压缩开始、增量或完成。
	EventTypeMessageCompress EventType = "message.compress"
	// EventTypeInteractionWaiting 进入待审批 / 待作答。
	EventTypeInteractionWaiting EventType = "interaction.waiting"
	// EventTypeInteractionResumed 用户提交交互后回合继续。
	EventTypeInteractionResumed EventType = "interaction.resumed"
	// EventTypeTurnCompleted 回合到达终态。
	EventTypeTurnCompleted EventType = "turn.completed"
	// EventTypeTurnError 请求或生成失败。
	EventTypeTurnError EventType = "turn.error"
)

// Event 是 SSE 的单条事件信封：data 行即该结构的 JSON。
type Event struct {
	// V 协议版本。
	V int `json:"v"`
	// Seq 实时游标：同一 RunID 内从 1 单调递增；历史快照内为回放序号。
	Seq int `json:"seq"`
	// Type 事件类型。
	Type EventType `json:"type"`
	// SessionID 会话 ID。
	SessionID uint `json:"sessionID"`
	// RunID 实时运行 ID；历史快照不携带。
	RunID string `json:"runID,omitempty"`
	// MessageID 事件所属助手消息 ID；尚未落库时缺省。
	MessageID uint `json:"messageID,omitempty"`
	// TS 服务端时间（JSON 序列化为 RFC3339）。
	TS time.Time `json:"ts"`
	// Payload 事件载荷：按 Type 解释。
	Payload any `json:"payload,omitempty"`
}

// BlockKind 是助手消息内容块的类型。
type BlockKind string

// 内容块类型常量：与前端 AgentBlockKind 及持久化块模型一致。
const (
	// BlockKindReasoning 一段连续思考文本。
	BlockKindReasoning BlockKind = "reasoning"
	// BlockKindContent 一段连续正文文本。
	BlockKindContent BlockKind = "content"
	// BlockKindToolCalls 同一轮发起的工具调用组。
	BlockKindToolCalls BlockKind = "tool_calls"
	// BlockKindApproval 一轮待审批调用清单。
	BlockKindApproval BlockKind = "approval"
	// BlockKindQuestionnaire 一轮 ask_user 问卷。
	BlockKindQuestionnaire BlockKind = "questionnaire"
	// BlockKindNextSteps 回答末尾的推荐下一步。
	BlockKindNextSteps BlockKind = "next_steps"
)

// ToolCallStatus 是工具调用的生命周期状态。
type ToolCallStatus string

// 工具调用状态常量。
const (
	// ToolCallStatusStart 调用已发起、结果尚未返回。
	ToolCallStatusStart ToolCallStatus = "start"
	// ToolCallStatusDone 调用成功完成。
	ToolCallStatusDone ToolCallStatus = "done"
	// ToolCallStatusFailed 调用失败。
	ToolCallStatusFailed ToolCallStatus = "failed"
)

// InteractionSubstate 是会话等待用户输入的子状态。
type InteractionSubstate string

// 交互子状态常量：审批等待 / 问卷作答等待。
const (
	// InteractionSubstateApproval 等待人工审批。
	InteractionSubstateApproval InteractionSubstate = "approval"
	// InteractionSubstateAnswer 等待问卷作答。
	InteractionSubstateAnswer InteractionSubstate = "answer"
)

// CompressPhase 是上下文压缩的展示阶段。
type CompressPhase string

// 压缩阶段常量。
const (
	// CompressPhaseRunning 压缩进行中（摘要增量流式产出）。
	CompressPhaseRunning CompressPhase = "running"
	// CompressPhaseDone 压缩完成。
	CompressPhaseDone CompressPhase = "done"
	// CompressPhaseFailed 压缩失败。
	CompressPhaseFailed CompressPhase = "failed"
)
