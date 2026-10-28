package knowledge

import (
	// 标准库
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
)

// TestReindexNamespace_ReportsEmptyRun 校验知识库重索引在未知命名空间（零文档）下仍上报一次
// Start(0)、一次 Finish 且零个 Step。
func TestReindexNamespace_ReportsEmptyRun(t *testing.T) {
	index, err := NewKnowledge(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, aitest.Params(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	rec := &aitest.RecordingReporter{}
	got, err := index.ReindexNamespace(context.Background(), "absent", rec)
	if err != nil {
		t.Fatalf("ReindexNamespace(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexNamespace(absent) = %d, want 0", got)
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

// TestSyncGeneratedDocs_EmptyPlanReports 校验空差异计划（库为空且无派生文档）下上报 Start(0) 与
// Finish，且零个 Step；不触发任何向量化落库（sqlite 不识别 to_tsvector，故仅覆盖空计划路径）。
func TestSyncGeneratedDocs_EmptyPlanReports(t *testing.T) {
	index, err := NewKnowledge(aitest.NewKnowledgeDB(t), aitest.StubEmbedder{}, aitest.Params(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	rec := &aitest.RecordingReporter{}
	if err := index.SyncGeneratedDocs(context.Background(), "ns", docconv.FormatGenerated, nil, rec); err != nil {
		t.Fatalf("SyncGeneratedDocs() error = %v", err)
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
