package ai

import (
	// stdlib
	"context"
	"net/http"
	"strings"
	"sync/atomic"

	// third-party
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/creasty/defaults"

	// internal

	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// runtimeState 是运行时的一份完整装配态：由配置快照与据此构建的客户端、组件组成。
// 构造后整体只读，因此可经原子指针一次性替换，读取方无需加锁。
// runtimeState is one complete assembly state of the runtime: a config snapshot plus the
// clients and components built from it. It is wholly read-only after construction, so it
// can be swapped atomically through a pointer and readers need no locking.
type runtimeState struct {
	snapshot *Snapshot                        // 配置快照
	chats    map[llm.Slot]model.BaseChatModel // 对话档位 → 已构建的对话客户端
	embedder embedding.Embedder               // 向量化客户端（未启用时为 nil）
	comps    *Components                      // 已构建的 AI 组件
}

// emptyState 返回初始装配态（未启用、无客户端、仅携带空的参数读取器），供首次装配完成前
// 的安全读取；各读取方法均返回零值或错误，不会 panic。
// emptyState returns the initial assembly state (disabled, no clients, carrying only the empty
// parameter reader) for safe reads before the first assembly completes; every read returns a
// zero value or an error and never panics.
func emptyState() *runtimeState {
	snap := emptySnapshot()
	return &runtimeState{
		snapshot: snap,
		chats:    map[llm.Slot]model.BaseChatModel{},
		comps:    &Components{Params: snap.Params()},
	}
}

// Runtime 是 AI 助手（RAG + Agent）的运行时装配层：以数据库模型注册表为唯一配置来源，
// 持有一份原子快照（含已构建的客户端与 *Components），在配置变更（etcd 广播）时整体
// 重建。快照重建失败时保留上一可用快照并记录英文错误，绝不 panic、也不以残缺能力降级。
// Runtime is the runtime assembly layer of the AI assistant (RAG + Agent): it takes the
// database model registry as the single config source and holds one atomic snapshot
// (including the built clients and the *Components), wholly rebuilt when the config
// changes (an etcd broadcast). A failed rebuild keeps the previous usable snapshot and
// logs an English error, never panicking and never degrading to a crippled capability.
type Runtime struct {
	db      *database.DB                 // core 库句柄（读取模型注册表、档位绑定与参数覆盖）
	store   *Store                       // 配置读取器
	client  *etcd.EtcdClient             // etcd 客户端（可为 nil，此时无变更监听能力）
	logger  *logging.Logger              // 日志记录器
	watcher *etcd.Watcher                // 变更监听器
	current atomic.Pointer[runtimeState] // 当前装配态（原子替换）
}

// NewRuntime 构造运行时并完成首次装配：读取数据库配置、构建客户端与组件、建立维度
// 强校验，并创建 etcd 变更监听器。首次装配失败（如 embedding 维度不一致、绑定模型
// 端点信息不完整）返回错误，由启动路径中止启动；模型注册表为空时装配为未启用状态而
// 不报错，服务可正常启动等待前端注册模型。
// NewRuntime constructs the runtime and completes the first assembly: it reads the database
// config, builds the clients and components, applies the strict dimension check and creates
// the etcd change watcher. A failed first assembly (e.g. an embedding dimension mismatch or
// an incomplete bound-model endpoint) returns an error so the startup path aborts; an empty
// model registry assembles into the disabled state without error, letting the service start
// and wait for the frontend to register models.
func NewRuntime(ctx context.Context, db *database.DB, client *etcd.EtcdClient, logger *logging.Logger) (*Runtime, error) {
	store, err := NewStore(db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if logger == nil {
		return nil, errors.New("ai runtime: nil logger")
	}
	rt := &Runtime{db: db, store: store, client: client, logger: logger}
	rt.swapState(emptyState())
	st, err := rt.build(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rt.swapState(st)
	rt.watcher = newWatcher(rt, client, logger)
	return rt, nil
}

// Current 返回当前已构建的 AI 组件快照（未启用时返回仅携带参数解析器的空组件）。
// 读取走原子指针，无锁，可被任意 goroutine 并发调用。
// Current returns the currently built AI components snapshot (a components carrying only
// the parameter resolver when disabled). The read goes through the atomic pointer without
// locking and is safe for concurrent use by any goroutine.
func (r *Runtime) Current() *Components {
	st := r.current.Load()
	if st == nil {
		return &Components{}
	}
	return st.comps
}

// Enabled 报告 AI 能力当前是否可用（standard 与 embedding 档位均有可用绑定）。
// Enabled reports whether the AI capability is currently available (both the standard and
// the embedding slot have usable bindings).
func (r *Runtime) Enabled() bool {
	return r.Snapshot().Enabled()
}

// Snapshot 返回当前配置快照（尚未完成首次装配时返回未启用的空快照）。
// Snapshot returns the current config snapshot (a disabled empty snapshot before the first
// assembly completes).
func (r *Runtime) Snapshot() *Snapshot {
	st := r.current.Load()
	if st == nil {
		return emptySnapshot()
	}
	return st.snapshot
}

// DisabledReason 返回 AI 能力不可用的原因（按请求语言本地化；可用时为空串），委托当前
// 配置快照（Snapshot.DisabledReason），供功能开关列表向前端说明「为什么功能不可用」。
// DisabledReason returns the localized reason why the AI capability is unavailable (empty
// when available), delegating to the current config snapshot (Snapshot.DisabledReason) so
// the feature-toggle list can tell the frontend why the feature is unavailable.
func (r *Runtime) DisabledReason(lang *base.Language) string {
	return r.Snapshot().DisabledReason(lang)
}

// Params 返回当前行为参数读取器（值全部取自库内 agent_params.value）。
// Params returns the current behavior-parameter reader (every value comes from
// agent_params.value in the database).
func (r *Runtime) Params() *llm.Params {
	return r.Snapshot().Params()
}

// Chat 按档位返回已构建的对话客户端；档位未绑定或绑定不可用时返回英文错误，调用方
// 据此报告「指定档位不可用」而非静默切换到其他档位。
// Chat returns the built chat client of the slot; an unbound or unusable slot yields an
// English error so the caller reports "the requested slot is unavailable" instead of
// silently switching to another slot.
func (r *Runtime) Chat(slot llm.Slot) (model.BaseChatModel, error) {
	st := r.current.Load()
	if st == nil {
		return nil, errors.New("ai runtime: not initialized")
	}
	if chat, ok := st.chats[slot]; ok && chat != nil {
		return chat, nil
	}
	return nil, errors.Newf("ai runtime: chat slot %q is not bound", slot)
}

// Embed 返回已构建的向量化客户端；embedding 档位未绑定或绑定不可用时返回英文错误。
// Embed returns the built embedding client; an unbound or unusable embedding slot yields
// an English error.
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
// Invalidate re-reads the database config and wholly swaps the current assembly state, for
// use on a config change (an etcd broadcast). A failed rebuild keeps the previous usable
// snapshot and logs an English error (no panic, no crippled degradation) while still
// returning the error so the watcher can record a warning. After a successful swap it closes
// the MCP sessions held by the replaced snapshot (so repeated rebuilds never leak connections);
// a close failure only logs an English warning and never affects the result.
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
// swapState atomically swaps the current assembly state and returns the replaced one (nil on the
// first swap). Extracting it lets Invalidate / NewRuntime / Shutdown share one swap point and lets
// the package tests verify the lifecycle semantics "the replaced snapshot's connections are closed"
// without a database. A nil next is stored as nil per the existing atomic.Pointer semantics, and
// the read methods already handle a nil state safely.
func (r *Runtime) swapState(next *runtimeState) *runtimeState {
	return r.current.Swap(next)
}

// TestModel 对给定模型端点配置做一次轻量探活（chat 走 GET {baseURL}/models，embedding
// 走一次最小向量化请求），供管理端「测试连接」与依赖状态探测共用。
// TestModel performs a lightweight probe of the given model endpoint config (a chat model
// uses GET {baseURL}/models, an embedding model makes one minimal embedding request),
// shared by the admin "test connection" action and the dependency status probes.
func (r *Runtime) TestModel(ctx context.Context, cfg llm.ModelConfig) error {
	return TestModel(ctx, cfg)
}

// Start 阻塞运行配置变更监听，直到 ctx 取消（实现 runtime.Service 生命周期）。
// 未绑定 etcd 客户端时直接返回（无监听能力）。
// Start blocks running the config change watcher until ctx is cancelled (the
// runtime.Service lifecycle). It returns immediately when no etcd client is bound (no
// watching capability).
func (r *Runtime) Start(ctx context.Context) error {
	if r.watcher == nil {
		r.logger.Ctx(ctx).Info("ai change watch skipped: no etcd client")
		return nil
	}
	return r.watcher.Start(ctx)
}

// Shutdown 停止配置变更监听（实现 runtime.Service 生命周期，实际停止依赖 Start 的 ctx
// 取消）；同时满足 samber/do 的 ShutdownerWithContext 约定，由注入器在关闭阶段调用。
// 除既有监听关闭外，再关闭当前快照持有的 MCP 会话（失败只记英文告警）；CloseTools 幂等，
// 故重复调用安全。
// Shutdown stops the config change watcher (the runtime.Service lifecycle; the actual stop
// relies on the Start ctx cancellation). It also satisfies the samber/do
// ShutdownerWithContext contract so the injector drives it during shutdown. Besides the
// existing watcher shutdown it also closes the MCP sessions held by the current snapshot (a
// failure only logs an English warning); CloseTools is idempotent, so a repeated call is safe.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if st := r.current.Load(); st != nil {
		if err := st.snapshot.CloseTools(); err != nil {
			r.logger.Ctx(ctx).Warnf("ai: closing the MCP sessions on shutdown failed: %v", err)
		}
	}
	if r.watcher == nil {
		return nil
	}
	return r.watcher.Shutdown(ctx)
}

// build 依据数据库当前状态装配一份完整运行态：快照 → 已启用工具 → 各对话档位客户端 →
// 向量化客户端 → 会话存储 / 知识库 / 检索器 / Agent。未启用（standard 或 embedding 档位不可用）
// 时仅返回携带参数读取器的空组件，不构建任何客户端，也不拨号任何 MCP 服务器（未启用时工具无
// 意义，且不应产生网络副作用）。工具装配按服务器粒度降级：MCP 端点故障只转为告警而不使装配
// 失败，仅读库失败才返回错误。
// build assembles one complete runtime state from the current database state: snapshot → enabled
// tools → per-slot chat clients → embedding client → session store / knowledge / retriever /
// Agent. When disabled (the standard or the embedding slot is unusable) it returns only the empty
// components carrying the parameter reader, builds no client and dials no MCP server (tools are
// meaningless while disabled and must not cause network side effects). The tool assembly degrades
// per server: an MCP endpoint failure only becomes a warning rather than failing the assembly, and
// only a failed database read returns an error.
func (r *Runtime) build(ctx context.Context) (state *runtimeState, err error) {
	snap, err := r.store.Load(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st := &runtimeState{
		snapshot: snap,
		chats:    map[llm.Slot]model.BaseChatModel{},
		comps:    &Components{Params: snap.Params()},
	}
	if !snap.Enabled() {
		return st, nil
	}
	// 工具装配产出的 MCP 会话句柄由快照持有。若后续构建中途失败，本函数返回错误且新快照不会
	// 被发布（st 不会进入 r.current），此时必须关闭刚建立的连接，避免装配失败时的连接泄漏。
	// The MCP session handles produced by the tool assembly are held by the snapshot. If a later
	// step fails, this function returns an error and the new snapshot is never published (st never
	// enters r.current), so the freshly established connections must be closed to avoid a leak on a
	// failed assembly.
	set, err := r.store.LoadTools(ctx, r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer func() {
		if err != nil {
			_ = set.Close()
		}
	}()
	for _, warning := range set.Warnings {
		r.logger.Ctx(ctx).Warnf("%s", warning)
	}
	snap.mountTools(set)

	for slot, info := range snap.Slots() {
		if !info.Bound || info.Kind == llm.ModelKindEmbedding {
			continue
		}
		cfg, err := snap.ModelConfig(slot)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		chat, err := llm.NewChatClient(ctx, cfg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		st.chats[slot] = chat
	}
	embedCfg, err := snap.ModelConfig(llm.SlotEmbedding)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	embedder, err := llm.NewEmbedClient(ctx, embedCfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st.embedder = embedder

	sessions, err := NewSessionStore(r.db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	knowledge, err := NewKnowledge(r.db, embedder, snap.Params())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 重排与 dry-run 评估在调用点声明使用低档位（flash，见 ai 包的 rerankSlot），故注入
	// 按档位解析对话客户端的回调而非固定客户端；flash 未绑定时由解析回调返回明确英文
	// 错误，绝不静默切换到其他档位。
	// The rerank and the dry-run evaluation declare the low slot (flash, see the rerankSlot
	// constant of the ai package) at their call sites, so the slot-to-chat-client resolver is
	// injected instead of a fixed client; an unbound flash yields an explicit English error
	// from the resolver and never silently switches to another slot.
	retriever, err := NewRetriever(r.db, embedder, r.chatResolver(st.chats), snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 会话记忆服务：长期记忆存储（MemoryStore，向量/关键词检索与落库）与服务层 Memory
	// （滚动摘要 / 会话压缩 / 记忆检索），后者注入 Agent 并暴露给视图层的压缩接口。
	// The session memory service: the long-term memory store (MemoryStore, hybrid retrieval and
	// persistence) and the service layer Memory (rolling summary / session compaction / memory
	// retrieval), the latter injected into the Agent and exposed to the view-layer compaction
	// endpoint.
	memoryStore, err := NewMemoryStore(r.db, embedder, snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	memoryService, err := NewMemory(memoryStore, r.chatResolver(st.chats), snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	agent, err := NewAgent(ctx, r.chatResolver(st.chats), retriever, sessions, snap.Params(), memoryService, r.logger, snap.Tools(), snap.ToolInfos())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st.comps = &Components{
		Agent:     agent,
		Knowledge: knowledge,
		Retriever: retriever,
		Sessions:  sessions,
		Memory:    memoryService,
		Params:    snap.Params(),
		Tools:     snap.Tools(),
	}
	return st, nil
}

// chatResolver 构造按档位解析对话客户端的回调：从本次装配的客户端表取用，档位未绑定
// 时返回英文错误，绝不静默切换到其他档位。
// chatResolver builds the slot-to-chat-client resolver: it reads the client table of this
// assembly and returns an English error for an unbound slot, never silently switching to
// another slot.
func (r *Runtime) chatResolver(chats map[llm.Slot]model.BaseChatModel) ChatResolver {
	return func(slot llm.Slot) (model.BaseChatModel, error) {
		if chat, ok := chats[slot]; ok && chat != nil {
			return chat, nil
		}
		return nil, errors.Newf("ai: chat slot %q is not bound", slot)
	}
}

// TestModel 对给定模型端点配置做一次轻量探活：chat 模型以 GET {baseURL}/models 判定
// 端点可达（OpenAI 兼容端点的标准能力，任何 HTTP 响应即视为已响应，仅传输层错误视为
// 不可达），避免以完整生成请求探活；embedding 模型执行一次最小向量化请求并校验返回
// 向量非空。配置不完整时返回英文错误。
// TestModel performs a lightweight probe of the given model endpoint config: a chat model
// uses GET {baseURL}/models to determine reachability (a standard OpenAI-compatible
// capability; any HTTP response counts as answered and only transport errors count as
// unreachable), avoiding a full generation request; an embedding model makes one minimal
// embedding request and verifies a non-empty vector came back. An incomplete config yields
// an English error.
func TestModel(ctx context.Context, cfg llm.ModelConfig) error {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return errors.New("incomplete model config: baseURL/apiKey/model must all be non-empty")
	}
	// 超时缺省值由 ModelConfig.Timeout 的 default 标签声明，未声明（0）时回填
	if err := defaults.Set(&cfg); err != nil {
		return errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if cfg.Kind == llm.ModelKindEmbedding {
		if _, err := llm.ProbeEmbeddingDimensions(ctx, cfg); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	resp.Body.Close()
	return nil
}
