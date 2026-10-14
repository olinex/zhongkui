// 本文件定义长期记忆模型，承载会话级的可复用记忆：每条记忆携带向量化表示
// （Embedding）与全文检索表示（Search），支持向量相似度与关键词混合检索，并
// 记录来源会话、内容摘要（Hash）与召回统计。记忆按「命名空间 + 属主用户 + Agent
// 类型 + 来源会话」隔离，仅在其来源会话内可见。记忆正文经 crypto 序列化器加密存储
// （列类型 BYTEA），内容摘要为带密钥的 HMAC-SHA256，均避免明文落库。
package ai

import (
	// 标准库
	"time"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	coremodel "zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AgentMemory 表示一条长期记忆，按「命名空间 + 属主用户 + Agent 类型 + 来源会话」归属与隔离，
// 仅在其来源会话内可见。对应表为 agent_memories。
type AgentMemory struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段，与 UserID、AgentKind、SessionID 组成复合索引
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_agent_memories_namespace_kind,priority:1;uniqueIndex:uq_agent_memories_scope_hash,priority:1"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离，与 NamespaceName、AgentKind、SessionID 组成复合索引
	UserID uint `gorm:"type:BIGINT;not null;index:idx_agent_memories_namespace_kind,priority:2;uniqueIndex:uq_agent_memories_scope_hash,priority:2"`
	// AgentKind 所属 Agent 类型（如 knowledge/rego），与 NamespaceName、UserID、SessionID 组成复合索引
	AgentKind AgentKind `gorm:"type:VARCHAR(32);index:idx_agent_memories_namespace_kind,priority:3;uniqueIndex:uq_agent_memories_scope_hash,priority:3"`
	// SessionID 来源会话 ID（真外键，无哨兵值，随会话级联删除），检索与唯一性均以它为隔离维度
	// （记忆只在来源会话内可见，不手动新增）
	SessionID uint `gorm:"type:BIGINT;index:idx_agent_memories_namespace_kind,priority:4;uniqueIndex:uq_agent_memories_scope_hash,priority:4"`
	// Kind 记忆类别（fact/preference/decision/artifact），默认 fact
	Kind MemoryKind `gorm:"type:VARCHAR(16)"`
	// Title 一句话标题，便于在记忆列表中识别
	Title string `gorm:"type:VARCHAR(256)"`
	// Content 记忆正文内容，经 crypto 序列化器加密存储
	Content *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12805,length=32"`
	// Embedding 记忆正文的向量化表示（1536 维），用于向量相似度检索
	Embedding []float32 `gorm:"type:vector(1536);serializer:json"`
	// Search 记忆正文的全文检索表示（tsvector），用于关键词检索
	Search string `gorm:"type:tsvector"`
	// Hash 规范化正文的 HMAC-SHA256 十六进制摘要（带密钥，防离线爆破），经 hash 序列化器封装，用于精确去重，与隔离维度组成唯一索引
	Hash serializer.Hash `json:"hash" gorm:"type:CHAR(64);serializer:hash;uniqueIndex:uq_agent_memories_scope_hash,priority:5" secure:"mode=\"hmac-sha256\",offset=12806,length=32"`
	// Status 记忆状态（active/archived），默认 active
	Status MemoryStatus `gorm:"type:VARCHAR(16)"`
	// UseCount 被检索命中的次数，默认 0
	UseCount int `gorm:"type:INT"`
	// LastUsedAt 最近一次被检索召回的时间，可空（从未命中为 NULL）
	LastUsedAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}
