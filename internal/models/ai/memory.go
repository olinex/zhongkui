// 本文件定义长期记忆模型，承载命名空间 + Agent 类型级的可复用记忆：
// 每条记忆携带向量化表示（Embedding）与全文检索表示（Search），支持向量
// 相似度与关键词混合检索，并记录来源会话、内容摘要（Hash）与召回统计。
//
// This file defines the long-term memory model, carrying reusable memories
// scoped by namespace plus agent kind: each memory holds a vectorized
// representation (Embedding) and a full-text search representation (Search),
// enabling hybrid vector-similarity and keyword retrieval, and records the
// originating session, the content digest (Hash) and the recall statistics.
package ai

import (
	// stdlib
	"time"

	// third-party
	"gorm.io/gorm"

	// internal
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// AgentMemory 表示一条长期记忆，按「命名空间 + Agent 类型」归属与隔离，
// 是跨会话复用结论与偏好的最小单元。对应表为 agent_memories。
// AgentMemory represents a long-term memory, owned and isolated by the
// "namespace + agent kind" scope, the smallest unit for reusing conclusions
// and preferences across sessions. It maps to the agent_memories table.
type AgentMemory struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this memory belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段，与 AgentKind 组成复合索引
	// NamespaceName the namespace name, for multi-tenant isolation, forming a
	// composite index with AgentKind
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_agent_memories_namespace_kind,priority:1"`
	// AgentKind 所属 Agent 类型（如 knowledge/rego），与 NamespaceName 组成复合索引
	// AgentKind the agent kind the memory belongs to (e.g. knowledge/rego), forming a
	// composite index with NamespaceName
	AgentKind llm.AgentKind `gorm:"type:VARCHAR(32);index:idx_agent_memories_namespace_kind,priority:2"`
	// SessionID 来源会话 ID，手动新增为 0（不做外键级联，会话删除后记忆保留）
	// SessionID the originating session id, 0 for memories added manually (no cascade so
	// memories survive session deletion)
	SessionID uint `gorm:"type:BIGINT"`
	// Kind 记忆类别（fact/preference/decision/artifact），默认 fact
	// Kind the category of the memory (fact/preference/decision/artifact), default fact
	Kind llm.MemoryKind `gorm:"type:VARCHAR(16)"`
	// Title 一句话标题，便于在记忆列表中识别
	// Title the one-line title of the memory, for identification in the memory list
	Title string `gorm:"type:VARCHAR(256)"`
	// Content 记忆正文内容
	// Content the body of the memory
	Content string `gorm:"type:TEXT"`
	// Embedding 记忆正文的向量化表示（1536 维），用于向量相似度检索
	// Embedding the vectorized representation (1536 dimensions) of the memory content,
	// for vector similarity retrieval
	Embedding []float32 `gorm:"type:vector(1536);serializer:json"`
	// Search 记忆正文的全文检索表示（tsvector），用于关键词检索
	// Search the full-text search representation (tsvector) of the memory content, for
	// keyword retrieval
	Search string `gorm:"type:tsvector"`
	// Hash 规范化正文的 sha256 十六进制摘要，用于精确去重
	// Hash the sha256 hex digest of the normalized memory content, used for exact
	// deduplication
	Hash string `gorm:"type:CHAR(64)"`
	// Status 记忆状态（active/archived），默认 active
	// Status the status of the memory (active/archived), default active
	Status llm.MemoryStatus `gorm:"type:VARCHAR(16)"`
	// UseCount 被检索命中的次数，默认 0
	// UseCount the number of times the memory has been recalled by retrieval, default 0
	UseCount int `gorm:"type:INT"`
	// LastUsedAt 最近一次被检索召回的时间，可空（从未命中为 NULL）
	// LastUsedAt the last time the memory was recalled by retrieval, nullable (NULL when
	// never used)
	LastUsedAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}
