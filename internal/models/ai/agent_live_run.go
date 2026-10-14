package ai

import (
	// 标准库
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AgentLiveRun 表示一次驻留运行（RunHub 管理的 liveRun）的持久化登记：记录运行标识、
// 归属（命名空间 + 会话）、生命周期阶段、已发布的最大事件序号与过期时间，供刷新后
// 发现并重连在途运行。对应表为 agent_live_runs。该表不含用户内容，无需加密。
// 与 AgentRun（agent_runs，审批挂起现场）是不同概念：前者描述在途运行的生命周期，
// 后者保存挂起时刻的运行现场。
type AgentLiveRun struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `json:"namespaceName" gorm:"type:VARCHAR(32)"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `json:"userID" gorm:"type:BIGINT;not null;uniqueIndex:uq_agent_live_runs_running,priority:2"`
	// RunID 运行标识（全局唯一，随机生成的 UUID），事件日志与取消请求据此定位运行
	RunID serializer.HexUUID `json:"runID" gorm:"type:UUID;uniqueIndex:uq_agent_live_runs_run_id"`
	// SessionID 所属会话 ID（真外键，随会话级联删除，无哨兵值）；与 UserID 共同构成部分唯一索引
	// uq_agent_live_runs_running（仅 status='running' 行参与），保证单会话单在途驻留运行
	SessionID uint `json:"sessionID" gorm:"type:BIGINT;uniqueIndex:uq_agent_live_runs_running,priority:1,where:status = 'running'"`
	// Phase 运行生命周期阶段，取值来自枚举类 agentRunPhase，落库列名为 status
	// （running/done/failed/cancelled/suspended）
	Phase AgentRunPhase `json:"phase" gorm:"column:status;type:VARCHAR(16)"`
	// LastSeq 已发布的最大事件序号，续播起点（?after=）据此定位
	LastSeq uint64 `json:"lastSeq" gorm:"type:BIGINT"`
	// ExpiresAt 运行登记过期时间，超过该时刻的运行登记与事件可被 TTL 清理
	ExpiresAt time.Time `json:"expiresAt" gorm:"type:TIMESTAMPTZ;index:idx_agent_live_runs_expires_at"`
}
