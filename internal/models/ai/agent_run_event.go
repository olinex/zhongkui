package ai

import (
	// 标准库
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AgentRunEvent 表示驻留运行事件日志中的一条事件：按 (RunID, Seq) 唯一，payload 为
// 已序列化的 JSON 负载（含助手正文与引用等用户内容，经 crypto 序列化器加密落库）。
// 对应表为 agent_run_events，按 expires_at TTL 硬删除（表无 deleted_at 列，避免与
// (run_id, seq) 唯一索引冲突）。该表是断线续播的回放来源，也是落库兜底。
type AgentRunEvent struct {
	// ID 主键
	ID uint `gorm:"primarykey"`
	// CreatedAt 事件落库时间
	CreatedAt time.Time
	// RunID 所属运行标识
	RunID serializer.HexUUID `json:"runID" gorm:"type:UUID;uniqueIndex:uq_agent_run_events_run_seq,priority:1"`
	// SessionID 所属会话 ID（真外键，随会话级联删除，无哨兵值）
	SessionID uint `json:"sessionID" gorm:"type:BIGINT"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `json:"namespaceName" gorm:"type:VARCHAR(32)"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `json:"userID" gorm:"type:BIGINT;not null"`
	// Seq 事件在所属运行内的单调自增序号（从 1 起），与 RunID 共同唯一
	Seq uint64 `json:"seq" gorm:"type:BIGINT;uniqueIndex:uq_agent_run_events_run_seq,priority:2"`
	// Type 事件类型，取值来自内部枚举 AgentRunEventType（delta/stage/tool_request/done/error）
	Type AgentRunEventType `json:"type" gorm:"type:VARCHAR(16)"`
	// Payload 事件负载 JSON（含用户内容，经 crypto 序列化器加密落库），可空
	Payload *serializer.Crypto `json:"payload" gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12817,length=32"`
	// ExpiresAt 事件过期时间，超过该时刻的事件可被 TTL 清理任务硬删除
	ExpiresAt time.Time `json:"expiresAt" gorm:"type:TIMESTAMPTZ;index:idx_agent_run_events_expires_at"`
}
