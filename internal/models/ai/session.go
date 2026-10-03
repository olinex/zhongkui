package ai

import (
	// 第三方

	"gorm.io/gorm"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// AgentSession 表示一个 AI 助手会话，聚合同一话题下的多轮问答消息
// （AgentMessage），并记录会话缺省档位与状态。对应表为 agent_sessions。
type AgentSession struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_agent_sessions_namespace"`
	// Title 会话标题，默认取首条用户消息的摘要，可由用户重命名
	Title string `gorm:"type:VARCHAR(256)"`
	// TargetModel 会话缺省档位（列名沿用历史命名）：取值 flash/standard/advanced，空表示按
	// 参数 agent.defaultSlot 解析；生成请求未显式指定档位时使用该档位
	TargetModel Slot `gorm:"type:VARCHAR(255)"`
	// Status 会话状态（如 active/archived），默认 active
	Status SessionStatus `gorm:"type:VARCHAR(16)"`
	// AgentKind 会话所属 Agent 类型（如 knowledge/rego），记忆归属与检索据此隔离
	AgentKind AgentKind `gorm:"type:VARCHAR(32);default:knowledge"`
	// Summary 会话滚动摘要，压缩已折叠进其中的历史消息，无摘要时为空
	// （正文含用户原文，经 crypto 序列化器加密落库）
	Summary *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12810,length=32"`
	// SummaryUptoMessageID 滚动摘要已覆盖的最后一条消息 ID，0 表示尚未摘要任何消息
	SummaryUptoMessageID uint `gorm:"type:BIGINT"`
}

// AgentMessage 表示 AI 助手会话中的一条消息，记录消息角色（Role）、正文
// （Content），以及助手回复的草稿载荷（DraftJSON）与引用来源列表
// （CitationsJSON）。对应表为 agent_messages。
type AgentMessage struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// SessionID 所属会话 ID，会话删除时消息级联删除
	SessionID uint `gorm:"type:BIGINT;index"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// Role 消息角色：user/assistant/system 等
	Role MessageRole `gorm:"type:VARCHAR(16)"`
	// Content 消息正文内容（含用户原文与模型输出，经 crypto 序列化器加密落库）
	Content *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12811,length=32"`
	// DraftJSON 助手回复的草稿载荷（JSON，如工具调用的中间状态），可空，经 crypto 序列化器加密落库
	DraftJSON *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12812,length=32"`
	// CitationsJSON 引用来源列表（JSON），可空，经 crypto 序列化器加密落库（引用携带知识库正文）
	CitationsJSON *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12813,length=32"`
}
