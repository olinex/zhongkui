package knowledge

import (
	// 标准库
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
)

// TestKnowledgeReindexNamespace_EmptyNamespace 测试命名空间为空（含纯空白）时返回英文错误。
func TestKnowledgeReindexNamespace_EmptyNamespace(t *testing.T) {
	index, err := NewKnowledge(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, aitest.Params(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	for _, ns := range []string{"", "   ", "\t\n"} {
		got, err := index.ReindexNamespace(context.Background(), ns, nil)
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
	index, err := NewKnowledge(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, aitest.Params(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	got, err := index.ReindexNamespace(context.Background(), "absent", nil)
	if err != nil {
		t.Fatalf("ReindexNamespace(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexNamespace(absent) = %d, want 0", got)
	}
}
