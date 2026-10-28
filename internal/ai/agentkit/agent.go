package agentkit

import (
	"context"
	"sort"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/audit"
	"zhongkuisec.com/zhongkui/internal/ai/config"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// Runtime 是 agentkit 的装配态：每个对话档位一个 ADK Agent + Runner，以及审核评估器、
// checkpoint 存储与行为参数。构造后整体只读，可被 turns 并发取用。
type Runtime struct {
	// deps 装配依赖集合。
	deps Deps
	// params 行为参数读取器。
	params *config.Params
	// memory 长期记忆与上下文压缩服务。
	memory MemoryService
	// logger 日志器。
	logger *logging.Logger

	// evaluator 工具调用审核重要级评估器
	evaluator *audit.Evaluator
	// tools 本次装配的全部工具（builtin / MCP / client）
	tools []tool.BaseTool
	// toolInfos builtin / MCP 工具 schema 索引（审核评估按名取用途说明）
	toolInfos map[string]*schema.ToolInfo

	// maxToolRounds 工具调用轮数上限
	maxToolRounds int
	// maxToolResultBytes 单次工具结果字节上限（非正数不截断）
	maxToolResultBytes int

	// kinds 各档位的消息表示
	kinds map[modelsai.Slot]llm.APIMode
	// chatHandles chat 档位
	chatHandles map[modelsai.Slot]*SlotHandle[*schema.Message]
	// agenticHandles responses 档位
	agenticHandles map[modelsai.Slot]*SlotHandle[*schema.AgenticMessage]
}

// New 构造 agentkit Runtime：校验依赖、为每个档位构建一个 TypedChatModelAgent 与 Runner。
func New(ctx context.Context, deps Deps) (*Runtime, error) {
	if deps.Params == nil {
		return nil, errors.New("agentkit: nil params")
	}
	if len(deps.Models.Kinds) == 0 && len(deps.Models.Chat) == 0 && len(deps.Models.Agentic) == 0 {
		return nil, errors.New("agentkit: no model slots configured")
	}

	toolInfoIndex := make(map[string]*schema.ToolInfo, len(deps.ToolInfos))
	for _, info := range deps.ToolInfos {
		if info == nil {
			continue
		}
		toolInfoIndex[info.Name] = info
	}

	rt := &Runtime{
		deps:               deps,
		params:             deps.Params,
		memory:             deps.Memory,
		logger:             deps.Logger,
		tools:              append(append([]tool.BaseTool(nil), deps.Tools...), &askUserTool{}, &apiInvokeTool{capabilities: deps.Capabilities}),
		toolInfos:          toolInfoIndex,
		maxToolRounds:      deps.Params.Int("agent.maxToolRounds"),
		maxToolResultBytes: deps.Params.Int("agent.maxToolResultBytes"),
		kinds:              map[modelsai.Slot]llm.APIMode{},
		chatHandles:        map[modelsai.Slot]*SlotHandle[*schema.Message]{},
		agenticHandles:     map[modelsai.Slot]*SlotHandle[*schema.AgenticMessage]{},
	}
	rt.evaluator = audit.New(audit.Deps{
		Chats:     deps.ChatResolver,
		Params:    deps.Params,
		Logger:    deps.Logger,
		ToolInfos: toolInfoIndex,
	})
	if rt.maxToolRounds <= 0 {
		rt.maxToolRounds = 1
	}

	for _, slot := range orderedSlots(deps.Models) {
		kind, ok := rt.resolveKind(slot)
		if !ok {
			continue
		}
		rt.kinds[slot] = kind
		switch kind {
		case llm.APIModeResponses:
			handle, err := rt.buildAgenticHandle(ctx, slot)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			rt.agenticHandles[slot] = handle
		case llm.APIModeChat:
			handle, err := rt.buildChatHandle(ctx, slot)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			rt.chatHandles[slot] = handle
		}
	}
	if len(rt.chatHandles)+len(rt.agenticHandles) == 0 {
		return nil, errors.New("agentkit: no chat model slots could be built")
	}
	return rt, nil
}

// orderedSlots 按固定档位顺序列出需要构建的槽位，并追加注册表中未在固定清单内的额外档位。
func orderedSlots(models ModelSet) []modelsai.Slot {
	seen := make(map[modelsai.Slot]bool, len(models.Kinds))
	var slots []modelsai.Slot
	for _, slot := range modelsai.AllSlots {
		if slot == modelsai.SlotEmbedding {
			continue
		}
		if _, ok := models.Kinds[slot]; ok {
			slots = append(slots, slot)
			seen[slot] = true
			continue
		}
		if _, ok := models.Chat[slot]; ok {
			slots = append(slots, slot)
			seen[slot] = true
			continue
		}
		if _, ok := models.Agentic[slot]; ok {
			slots = append(slots, slot)
			seen[slot] = true
		}
	}
	var extra []modelsai.Slot
	for slot := range models.Kinds {
		if !seen[slot] {
			extra = append(extra, slot)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	return append(slots, extra...)
}

// resolveKind 返回档位的消息表示：优先取 ModelSet.Kinds，其次按已构建模型表推断。
func (r *Runtime) resolveKind(slot modelsai.Slot) (llm.APIMode, bool) {
	if kind, ok := r.deps.Models.Kinds[slot]; ok && kind != "" {
		return kind, true
	}
	if _, ok := r.deps.Models.Chat[slot]; ok {
		return llm.APIModeChat, true
	}
	if _, ok := r.deps.Models.Agentic[slot]; ok {
		return llm.APIModeResponses, true
	}
	return "", false
}

// buildChatHandle 构建一个 chat 档位。
func (r *Runtime) buildChatHandle(ctx context.Context, slot modelsai.Slot) (*SlotHandle[*schema.Message], error) {
	base := r.deps.Models.Chat[slot]
	if base == nil {
		return nil, errors.Newf("agentkit: chat slot %q has no model", slot)
	}
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.Message]{
		Name:          agentName(slot),
		Description:   "zhongkui AI assistant (" + string(slot) + ")",
		Model:         base,
		ToolsConfig:   r.toolsConfig(),
		GenModelInput: newGenModelInput[*schema.Message](r),
		MaxIterations: r.maxIterations(),
		Handlers:      r.handlers[*schema.Message](false),
		ModelRetryConfig: &adk.TypedModelRetryConfig[*schema.Message]{
			MaxRetries:  modelRetryAttempts,
			IsRetryAble: func(_ context.Context, err error) bool { return isTransientModelError(err) },
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "agentkit: build chat agent for slot %q", slot)
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: r.deps.Checkpoints,
	})
	return &SlotHandle[*schema.Message]{
		Slot:    slot,
		Kind:    llm.APIModeChat,
		Agent:   agent,
		Runner:  runner,
		Options: r.modelOptionsFn(slot),
	}, nil
}

// buildAgenticHandle 构建一个 responses 档位。
func (r *Runtime) buildAgenticHandle(ctx context.Context, slot modelsai.Slot) (*SlotHandle[*schema.AgenticMessage], error) {
	base := r.deps.Models.Agentic[slot]
	if base == nil {
		return nil, errors.Newf("agentkit: responses slot %q has no model", slot)
	}
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:          agentName(slot),
		Description:   "zhongkui AI assistant (" + string(slot) + ")",
		Model:         base,
		ToolsConfig:   r.toolsConfig(),
		GenModelInput: newGenModelInput[*schema.AgenticMessage](r),
		MaxIterations: r.maxIterations(),
		Handlers:      r.handlers[*schema.AgenticMessage](true),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "agentkit: build responses agent for slot %q", slot)
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: r.deps.Checkpoints,
	})
	return &SlotHandle[*schema.AgenticMessage]{
		Slot:    slot,
		Kind:    llm.APIModeResponses,
		Agent:   agent,
		Runner:  runner,
		Options: r.modelOptionsFn(slot),
	}, nil
}

// toolsConfig 返回所有档位共享的工具配置（builtin / MCP + client 工具）。
func (r *Runtime) toolsConfig() adk.ToolsConfig {
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{Tools: r.tools},
	}
}

// handlers 返回某个消息表示的中间件链：韧性 / 审核 / 工具结果包装（注册顺序即外层到内层）。
// agentic 模型没有原生 ModelRetryConfig，由韧性中间件承担瞬时重试；chat 由原生配置承担。
func (r *Runtime) handlers[M adk.MessageType](retry bool) []adk.TypedChatModelAgentMiddleware[M] {
	return []adk.TypedChatModelAgentMiddleware[M]{
		newResilientMiddleware[M](retry),
		newAuditMiddleware[M](r.evaluator, r.logger),
		newToolExecMiddleware[M](r.maxToolResultBytes),
	}
}

// adkIterationCeiling 是交给 ADK 的生成循环上界：ADK 的剩余轮数随检查点跨挂起累计，会把同一次
// 用户提问内的多轮人工审批也算进额度，导致「只有一轮对话、交互较多」时误触上限。实际工具轮数
// 改由 RunOptions 在每次 run 段内单独计数（见 maxToolRounds），故这里只保留一个足够大的硬上界。
const adkIterationCeiling = 1 << 20

// maxIterations 返回 ADK 的生成循环硬上界；真正的工具轮数约束由 RunOptions 施加。
func (r *Runtime) maxIterations() int {
	return adkIterationCeiling
}

// RunOptions 返回一次 agent run 的工具轮数闸门：只统计本次 run 内已完成的工具轮，达到
// agent.maxToolRounds 后以 ErrExceedMaxIterations 中止。人工审批 / 问卷挂起时工具轮并未完成，
// 续跑会开启新的 run 段并重新计数，因此交互次数不会耗尽额度；纯自主循环仍受该上限约束。
func (r *Runtime) RunOptions() []adk.AgentRunOption {
	limit := r.maxToolRounds
	if limit <= 0 {
		limit = 1
	}
	rounds := 0
	return []adk.AgentRunOption{adk.WithAfterToolCallsHook(func(context.Context) error {
		rounds++
		if rounds > limit {
			return adk.ErrExceedMaxIterations
		}
		return nil
	})}
}

// agentName 返回档位稳定的 Agent 名。
func agentName(slot modelsai.Slot) string {
	return "zhongkui-ai-" + string(slot)
}

// Kind 返回档位的消息表示；未知档位返回 false。
func (r *Runtime) Kind(slot modelsai.Slot) (llm.APIMode, bool) {
	if r == nil {
		return "", false
	}
	kind, ok := r.kinds[slot]
	return kind, ok
}

// ChatHandle 返回 chat 档位的构建结果；该档位不是 chat 时返回 false。
func (r *Runtime) ChatHandle(slot modelsai.Slot) (*SlotHandle[*schema.Message], bool) {
	if r == nil {
		return nil, false
	}
	handle, ok := r.chatHandles[slot]
	return handle, ok
}

// AgenticHandle 返回 responses 档位的构建结果；该档位不是 responses 时返回 false。
func (r *Runtime) AgenticHandle(slot modelsai.Slot) (*SlotHandle[*schema.AgenticMessage], bool) {
	if r == nil {
		return nil, false
	}
	handle, ok := r.agenticHandles[slot]
	return handle, ok
}

// Checkpoints 返回 eino checkpoint 存储。
func (r *Runtime) Checkpoints() adk.CheckPointStore {
	if r == nil {
		return nil
	}
	return r.deps.Checkpoints
}

// Params 返回行为参数读取器。
func (r *Runtime) Params() *config.Params {
	if r == nil {
		return nil
	}
	return r.params
}

// Memory 返回长期记忆/压缩服务。
func (r *Runtime) Memory() MemoryService {
	if r == nil {
		return nil
	}
	return r.memory
}

// Logger 返回日志记录器。
func (r *Runtime) Logger() *logging.Logger {
	if r == nil {
		return nil
	}
	return r.logger
}

// ContextLimit 返回档位的上下文窗口容量；未注入容量回调时返回 0。
func (r *Runtime) ContextLimit(slot modelsai.Slot) int {
	if r == nil || r.deps.SlotContextLimit == nil {
		return 0
	}
	return r.deps.SlotContextLimit(slot)
}

// ModelOptions 返回档位的每请求模型选项（思考开关 / extra body）。
func (r *Runtime) ModelOptions(slot modelsai.Slot, thinking string) []model.Option {
	if r == nil || r.deps.Models.Options == nil {
		return nil
	}
	optionFn, ok := r.deps.Models.Options[slot]
	if !ok || optionFn == nil {
		return nil
	}
	return optionFn(thinking)
}

// modelOptionsFn 把 ModelOptions 收拢为 SlotHandle 需要的函数形态。
func (r *Runtime) modelOptionsFn(slot modelsai.Slot) func(thinking string) []model.Option {
	return func(thinking string) []model.Option {
		return r.ModelOptions(slot, thinking)
	}
}

// warnf 在 logger 非空时输出告警。
func (r *Runtime) warnf(ctx context.Context, format string, args ...any) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Ctx(ctx).Warnf(format, args...)
}
