// Package ai 提供 AI 助手（RAG + Agent）领域的持久化模型，
// 包括知识文档（KnowledgeDocument）、知识分块（KnowledgeChunk）、
// Agent 会话（AgentSession）与 Agent 消息（AgentMessage）。
// 本文件定义知识文档与知识分块模型，承载知识库的上传文档、
// 分块内容、向量化表示（embedding）与全文检索表示（tsvector）。
//
// Package ai provides the persistence models of the AI assistant (RAG + Agent)
// domain, including knowledge documents, knowledge chunks, agent sessions and
// agent messages. This file defines the knowledge document and chunk models,
// carrying the uploaded documents, chunk contents, vectorized representations
// (embedding) and full-text search representations (tsvector) of the knowledge
// base.
package ai

import (
	// third-party
	"gorm.io/gorm"

	// internal
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// KnowledgeDocument 表示一份上传到知识库的文档，是 RAG 索引的输入单元：
// 文档按标题层级切分为多个知识分块（KnowledgeChunk），并保存内容摘要
// （Hash）以判断已有分块索引是否随内容变更而过期。对应表为 knowledge_documents。
// KnowledgeDocument represents an uploaded knowledge document, the input unit
// of the RAG index: the document is split into knowledge chunks by heading
// levels, and the content digest (Hash) is kept to detect whether existing
// chunk indexes are stale after the content changes.
type KnowledgeDocument struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this document belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	// NamespaceName the namespace name, for multi-tenant isolation
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_knowledge_documents_namespace"`
	// Title 文档标题，便于在文档列表中识别
	// Title the title of the document, for identification in the document list
	Title string `gorm:"type:VARCHAR(256)"`
	// Content 文档正文内容
	// Content the full content of the document
	Content string `gorm:"type:TEXT"`
	// ChunkCount 文档被切分的知识分块数量，0 表示尚未切分
	// ChunkCount the number of chunks the document is split into, 0 means not split yet
	ChunkCount int `gorm:"type:INT"`
	// Hash 文档内容的 sha256 十六进制摘要，用于判断已有分块索引是否过期
	// Hash the sha256 hex digest of the document content, used to detect whether
	// existing chunk indexes are stale
	Hash string `gorm:"type:CHAR(64)"`
}

// KnowledgeChunk 表示知识文档切分后的一个分块，是 RAG 检索的最小单元：
// 每个分块携带向量化表示（Embedding）与全文检索表示（Search），
// 支持向量相似度检索与关键词检索。对应表为 knowledge_chunks。
// KnowledgeChunk represents a chunk of a knowledge document, the smallest unit
// of RAG retrieval: each chunk carries a vectorized representation (Embedding)
// and a full-text search representation (Search), enabling both vector
// similarity retrieval and keyword retrieval.
type KnowledgeChunk struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this chunk belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// DocumentID 所属知识文档 ID，文档删除时分块级联删除
	// DocumentID the ID of the owning knowledge document, cascaded deleted with
	// the document
	DocumentID uint `gorm:"type:INT;index:idx_knowledge_chunks_namespace_document,priority:2"`
	// NamespaceName 所属命名空间名称，多租户隔离字段，与 DocumentID 组成复合索引
	// NamespaceName the namespace name, for multi-tenant isolation, forming a
	// composite index with DocumentID
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_knowledge_chunks_namespace_document,priority:1"`
	// ChunkIndex 分块在文档内的序号，从 0 开始
	// ChunkIndex the index of the chunk within the document, starting from 0
	ChunkIndex int `gorm:"type:INT"`
	// HeadingPath 分块所属的标题路径（如 "## 简介 / 架构"），用于引用溯源
	// HeadingPath the heading path the chunk belongs to (e.g. "## Introduction / Architecture"), used for citation traceability
	HeadingPath string `gorm:"type:TEXT"`
	// Content 分块正文内容
	// Content the content of the chunk
	Content string `gorm:"type:TEXT"`
	// Embedding 分块内容的向量化表示（1536 维），用于向量相似度检索
	// Embedding the vectorized representation (1536 dimensions) of the chunk
	// content, for vector similarity retrieval
	Embedding []float32 `gorm:"type:vector(1536);serializer:json"`
	// Search 分块内容的全文检索表示（tsvector），用于关键词检索
	// Search the full-text search representation (tsvector) of the chunk
	// content, for keyword retrieval
	Search string `gorm:"type:tsvector"`
}
