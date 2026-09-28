// Package ai 提供 AI 助手（RAG + Agent）领域的持久化模型，
// 包括知识文档（KnowledgeDocument）、知识分块（KnowledgeChunk）、
// Agent 会话（AgentSession）与 Agent 消息（AgentMessage）。
// 本文件定义知识文档与知识分块模型，承载知识库的上传文档、
// 分块内容、向量化表示（embedding）与全文检索表示（tsvector）。
package ai

import (
	// 标准库

	// 第三方
	"time"

	"gorm.io/gorm"

	// 项目内部
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// KnowledgeDocument 表示一份上传到知识库的文档，是 RAG 索引的输入单元：
// 文档按标题层级切分为多个知识分块（KnowledgeChunk），并保存内容摘要
// （Hash）以判断已有分块索引是否随内容变更而过期。对应表为 knowledge_documents。
type KnowledgeDocument struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_knowledge_documents_namespace"`
	// Title 文档标题，便于在文档列表中识别
	Title string `gorm:"type:VARCHAR(256)"`
	// Content 文档正文内容
	Content string `gorm:"type:TEXT"`
	// ChunkCount 文档被切分的知识分块数量，0 表示尚未切分
	ChunkCount int `gorm:"type:INT"`
	// Hash 文档内容的 sha256 十六进制摘要，用于判断已有分块索引是否过期
	Hash string `gorm:"type:CHAR(64)"`
	// Format 文档来源格式，取值来自枚举类 documentFormat；手工录入的文档为 markdown
	Format string `gorm:"type:VARCHAR(16)"`
	// OriginalName 导入文档的原始文件名，手工录入的文档为空
	OriginalName string `gorm:"type:VARCHAR(256)"`
	// ObjectKey 原件在 S3 中的对象键，手工录入的文档为空
	ObjectKey string `gorm:"type:VARCHAR(512)"`
	// SizeBytes 原件字节数，手工录入的文档为 0
	SizeBytes int64 `gorm:"type:BIGINT"`
	// Status 文档处理状态，取值来自枚举类 knowledgeDocStatus
	Status string `gorm:"type:VARCHAR(16)"`
	// ParseError 解析失败原因，解析成功或手工录入的文档为空
	ParseError string `gorm:"type:TEXT"`
	// UseCount 该文档的分块被检索命中的次数，默认 0
	UseCount int `gorm:"type:INT"`
	// LastUsedAt 最近一次被检索命中的时间，可空（从未命中为 NULL）
	LastUsedAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}

// KnowledgeChunk 表示知识文档切分后的一个分块，是 RAG 检索的最小单元：
// 每个分块携带向量化表示（Embedding）与全文检索表示（Search），
// 支持向量相似度检索与关键词检索。对应表为 knowledge_chunks。
type KnowledgeChunk struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// DocumentID 所属知识文档 ID，文档删除时分块级联删除
	DocumentID uint `gorm:"type:INT;index:idx_knowledge_chunks_namespace_document,priority:2"`
	// NamespaceName 所属命名空间名称，多租户隔离字段，与 DocumentID 组成复合索引
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_knowledge_chunks_namespace_document,priority:1"`
	// ChunkIndex 分块在文档内的序号，从 0 开始
	ChunkIndex int `gorm:"type:INT"`
	// HeadingPath 分块所属的标题路径（如 "## 简介 / 架构"），用于引用溯源
	HeadingPath string `gorm:"type:TEXT"`
	// Content 分块正文内容
	Content string `gorm:"type:TEXT"`
	// Embedding 分块内容的向量化表示（1536 维），用于向量相似度检索
	Embedding []float32 `gorm:"type:vector(1536);serializer:json"`
	// Search 分块内容的全文检索表示（tsvector），用于关键词检索
	Search string `gorm:"type:tsvector"`
}
