// Package turns 为每个 AI 会话驱动一个 ADK TurnLoop：把用户查询和
// 交互操作转换为 ChatItem，持久化原生消息与交互，并缓冲实时
// 领域事件流（assistant.Event）以支持订阅与断线重连。
package turns

import (
	"encoding/json"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// StreamItem 是轮次缓冲中的一条输出：领域事件。
type StreamItem struct {
	// Event 是领域事件。
	Event *assistant.Event
}

// StreamResult 是对一个 agent 事件流的投影结果（实时投影与历史重建共用）。
type StreamResult[M adk.MessageType] struct {
	// LastContent 是最终 assistant 文本。
	LastContent string
	// Reasoning 是累计的 reasoning 文本。
	Reasoning string
	// Intermediates 是要追加到会话的 assistant/tool 消息。
	Intermediates []M
	// InterruptIDs 是暂停该运行的根中断 ID。
	InterruptIDs []string
	// PendingCalls 是等待客户端执行/审批的调用。
	PendingCalls []contract.PendingCall
	// Interactions 是要持久化的审批/问卷/next_steps 卡片。
	Interactions []contract.Interaction
	// Citations 是运行期间收集的知识引用。
	Citations []retrieval.RetrievalResult
	// Draft 是从最终答案解析出的结构化草稿（如果有）。
	Draft json.RawMessage
	// PromptTokens / CompletionTokens / TotalTokens 是模型用量总计。
	PromptTokens int
	// CompletionTokens 是模型输出 token 总计。
	CompletionTokens int
	// TotalTokens 是模型输入输出 token 总计。
	TotalTokens int
	// Interrupted 表示运行是否因业务中断而结束。
	Interrupted bool
	// Err 是流错误（如果有）。
	Err error
}

func init() {
	// ChatItem 会以 gob 形式存入 TurnLoop 检查点。TurnAction 通过指针字段
	// 引用，因此两者都需要显式注册。
	schema.RegisterName[ChatItem]("zhongkui_turns_chat_item")
	schema.RegisterName[TurnAction]("zhongkui_turns_turn_action")
}

// Deps 是 turn 管理器的装配依赖集合。
type Deps struct {
	// Agents 返回当前的 ADK 运行时；以函数形式提供，便于热重载时替换运行时
	// 而无需重建管理器。
	Agents func() *agentkit.Runtime
	// Sessions 是会话存储（必填）。
	Sessions *session.Store
	// Retriever 是注入知识工具的知识检索器。
	Retriever retriever.Retriever
	// Params 是行为参数读取器。
	Params *config.Params
	// Logger 是日志器（可选）。
	Logger *logging.Logger
	// Memory 是长期记忆/压缩服务（可选）。
	Memory agentkit.MemoryService
	// Checkpoints 是 eino 检查点存储（可选；回退使用
	// 运行时存储）。
	Checkpoints adk.CheckPointStore
}

// ChatInput 启动（或抢占）一次对话轮次。
type ChatInput struct {
	// SessionID 是持久化的会话 ID。
	SessionID uint
	// UserID 是所属用户 ID。
	UserID uint
	// NamespaceName 是会话命名空间。
	NamespaceName string
	// Query 是用户消息文本。
	Query string
	// DocIDs 可选地限定知识检索的文档范围。
	DocIDs []uint
	// BusinessData 是脱敏后的业务快照。
	BusinessData contract.BusinessData
	// Thinking 为 "on"/"off"/""，用于按请求设置模型选项。
	Thinking string
	// Slot 可选地覆盖会话使用的槽位。
	Slot modelsai.Slot
	// SkillID 是本轮选用技能的 ID，0 表示未选技能。
	SkillID uint
	// SkillName 是本轮选用技能的名称，空表示未选技能。
	SkillName string
	// SkillDescription 是本轮选用技能的说明，空表示未提供说明。
	SkillDescription string
	// SkillContent 是本轮选用技能的正文指引，空表示未选技能。
	SkillContent string
	// SystemHint 是本轮附加的系统级约束，注入在系统提示词末尾；空表示不附加。
	// 用于需要固定输出契约的自动化场景（如周期报告要求统一 JSON 信封）。
	SystemHint string
	// Automation 是无人值守运行上下文，仅周期报告等后台任务使用（审核自动通过、
	// api_invoke 服务端回调、ask_user 降级提示）。携带不可序列化的回调，不持久化，
	// 也不会写入 ChatItem / checkpoint。
	Automation *agentkit.Automation
}

// ActionInput 通过审批 / 问卷操作恢复一个挂起的运行。
type ActionInput struct {
	// SessionID 是持久化的会话 ID。
	SessionID uint
	// UserID 是所属用户 ID。
	UserID uint
	// NamespaceName 是会话命名空间。
	NamespaceName string
	// Action 是审批 / 问卷操作负载。
	Action contract.ActionRequest
}

// ReopenInteractionInput 回退到一条已作答 / 已决策的交互（审批或问卷）：删除该交互之后的回答、把该
// 交互重置为待处理，并以挂起时的图现场重建运行现场，使用户可重新选择 / 作答。
type ReopenInteractionInput struct {
	// SessionID 是持久化的会话 ID。
	SessionID uint
	// UserID 是所属用户 ID。
	UserID uint
	// NamespaceName 是会话命名空间。
	NamespaceName string
	// CallIDs 审批卡片上的工具调用 ID（审批回退按调用定位交互）。
	CallIDs []string
	// InterruptIDs 问卷卡片上的根中断 ID（问卷回退按中断定位交互）。
	InterruptIDs []string
}

// Stream 是一次轮次的输出视图：已缓冲的条目加上实时通道。Close 只解除
// 订阅，不会停止该轮次。
type Stream struct {
	// SessionID 是该流所属会话 ID。
	SessionID uint
	// Replay 是已缓冲的条目；对 Chat/Action 是整个轮次缓冲区，对 RenderAssistant 是
	// 持久历史加上当前轮次快照。
	Replay []StreamItem
	// Events 接收快照之后写入的条目。
	Events <-chan StreamItem
	// Close 解除订阅。
	Close func()
	// Resume 仅在 Events 返回时非 nil：描述本次断线续播的起点或失效原因。
	Resume *ResumeInfo
}

// ResumeInfo 描述一次 /events 续播的判定结果。
type ResumeInfo struct {
	// RunID 正在续播的运行 ID。
	RunID string
	// FromSeq 客户端最后看到的游标。
	FromSeq int
	// Reset 非空表示游标失效（gap），客户端应改调 /render 重建；此时不重放缓冲。
	Reset string
}

// State 是会话对外可见的生命周期快照。
type State struct {
	// Running 表示当前是否有轮次正在执行。
	Running bool
	// RunID 是在途轮次的运行标识；无在途运行或尚未建立流时为空。前端据此经 /events?after= 续播。
	RunID string
	// Interrupted 表示会话是否挂起等待输入。
	Interrupted bool
	// PendingInterruptIDs 是等待输入的根中断 ID。
	PendingInterruptIDs []string
	// UserTurns 是会话已持久化的用户查询数量。
	UserTurns int
	// LastErr 是最近一次轮次的错误，最近一次轮次成功时为空。
	LastErr string
}

// ChatItem 是 TurnLoop 的元素类型。每次查询或审批 / 问卷操作都会作为一个
// ChatItem 推入；循环每轮消费一个查询（恢复时消费一个操作）。
//
// 所有字段都是具体类型且可 gob 编码：轮次中断时该元素会持久化到 TurnLoop
// 检查点中。
type ChatItem struct {
	// Query 是用户消息文本。操作项为空。
	Query string
	// UserID 是所属用户 ID。
	UserID uint
	// NamespaceName 是会话命名空间。
	NamespaceName string
	// DocIDs 可选地限定知识检索的文档范围。
	DocIDs []uint
	// BusinessDataJSON 是 JSON 编码的业务快照。
	BusinessDataJSON string
	// Thinking 为 "on"/"off"/""，用于按请求设置模型选项。
	Thinking string
	// Slot 是本轮次实际使用的模型槽位。
	Slot modelsai.Slot
	// SkillID 是本轮选用技能的 ID，0 表示未选技能。
	SkillID uint
	// SkillName 是本轮选用技能的名称，空表示未选技能。
	SkillName string
	// SkillDescription 是本轮选用技能的说明，空表示未提供说明。
	SkillDescription string
	// SkillContent 是本轮选用技能的正文指引，空表示未选技能。
	SkillContent string
	// SystemHint 是本轮附加的系统级约束，注入在系统提示词末尾；空表示不附加。
	SystemHint string
	// Action 携带用于恢复挂起运行的审批/问卷操作。
	Action *TurnAction
	// 当操作本身未携带根中断 ID 时，InterruptID 指定该操作针对的
	// 根中断。
	InterruptID string
}

// TurnAction 是审批 / 问卷操作请求经映射后的 gob 安全形式。
type TurnAction struct {
	// Kind 是操作名（approval.decide / questionnaire.submit）。
	Kind string
	// Targets 把根中断 ID 映射到交给工具的恢复数据。
	Targets map[string]contract.ToolResumeResult
}

// 包级错误哨兵。
var (
	// ErrNoQuery 表示不带文本的对话请求。
	ErrNoQuery = errors.New("turns: empty query")
	// ErrNoPendingRun 表示没有挂起 agent 运行的操作。
	ErrNoPendingRun = errors.New("turns: no pending agent run")
	// ErrCheckpointMissing 表示恢复时检查点已不存在。
	ErrCheckpointMissing = errors.New("turns: resume checkpoint is missing")
	// ErrNoActionItem 表示恢复尝试缺少操作项。
	ErrNoActionItem = errors.New("turns: no action item for resume")
	// ErrNoResumeTargets 表示操作未映射到任何中断目标。
	ErrNoResumeTargets = errors.New("turns: action produced no resume target")
	// ErrLoopClosed 表示向已退出的 TurnLoop 推入元素。
	ErrLoopClosed = errors.New("turns: session loop is closed")
	// ErrManagerClosed 表示管理器关闭后的操作。
	ErrManagerClosed = errors.New("turns: manager is shut down")
	// ErrRedoUnavailable 表示审批已无法回退到待审状态（挂起时的图现场快照缺失、目标交互不存在，
	// 或回退点已被上下文摘要覆盖）。
	ErrRedoUnavailable = errors.New("turns: approval cannot be rewound")
)
