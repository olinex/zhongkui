package assistant

import (
	"fmt"
	"sync"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// SnapshotBeginPayload 是历史快照开始事件的载荷。
type SnapshotBeginPayload struct {
	// Mode 快照模式；当前固定为 history。
	Mode string `json:"mode"`
}

// SnapshotMessagePayload 是历史快照中一条消息事件的载荷。
type SnapshotMessagePayload struct {
	// Message 消息终态：含内容块、引用、压缩与挂起调用。
	Message MessageSnapshot `json:"message"`
}

// SnapshotEndPayload 是历史快照结束事件的载荷（无字段）。
type SnapshotEndPayload struct{}

// ResumeBeginPayload 是断线续播开始事件的载荷。
type ResumeBeginPayload struct {
	// RunID 正在续播的运行 ID。
	RunID string `json:"runID"`
	// FromSeq 客户端最后看到的游标，服务端重放其后的缓冲事件。
	FromSeq int `json:"fromSeq"`
}

// ResumeResetPayload 是续播游标失效事件的载荷。
type ResumeResetPayload struct {
	// Reason 失效原因（gap 表示游标已超出缓冲区，expired 表示运行已结束）。
	Reason string `json:"reason"`
}

// UserMessageSnapshot 是一次新提问的用户消息事实，前端据此替换本地回显。
type UserMessageSnapshot struct {
	// MessageID 用户消息主键。
	MessageID uint `json:"messageID"`
	// Content 提问正文。
	Content string `json:"content"`
	// CreatedAt 创建时间（RFC3339）。
	CreatedAt string `json:"createdAt"`
	// UserIndex 该提问在会话内的序号（从 1 起）。
	UserIndex int `json:"userIndex"`
	// SkillID 本轮选用的技能 ID，0 表示未选技能。
	SkillID uint `json:"skillID,omitempty"`
	// SkillName 本轮选用的技能名称快照，空表示未选技能。
	SkillName string `json:"skillName,omitempty"`
}

// TurnStartedPayload 是回合开始事件的载荷。
type TurnStartedPayload struct {
	// RunID 本次运行 ID。
	RunID string `json:"runID"`
	// Resumed 是否为审批 / 问卷提交后的续跑；续跑不新建助手气泡。
	Resumed bool `json:"resumed"`
	// AssistantMessageID 续跑或已完成回合的助手消息 ID。
	AssistantMessageID uint `json:"assistantMessageID,omitempty"`
	// UserMessage 新提问的用户消息事实；续跑时缺省。
	UserMessage *UserMessageSnapshot `json:"userMessage,omitempty"`
}

// TurnPreemptedPayload 是回合被抢占事件的载荷。
type TurnPreemptedPayload struct {
	// ByRunID 抢占该回合的新运行 ID。
	ByRunID string `json:"byRunID"`
}

// MessageStartedPayload 是助手消息开始事件的载荷。
type MessageStartedPayload struct {
	// Role 消息角色，当前固定为 assistant。
	Role string `json:"role"`
	// MessageID 助手消息 ID；尚未落库时缺省。
	MessageID uint `json:"messageID,omitempty"`
}

// MessagePersistedPayload 是助手消息落库事件的载荷。
type MessagePersistedPayload struct {
	// MessageID 已落库的助手消息主键。
	MessageID uint `json:"messageID"`
}

// BlockCreatedPayload 是新建内容块事件的载荷。
type BlockCreatedPayload struct {
	// BlockSeq 块在助手消息内的序号（从 1 起）。
	BlockSeq int `json:"blockSeq"`
	// Kind 块类型。
	Kind BlockKind `json:"kind"`
	// Payload 块的初始载荷。
	Payload any `json:"payload"`
}

// BlockDeltaPayload 是思考 / 正文增量事件的载荷。
type BlockDeltaPayload struct {
	// BlockSeq 目标块的序号。
	BlockSeq int `json:"blockSeq"`
	// Kind 块类型（仅 reasoning / content）。
	Kind BlockKind `json:"kind"`
	// Text 本次追加的增量文本。
	Text string `json:"text"`
}

// BlockUpdatedPayload 是块收敛或原位更新事件的载荷。
type BlockUpdatedPayload struct {
	// BlockSeq 目标块的序号。
	BlockSeq int `json:"blockSeq"`
	// Kind 块类型。
	Kind BlockKind `json:"kind"`
	// Payload 更新后的整块载荷。
	Payload any `json:"payload"`
}

// ToolCallUpdatedPayload 是单条工具调用状态变化事件的载荷。
type ToolCallUpdatedPayload struct {
	// BlockSeq 该调用所属 tool_calls 块的序号。
	BlockSeq int `json:"blockSeq"`
	// Call 更新后的调用记录。
	Call ToolCallRecord `json:"call"`
}

// MessageCitationsPayload 是检索引用就绪事件的载荷。
type MessageCitationsPayload struct {
	// Citations 检索引用列表。
	Citations []retrieval.RetrievalResult `json:"citations"`
}

// MessageCompressPayload 是上下文压缩状态事件的载荷。
type MessageCompressPayload struct {
	// Compress 压缩展示态。
	Compress CompressState `json:"compress"`
}

// InteractionWaitingPayload 是进入待审批 / 待作答事件的载荷。
type InteractionWaitingPayload struct {
	// Substate 等待子状态（approval / answer）。
	Substate InteractionSubstate `json:"substate"`
	// InterruptIDs 本轮挂起的根中断 ID。
	InterruptIDs []string `json:"interruptIDs"`
	// BlockSeq 触发等待的块序号，供多轮挂起时定位。
	BlockSeq int `json:"blockSeq,omitempty"`
}

// InteractionResumedPayload 是用户提交交互后续跑事件的载荷。
type InteractionResumedPayload struct {
	// InterruptIDs 已消费的根中断 ID。
	InterruptIDs []string `json:"interruptIDs"`
}

// TurnCompletedPayload 是回合终态事件的载荷。
type TurnCompletedPayload struct {
	// Phase 回合终态阶段（done / failed / cancelled）。
	Phase modelsai.AgentRunPhase `json:"phase"`
	// MessageID 落库后的助手消息 ID。
	MessageID uint `json:"messageID,omitempty"`
	// Usage 本轮用量合计。
	Usage *Usage `json:"usage,omitempty"`
}

// TurnErrorPayload 是请求或生成失败事件的载荷。
type TurnErrorPayload struct {
	// Message 本地化错误文案。
	Message string `json:"message"`
	// Code 可选错误码，供前端区分错误类型。
	Code string `json:"code,omitempty"`
}

// Emitter 生成带版本、游标与时间戳的事件信封；同一实例内 Seq 单调递增。
type Emitter struct {
	// mu 保护 Seq 与 MessageID 的并发读写。
	mu sync.Mutex
	// sessionID 会话 ID。
	sessionID uint
	// runID 实时运行 ID；快照构造时为空。
	runID string
	// messageID 当前事件所属助手消息 ID。
	messageID uint
	// seq 已分配的最大游标。
	seq int
	// now 时间源，便于测试注入。
	now func() time.Time
}

// NewEmitter 创建实时事件构造器：Seq 从 1 开始。
func NewEmitter(sessionID uint, runID string) *Emitter {
	return &Emitter{
		sessionID: sessionID,
		runID:     runID,
		now:       time.Now,
	}
}

// NewSnapshotEmitter 创建历史快照构造器：不携带 runID，Seq 仅用于回放排序。
func NewSnapshotEmitter(sessionID uint) *Emitter {
	return &Emitter{
		sessionID: sessionID,
		now:       time.Now,
	}
}

// SetMessageID 设置后续事件携带的助手消息 ID；落库后回填。
func (e *Emitter) SetMessageID(messageID uint) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messageID = messageID
}

// MessageID 返回当前事件所属助手消息 ID。
func (e *Emitter) MessageID() uint {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.messageID
}

// Seq 返回已分配的最大游标。
func (e *Emitter) Seq() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.seq
}

// Next 生成一条事件：分配游标、写入版本、会话 / 运行 / 消息标识与时间戳。
func (e *Emitter) Next(eventType EventType, payload any) Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq++
	return Event{
		V:         Version,
		Seq:       e.seq,
		Type:      eventType,
		SessionID: e.sessionID,
		RunID:     e.runID,
		MessageID: e.messageID,
		TS:        e.now().UTC(),
		Payload:   payload,
	}
}

// SnapshotBegin 生成历史快照开始事件。
func (e *Emitter) SnapshotBegin() Event {
	e.mu.Lock()
	e.messageID = 0
	e.mu.Unlock()
	return e.Next(EventTypeSnapshotBegin, SnapshotBeginPayload{Mode: "history"})
}

// SnapshotMessage 生成历史快照消息事件：信封 messageID 同步为该消息主键。
func (e *Emitter) SnapshotMessage(message MessageSnapshot) Event {
	e.mu.Lock()
	e.messageID = message.ID
	e.mu.Unlock()
	return e.Next(EventTypeSnapshotMessage, SnapshotMessagePayload{Message: message})
}

// SnapshotEnd 生成历史快照结束事件。
func (e *Emitter) SnapshotEnd() Event {
	e.mu.Lock()
	e.messageID = 0
	e.mu.Unlock()
	return e.Next(EventTypeSnapshotEnd, SnapshotEndPayload{})
}

// BlockCreated 生成新建内容块事件。
func (e *Emitter) BlockCreated(blockSeq int, kind BlockKind, payload any) Event {
	return e.Next(EventTypeBlockCreated, BlockCreatedPayload{
		BlockSeq: blockSeq,
		Kind:     kind,
		Payload:  payload,
	})
}

// BlockDelta 生成思考 / 正文增量事件。
func (e *Emitter) BlockDelta(blockSeq int, kind BlockKind, text string) Event {
	return e.Next(EventTypeBlockDelta, BlockDeltaPayload{
		BlockSeq: blockSeq,
		Kind:     kind,
		Text:     text,
	})
}

// BlockUpdated 生成块收敛或原位更新事件。
func (e *Emitter) BlockUpdated(blockSeq int, kind BlockKind, payload any) Event {
	return e.Next(EventTypeBlockUpdated, BlockUpdatedPayload{
		BlockSeq: blockSeq,
		Kind:     kind,
		Payload:  payload,
	})
}

// ToolCallUpdated 生成单条工具调用状态变化事件。
func (e *Emitter) ToolCallUpdated(blockSeq int, call ToolCallRecord) Event {
	return e.Next(EventTypeToolCallUpdated, ToolCallUpdatedPayload{
		BlockSeq: blockSeq,
		Call:     call,
	})
}

// InteractionWaiting 生成进入待审批 / 待作答事件。
func (e *Emitter) InteractionWaiting(substate InteractionSubstate, interruptIDs []string, blockSeq int) Event {
	return e.Next(EventTypeInteractionWaiting, InteractionWaitingPayload{
		Substate:     substate,
		InterruptIDs: interruptIDs,
		BlockSeq:     blockSeq,
	})
}

// InteractionResumed 生成交互提交后续跑事件。
func (e *Emitter) InteractionResumed(interruptIDs []string) Event {
	return e.Next(EventTypeInteractionResumed, InteractionResumedPayload{InterruptIDs: interruptIDs})
}

// TurnCompleted 生成回合终态事件。
func (e *Emitter) TurnCompleted(phase modelsai.AgentRunPhase, messageID uint, usage *Usage) Event {
	return e.Next(EventTypeTurnCompleted, TurnCompletedPayload{
		Phase:     phase,
		MessageID: messageID,
		Usage:     usage,
	})
}

// TurnError 生成请求或生成失败事件。
func (e *Emitter) TurnError(message, code string) Event {
	return e.Next(EventTypeTurnError, TurnErrorPayload{Message: message, Code: code})
}

// Validate 校验事件的必填字段，供构造与测试使用。
func (e Event) Validate() error {
	if e.V != Version {
		return fmt.Errorf("assistant: unsupported event version %d", e.V)
	}
	if e.Seq <= 0 {
		return fmt.Errorf("assistant: invalid event seq %d", e.Seq)
	}
	if e.Type == "" {
		return fmt.Errorf("assistant: empty event type")
	}
	if e.SessionID == 0 {
		return fmt.Errorf("assistant: empty session id")
	}
	return nil
}
