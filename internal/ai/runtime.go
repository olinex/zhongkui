package ai

import (
	// 标准库
	"context"
	"sync/atomic"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/change"
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/knowledge"
	"zhongkuisec.com/zhongkui/internal/ai/llmconfig"
	"zhongkuisec.com/zhongkui/internal/ai/llminfra"
	"zhongkuisec.com/zhongkui/internal/ai/memory"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	"zhongkuisec.com/zhongkui/internal/ai/skill"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// RuntimeOption 是运行时的可选装配项，用于注入由 provider 统一管理的可选依赖；
// 不传时运行时按原有能力装配，测试与无依赖场景无需构造这些组件。
type RuntimeOption func(*runtimeOptions)

// runtimeOptions 是可选项的落地结构。
type runtimeOptions struct {
	// jsEngine 是进程内 JS 运行时（mermaid_check 等依赖嵌入式脚本的内置工具使用）
	jsEngine *jsengine.Engine
}

// WithJSEngine 注入 JS 运行时；为空时依赖脚本的内置工具在装配阶段报错，其他能力不受影响。
func WithJSEngine(engine *jsengine.Engine) RuntimeOption {
	return func(options *runtimeOptions) {
		options.jsEngine = engine
	}
}

// runtimeState 是运行时的一份完整装配态：由配置快照与据此构建的客户端、组件组成。
// 构造后整体只读，因此可经原子指针一次性替换，读取方无需加锁。
type runtimeState struct {
	snapshot *llmconfig.Snapshot             // 配置快照
	chats    map[ai.Slot]model.BaseChatModel // 对话档位 → 已构建的 chat 客户端
	agentics map[ai.Slot]model.AgenticModel  // responses 档位 → 已构建的 agentic 客户端
	embedder embedding.Embedder              // 向量化客户端（未启用时为 nil）
	comps    *Components                     // 已构建的 AI 组件
}

// emptyState 返回初始装配态（未启用、无客户端、仅携带空的参数读取器），供首次装配完成前
// 的安全读取；各读取方法均返回零值或错误，不会 panic。
func emptyState() *runtimeState {
	snap := llmconfig.EmptySnapshot()
	return &runtimeState{
		snapshot: snap,
		chats:    map[ai.Slot]model.BaseChatModel{},
		agentics: map[ai.Slot]model.AgenticModel{},
		comps:    &Components{Params: snap.Params()},
	}
}

// Runtime 是 AI 助手（RAG + Agent）的运行时装配层：以数据库模型注册表为唯一配置来源，
// 持有一份原子快照（含已构建的客户端与 *Components），在配置变更（etcd 广播）时整体
// 重建。快照重建失败时保留上一可用快照并记录英文错误，绝不 panic、也不以残缺能力降级。
type Runtime struct {
	db      *database.DB                 // core 库句柄（读取模型注册表、档位绑定与参数覆盖）
	store   *llmconfig.Store             // 配置读取器
	client  *etcd.EtcdClient             // etcd 客户端（可为 nil，此时无变更监听能力）
	logger  *logging.Logger              // 日志记录器
	watcher *etcd.Watcher                // 变更监听器
	current atomic.Pointer[runtimeState] // 当前装配态（原子替换）
	// capabilities 是视图层声明的能力快照，仅供 api_invoke 白名单校验，不注入模型上下文。
	capabilities atomic.Pointer[[]capability.Capability]
	// checkpoints 是 eino checkpoint 的 core 库持久化存储（agent_checkpoints）：挂起现场跨进程 /
	// 重启存活，续跑按同一键恢复；跨装配重建复用。
	checkpoints *agentkit.DBCheckPointStore
}

// SetCapabilities 注入能力快照：能力声明位于视图层，AI 包不反向依赖 manager，故由装配层
// 在 registerViewRoutes 之后调用。该快照只用于 api_invoke 的只读白名单校验，不进入提示词。
// 传入的切片会被复制，调用方后续修改不影响运行时。
func (r *Runtime) SetCapabilities(items []capability.Capability) {
	copied := append([]capability.Capability(nil), items...)
	r.capabilities.Store(&copied)
}

// capabilityItems 返回当前能力快照；尚未注入时返回 nil。
func (r *Runtime) capabilityItems() []capability.Capability {
	items := r.capabilities.Load()
	if items == nil {
		return nil
	}
	return *items
}

// NewRuntime 构造运行时并完成首次装配：读取数据库配置、构建客户端与组件、建立维度
// 强校验，并创建 etcd 变更监听器。首次装配失败（如 embedding 维度不一致、绑定模型
// 端点信息不完整）返回错误，由启动路径中止启动；模型注册表为空时装配为未启用状态而
// 不报错，服务可正常启动等待前端注册模型。
func NewRuntime(ctx context.Context, db *database.DB, client *etcd.EtcdClient, logger *logging.Logger, options ...RuntimeOption) (*Runtime, error) {
	applied := runtimeOptions{}
	for _, option := range options {
		if option != nil {
			option(&applied)
		}
	}
	store, err := llmconfig.NewStore(db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	store.SetJSEngine(applied.jsEngine)
	if logger == nil {
		return nil, errors.New("ai runtime: nil logger")
	}
	checkpoints, err := agentkit.NewDBCheckPointStore(db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rt := &Runtime{db: db, store: store, client: client, logger: logger, checkpoints: checkpoints}
	rt.swapState(emptyState())
	st, err := rt.build(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rt.swapState(st)
	rt.watcher = change.NewWatcher(client, rt.Invalidate, logger)
	return rt, nil
}

// Current 返回当前已构建的 AI 组件快照（未启用时返回仅携带参数解析器的空组件）。
// 读取走原子指针，无锁，可被任意 goroutine 并发调用。
func (r *Runtime) Current() *Components {
	st := r.current.Load()
	if st == nil {
		return &Components{}
	}
	return st.comps
}

// Enabled 报告 AI 能力当前是否可用（standard 与 embedding 档位均有可用绑定）。
func (r *Runtime) Enabled() bool {
	return r.Snapshot().Enabled()
}

// Snapshot 返回当前配置快照（尚未完成首次装配时返回未启用的空快照）。
func (r *Runtime) Snapshot() *llmconfig.Snapshot {
	st := r.current.Load()
	if st == nil {
		return llmconfig.EmptySnapshot()
	}
	return st.snapshot
}

// DisabledReason 返回 AI 能力不可用的原因（按请求语言本地化；可用时为空串），委托当前
// 配置快照（Snapshot.DisabledReason），供功能开关列表向前端说明「为什么功能不可用」。
func (r *Runtime) DisabledReason(lang *base.Language) string {
	return r.Snapshot().DisabledReason(lang)
}

// Params 返回当前行为参数读取器（值全部取自库内 agent_params.value）。
func (r *Runtime) Params() *config.Params {
	return r.Snapshot().Params()
}

// Turns 返回 TurnLoop 会话驱动（随装配态重建），供视图层取用；AI 未启用时为 nil。
func (r *Runtime) Turns() *turns.Manager {
	return r.Current().Turns
}

// Chat 按档位返回已构建的对话客户端；档位未绑定或绑定不可用时返回英文错误，调用方
// 据此报告「指定档位不可用」而非静默切换到其他档位。
func (r *Runtime) Chat(slot ai.Slot) (model.BaseChatModel, error) {
	st := r.current.Load()
	if st == nil {
		return nil, errors.New("ai runtime: not initialized")
	}
	if chat, ok := st.chats[slot]; ok && chat != nil {
		return chat, nil
	}
	return nil, errors.Newf("ai runtime: chat slot %q is not bound", slot)
}

// AgenticChat 按档位返回已构建的 responses 客户端；该档位不是 responses 模式或未绑定时
// 返回英文错误，调用方据此报告「指定档位不可用」而非静默降级为 chat。
func (r *Runtime) AgenticChat(slot ai.Slot) (model.AgenticModel, error) {
	st := r.current.Load()
	if st == nil {
		return nil, errors.New("ai runtime: not initialized")
	}
	if agentic, ok := st.agentics[slot]; ok && agentic != nil {
		return agentic, nil
	}
	return nil, errors.Newf("ai runtime: agentic slot %q is not bound", slot)
}

// Embed 返回已构建的向量化客户端；embedding 档位未绑定或绑定不可用时返回英文错误。
func (r *Runtime) Embed() (embedding.Embedder, error) {
	st := r.current.Load()
	if st == nil || st.embedder == nil {
		return nil, errors.New("ai runtime: embedding model is not available")
	}
	return st.embedder, nil
}

// Invalidate 重新读取数据库配置并整体替换当前装配态，供配置变更（etcd 广播）时调用。
// 重建失败时保留上一可用快照并记录英文错误（不 panic、不残缺降级），同时向上返回错误
// 供监听方记录告警。替换成功后关闭被替换掉的旧快照持有的 MCP 会话（避免重建次数增长导致
// 连接泄漏）；关闭失败只记英文告警，不影响返回值。
func (r *Runtime) Invalidate(ctx context.Context) error {
	st, err := r.build(ctx)
	if err != nil {
		r.logger.Ctx(ctx).Errorf("ai: rebuild failed, keeping the previous snapshot: %v", err)
		return errors.WithStack(err)
	}
	if old := r.swapState(st); old != nil {
		if err := old.snapshot.CloseTools(); err != nil {
			r.logger.Ctx(ctx).Warnf("ai: closing the replaced snapshot's MCP sessions failed: %v", err)
		}
	}
	return nil
}

// swapState 原子替换当前装配态并返回被替换掉的旧装配态（首次替换时为 nil）。抽出它使
// Invalidate / NewRuntime / Shutdown 复用同一替换点，并让同包单测无需数据库即可验证「替换后旧
// 快照的连接被关闭」的生命周期语义。next 为 nil 时按 atomic.Pointer 的既有语义存入 nil，各读取
// 方法对 nil 装配态已做零值安全处理。
func (r *Runtime) swapState(next *runtimeState) *runtimeState {
	return r.current.Swap(next)
}

// TestModel 对给定模型端点配置做一次轻量探活，委托 llminfra.TestModel。
func (r *Runtime) TestModel(ctx context.Context, cfg llm.ModelConfig) error {
	return llminfra.TestModel(ctx, cfg)
}

// Start 阻塞运行配置变更监听，直到 ctx 取消（实现 runtime.Service 生命周期）。
func (r *Runtime) Start(ctx context.Context) error {
	if r.watcher == nil {
		r.logger.Ctx(ctx).Info("ai change watch skipped: no etcd client")
		return nil
	}
	return r.watcher.Start(ctx)
}

// Shutdown 停止配置变更监听（实现 runtime.Service 生命周期，实际停止依赖 Start 的 ctx
// 取消）；同时满足 samber/do 的 ShutdownerWithContext 约定，由注入器在关闭阶段调用。
// 先关闭 TurnLoop 会话驱动（中止全部在途回合并等待终态落库），再关闭当前快照持有的 MCP
// 会话（失败只记英文告警；CloseTools 幂等，故重复调用安全），最后关闭监听。
func (r *Runtime) Shutdown(ctx context.Context) error {
	if turnsManager := r.Current().Turns; turnsManager != nil {
		turnsManager.Shutdown(ctx)
	}
	if st := r.current.Load(); st != nil {
		if err := st.snapshot.CloseTools(); err != nil {
			r.logger.Ctx(ctx).Warnf("ai: closing the MCP sessions on shutdown failed: %v", err)
		}
	}
	var err error
	if r.watcher != nil {
		err = r.watcher.Shutdown(ctx)
	}
	return errors.WithStack(err)
}

// build 依据数据库当前状态装配一份完整运行态：快照 → 已启用工具 → 各对话档位客户端 →
// 向量化客户端 → 会话存储 / 知识库 / 检索器 / Agent。未启用（standard 或 embedding 档位不可用）
// 时仅返回携带参数读取器的空组件，不构建任何客户端，也不拨号任何 MCP 服务器（未启用时工具无
// 意义，且不应产生网络副作用）。工具装配按服务器粒度降级：MCP 端点故障只转为告警而不使装配
// 失败，仅读库失败才返回错误。
func (r *Runtime) build(ctx context.Context) (state *runtimeState, err error) {
	snap, set, err := r.buildSnapshot(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 工具装配产出的 MCP 会话句柄由快照持有。若后续构建中途失败，本函数返回错误且新快照不会
	// 被发布（st 不会进入 r.current），此时必须关闭刚建立的连接，避免装配失败时的连接泄漏。
	defer func() {
		if err != nil && set != nil {
			_ = set.Close()
		}
	}()
	st := &runtimeState{
		snapshot: snap,
		chats:    map[ai.Slot]model.BaseChatModel{},
		agentics: map[ai.Slot]model.AgenticModel{},
		comps:    &Components{Params: snap.Params()},
	}
	if !snap.Enabled() {
		return st, nil
	}
	chats, agentics, models, embedder, err := r.buildChats(ctx, snap)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st.chats = chats
	st.agentics = agentics
	st.embedder = embedder
	comps, err := r.buildComponents(ctx, snap, chats, models, embedder)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st.comps = comps
	return st, nil
}

// buildSnapshot 读取配置快照并装配工具：未启用时不拨号任何 MCP 服务器（返回的 ToolSet 为 nil）。
// 启用时按服务器粒度降级装配，告警逐条记录；读库失败返回错误。
func (r *Runtime) buildSnapshot(ctx context.Context) (*llmconfig.Snapshot, *llmconfig.ToolSet, error) {
	snap, err := r.store.Load(ctx)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if !snap.Enabled() {
		return snap, nil, nil
	}
	set, err := r.store.LoadTools(ctx, r.logger)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	for _, warning := range set.Warnings {
		r.logger.Ctx(ctx).Warnf("%s", warning)
	}
	snap.MountTools(set)
	return snap, set, nil
}

// buildChats 构建各对话档位的 chat / agentic 客户端、agentkit 模型表与向量化客户端。
// 档位为 responses 时以 agentic 客户端为主模型（构建失败即装配失败，绝不静默降级为 chat），
// chat 客户端仅作为辅助任务（审核 / 下一步 / 记忆维护）的尽力选项；档位为 chat 时 chat 客户端
// 是主模型，构建失败同样使装配失败。档位未绑定 embedding 时跳过。
func (r *Runtime) buildChats(ctx context.Context, snap *llmconfig.Snapshot) (map[ai.Slot]model.BaseChatModel, map[ai.Slot]model.AgenticModel, agentkit.ModelSet, embedding.Embedder, error) {
	chats := map[ai.Slot]model.BaseChatModel{}
	agentics := map[ai.Slot]model.AgenticModel{}
	models := agentkit.ModelSet{
		Chat:    map[ai.Slot]model.BaseChatModel{},
		Agentic: map[ai.Slot]model.AgenticModel{},
		Kinds:   map[ai.Slot]llm.APIMode{},
		Options: map[ai.Slot]func(thinking string) []model.Option{},
	}
	// 超时重试策略来自 agent_params（热配置）：参数变更触发运行时重建后，新客户端即采用新策略。
	params := snap.Params()
	timeoutRetry := llm.TimeoutRetryPolicy{
		Enabled:    params.Bool("agent.llmTimeoutRetryEnabled"),
		MaxRetries: params.Int("agent.llmTimeoutRetryMaxRetries"),
		Multiplier: params.Int("agent.llmTimeoutRetryMultiplier"),
	}
	for slot, info := range snap.Slots() {
		if !info.Bound || info.Kind == llm.ModelKindEmbedding {
			continue
		}
		cfg, err := snap.ModelConfig(slot)
		if err != nil {
			return nil, nil, agentkit.ModelSet{}, nil, errors.WithStack(err)
		}
		cfg.TimeoutRetry = timeoutRetry
		responses := cfg.APIMode == llm.APIModeResponses
		chat, chatErr := llm.NewChatClient(ctx, cfg)
		if chatErr != nil && !responses {
			return nil, nil, agentkit.ModelSet{}, nil, errors.WithStack(chatErr)
		}
		if chatErr == nil {
			chats[slot] = chat
			models.Chat[slot] = chat
		}
		if !responses {
			models.Kinds[slot] = llm.APIModeChat
			models.Options[slot] = requestOptionsFn(chat)
			continue
		}
		agentic, err := llm.NewAgenticChatClient(ctx, cfg)
		if err != nil {
			return nil, nil, agentkit.ModelSet{}, nil, errors.WithStack(err)
		}
		agentics[slot] = agentic
		models.Agentic[slot] = agentic
		models.Kinds[slot] = llm.APIModeResponses
		models.Options[slot] = requestOptionsFn(agentic)
	}
	embedCfg, err := snap.ModelConfig(ai.SlotEmbedding)
	if err != nil {
		return nil, nil, agentkit.ModelSet{}, nil, errors.WithStack(err)
	}
	embedder, err := llm.NewEmbedClient(ctx, embedCfg)
	if err != nil {
		return nil, nil, agentkit.ModelSet{}, nil, errors.WithStack(err)
	}
	return chats, agentics, models, embedder, nil
}

// requestOptionsFn 把模型客户端的 RequestOptions 收敛为 agentkit 需要的函数形态；客户端
// 不支持该可选能力时返回空选项（不注入任何高级请求字段）。
func requestOptionsFn(client any) func(thinking string) []model.Option {
	if provider, ok := client.(interface{ RequestOptions(string) []model.Option }); ok {
		return provider.RequestOptions
	}
	return func(string) []model.Option { return nil }
}

// buildComponents 构建持久化与能力层组件并装配 ADK Agent 与 TurnLoop 会话驱动：会话存储、
// 技能存储、知识索引、检索器、记忆服务、Agent 运行时与回合管理器。各组件共享同一份快照参数与
// 对话解析回调，保证热重载后口径一致。
func (r *Runtime) buildComponents(ctx context.Context, snap *llmconfig.Snapshot, chats map[ai.Slot]model.BaseChatModel, models agentkit.ModelSet, embedder embedding.Embedder) (*Components, error) {
	sessions, err := session.NewStore(r.db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	skills, err := skill.NewStore(r.db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kbIndex, err := knowledge.NewKnowledge(r.db, embedder, snap.Params())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 重排与 dry-run 评估在调用点声明使用低档位（flash，见 knowledge 包的 rerankSlot），故注入
	// 按档位解析对话客户端的回调而非固定客户端；flash 未绑定时由解析回调返回明确英文
	// 错误，绝不静默切换到其他档位。
	retriever, err := knowledge.NewRetriever(r.db, embedder, r.chatResolver(chats), snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 会话记忆服务：长期记忆存储（MemoryStore，向量/关键词检索与落库）与服务层 Memory
	// （滚动摘要 / 会话压缩 / 记忆检索），后者注入 Agent 并暴露给视图层的压缩接口。
	memoryStore, err := memory.NewStore(r.db, embedder, snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	memoryService, err := memory.NewService(memoryStore, sessions, r.chatResolver(chats), snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	executor, err := agentkit.New(ctx, agentkit.Deps{
		Models:           models,
		Params:           snap.Params(),
		Memory:           memoryService,
		Logger:           r.logger,
		Tools:            snap.Tools(),
		ToolInfos:        snap.ToolInfos(),
		Capabilities:     r.capabilityItems,
		ChatResolver:     r.chatResolver(chats),
		Checkpoints:      r.checkpoints,
		SlotContextLimit: snap.SlotContextLimit,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	turnsManager, err := turns.New(ctx, turns.Deps{
		Agents: func() *agentkit.Runtime {
			if current := r.Current(); current != nil {
				return current.Agent
			}
			return nil
		},
		Sessions:    sessions,
		Retriever:   retriever,
		Params:      snap.Params(),
		Logger:      r.logger,
		Memory:      memoryService,
		Checkpoints: r.checkpoints,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Components{
		Agent:     executor,
		Turns:     turnsManager,
		Knowledge: kbIndex,
		Retriever: retriever,
		Sessions:  sessions,
		Skills:    skills,
		Memory:    memoryService,
		Params:    snap.Params(),
		Tools:     snap.Tools(),
	}, nil
}

// chatResolver 构造按档位解析对话客户端的回调：从本次装配的客户端表取用，档位未绑定
// 时返回英文错误，绝不静默切换到其他档位。
func (r *Runtime) chatResolver(chats map[ai.Slot]model.BaseChatModel) port.ChatResolver {
	return func(slot ai.Slot) (model.BaseChatModel, error) {
		if chat, ok := chats[slot]; ok && chat != nil {
			return chat, nil
		}
		return nil, errors.Newf("ai: chat slot %q is not bound", slot)
	}
}
