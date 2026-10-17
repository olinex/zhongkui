package ai

import (
	// 标准库
	"time"

	// 第三方

	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AgentRun 表示一次 Agent 运行在挂起时刻的运行现场：命中客户端工具后，
// 后端把完整消息链、待执行调用列表、检索引用、档位与过期时间落库，结束本次流等待前端回传
// 审批决定与执行结果，再据该现场从中断处续跑。对应表为 agent_runs。
type AgentRun struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `json:"namespaceName" gorm:"type:VARCHAR(32)"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `json:"userID" gorm:"type:BIGINT;not null"`
	// SessionID 所属会话 ID（真外键，随会话级联删除，无哨兵值），恢复请求据此校验运行现场归属
	SessionID uint `json:"sessionID" gorm:"type:BIGINT"`
	// Status 运行现场状态，取值来自枚举类 agentRunStatus（pending/consumed/expired）
	Status AgentRunStatus `json:"status" gorm:"type:VARCHAR(16)"`
	// MessagesJSON 挂起时刻的完整消息链 JSON（含用户原文，经 crypto 序列化器加密落库）
	MessagesJSON *serializer.Crypto `json:"messages" gorm:"column:messages;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12814,length=32"`
	// PendingCallsJSON 待执行调用列表 JSON：callID、工具名与调用入参文本
	// （工具入参可能携带凭据，经 crypto 序列化器加密落库）
	PendingCallsJSON *serializer.Crypto `json:"pendingCalls" gorm:"column:pending_calls;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12815,length=32"`
	// CitationsJSON 检索引用列表 JSON，用于恢复后的最终轮产出引用
	// （引用携带知识库正文，经 crypto 序列化器加密落库）
	CitationsJSON *serializer.Crypto `json:"citations" gorm:"column:citations;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12816,length=32"`
	// Slot 本次运行使用的档位，取值来自枚举类 llmSlot
	Slot Slot `json:"slot" gorm:"type:VARCHAR(255)"`
	// AnchorMessageID 本现场的挂起锚点消息 ID（agent_messages.id）：挂起时承载本回合最终回答（问卷挂起
	// 还承载问卷载荷）的助手消息行；续跑据此原地终态化该行而非新增。旁路调用（运行期共享状态缺失）为 0
	AnchorMessageID uint `json:"anchorMessageID" gorm:"type:BIGINT"`
	// ExpiresAt 运行现场过期时间，超过该时刻的现场不可恢复
	ExpiresAt time.Time `json:"expiresAt" gorm:"type:TIMESTAMPTZ;index:idx_agent_runs_expires_at"`
}
