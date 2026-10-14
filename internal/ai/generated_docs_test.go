package ai

import (
	// 标准库
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
)

// derivedGeneratedDocs 返回合成的叙述页生成文档（generated 分区），供分区差异计划测试使用。
func derivedGeneratedDocs() []GeneratedKnowledgeDoc {
	return []GeneratedKnowledgeDoc{
		{Key: "explanation/alpha.md", Title: "说明 A", Content: "# 说明 A\n\n正文 A\n"},
		{Key: "how-to/beta.md", Title: "指南 B", Content: "# 指南 B\n\n正文 B\n"},
	}
}

// existingFromDerived 把派生文档转为「已入库且摘要一致」的既有文档视图（模拟已同步状态）。
func existingFromDerived(t *testing.T, derived []GeneratedKnowledgeDoc) []existingGeneratedDoc {
	t.Helper()
	out := make([]existingGeneratedDoc, 0, len(derived))
	for i, doc := range derived {
		hash, err := contentDigest(doc.Content)
		if err != nil {
			t.Fatalf("contentDigest(%q) error = %v", doc.Title, err)
		}
		out = append(out, existingGeneratedDoc{ID: uint(i + 1), Title: doc.Title, Hash: hash})
	}
	return out
}

// TestBuildPartitionRebuildPlan_GeneratedFromEmpty 校验 generated 分区在库里为空时计划为「全部新增」。
func TestBuildPartitionRebuildPlan_GeneratedFromEmpty(t *testing.T) {
	derived := derivedGeneratedDocs()
	plan, err := buildPartitionRebuildPlan(derived, nil)
	if err != nil {
		t.Fatalf("buildPartitionRebuildPlan() error = %v", err)
	}
	if len(plan.create) != len(derived) || len(plan.update) != 0 || len(plan.delete) != 0 {
		t.Errorf("plan = create %d/update %d/delete %d, want %d/0/0",
			len(plan.create), len(plan.update), len(plan.delete), len(derived))
	}
}

// TestBuildPartitionRebuildPlan_GeneratedIdempotent 校验 generated 分区已同步状态下计划为空：
// 重复入库不产生重复条目、不重复向量化。
func TestBuildPartitionRebuildPlan_GeneratedIdempotent(t *testing.T) {
	derived := derivedGeneratedDocs()
	existing := existingFromDerived(t, derived)
	plan, err := buildPartitionRebuildPlan(derived, existing)
	if err != nil {
		t.Fatalf("buildPartitionRebuildPlan() error = %v", err)
	}
	if !plan.empty() {
		t.Errorf("plan is not empty for a synced generated partition: create %d/update %d/delete %d",
			len(plan.create), len(plan.update), len(plan.delete))
	}
}

// TestBuildPartitionRebuildPlan_GeneratedUpdatesChangedContent 校验 generated 分区正文摘要变化的
// 既有文档进入更新集，且以标题为稳定键、不重复新增。
func TestBuildPartitionRebuildPlan_GeneratedUpdatesChangedContent(t *testing.T) {
	derived := derivedGeneratedDocs()
	existing := existingFromDerived(t, derived)
	existing[0].Hash = "stale-hash"

	plan, err := buildPartitionRebuildPlan(derived, existing)
	if err != nil {
		t.Fatalf("buildPartitionRebuildPlan() error = %v", err)
	}
	if len(plan.update) != 1 || len(plan.create) != 0 || len(plan.delete) != 0 {
		t.Fatalf("plan = create %d/update %d/delete %d, want 0/1/0",
			len(plan.create), len(plan.update), len(plan.delete))
	}
	if plan.update[0].DocID != existing[0].ID || plan.update[0].Title != derived[0].Title {
		t.Errorf("update target = %+v, want doc %d / title %q", plan.update[0], existing[0].ID, derived[0].Title)
	}
}

// TestBuildPartitionRebuildPlan_GeneratedDeletesRemoved 校验 generated 分区已移除的叙述页进入删除集。
func TestBuildPartitionRebuildPlan_GeneratedDeletesRemoved(t *testing.T) {
	derived := derivedGeneratedDocs()
	existing := existingFromDerived(t, derived)
	existing = append(existing, existingGeneratedDoc{ID: 999, Title: "obsolete.narrative", Hash: "h"})

	plan, err := buildPartitionRebuildPlan(derived, existing)
	if err != nil {
		t.Fatalf("buildPartitionRebuildPlan() error = %v", err)
	}
	if len(plan.delete) != 1 || plan.delete[0] != 999 {
		t.Fatalf("plan.delete = %v, want [999]", plan.delete)
	}
	if len(plan.create) != 0 || len(plan.update) != 0 {
		t.Errorf("plan = create %d/update %d, want 0/0", len(plan.create), len(plan.update))
	}
}

// TestSyncGeneratedDocs_EmptyNamespace 校验空命名空间下同步在使用 db 之前即返回英文错误，
// 故可用零值 Knowledge 验证。
func TestSyncGeneratedDocs_EmptyNamespace(t *testing.T) {
	k := &Knowledge{}
	if err := k.SyncGeneratedDocs(context.Background(), "", docconv.FormatGenerated, nil, nil); err == nil {
		t.Error("SyncGeneratedDocs(empty namespace) error = nil, want an error")
	}
}
