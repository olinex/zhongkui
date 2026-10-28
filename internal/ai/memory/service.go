// 本文件实现长期记忆（agent_memories）的存储与检索：按「命名空间 + Agent 类型 + 会话」归属与
// 隔离（记忆只在其来源会话内可见），写入时做「规范化正文带密钥 HMAC-SHA256 精确去重 + pgvector
// 余弦相似度语义合并」并同步向量列与全文检索列，检索时做「向量路 + 关键词路」混合召回、RRF
// 融合、按参数阈值内容去重后取 topK，命中异步回写使用统计。记忆正文经 crypto 序列化器加密存储
// （列类型 BYTEA）。检索结果的去重复用知识库同包的文本 n-gram Jaccard 逻辑（与写入的余弦去重
// 是两回事，勿混用）。

package memory

import (
	// 标准库
	"context"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// Service 是会话记忆服务：短期记忆（滚动摘要 / 会话压缩）与长期记忆（检索 / 抽取写入）的读
// 写入口。它持有一个 Store（长期记忆落库与检索）、会话存储（摘要读写用）、按档位解析
// 对话客户端的回调与行为参数解析器。视图层的会话压缩接口与 Agent 的滚动摘要均经它执行。
type Service struct {
	// store 长期记忆存储（落库与检索）。
	store *Store
	// sessions 会话读写（摘要压缩用），由装配层注入
	sessions Sessions
	resolver port.ChatResolver // 可空：Summarize/抽取时缺失即返回英文错误
	// params 行为参数解析器（滚动摘要、记忆抽取与记忆开关等）。
	params *config.Params
	logger *logging.Logger // 可空：为 nil 时跳过告警日志
}

// NewService 构造 Service 服务：store/sessions/params 为空时返回英文错误，resolver 与 logger
// 允许为 nil（resolver 缺失在调用 Summarize/抽取时才报错）。会话能力由装配层注入，记忆层不
// 自行构造会话存储。
func NewService(store *Store, sessions Sessions, resolver port.ChatResolver, params *config.Params, logger *logging.Logger) (*Service, error) {
	if store == nil {
		return nil, errors.New("memory: nil store")
	}
	if sessions == nil {
		return nil, errors.New("memory: nil sessions")
	}
	if params == nil {
		return nil, errors.New("memory: nil params")
	}
	return &Service{store: store, sessions: sessions, resolver: resolver, params: params, logger: logger}, nil
}

// warnf 在 logger 非空时输出告警。
func (m *Service) warnf(ctx context.Context, format string, args ...any) {
	if m.logger == nil {
		return
	}
	m.logger.Ctx(ctx).Warnf(format, args...)
}

// Recall 以查询文本在「命名空间 + Agent 类型 + 会话」范围内检索长期记忆，记忆只在其来源会话内
// 可见；语义完全委托 Store.Retrieve（memory.enabled=false 时返回空且不报错）。
func (m *Service) Recall(ctx context.Context, namespaceName string, agentKind ai.AgentKind, sessionID, userID uint, query string) ([]contract.MemoryHit, error) {
	return m.store.Retrieve(ctx, namespaceName, agentKind, sessionID, userID, query)
}

// 以下薄委托方法把长期记忆的管理接口（列表/详情/删除/启停）暴露给视图层，使视图层经
// Components.Service 直接复用 Store 的落库与去重语义，无需自己持有 store 或重写任何
// SQL。语义与参数完全等同 Store 的同名方法。

// List 分页列出命名空间下的长期记忆，语义完全委托 Store.List。
func (m *Service) List(ctx context.Context, q MemoryQuery) ([]*ai.AgentMemory, int64, error) {
	return m.store.List(ctx, q)
}

// Get 读取指定命名空间下属于该用户的一条长期记忆，未找到时返回 ErrMemoryNotFound。
func (m *Service) Get(ctx context.Context, namespaceName string, userID uint, id uint) (*ai.AgentMemory, error) {
	return m.store.Get(ctx, namespaceName, userID, id)
}

// Write 写入一条长期记忆（向量化 + 精确哈希去重 + 语义合并），语义完全委托 Store.Write。
func (m *Service) Write(ctx context.Context, in tools.MemoryWriteInput) (*ai.AgentMemory, error) {
	return m.store.Write(ctx, in)
}

// Delete 软删除指定命名空间下属于该用户的若干长期记忆，语义完全委托 Store.Delete。
func (m *Service) Delete(ctx context.Context, namespaceName string, userID uint, ids []uint) error {
	return m.store.Delete(ctx, namespaceName, userID, ids)
}

// SetStatus 设置指定命名空间下属于该用户的一条长期记忆的状态（active/archived），语义完全委托
// Store.SetStatus。
func (m *Service) SetStatus(ctx context.Context, namespaceName string, userID uint, id uint, status ai.MemoryStatus) error {
	return m.store.SetStatus(ctx, namespaceName, userID, id, status)
}

// ReindexEmbeddings 以当前 embedding 模型为准重算命名空间下全部 active 长期记忆的向量表示，
// 委托长期记忆存储执行（见 Store.ReindexEmbeddings）；report 为进度上报入口，原样透传。
func (m *Service) ReindexEmbeddings(ctx context.Context, namespaceName string, report contract.SyncReporter) (int, error) {
	return m.store.ReindexEmbeddings(ctx, namespaceName, report)
}

// Writer 返回长期记忆写入器（供 memory_write 工具使用），归属信息由运行上下文注入。
func (s *Service) Writer() tools.MemoryWriter {
	return NewWriter(s.store)
}

// NewWriter 构造长期记忆写入器（供 memory_write 工具使用）。
func NewWriter(store *Store) tools.MemoryWriter {
	return memoryStoreWriter{store: store}
}
