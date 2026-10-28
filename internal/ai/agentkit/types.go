package agentkit

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// Request 是一轮对话的业务上下文。它有意与消息类型无关；turns 通过泛型
// RunContext 注入它。
type Request struct {
	// Query 是当前用户查询。
	Query string
	// NamespaceName 限定知识检索和记忆的命名空间。
	NamespaceName string
	// DocIDs 可选地限定知识检索的文档范围。
	DocIDs []uint
	// SessionID 是持久化的会话 ID（无状态运行时为 0）。
	SessionID uint
	// UserID 是所属用户 ID。
	UserID uint
	// AgentKind 选择提示词和记忆分区。
	AgentKind modelsai.AgentKind
	// BusinessData 是用于提示词的脱敏业务快照。
	BusinessData contract.BusinessData
	// Thinking 为 "on"/"off"/""，用于按请求设置模型选项。
	Thinking string
	// Slot 是实际生效的 LLM 槽位。
	Slot modelsai.Slot
	// ReviewValve 是会话级审批阀门。
	ReviewValve modelsai.ReviewValve
}

// RunContext[M] 会在 Runner.Run 之前注入 ctx。GenModelInput 读取它以
// 组装模型输入（系统提示词、摘要、记忆、历史、查询）。
type RunContext[M adk.MessageType] struct {
	// History 是当前用户消息之前的持久化历史，已规范化为
	// M。
	History []M
	// Request 是业务上下文。
	Request Request
	// Summary 是会话的滚动摘要。
	Summary string
	// Memories 是召回的长时记忆命中项。
	Memories []contract.MemoryHit
}

// MemoryService 是 agentkit 和 turns 所需的
// 记忆/压缩能力。
type MemoryService interface {
	// Recall 按命名空间与查询召回相关长时记忆。
	Recall(ctx context.Context, namespaceName string, agentKind modelsai.AgentKind, sessionID, userID uint, query string) ([]contract.MemoryHit, error)
	// Summarize 为会话生成滚动摘要。
	Summarize(ctx context.Context, session *modelsai.AgentSession, evicted []*modelsai.AgentMessage) (string, uint, error)
	// CompactStream 以流式方式压缩上下文。
	CompactStream(ctx context.Context, session *modelsai.AgentSession, evicted []*modelsai.AgentMessage, reason string, metrics contract.CompactionMetrics, onDelta func(string) error) (*contract.CompactResult, error)
	// Writer 返回长期记忆写入器。
	Writer() tools.MemoryWriter
}

// ModelSet 是按槽位的模型表。Kinds 选择槽位使用的消息表示；Options 提供
// 按请求的模型选项（thinking/额外 body）。
type ModelSet struct {
	// Chat chat 消息表示下的按槽位模型。
	Chat map[modelsai.Slot]model.BaseModel[*schema.Message]
	// Agentic responses 消息表示下的按槽位模型。
	Agentic map[modelsai.Slot]model.BaseModel[*schema.AgenticMessage]
	// Kinds 各槽位使用的消息表示。
	Kinds map[modelsai.Slot]llm.APIMode
	// Options 各槽位的按请求模型选项构造器。
	Options map[modelsai.Slot]func(thinking string) []model.Option
}

// Deps 是 agentkit 的装配依赖集合。
type Deps struct {
	// Models 是按槽位的模型表（必填）。
	Models ModelSet
	// Params 是行为参数读取器（必填）。
	Params *config.Params
	// Memory 是长期记忆服务（可选）。
	Memory MemoryService
	// Logger 是日志器（可选）。
	Logger *logging.Logger
	// Tools 是所有槽位都可用的内置/MCP 工具。
	Tools []tool.BaseTool
	// ToolInfos 是与 Tools 对应的 schema。
	ToolInfos []*schema.ToolInfo
	// Capabilities 返回供 api_invoke 使用的平台能力快照。
	Capabilities func() []capability.Capability
	// ChatResolver 按槽位解析辅助聊天客户端（审核/下一步）。
	ChatResolver func(modelsai.Slot) (model.BaseChatModel, error)
	// Checkpoints 是 eino 检查点存储（中断/恢复所必需）。
	Checkpoints adk.CheckPointStore
	// SlotContextLimit 解析槽位的上下文窗口大小（可选）。
	SlotContextLimit func(modelsai.Slot) int
}

// SlotHandle[M] 是某个槽位构建好的 agent。
type SlotHandle[M adk.MessageType] struct {
	// Slot 是 LLM 槽位。
	Slot modelsai.Slot
	// Kind 是消息表示（对话模型接口形态）。
	Kind llm.APIMode
	// Agent 是 ADK agent。
	Agent adk.TypedAgent[M]
	// Runner 驱动 Run/Resume 并处理检查点。
	Runner *adk.TypedRunner[M]
	// Options 构建按请求的模型选项（thinking/额外 body）。
	Options func(thinking string) []model.Option
}

// NewInput 构建一次运行的 ADK 输入。模型输入消息是历史；GenModelInput
// 会追加当前用户查询和提示词各段落。
func (h *SlotHandle[M]) NewInput(rc RunContext[M]) *adk.TypedAgentInput[M] {
	return &adk.TypedAgentInput[M]{
		Messages:        rc.History,
		EnableStreaming: true,
	}
}
