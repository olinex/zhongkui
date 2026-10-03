package ai

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// newTestReindexDB 构造重嵌入测试用的 sqlite 内存库并迁移知识文档与长期记忆两表（sqlite 不支持
// pgvector/tsvector，本文件只覆盖空命名空间与不触发向量写入的路径）。
func newTestReindexDB(t *testing.T) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(&ai.KnowledgeDocument{}, &ai.AgentMemory{}); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// TestKnowledgeReindexNamespace_EmptyNamespace 测试命名空间为空（含纯空白）时返回英文错误。
func TestKnowledgeReindexNamespace_EmptyNamespace(t *testing.T) {
	knowledge, err := NewKnowledge(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	for _, ns := range []string{"", "   ", "\t\n"} {
		got, err := knowledge.ReindexNamespace(context.Background(), ns)
		if err == nil {
			t.Fatalf("ReindexNamespace(%q) = (%d, nil), want an error", ns, got)
		}
		if got != 0 {
			t.Errorf("ReindexNamespace(%q) = (%d, err), want count 0", ns, got)
		}
	}
}

// TestKnowledgeReindexNamespace_UnknownNamespace 测试不存在的命名空间返回 (0, nil)。
func TestKnowledgeReindexNamespace_UnknownNamespace(t *testing.T) {
	knowledge, err := NewKnowledge(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	got, err := knowledge.ReindexNamespace(context.Background(), "absent")
	if err != nil {
		t.Fatalf("ReindexNamespace(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexNamespace(absent) = %d, want 0", got)
	}
}

// TestMemoryStoreReindexEmbeddings_EmptyNamespace 测试命名空间为空（含纯空白）时返回英文错误。
func TestMemoryStoreReindexEmbeddings_EmptyNamespace(t *testing.T) {
	store, err := NewMemoryStore(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	for _, ns := range []string{"", "   ", "\t\n"} {
		got, err := store.ReindexEmbeddings(context.Background(), ns)
		if err == nil {
			t.Fatalf("ReindexEmbeddings(%q) = (%d, nil), want an error", ns, got)
		}
		if got != 0 {
			t.Errorf("ReindexEmbeddings(%q) = (%d, err), want count 0", ns, got)
		}
	}
}

// TestMemoryStoreReindexEmbeddings_UnknownNamespace 测试不存在的命名空间返回 (0, nil)。
func TestMemoryStoreReindexEmbeddings_UnknownNamespace(t *testing.T) {
	store, err := NewMemoryStore(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	got, err := store.ReindexEmbeddings(context.Background(), "absent")
	if err != nil {
		t.Fatalf("ReindexEmbeddings(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexEmbeddings(absent) = %d, want 0", got)
	}
}
