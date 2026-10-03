package ai

import (
	// 标准库
	"context"
	"net/http"
	"strings"
	"sync/atomic"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/creasty/defaults"

	// 项目内部

	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// runtimeState 是运行时的一份完整装配态：由配置快照与据此构建的客户端、组件组成。
// 构造后整体只读，因此可经原子指针一次性替换，读取方无需加锁。
type runtimeState struct {
	snapshot *Snapshot                       // 配置快照
	chats    map[ai.Slot]model.BaseChatModel // 对话档位 → 已构建的对话客户端
	embedder embedding.Embedder              // 向量化客户端（未启用时为 nil）
	comps    *Components                     // 已构建的 AI 组件
}

// emptyState 返回初始装配态（未启用、无客户端、仅携带空的参数读取器），供首次装配完成前
// 的安全读取；各读取方法均返回零值或错误，不会 panic。
func emptyState() *runtimeState {
	snap := emptySnapshot()
	return &runtimeState{
		snapshot: snap,
		chats:    map[ai.Slot]model.BaseChatModel{},
		comps:    &Components{Params: snap.Params()},
	}
}

// Runtime 是 AI 助手（RAG + Agent）的运行时装配层：以数据库模型注册表为唯一配置来源，
// 持有一份原子快照（含已构建的客户端与 *Components），在配置变更（etcd 广播）时整体
// 重建。快照重建失败时保留上一可用快照并记录英文错误，绝不 panic、也不以残缺能力降级。
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
func (r *Runtime) Snapshot() *Snapshot {
	st := r.current.Load()
	if st == nil {
		return emptySnapshot()
	}
	return st.snapshot
}

// DisabledReason 返回 AI 能力不可用的原因（按请求语言本地化；可用时为空串），委托当前
// 配置快照（Snapshot.DisabledReason），供功能开关列表向前端说明「为什么功能不可用」。
func (r *Runtime) DisabledReason(lang *base.Language) string {
	return r.Snapshot().DisabledReason(lang)
}

// Params 返回当前行为参数读取器（值全部取自库内 agent_params.value）。
func (r *Runtime) Params() *Params {
	return r.Snapshot().Params()
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

// TestModel 对给定模型端点配置做一次轻量探活（chat 走 GET {baseURL}/models，embedding
// 走一次最小向量化请求），供管理端「测试连接」与依赖状态探测共用。
func (r *Runtime) TestModel(ctx context.Context, cfg llm.ModelConfig) error {
	return TestModel(ctx, cfg)
}

// Start 阻塞运行配置变更监听，直到 ctx 取消（实现 runtime.Service 生命周期）。
// 未绑定 etcd 客户端时直接返回（无监听能力）。
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
func (r *Runtime) build(ctx context.Context) (state *runtimeState, err error) {
	snap, err := r.store.Load(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st := &runtimeState{
		snapshot: snap,
		chats:    map[ai.Slot]model.BaseChatModel{},
		comps:    &Components{Params: snap.Params()},
	}
	if !snap.Enabled() {
		return st, nil
	}
	// 工具装配产出的 MCP 会话句柄由快照持有。若后续构建中途失败，本函数返回错误且新快照不会
	// 被发布（st 不会进入 r.current），此时必须关闭刚建立的连接，避免装配失败时的连接泄漏。
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
	embedCfg, err := snap.ModelConfig(ai.SlotEmbedding)
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
	retriever, err := NewRetriever(r.db, embedder, r.chatResolver(st.chats), snap.Params(), r.logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 会话记忆服务：长期记忆存储（MemoryStore，向量/关键词检索与落库）与服务层 Memory
	// （滚动摘要 / 会话压缩 / 记忆检索），后者注入 Agent 并暴露给视图层的压缩接口。
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
func (r *Runtime) chatResolver(chats map[ai.Slot]model.BaseChatModel) ChatResolver {
	return func(slot ai.Slot) (model.BaseChatModel, error) {
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
