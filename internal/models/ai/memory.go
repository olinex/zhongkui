// 本文件定义长期记忆模型，承载会话级的可复用记忆：每条记忆携带向量化表示
// （Embedding）与全文检索表示（Search），支持向量相似度与关键词混合检索，并
// 记录来源会话、内容摘要（Hash）与召回统计。记忆按「命名空间 + Agent 类型 +
// 来源会话」隔离，仅在其来源会话内可见。
package ai

import (
	// 标准库
	"time"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// AgentMemory 表示一条长期记忆，按「命名空间 + Agent 类型 + 来源会话」归属与隔离，
// 仅在其来源会话内可见。对应表为 agent_memories。
type AgentMemory struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段，与 AgentKind、SessionID 组成复合索引
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_agent_memories_namespace_kind,priority:1"`
	// AgentKind 所属 Agent 类型（如 knowledge/rego），与 NamespaceName、SessionID 组成复合索引
	AgentKind llm.AgentKind `gorm:"type:VARCHAR(32);index:idx_agent_memories_namespace_kind,priority:2"`
	// SessionID 来源会话 ID，检索与唯一性均以它为隔离维度（记忆只在来源会话内可见，不手动新增）
	SessionID uint `gorm:"type:BIGINT;index:idx_agent_memories_namespace_kind,priority:3"`
	// Kind 记忆类别（fact/preference/decision/artifact），默认 fact
	Kind llm.MemoryKind `gorm:"type:VARCHAR(16)"`
	// Title 一句话标题，便于在记忆列表中识别
	Title string `gorm:"type:VARCHAR(256)"`
	// Content 记忆正文内容
	Content string `gorm:"type:TEXT"`
	// Embedding 记忆正文的向量化表示（1536 维），用于向量相似度检索
	Embedding []float32 `gorm:"type:vector(1536);serializer:json"`
	// Search 记忆正文的全文检索表示（tsvector），用于关键词检索
	Search string `gorm:"type:tsvector"`
	// Hash 规范化正文的 sha256 十六进制摘要，用于精确去重
	Hash string `gorm:"type:CHAR(64)"`
	// Status 记忆状态（active/archived），默认 active
	Status llm.MemoryStatus `gorm:"type:VARCHAR(16)"`
	// UseCount 被检索命中的次数，默认 0
	UseCount int `gorm:"type:INT"`
	// LastUsedAt 最近一次被检索召回的时间，可空（从未命中为 NULL）
	LastUsedAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}
