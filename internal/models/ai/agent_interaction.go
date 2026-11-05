package ai

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// InteractionKind 是持久化交互（agent_interactions.kind）的类型标识：审批（approval）、问卷
// （questionnaire）与推荐的下一步（next_steps）。取值与 agent_interactions.kind 列一致。
type InteractionKind string

// 持久化交互类型常量。
const (
	// InteractionKindApproval 审批：后端 must 级工具调用的待审清单与用户决策
	InteractionKindApproval InteractionKind = "approval"
	// InteractionKindQuestionnaire 问卷：ask_user 主动提问的题目与用户作答
	InteractionKindQuestionnaire InteractionKind = "questionnaire"
	// InteractionKindNextSteps 推荐的下一步：回答完成后供用户一键发起的可选诉求
	InteractionKindNextSteps InteractionKind = "next_steps"
)

// InteractionStatus 是持久化交互（agent_interactions.status）的生命周期状态：待处理（pending）、
// 已作答 / 已决策（answered）与已取消（cancelled）。取值与 agent_interactions.status 列一致。
type InteractionStatus string

// 持久化交互状态常量。
const (
	// InteractionStatusPending 待处理：交互已渲染，等待用户审批或作答
	InteractionStatusPending InteractionStatus = "pending"
	// InteractionStatusAnswered 已作答：用户已提交审批决策或问卷答案
	InteractionStatusAnswered InteractionStatus = "answered"
	// InteractionStatusCancelled 已取消：交互随中止 / 回滚 / 抢占而作废
	InteractionStatusCancelled InteractionStatus = "cancelled"
)

// AgentInteraction 保存业务交互：审批、问卷与推荐的下一步各落一行，供刷新 / 重进会话时重建
// 可交互块（块本身由消息载荷与交互类型重新投影，不落库）。Seq 在所属会话内单调递增（从 1 起），
// 即交互的真实产生顺序；Payload 为业务 JSON（审批调用清单与决策 / 问卷题目与作答 / 下一步列表），
// 经 crypto 序列化器加密落库。对应表为 agent_interactions。
type AgentInteraction struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `gorm:"type:BIGINT;not null"`
	// SessionID 所属会话 ID（真外键，随会话级联删除），与 Seq 共同构成会话内排序索引
	SessionID uint `gorm:"type:BIGINT;not null;index:idx_agent_interactions_session_seq,priority:1"`
	// MessageID 交互所属的助手消息 ID（agent_messages.id），0 表示尚未或无需绑定消息；
	// 历史回放据此把交互挂到对应消息卡片上
	MessageID uint `gorm:"type:BIGINT;not null;default:0;index:idx_agent_interactions_message_id"`
	// Seq 交互在所属会话内的单调递增序号（从 1 起），即交互的真实产生顺序
	Seq uint64 `gorm:"type:BIGINT;not null;index:idx_agent_interactions_session_seq,priority:2"`
	// Kind 交互类型：approval | questionnaire | next_steps（取值见 InteractionKind）
	Kind InteractionKind `gorm:"type:VARCHAR(16);not null"`
	// Status 交互生命周期状态：pending | answered | cancelled（取值见 InteractionStatus）
	Status InteractionStatus `gorm:"type:VARCHAR(16);not null;default:pending"`
	// Payload 交互业务 JSON（审批调用清单与决策 / 问卷题目与作答 / 下一步列表），经 crypto
	// 序列化器加密落库；offset 12822 为本次新增分配（12821 已分配予 agent_messages.payload）
	Payload *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12822,length=32"`
}
