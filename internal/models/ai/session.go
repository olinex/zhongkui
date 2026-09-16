package ai

import (
	// third-party
	"gorm.io/datatypes"
	"gorm.io/gorm"

	// internal
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// AgentSession 表示一个 AI 助手会话，聚合同一话题下的多轮问答消息
// （AgentMessage），并记录会话绑定的目标模型与状态。对应表为 agent_sessions。
// AgentSession represents an AI assistant session, aggregating the multi-turn
// question-answer messages (AgentMessage) of the same topic, and recording the
// bound target model and the status of the session.
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
	// TargetModel 会话绑定的目标模型标识（如模型供应商与模型名），空表示使用默认模型
	// TargetModel the target model identifier bound to the session (e.g. the
	// provider and the model name), empty means the default model
	TargetModel string `gorm:"type:VARCHAR(255)"`
	// Status 会话状态（如 active/archived），默认 active
	// Status the status of the session (e.g. active/archived), default active
	Status string `gorm:"type:VARCHAR(16)"`
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
	Role string `gorm:"type:VARCHAR(16)"`
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
