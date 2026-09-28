package ai

import (
	// third-party
	"gorm.io/datatypes"
	"gorm.io/gorm"

	// internal
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// AgentSession 表示一个 AI 助手会话，聚合同一话题下的多轮问答消息
// （AgentMessage），并记录会话缺省档位与状态。对应表为 agent_sessions。
// AgentSession represents an AI assistant session, aggregating the multi-turn
// question-answer messages (AgentMessage) of the same topic, and recording the
// default slot and the status of the session.
type AgentSession struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this session belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	// NamespaceName the namespace name, for multi-tenant isolation
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_agent_sessions_namespace"`
	// Title 会话标题，默认取首条用户消息的摘要，可由用户重命名
	// Title the title of the session, defaulting to a summary of the first user
	// message, renameable by the user
	Title string `gorm:"type:VARCHAR(256)"`
	// TargetModel 会话缺省档位（列名沿用历史命名）：取值 flash/standard/advanced，空表示按
	// 参数 agent.defaultSlot 解析；生成请求未显式指定档位时使用该档位
	// TargetModel the default slot of the session (the column name keeps its historical
	// naming): one of flash/standard/advanced, empty means resolving through the
	// agent.defaultSlot parameter; it is used when a generation request specifies no slot
	// explicitly
	TargetModel llm.Slot `gorm:"type:VARCHAR(255)"`
	// Status 会话状态（如 active/archived），默认 active
	// Status the status of the session (e.g. active/archived), default active
	Status llm.SessionStatus `gorm:"type:VARCHAR(16)"`
	// AgentKind 会话所属 Agent 类型（如 knowledge/rego），记忆归属与检索据此隔离
	// AgentKind the agent kind the session belongs to (e.g. knowledge/rego), isolating the
	// ownership and retrieval of memories
	AgentKind llm.AgentKind `gorm:"type:VARCHAR(32);default:knowledge"`
	// Summary 会话滚动摘要，压缩已折叠进其中的历史消息，无摘要时为空串
	// Summary the rolling summary of the session, compressing the messages already folded
	// into it, empty when none
	Summary string `gorm:"type:TEXT"`
	// SummaryUptoMessageID 滚动摘要已覆盖的最后一条消息 ID，0 表示尚未摘要任何消息
	// SummaryUptoMessageID the id of the last message covered by the rolling summary,
	// 0 means nothing is summarized yet
	SummaryUptoMessageID uint `gorm:"type:BIGINT"`
}

// AgentMessage 表示 AI 助手会话中的一条消息，记录消息角色（Role）、正文
// （Content），以及助手回复的草稿载荷（DraftJSON）与引用来源列表
// （CitationsJSON）。对应表为 agent_messages。
// AgentMessage represents a message in an AI assistant session, recording the
// role (Role), the content (Content), and the draft payload (DraftJSON) and the
// citation source list (CitationsJSON) of the assistant reply.
type AgentMessage struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this message belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// SessionID 所属会话 ID，会话删除时消息级联删除
	// SessionID the ID of the owning session, cascaded deleted with the session
	SessionID uint `gorm:"type:BIGINT;index"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	// NamespaceName the namespace name, for multi-tenant isolation
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// Role 消息角色：user/assistant/system 等
	// Role the role of the message: user/assistant/system etc.
	Role llm.MessageRole `gorm:"type:VARCHAR(16)"`
	// Content 消息正文内容
	// Content the content of the message
	Content string `gorm:"type:TEXT"`
	// DraftJSON 助手回复的草稿载荷（JSON，如工具调用的中间状态），可空
	// DraftJSON the draft payload (JSON) of the assistant reply, e.g. the
	// intermediate tool-call states, nullable
	DraftJSON datatypes.JSON `gorm:"type:JSONB"`
	// CitationsJSON 引用来源列表（JSON），可空
	// CitationsJSON the citation source list (JSON), nullable
	CitationsJSON datatypes.JSON `gorm:"type:JSONB"`
}
