package memory

import (
	// 标准库
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
)

// TestMemoryStoreReindexEmbeddings_EmptyNamespace 测试命名空间为空（含纯空白）时返回英文错误。
func TestMemoryStoreReindexEmbeddings_EmptyNamespace(t *testing.T) {
	store, err := NewStore(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, testParams(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	for _, ns := range []string{"", "   ", "\t\n"} {
		got, err := store.ReindexEmbeddings(context.Background(), ns, nil)
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
	store, err := NewStore(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, testParams(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	got, err := store.ReindexEmbeddings(context.Background(), "absent", nil)
	if err != nil {
		t.Fatalf("ReindexEmbeddings(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexEmbeddings(absent) = %d, want 0", got)
	}
}

// TestMemoryStoreReindexEmbeddings_ReportsEmptyRun 校验记忆重嵌入在未知命名空间（零记忆）下仍
// 上报一次 Start(0)、一次 Finish 且零个 Step。
func TestMemoryStoreReindexEmbeddings_ReportsEmptyRun(t *testing.T) {
	store, err := NewStore(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, testParams(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	rec := &aitest.RecordingReporter{}
	got, err := store.ReindexEmbeddings(context.Background(), "absent", rec)
	if err != nil {
		t.Fatalf("ReindexEmbeddings(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexEmbeddings(absent) = %d, want 0", got)
	}
	if len(rec.Starts) != 1 || rec.Starts[0] != 0 {
		t.Errorf("reporter starts = %v, want [0]", rec.Starts)
	}
	if rec.Finishs != 1 {
		t.Errorf("reporter finishs = %d, want 1", rec.Finishs)
	}
	if len(rec.Steps) != 0 {
		t.Errorf("reporter steps = %v, want none", rec.Steps)
	}
}
