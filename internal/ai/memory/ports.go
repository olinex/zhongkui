package memory

import (
	// 标准库
	"context"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
)

// Sessions 是记忆服务所需的会话读写能力：摘要压缩时读取待折叠消息、读取上下文用量并写回
// 摘要游标。由 *session.Store 实现，生产装配在根包传入；声明为窄接口使记忆层不反向依赖
// 会话持久化实现。
type Sessions interface {
	// ListMessagesAfter 读取会话内序号大于 afterSeq 的消息
	ListMessagesAfter(ctx context.Context, sessionID uint, namespaceName string, userID uint, afterSeq uint) ([]*ai.AgentMessage, error)
	// ContextTokens 读取会话上下文已测得的 token 总量
	ContextTokens(ctx context.Context, sessionID uint, namespaceName string, userID uint) (int, error)
	// UpdateSummaryAndCheckpoint 写回滚动摘要并推进摘要游标
	UpdateSummaryAndCheckpoint(ctx context.Context, session *ai.AgentSession, summary string, uptoSeq uint, reason string, metrics contract.CompactionMetrics) error
}
